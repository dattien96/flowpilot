package flowgate

import (
	"bufio"
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type OracleResult struct {
	Regressed     []string `json:"regressed"`
	Tampered      []string `json:"tampered"`
	HasRegression bool     `json:"has_regression"`
	HasTampering  bool     `json:"has_tampering"`
	Message       string   `json:"message,omitempty"`
}

func RunOracle(repoDir string, baseline *Baseline, diff []ChangedFile) OracleResult {
	if baseline == nil || baseline.TestCmd == "" {
		return OracleResult{}
	}

	nowFailed := runTests(repoDir, baseline.TestCmd, baseline.TestDir)

	var changedTestFiles []string
	for _, f := range diff {
		if IsTestFile(f.Path) {
			changedTestFiles = append(changedTestFiles, f.Path)
		}
	}

	var regressed []string
	for _, t := range nowFailed {
		if isInBaseline(t, baseline.GreenTests) && !isTestFromChangedFile(t, changedTestFiles) {
			regressed = append(regressed, t)
		}
	}

	var tampered []string
	for _, f := range diff {
		if IsTestFile(f.Path) && f.Status == "M" {
			tampered = append(tampered, f.Path)
		}
	}

	msg := ""
	if len(regressed) > 0 {
		msg = "Previously-passing tests now fail: " + strings.Join(regressed, ", ") + ". Fix the code; do not change these tests."
	}

	return OracleResult{
		Regressed:     regressed,
		Tampered:      tampered,
		HasRegression: len(regressed) > 0,
		HasTampering:  len(tampered) > 0,
		Message:       msg,
	}
}

func runTests(repoDir, testCmd, testDir string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	parts := strings.Fields(testCmd)
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Dir = filepath.Join(repoDir, filepath.FromSlash(testDir))
	out, _ := cmd.CombinedOutput()

	var failed []string
	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(testCmd, "go test"):
			// "--- FAIL: TestName (0.00s)"
			if strings.Contains(line, "--- FAIL:") {
				after := strings.TrimPrefix(line, "--- FAIL:")
				after = strings.TrimSpace(after)
				fields := strings.Fields(after)
				if len(fields) > 0 {
					failed = append(failed, fields[0])
				}
			}
		case strings.HasPrefix(testCmd, "npm"):
			// best-effort: lines with "failing" or "✗"
			if strings.Contains(line, "failing") || strings.Contains(line, "✗") {
				name := strings.TrimSpace(strings.TrimPrefix(line, "✗"))
				if name != "" && !strings.Contains(name, "failing") {
					failed = append(failed, name)
				}
			}
		case strings.HasPrefix(testCmd, "pytest"):
			// "FAILED test_name" or "test_name FAILED"
			if strings.Contains(line, "FAILED") {
				fields := strings.Fields(line)
				for i, f := range fields {
					if f == "FAILED" {
						if i+1 < len(fields) {
							failed = append(failed, fields[i+1])
						} else if i > 0 {
							failed = append(failed, fields[i-1])
						}
						break
					}
				}
			}
		}
	}
	return failed
}

func isInBaseline(testName string, baseline []string) bool {
	if testName == "" {
		return false
	}
	for _, b := range baseline {
		if b == "" {
			continue
		}
		if b == testName || strings.HasSuffix(b, testName) || strings.HasSuffix(testName, b) {
			return true
		}
	}
	return false
}

// isTestFromChangedFile checks if a failed test name plausibly came from one of
// the changed test files. We use a best-effort heuristic: if the test file path
// contains something that matches a substring of the test name (or vice-versa),
// we consider it covered by the diff.
func isTestFromChangedFile(testName string, changedTestFiles []string) bool {
	testLower := strings.ToLower(testName)
	for _, path := range changedTestFiles {
		base := filepath.Base(path)
		// strip extension(s): "foo_test.go" → "foo", "foo.test.ts" → "foo"
		stem := base
		for strings.Contains(stem, ".") {
			stem = stem[:strings.LastIndex(stem, ".")]
		}
		stem = strings.ToLower(strings.TrimSuffix(strings.TrimSuffix(stem, "_test"), ".test"))
		if stem != "" && strings.Contains(testLower, stem) {
			return true
		}
	}
	return false
}
