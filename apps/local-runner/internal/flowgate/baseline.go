package flowgate

import (
	"bufio"
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

func IsTestFile(path string) bool {
	return reTestGo.MatchString(path) || reTestJS.MatchString(path) || reTestPy.MatchString(path)
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
	if hasNpmTestScript(filepath.Join(dir, "package.json")) {
		return "npm test"
	}
	return ""
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
func runnerRank(cmd string) int {
	switch {
	case strings.HasPrefix(cmd, "go test"):
		return 0
	case strings.HasPrefix(cmd, "pytest"):
		return 1
	default:
		return 2
	}
}

func CaptureBaseline(repoDir, dotFlowpilotDir string) (*Baseline, error) {
	runner := DetectTestRunner(repoDir)
	if runner.Cmd == "" {
		return &Baseline{}, nil
	}

	names := runAndParseGreen(repoDir, runner.Cmd, runner.Dir)

	guardDir := filepath.Join(dotFlowpilotDir, "guard")
	if err := os.MkdirAll(guardDir, 0o755); err != nil {
		return nil, err
	}

	bl := &Baseline{
		CapturedAt: time.Now().UTC().Format(time.RFC3339),
		GreenTests: names,
		TestCmd:    runner.Cmd,
		TestDir:    runner.Dir,
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

// runAndParseGreen executes the test command in repoDir/testDir and returns names
// of passing tests.
func runAndParseGreen(repoDir, testCmd, testDir string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	parts := strings.Fields(testCmd)
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Dir = filepath.Join(repoDir, filepath.FromSlash(testDir))
	out, _ := cmd.CombinedOutput()

	var names []string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(testCmd, "go test"):
			if strings.Contains(line, "--- PASS:") {
				// "--- PASS: TestName (0.00s)"
				after := strings.TrimPrefix(line, "--- PASS:")
				after = strings.TrimSpace(after)
				fields := strings.Fields(after)
				if len(fields) > 0 {
					names = append(names, fields[0])
				}
			}
		case strings.HasPrefix(testCmd, "npm"):
			// best-effort: lines containing "✓" or "passing"
			if strings.Contains(line, "✓") || strings.Contains(line, "passing") {
				name := strings.TrimSpace(strings.TrimPrefix(line, "✓"))
				if name != "" && !strings.Contains(name, "passing") {
					names = append(names, name)
				}
			}
		case strings.HasPrefix(testCmd, "pytest"):
			// "test_foo PASSED"
			if strings.Contains(line, " PASSED") {
				fields := strings.Fields(line)
				if len(fields) >= 1 {
					names = append(names, fields[0])
				}
			}
		}
	}
	return names
}
