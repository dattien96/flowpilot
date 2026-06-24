package flowgate

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
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
}

var reTestGo = regexp.MustCompile(`_test\.go$`)
var reTestJS = regexp.MustCompile(`\.test\.[jt]sx?$|\.spec\.[jt]sx?$`)
var reTestPy = regexp.MustCompile(`(^|/)test_.*\.py$`)

func IsTestFile(path string) bool {
	return reTestGo.MatchString(path) || reTestJS.MatchString(path) || reTestPy.MatchString(path)
}

func DetectTestCommand(repoDir string) string {
	if _, err := os.Stat(filepath.Join(repoDir, "go.mod")); err == nil {
		// -v is required: without it Go only prints "ok package/path" with no per-test
		// "--- PASS:" / "--- FAIL:" lines, so the baseline captures zero test names.
		return "go test -v ./..."
	}
	if _, err := os.Stat(filepath.Join(repoDir, "package.json")); err == nil {
		data, err := os.ReadFile(filepath.Join(repoDir, "package.json"))
		if err == nil {
			var pkg struct {
				Scripts map[string]string `json:"scripts"`
			}
			if json.Unmarshal(data, &pkg) == nil && pkg.Scripts["test"] != "" {
				return "npm test"
			}
		}
		return ""
	}
	if _, err := os.Stat(filepath.Join(repoDir, "pytest.ini")); err == nil {
		// -v is required: -q suppresses per-test PASSED/FAILED lines.
		return "pytest -v"
	}
	if _, err := os.Stat(filepath.Join(repoDir, "pyproject.toml")); err == nil {
		return "pytest -v"
	}
	return ""
}

func CaptureBaseline(repoDir, dotFlowpilotDir string) (*Baseline, error) {
	testCmd := DetectTestCommand(repoDir)
	if testCmd == "" {
		return &Baseline{}, nil
	}

	names := runAndParseGreen(repoDir, testCmd)

	guardDir := filepath.Join(dotFlowpilotDir, "guard")
	if err := os.MkdirAll(guardDir, 0o755); err != nil {
		return nil, err
	}

	bl := &Baseline{
		CapturedAt: time.Now().UTC().Format(time.RFC3339),
		GreenTests: names,
		TestCmd:    testCmd,
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

// runAndParseGreen executes the test command and returns names of passing tests.
func runAndParseGreen(repoDir, testCmd string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	parts := strings.Fields(testCmd)
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Dir = repoDir
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
