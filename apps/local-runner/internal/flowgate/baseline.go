package flowgate

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type Baseline struct {
	CapturedAt string   `json:"captured_at"`
	GreenTests []string `json:"green_tests"`
	TestCmd    string   `json:"test_command"`
	// TestDir is the directory the test command must run in, relative to the bound
	// project root ("" = root). It is non-empty for monorepos where the runner lives
	// in a subdirectory (e.g. a nested go.mod). (CP-35)
	TestDir string `json:"test_dir,omitempty"`
	// HeadSHA is the git HEAD SHA at capture time. Used to detect when the baseline is
	// stale and must be refreshed. Empty on baselines captured before Task-156. (Task-156)
	HeadSHA string `json:"head_sha,omitempty"`
	// Dirty records whether the working tree was dirty at capture time. (Task-156)
	Dirty bool `json:"dirty,omitempty"`
	// SuitePassed records whether the test command exited 0 at capture time.
	// The exit-code primary regression signal compares this against the current run. (Task-156)
	SuitePassed bool `json:"suite_passed,omitempty"`
}

// TestConfig holds explicit test runner configuration from the project's
// .flowpilot/settings/test-config.json. When present it takes precedence over
// auto-detection so any language with a runnable test command is covered. (Task-156)
type TestConfig struct {
	TestCommand  string `json:"test_command,omitempty"`
	TestDir      string `json:"test_dir,omitempty"`
	ResultFormat string `json:"result_format,omitempty"`
}

// LoadTestConfig reads the optional per-project test runner config. Missing file → empty config.
func LoadTestConfig(dotFP string) TestConfig {
	data, err := os.ReadFile(filepath.Join(dotFP, "settings", "test-config.json"))
	if err != nil {
		return TestConfig{}
	}
	var cfg TestConfig
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

// captureGitState returns the current HEAD SHA and whether the working tree is dirty.
// Both values are empty/false on error (non-fatal).
func captureGitState(repoDir string) (headSHA string, dirty bool) {
	if out, err := exec.Command("git", "-C", repoDir, "rev-parse", "HEAD").Output(); err == nil {
		headSHA = strings.TrimSpace(string(out))
	}
	if out, err := exec.Command("git", "-C", repoDir, "status", "--porcelain").Output(); err == nil {
		dirty = len(strings.TrimSpace(string(out))) > 0
	}
	return
}

// RefreshBaselineIfStale loads the stored baseline and returns it unchanged when
// HEAD and working-tree state match. When they differ (or the baseline is missing
// or pre-Task-156 with no HeadSHA), it may re-capture a fresh baseline. (Task-156)
//
// V9-30 / V10 P2:
//   - Never recapture while dirty (coding mid-flight would replace green with red).
//   - Never recapture over a green baseline just because HEAD advanced (broken
//     commits would erase regression detection).
//   - DO recapture when HEAD advanced, tree is clean, AND the stored baseline
//     was already red (SuitePassed=false) or missing HeadSHA — so legitimate
//     green tests after a prior red baseline can become the new truth.
//   - Always capture when no usable baseline exists.
func RefreshBaselineIfStale(repoDir, dotFP string) (*Baseline, error) {
	return RefreshBaselineIfStaleContext(context.Background(), repoDir, dotFP)
}

// RefreshBaselineIfStaleContext is RefreshBaselineIfStale with an explicit
// context (BUG-288 P2-04, Vòng 12). The first baseline capture can run the
// project's full test suite (up to CaptureBaseline's ~5 minute timeout);
// previously that suite always ran under context.Background(), so Stop could
// not cut it short. Callers with a run/loop-scoped cancellable context
// (mirroring flowInlineContext/postTurnGateCancel elsewhere in this codebase)
// should use this entry point so Stop propagates into the suite run.
func RefreshBaselineIfStaleContext(ctx context.Context, repoDir, dotFP string) (*Baseline, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	bl, _ := LoadBaseline(dotFP)
	headSHA, dirty := captureGitState(repoDir)
	if bl != nil && bl.HeadSHA != "" && bl.HeadSHA == headSHA && bl.Dirty == dirty {
		return bl, nil // still fresh
	}
	if bl != nil && bl.HeadSHA != "" {
		if dirty {
			// Mid-edit — keep prior truth.
			return bl, nil
		}
		// Clean tree, HEAD moved.
		if bl.SuitePassed {
			// Green baseline is frozen against accidental broken commits (V9-30).
			return bl, nil
		}
		// Prior baseline was red (or stale red) — allow recapture of a new clean HEAD
		// so production can pick up fixed green suites (V10 P2).
		return CaptureBaselineContext(ctx, repoDir, dotFP)
	}
	// No usable baseline yet — capture.
	return CaptureBaselineContext(ctx, repoDir, dotFP)
}

// TestRunner is a detected test command plus the directory it runs in, relative to
// the bound project root. Dir is "" when the runner lives at the root.
type TestRunner struct {
	Cmd string
	Dir string
}

var reTestGo = regexp.MustCompile(`_test\.go$`)
var reTestJS = regexp.MustCompile(`\.test\.[jt]sx?$|\.spec\.[jt]sx?$`)
var reTestPy = regexp.MustCompile(`(^|/)test_.*\.py$`)
var reTestDart = regexp.MustCompile(`_test\.dart$`)
var reTestKt = regexp.MustCompile(`Test\.kt$`)
var reTestJava = regexp.MustCompile(`Tests?\.java$`)
var reTestSwift = regexp.MustCompile(`Tests?\.swift$`)

func IsTestFile(path string) bool {
	return reTestGo.MatchString(path) ||
		reTestJS.MatchString(path) ||
		reTestPy.MatchString(path) ||
		reTestDart.MatchString(path) ||
		reTestKt.MatchString(path) ||
		reTestJava.MatchString(path) ||
		reTestSwift.MatchString(path)
}

// DetectTestCommand returns just the command string for the detected runner (root
// or nested). Retained for callers/tests that only need the command.
func DetectTestCommand(repoDir string) string {
	return DetectTestRunner(repoDir).Cmd
}

// DetectTestRunner finds a test runner for the project. It first checks the repo
// root (fast path, Dir=""); when the root has no usable runner — common in
// monorepos where the runner lives in a subdirectory (e.g. a nested go.mod) — it
// walks the tree and returns the best nested runner. Ecosystem priority is
// go > python > node (the oracle parses go/pytest output reliably while npm
// parsing is best-effort); ties break on the shallowest path. (CP-35)
func DetectTestRunner(repoDir string) TestRunner {
	if cmd := detectRunnerInDir(repoDir); cmd != "" {
		return TestRunner{Cmd: cmd, Dir: ""}
	}
	return detectNestedRunner(repoDir)
}

// detectRunnerInDir returns the test command for a single directory, or "" if no
// recognised runner marker is present.
func detectRunnerInDir(dir string) string {
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
		// -v is required: without it Go only prints "ok package/path" with no per-test
		// "--- PASS:" / "--- FAIL:" lines, so the baseline captures zero test names.
		return "go test -v ./..."
	}
	if _, err := os.Stat(filepath.Join(dir, "pytest.ini")); err == nil {
		// -v is required: -q suppresses per-test PASSED/FAILED lines.
		return "pytest -v"
	}
	if _, err := os.Stat(filepath.Join(dir, "pyproject.toml")); err == nil {
		return "pytest -v"
	}
	// Flutter — require flutter on PATH; skip silently if SDK is absent. (Task-159)
	if _, err := os.Stat(filepath.Join(dir, "pubspec.yaml")); err == nil {
		if flutterOnPath() {
			return "flutter test"
		}
	}
	// Android / Java Gradle — prefer Unix wrapper, fall back to Windows bat. (Task-159)
	if _, err := os.Stat(filepath.Join(dir, "build.gradle")); err == nil {
		if _, err2 := os.Stat(filepath.Join(dir, "gradlew")); err2 == nil {
			return "./gradlew test"
		}
		if _, err2 := os.Stat(filepath.Join(dir, "gradlew.bat")); err2 == nil {
			return "gradlew.bat test"
		}
	}
	// Java Maven (Task-159)
	if _, err := os.Stat(filepath.Join(dir, "pom.xml")); err == nil {
		return "mvn test -q"
	}
	// Angular: ng test default is watch-mode (never exits). Append --watch=false so
	// executeSuite always gets a process that terminates. (Task-159)
	if hasNpmTestScript(filepath.Join(dir, "package.json")) {
		if _, err := os.Stat(filepath.Join(dir, "angular.json")); err == nil {
			return "npx ng test --watch=false --no-progress"
		}
		return "npm test"
	}
	return ""
}

// flutterOnPath reports whether the flutter CLI is available on PATH.
// Used to guard flutter test detection so missing SDK doesn't produce a
// confusing 5-minute executeSuite timeout. (Task-159)
func flutterOnPath() bool {
	_, err := exec.LookPath("flutter")
	return err == nil
}

// hasNpmTestScript reports whether package.json at pkgPath declares a "test" script.
func hasNpmTestScript(pkgPath string) bool {
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return false
	}
	var pkg struct {
		Scripts map[string]string `json:"scripts"`
	}
	if json.Unmarshal(data, &pkg) != nil {
		return false
	}
	return pkg.Scripts["test"] != ""
}

// detectNestedRunner walks repoDir (skipping vendor/build/hidden dirs) for a runner
// marker in a subdirectory. It returns the highest-priority, shallowest match.
func detectNestedRunner(repoDir string) TestRunner {
	skip := map[string]bool{
		"node_modules": true, "vendor": true, "dist": true,
		"build": true, "target": true, "testdata": true,
	}
	best := TestRunner{}
	bestRank, bestDepth := 99, 1<<30
	_ = filepath.WalkDir(repoDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if path == repoDir {
			return nil // root already checked by the fast path
		}
		name := d.Name()
		if skip[name] || strings.HasPrefix(name, ".") {
			return fs.SkipDir
		}
		rel, relErr := filepath.Rel(repoDir, path)
		if relErr != nil {
			return nil
		}
		depth := strings.Count(rel, string(os.PathSeparator)) + 1
		if depth > 5 {
			return fs.SkipDir
		}
		cmd := detectRunnerInDir(path)
		if cmd == "" {
			return nil
		}
		rank := runnerRank(cmd)
		if rank < bestRank || (rank == bestRank && depth < bestDepth) {
			best = TestRunner{Cmd: cmd, Dir: filepath.ToSlash(rel)}
			bestRank, bestDepth = rank, depth
		}
		return nil
	})
	return best
}

// runnerRank orders ecosystems by how reliably the oracle parses their output.
// Lower = higher priority; ties break on shallowest path in detectNestedRunner. (Task-159)
func runnerRank(cmd string) int {
	switch {
	case strings.HasPrefix(cmd, "go test"):
		return 0
	case strings.HasPrefix(cmd, "pytest"):
		return 1
	case strings.HasPrefix(cmd, "flutter"):
		return 2
	case strings.HasPrefix(cmd, "./gradlew"),
		strings.HasPrefix(cmd, "gradlew.bat"):
		return 3
	case strings.HasPrefix(cmd, "mvn"):
		return 4
	default:
		return 5 // npm / npx
	}
}

func CaptureBaseline(repoDir, dotFlowpilotDir string) (*Baseline, error) {
	return CaptureBaselineContext(context.Background(), repoDir, dotFlowpilotDir)
}

// CaptureBaselineContext is CaptureBaseline with an explicit context
// (BUG-288 P2-04, Vòng 12): Stop previously could not cut short the first
// baseline capture's suite run because it always used context.Background()
// regardless of caller. Threading the caller's cancellable context here lets
// Stop cancel it the same way runFlowGate's suite runs already are.
func CaptureBaselineContext(ctx context.Context, repoDir, dotFlowpilotDir string) (*Baseline, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	// Explicit config takes precedence over auto-detection (Task-156 T-2).
	cfg := LoadTestConfig(dotFlowpilotDir)
	runner := TestRunner{}
	if cfg.TestCommand != "" {
		runner = TestRunner{Cmd: cfg.TestCommand, Dir: cfg.TestDir}
	} else {
		runner = DetectTestRunner(repoDir)
	}

	guardDir := filepath.Join(dotFlowpilotDir, "guard")
	if err := os.MkdirAll(guardDir, 0o755); err != nil {
		return nil, err
	}

	headSHA, dirty := captureGitState(repoDir)

	var names []string
	suitePassed := false
	if runner.Cmd != "" {
		// Capture baseline green tests; env/start failures leave SuitePassed=false
		// with empty green list (same as a failed suite for capture purposes).
		var envErr string
		suitePassed, names, _, envErr, _ = executeSuite(ctx, repoDir, runner.Cmd, runner.Dir)
		if envErr != "" {
			suitePassed = false
			names = nil
		}
	}

	bl := &Baseline{
		CapturedAt:  time.Now().UTC().Format(time.RFC3339),
		GreenTests:  names,
		TestCmd:     runner.Cmd,
		TestDir:     runner.Dir,
		HeadSHA:     headSHA,
		Dirty:       dirty,
		SuitePassed: suitePassed,
	}

	data, err := json.MarshalIndent(bl, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(guardDir, "test_baseline.json"), data, 0o644); err != nil {
		return nil, err
	}
	return bl, nil
}

func LoadBaseline(dotFlowpilotDir string) (*Baseline, error) {
	path := filepath.Join(dotFlowpilotDir, "guard", "test_baseline.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var bl Baseline
	if err := json.Unmarshal(data, &bl); err != nil {
		return nil, err
	}
	return &bl, nil
}
