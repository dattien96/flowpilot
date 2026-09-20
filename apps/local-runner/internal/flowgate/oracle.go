package flowgate

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

type OracleResult struct {
	Regressed     []string `json:"regressed"`
	Passed        []string `json:"passed,omitempty"`
	Failed        []string `json:"failed,omitempty"` // named failures this run (Task-242 validate mapping)
	Tampered      []string `json:"tampered"`
	HasRegression bool     `json:"has_regression"`
	HasTampering  bool     `json:"has_tampering"`
	SuitePassed   bool     `json:"suite_passed"` // exit-code of the suite run (Task-242 D-4)
	Message       string   `json:"message,omitempty"`
	// Output is the suite's combined stdout+stderr (bounded by the process pipe).
	// Validate/retry maps this into ValidationResult so compile errors and
	// unstructured failures produce actionable failure summaries (Task-242).
	Output string `json:"-"`
	// EnvError is set when the suite command could not start (not found, permission,
	// empty argv). Callers must treat this like ValidationResult.EnvError — skip
	// retry/regression, not "suite regressed". (Task-242 validate path)
	EnvError string `json:"env_error,omitempty"`
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
//
// Background context (legacy call sites). Prefer RunOracleContext when a turn
// context should cancel/timeout the suite.
func RunOracle(repoDir string, baseline *Baseline, diff []ChangedFile, overrides map[string]Override) OracleResult {
	return RunOracleContext(context.Background(), repoDir, baseline, diff, overrides)
}

// RunOracleContext is RunOracle with an explicit context (Task-242 validate path
// propagates the turn ctx so cancel/timeout are honored).
func RunOracleContext(ctx context.Context, repoDir string, baseline *Baseline, diff []ChangedFile, overrides map[string]Override) OracleResult {
	if baseline == nil {
		return OracleResult{}
	}
	if baseline.TestCmd == "" {
		return OracleResult{Disabled: true}
	}
	if ctx == nil {
		ctx = context.Background()
	}

	testCmd := baseline.TestCmd
	if scoped := scopeTestCommand(repoDir, baseline.TestDir, testCmd, diff); scoped != "" {
		testCmd = scoped
		log.Printf("[gate] scoped oracle run: %s", testCmd)
	}
	suitePassed, nowPassed, nowFailed, envErr, suiteOut := executeSuite(ctx, repoDir, testCmd, baseline.TestDir)
	if envErr != "" {
		// Command did not start, or turn/timeout cancelled the suite — not a regression.
		msg := "test command failed to start: " + envErr
		if isContextAbortError(envErr) {
			msg = "test suite cancelled: " + envErr
		}
		return OracleResult{
			EnvError:    envErr,
			Message:     msg,
			Output:      suiteOut,
			Disabled:    false,
			SuitePassed: false,
		}
	}

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
			// Named granularity — filter to tests that were green, not in diff, not overridden.
			for _, t := range nowFailed {
				if isInBaseline(t, baseline.GreenTests) && !isTestFromChangedFile(t, changedTestFiles) && !IsOverridden(overrides, t) {
					regressed = append(regressed, t)
				}
			}
			// V9-26: when every named failure was overridden/in-diff, leave regressed
			// empty (do not invent suite_regressed — that is "all approved", not unknown).
		} else {
			// Coarse-mode: suite failed with no named tests (polyglot). Sentinel. (Task-156)
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
		// BUG-288 #20: treat modify/delete/rename of existing tests as tamper.
		if IsTestFile(f.Path) && (f.Status == "M" || f.Status == "D" || f.Status == "R" || f.Status == "C") &&
			!IsOverridden(overrides, filepath.Base(f.Path)) {
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
		Failed:        nowFailed,
		Tampered:      tampered,
		HasRegression: len(regressed) > 0,
		HasTampering:  len(tampered) > 0,
		SuitePassed:   suitePassed,
		Message:       msg,
		Output:        suiteOut,
	}
}

// maxSuiteOutputBytes caps suite stdout/stderr retained on OracleResult so a
// noisy suite cannot unbounded-allocate. Tail is kept (failures land near end).
const maxSuiteOutputBytes = 64 * 1024

// tailCapWriter is an io.Writer that retains only the last max bytes while
// still draining the full stream (process never blocks on a full pipe).
// Peak retained memory is O(max), not O(total suite log).
type tailCapWriter struct {
	max int
	buf []byte
	// truncated is true once we have discarded older bytes.
	truncated bool
}

func (t *tailCapWriter) Write(p []byte) (int, error) {
	if t.max <= 0 {
		return len(p), nil
	}
	if len(p) >= t.max {
		// Keep only the tail of this write.
		t.buf = append(t.buf[:0], p[len(p)-t.max:]...)
		t.truncated = true
		return len(p), nil
	}
	need := len(t.buf) + len(p) - t.max
	if need > 0 {
		t.buf = t.buf[need:]
		t.truncated = true
	}
	t.buf = append(t.buf, p...)
	return len(p), nil
}

func (t *tailCapWriter) String() string {
	if !t.truncated || len(t.buf) == 0 {
		return string(t.buf)
	}
	const marker = "…(suite output truncated)…\n"
	return marker + string(t.buf)
}

// executeSuite runs testCmd in repoDir/testDir and returns
// (suitePassed, passedTests, failedTests, envError, combinedOutput).
// suitePassed reflects the exit code (true = 0). Named test lists are best-effort for
// structured formats (go test -v, pytest -v, npm). (Task-156)
// envError is non-empty when the command cannot start (not found / permission / empty)
// OR when the turn/timeout context cancelled the suite (not a test failure).
// combinedOutput is capped via streaming tail writers (not post-hoc CombinedOutput).
// suiteGraceAfterCancel bounds how long executeSuite waits for cmd.Wait after
// the suite was killed on ctx cancel/timeout (BUG-354 run-540927): a process
// tree that still holds the output pipes must not pin the post-turn gate
// forever — after the grace the suite is abandoned with an EnvError.
const suiteGraceAfterCancel = 2 * time.Second

func executeSuite(ctx context.Context, repoDir, testCmd, testDir string) (suitePassed bool, passed []string, failed []string, envError string, combinedOutput string) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// BUG-288 #25: quote-aware split (not strings.Fields) for -run 'Test Foo' etc.
	parts := shellSplit(testCmd)
	if len(parts) == 0 {
		return false, nil, nil, "empty_command", ""
	}
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Dir = filepath.Join(repoDir, filepath.FromSlash(testDir))
	// BUG-354 (run-540927): own process group so cancel can kill the whole
	// tree — a grandchild holding the output pipes used to keep cmd.Wait
	// blocked past the 5-minute deadline with no log and no watchdog escape.
	setSuiteProcessGroup(cmd)
	// Stream into a shared tail buffer so peak RAM is O(maxSuiteOutputBytes).
	outCap := &tailCapWriter{max: maxSuiteOutputBytes}
	cmd.Stdout = outCap
	cmd.Stderr = outCap
	start := time.Now()
	log.Printf("[gate] suite start cmd=%q dir=%q", testCmd, cmd.Dir)
	waitErr := make(chan error, 1)
	// Publish cmd.Process before the cancellation path can inspect it.
	// Running Start concurrently with killSuiteProcessGroup races on Process
	// and its PID; only Wait needs to run in the background for bounded kill.
	if startErr := cmd.Start(); startErr != nil {
		waitErr <- startErr
	} else {
		go func() { waitErr <- cmd.Wait() }()
	}
	var err error
	aborted := false
WaitLoop:
	for {
		select {
		case err = <-waitErr:
			break WaitLoop
		case <-ctx.Done():
			aborted = true
			killSuiteProcessGroup(cmd)
			select {
			case err = <-waitErr:
			case <-time.After(suiteGraceAfterCancel):
				err = fmt.Errorf("suite wait stuck after kill (run-540927 guard): %w", ctx.Err())
			}
			break WaitLoop
		}
	}
	combinedOutput = outCap.String()
	log.Printf("[gate] suite end cmd=%q duration=%s aborted=%v err=%v outputBytes=%d",
		testCmd, time.Since(start).Round(time.Millisecond), aborted, err, len(combinedOutput))

	// CommandContext kill on cancel/timeout often surfaces as *exec.ExitError
	// (signal). Prefer ctx.Err() so cancellation is never a suite regression.
	if cerr := ctx.Err(); cerr != nil {
		return false, nil, nil, cerr.Error(), combinedOutput
	}
	if err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			// Not an exit-code failure — binary missing, permission, etc.
			return false, nil, nil, err.Error(), combinedOutput
		}
		suitePassed = false
	} else {
		suitePassed = true
	}

	scanner := bufio.NewScanner(strings.NewReader(combinedOutput))
	// Default MaxScanTokenSize is 64KiB; retained output is maxSuiteOutputBytes
	// plus the truncation marker and can be one long line. Raise the limit so
	// named PASS/FAIL lines after a long spam line are still parsed.
	scanBuf := make([]byte, 0, 64*1024)
	scanner.Buffer(scanBuf, maxSuiteOutputBytes+8*1024)
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

func isContextAbortError(msg string) bool {
	// Match context.Canceled / DeadlineExceeded text from ctx.Err().Error().
	low := strings.ToLower(msg)
	return strings.Contains(low, "context canceled") ||
		strings.Contains(low, "context deadline exceeded")
}

// compileErrorRegex matches the toolchain's own diagnostic prefix shapes
// (`error TS2345:`, `SyntaxError:`, `E   SyntaxError:`) without hard-coding a
// full grammar per runner.
var compileErrorRegex = regexp.MustCompile(`(?i)(\berror ts\d+\b|\bsyntaxerror\b)`)

// ClassifySuiteOutput reports whether a suite's combined output shows a
// COMPILE/COLLECTION failure rather than an assertion failure (CP-64 P-1
// T-2/T-3). The runner's reproduce gate uses this to refuse a compile error as
// evidence of reproduction: a test that never built proves nothing about the
// bug, while a test that built and failed on `expected X got Y` is the
// reproduce-first oracle.
//
// Deliberately additive: it only reads the output string executeSuite already
// captured on OracleResult.Output, so RunOracleContext's behavior is unchanged.
// Detection is best-effort per runner family — a miss degrades to "assertion
// failure", which is the pre-CP-64 verdict, never a false block.
func ClassifySuiteOutput(testCmd, output string) bool {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return false
	}
	low := strings.ToLower(trimmed)
	cmd := strings.ToLower(strings.TrimSpace(testCmd))

	switch {
	case strings.Contains(cmd, "go test"):
		for _, sig := range []string{
			"[build failed]",
			"[setup failed]",
			"syntax error:",
			"missing ','",
			"expected 'package'",
			"undefined:",
			"cannot use ",
			"declared and not used",
			"imported and not used",
			"is not a type",
			"missing return",
			"expected declaration",
			"too many arguments",
			"not enough arguments",
		} {
			if strings.Contains(low, sig) {
				return true
			}
		}
		return false
	case strings.Contains(cmd, "pytest"):
		for _, sig := range []string{
			"error collecting",
			"errors during collection",
			"syntaxerror",
			"indentationerror",
			"modulenotfounderror",
			"importerror",
			"e   syntaxerror",
		} {
			if strings.Contains(low, sig) {
				return true
			}
		}
		return false
	case strings.Contains(cmd, "npm"), strings.Contains(cmd, "npx"),
		strings.Contains(cmd, "yarn"), strings.Contains(cmd, "pnpm"),
		strings.Contains(cmd, "node "), strings.Contains(cmd, "jest"),
		strings.Contains(cmd, "vitest"):
		// TypeScript/Jest/Vitest fail to BUILD on a module-resolution or
		// parse error; anything else is an ordinary assertion failure.
		for _, sig := range []string{"cannot find module", "unexpected token", "failed to load esm"} {
			if strings.Contains(low, sig) {
				return true
			}
		}
		return compileErrorRegex.MatchString(trimmed)
	default:
		// Unknown runner: only the unambiguous cross-toolchain markers.
		if strings.Contains(trimmed, "[build failed]") || strings.Contains(trimmed, "[setup failed]") {
			return true
		}
		return compileErrorRegex.MatchString(trimmed)
	}
}

// shellSplit splits a command with simple single/double quote awareness so
// go test -run 'Test Foo' keeps the pattern as one arg (BUG-288 #25).
// Does not run a shell: no env expansion, pipes, or &&.
func shellSplit(cmd string) []string {
	var out []string
	var b strings.Builder
	var quote rune
	flush := func() {
		if b.Len() == 0 {
			return
		}
		out = append(out, b.String())
		b.Reset()
	}
	for _, r := range cmd {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				b.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote = r
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			flush()
		default:
			b.WriteRune(r)
		}
	}
	flush()
	return out
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
