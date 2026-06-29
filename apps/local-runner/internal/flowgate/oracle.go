package flowgate

import (
	"bufio"
	"context"
	"log"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type OracleResult struct {
	Regressed     []string `json:"regressed"`
	Passed        []string `json:"passed,omitempty"`
	Tampered      []string `json:"tampered"`
	HasRegression bool     `json:"has_regression"`
	HasTampering  bool     `json:"has_tampering"`
	Message       string   `json:"message,omitempty"`
	// Disabled is true when no test command is configured or resolvable, so the oracle
	// cannot provide regression safety. The UI should surface this as a warning. (Task-156)
	Disabled bool `json:"disabled,omitempty"`
}

// RunOracle checks whether the current state of repoDir represents a regression
// against baseline. overrides is a set of test names the user has explicitly agreed
// to change (Task-155): those tests are excluded from regression/tamper counting.
//
// Primary signal (Task-156): if the baseline recorded suite_passed=true and the
// suite now exits non-zero, that is a regression. Named test diffing is an optional
// refinement layered on structured output formats and is never the sole signal.
func RunOracle(repoDir string, baseline *Baseline, diff []ChangedFile, overrides map[string]Override) OracleResult {
	if baseline == nil {
		return OracleResult{}
	}
	if baseline.TestCmd == "" {
		return OracleResult{Disabled: true}
	}

	testCmd := baseline.TestCmd
	if scoped := scopeTestCommand(repoDir, baseline.TestDir, testCmd, diff); scoped != "" {
		testCmd = scoped
		log.Printf("[gate] scoped oracle run: %s", testCmd)
	}
	suitePassed, nowPassed, nowFailed := executeSuite(repoDir, testCmd, baseline.TestDir)

	var changedTestFiles []string
	for _, f := range diff {
		if IsTestFile(f.Path) {
			changedTestFiles = append(changedTestFiles, f.Path)
		}
	}

	var regressed []string
	if baseline.SuitePassed && !suitePassed {
		// Exit-code primary signal: suite was green at baseline and is red now.
		if len(nowFailed) > 0 {
			// Named granularity available — filter to tests that were green, not in diff, not overridden.
			for _, t := range nowFailed {
				if isInBaseline(t, baseline.GreenTests) && !isTestFromChangedFile(t, changedTestFiles) && !IsOverridden(overrides, t) {
					regressed = append(regressed, t)
				}
			}
		}
		// Coarse-mode: suite failed but no named test attributed (e.g. polyglot without
		// structured output). Use a sentinel so the gate still fires. (Task-156)
		if len(regressed) == 0 {
			regressed = append(regressed, "suite_regressed")
		}
	} else {
		// Fallback: named-test detection. Works for old baselines (SuitePassed=false) and
		// suites that were partially green at baseline (some tests pass, others fail).
		for _, t := range nowFailed {
			if isInBaseline(t, baseline.GreenTests) && !isTestFromChangedFile(t, changedTestFiles) && !IsOverridden(overrides, t) {
				regressed = append(regressed, t)
			}
		}
	}

	var tampered []string
	for _, f := range diff {
		if IsTestFile(f.Path) && f.Status == "M" && !IsOverridden(overrides, filepath.Base(f.Path)) {
			tampered = append(tampered, f.Path)
		}
	}

	msg := ""
	if len(regressed) > 0 {
		if len(regressed) == 1 && regressed[0] == "suite_regressed" {
			msg = "The test suite regressed (exit-code signal; no structured output). Fix the code; do not change tests."
		} else {
			msg = "Previously-passing tests now fail: " + strings.Join(regressed, ", ") + ". Fix the code; do not change these tests."
		}
	}

	return OracleResult{
		Regressed:     regressed,
		Passed:        nowPassed,
		Tampered:      tampered,
		HasRegression: len(regressed) > 0,
		HasTampering:  len(tampered) > 0,
		Message:       msg,
	}
}

// executeSuite runs testCmd in repoDir/testDir and returns (suitePassed, passedTests, failedTests).
// suitePassed reflects the exit code (true = 0). Named test lists are best-effort for
// structured formats (go test -v, pytest -v, npm). (Task-156)
func executeSuite(repoDir, testCmd, testDir string) (suitePassed bool, passed []string, failed []string) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	parts := strings.Fields(testCmd)
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Dir = filepath.Join(repoDir, filepath.FromSlash(testDir))
	out, err := cmd.CombinedOutput()
	suitePassed = err == nil

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(testCmd, "go test"):
			if strings.Contains(line, "--- PASS:") {
				after := strings.TrimSpace(strings.TrimPrefix(line, "--- PASS:"))
				if f := strings.Fields(after); len(f) > 0 {
					passed = append(passed, f[0])
				}
			}
			if strings.Contains(line, "--- FAIL:") {
				after := strings.TrimSpace(strings.TrimPrefix(line, "--- FAIL:"))
				if f := strings.Fields(after); len(f) > 0 {
					failed = append(failed, f[0])
				}
			}
		case strings.HasPrefix(testCmd, "npm"):
			if strings.Contains(line, "✓") || strings.Contains(line, "passing") {
				name := strings.TrimSpace(strings.TrimPrefix(line, "✓"))
				if name != "" && !strings.Contains(name, "passing") {
					passed = append(passed, name)
				}
			}
			if strings.Contains(line, "✗") || strings.Contains(line, "failing") {
				name := strings.TrimSpace(strings.TrimPrefix(line, "✗"))
				if name != "" && !strings.Contains(name, "failing") {
					failed = append(failed, name)
				}
			}
		case strings.HasPrefix(testCmd, "pytest"):
			if strings.Contains(line, " PASSED") {
				if f := strings.Fields(line); len(f) >= 1 {
					passed = append(passed, f[0])
				}
			}
			if strings.Contains(line, "FAILED") {
				f := strings.Fields(line)
				for i, tok := range f {
					if tok == "FAILED" {
						if i+1 < len(f) {
							failed = append(failed, f[i+1])
						} else if i > 0 {
							failed = append(failed, f[i-1])
						}
						break
					}
				}
			}
		}
	}
	return
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
