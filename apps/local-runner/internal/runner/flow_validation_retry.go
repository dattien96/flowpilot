package runner

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

const (
	defaultMaxRetries      = 3
	maxFailureSummaryLines = 50
	maxFailureSummaryBytes = 4 * 1024 // 4 KB cap on prompt-injected failure text
)

// ValidationResult holds the raw outcome of running a validation command.
// Raw stdout/stderr are kept in-process for summarization only; they must
// NOT be pasted into prompts or persisted to events (T-2, Task-170).
type ValidationResult struct {
	Command    string
	WorkDir    string
	ExitCode   int
	Stdout     string
	Stderr     string
	StartedAt  string
	FinishedAt string
	// EnvError is non-empty for environmental failures (command not found,
	// permission denied, dependency unavailable). These must NOT trigger a
	// Coding retry (T-4, Task-170).
	EnvError string
}

func (r ValidationResult) Passed() bool     { return r.ExitCode == 0 && r.EnvError == "" }
func (r ValidationResult) IsEnvError() bool { return r.EnvError != "" }

// ValidationResultMeta is the event-safe subset of ValidationResult.
// It records metadata and exit code only; raw logs are omitted (T-2).
type ValidationResultMeta struct {
	Command    string `json:"command"`
	WorkDir    string `json:"workDir,omitempty"`
	ExitCode   int    `json:"exitCode"`
	EnvError   string `json:"envError,omitempty"`
	StartedAt  string `json:"startedAt"`
	FinishedAt string `json:"finishedAt"`
	StdoutLen  int    `json:"stdoutLen,omitempty"`
	StderrLen  int    `json:"stderrLen,omitempty"`
}

func metaFromResult(r ValidationResult) ValidationResultMeta {
	return ValidationResultMeta{
		Command:    r.Command,
		WorkDir:    r.WorkDir,
		ExitCode:   r.ExitCode,
		EnvError:   r.EnvError,
		StartedAt:  r.StartedAt,
		FinishedAt: r.FinishedAt,
		StdoutLen:  len(r.Stdout),
		StderrLen:  len(r.Stderr),
	}
}

// ValidationSummary is the bounded, prompt-safe distillation of a
// ValidationResult. It replaces raw logs in retry prompts (T-2).
type ValidationSummary struct {
	Command      string
	ExitCode     int
	FailureLines []string
	TotalLines   int
	Truncated    bool
}

// FlowValidationRetryState holds the mutable retry loop state for one
// Testing→Coding cycle within a workflow run (T-3, Task-170).
type FlowValidationRetryState struct {
	RetryAttempt          int                `json:"retryAttempt"`
	MaxRetries            int                `json:"maxRetries"`
	OriginalPlanPackageID string             `json:"originalPlanPackageId"`
	PreviousCodingTurnID  string             `json:"previousCodingTurnId,omitempty"`
	ValidationCommand     string             `json:"validationCommand"`
	FailureSummary        *ValidationSummary `json:"failureSummary,omitempty"`
	ChangedFiles          []string           `json:"changedFiles,omitempty"`
	NextInstruction       string             `json:"nextInstruction,omitempty"`
	// Status is one of:
	//   "pending"                     — not yet run
	//   "retrying"                    — retry eligible, attempt < MaxRetries
	//   "passed"                      — validation passed
	//   "skipped_no_command"          — no command configured
	//   "skipped_env_error"           — environment/setup failure (T-4)
	//   "failed_validation_max_retries" — max retries exhausted (T-5)
	Status string `json:"status"`
}

// NewFlowValidationRetryState creates the initial retry state for a run.
func NewFlowValidationRetryState(planPackageID, command string) FlowValidationRetryState {
	if strings.TrimSpace(command) == "" {
		return FlowValidationRetryState{
			OriginalPlanPackageID: planPackageID,
			ValidationCommand:     command,
			MaxRetries:            defaultMaxRetries,
			Status:                "skipped_no_command",
		}
	}
	return FlowValidationRetryState{
		OriginalPlanPackageID: planPackageID,
		ValidationCommand:     command,
		MaxRetries:            defaultMaxRetries,
		Status:                "pending",
	}
}

// ShouldRetry returns true when the state allows a Coding retry.
func ShouldRetry(state FlowValidationRetryState) bool {
	return state.Status == "retrying" && state.RetryAttempt < state.MaxRetries
}

// AdvanceRetryState updates state in-place after a validation run.
// turnID is the Coding step turn that preceded this validation.
// changedFiles are the files modified during that Coding turn.
func AdvanceRetryState(state *FlowValidationRetryState, result ValidationResult, changedFiles []string, turnID string) {
	if result.Passed() {
		state.Status = "passed"
		return
	}
	if result.IsEnvError() {
		// Environmental failures do not count as retry attempts (T-4).
		state.Status = "skipped_env_error"
		return
	}
	state.RetryAttempt++
	state.PreviousCodingTurnID = turnID
	state.ChangedFiles = changedFiles
	summary := SummarizeValidationFailure(result, maxFailureSummaryLines)
	state.FailureSummary = &summary
	if state.RetryAttempt >= state.MaxRetries {
		state.Status = "failed_validation_max_retries"
	} else {
		state.Status = "retrying"
	}
}

// RunValidationCommand executes the validation command in cwd and returns its
// result. When the command binary cannot be found or fails to start, EnvError
// is set and ExitCode is 0 (T-4: environment failures must not trigger retry).
func RunValidationCommand(ctx context.Context, command, cwd string) ValidationResult {
	started := time.Now().UTC().Format(time.RFC3339)
	if strings.TrimSpace(command) == "" {
		return ValidationResult{
			Command:    command,
			WorkDir:    cwd,
			StartedAt:  started,
			FinishedAt: time.Now().UTC().Format(time.RFC3339),
			EnvError:   "empty_command",
		}
	}
	// V9-16: same quote-aware split as flowgate.shellSplit (avoid Fields divergence).
	// V10 P2: empty after split (e.g. quotes-only) must not panic on args[0].
	args := shellFields(command)
	if len(args) == 0 {
		return ValidationResult{
			Command:    command,
			WorkDir:    cwd,
			StartedAt:  started,
			FinishedAt: time.Now().UTC().Format(time.RFC3339),
			EnvError:   "empty_command",
		}
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	if cwd != "" {
		cmd.Dir = cwd
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	finished := time.Now().UTC().Format(time.RFC3339)
	result := ValidationResult{
		Command:    command,
		WorkDir:    cwd,
		Stdout:     stdout.String(),
		Stderr:     stderr.String(),
		StartedAt:  started,
		FinishedAt: finished,
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			// Command not found, permission denied, or other start failure.
			result.EnvError = err.Error()
		}
	}
	return result
}

// SummarizeValidationFailure extracts up to maxLines key failure lines from a
// ValidationResult and applies a hard byte cap (maxFailureSummaryBytes).
// Raw stdout/stderr are never returned; only the bounded summary is (T-2).
func SummarizeValidationFailure(result ValidationResult, maxLines int) ValidationSummary {
	// Prefer stderr (compiler/tool errors); fall back to stdout; then combine.
	combined := result.Stderr
	if strings.TrimSpace(combined) == "" {
		combined = result.Stdout
	}
	if strings.TrimSpace(combined) == "" {
		combined = result.Stdout + "\n" + result.Stderr
	}
	all := strings.Split(combined, "\n")
	summary := ValidationSummary{
		Command:    result.Command,
		ExitCode:   result.ExitCode,
		TotalLines: len(all),
	}

	// First pass: collect lines that look like failure markers.
	var kept []string
	for _, line := range all {
		if l := strings.TrimSpace(line); l != "" && isFailureLine(l) {
			kept = append(kept, line)
		}
	}
	// Fallback: if no markers found, take first N non-empty lines.
	if len(kept) == 0 {
		for _, line := range all {
			if strings.TrimSpace(line) != "" {
				kept = append(kept, line)
			}
		}
	}
	if len(kept) > maxLines {
		kept = kept[:maxLines]
		summary.Truncated = true
	}

	// Apply final byte cap.
	var sb strings.Builder
	for _, l := range kept {
		if sb.Len()+len(l)+1 > maxFailureSummaryBytes {
			summary.Truncated = true
			break
		}
		sb.WriteString(l)
		sb.WriteByte('\n')
	}
	lines := strings.Split(strings.TrimRight(sb.String(), "\n"), "\n")
	// Filter empty tail from Split.
	var filtered []string
	for _, l := range lines {
		if l != "" {
			filtered = append(filtered, l)
		}
	}
	summary.FailureLines = filtered
	return summary
}

// isFailureLine returns true for lines that signal compiler errors, test
// failures, panics, or assertion messages.
func isFailureLine(line string) bool {
	lower := strings.ToLower(line)
	for _, marker := range []string{
		"error:", "--- fail", "panic:", "assert", "expected",
		"exit status", "undefined:", "cannot ", "not found",
		"error[", "❌", "✗", "fail\t", "fail ",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// ComposeRetryPrompt builds the Coding retry prompt: original Plan package
// (from Task-169) plus a bounded failure summary and retry instruction (T-4).
// The original FlowContextPackage is NEVER modified (T-1).
func ComposeRetryPrompt(pkg FlowContextPackage, state FlowValidationRetryState) string {
	var sb strings.Builder
	// Reuse the Task-169 Coding prompt composition with an empty instruction, then
	// append the failure block.
	basePrompt := ComposeFlowCodingPrompt(pkg, "")
	sb.WriteString(basePrompt)
	sb.WriteString("\n---\n\n")
	sb.WriteString(fmt.Sprintf("## Validation Failure — Retry %d/%d\n\n", state.RetryAttempt, state.MaxRetries))
	if state.FailureSummary != nil {
		sb.WriteString(fmt.Sprintf("**Command**: `%s`  \n", state.FailureSummary.Command))
		sb.WriteString(fmt.Sprintf("**Exit code**: %d\n\n", state.FailureSummary.ExitCode))
		if len(state.FailureSummary.FailureLines) > 0 {
			sb.WriteString("**Key failure lines**:\n```\n")
			for _, l := range state.FailureSummary.FailureLines {
				sb.WriteString(l + "\n")
			}
			sb.WriteString("```\n")
			if state.FailureSummary.Truncated {
				sb.WriteString("_(output truncated — raw logs available in workspace)_\n")
			}
		}
	}
	if len(state.ChangedFiles) > 0 {
		sb.WriteString("\n**Files changed in previous attempt**: ")
		sb.WriteString(strings.Join(state.ChangedFiles, ", ") + "\n")
	}
	sb.WriteString("\n[Retry instruction: Fix the validation failure above in production code. " +
		"Do NOT edit tests to make them pass. " +
		"If the failure points to a spec conflict, surface it for user review before changing tests.]\n\n")
	if state.NextInstruction != "" {
		sb.WriteString(state.NextInstruction)
	}
	return sb.String()
}

// PersistValidationResult emits an EventFlowValidationResult event with the
// event-safe metadata so the Testing step outcome is durable and inspectable.
// Raw stdout/stderr are not included in the event (T-2, T-5, T-6).
func PersistValidationResult(ctx context.Context, store InteractiveStateStore, runID, stepID string, result ValidationResult) error {
	meta := metaFromResult(result)
	return store.AppendEvent(ctx, ProviderEvent{
		Type:                 EventFlowValidationResult,
		WorkflowRunID:        runID,
		WorkflowStepRunID:    stepID,
		FlowValidationResult: &meta,
	})
}

// PersistRetryState emits an EventFlowValidationRetry event capturing the retry
// state transition so UI/replay can explain why a retry happened (T-3, T-6).
func PersistRetryState(ctx context.Context, store InteractiveStateStore, runID, stepID string, state FlowValidationRetryState) error {
	return store.AppendEvent(ctx, ProviderEvent{
		Type:                     EventFlowValidationRetry,
		WorkflowRunID:            runID,
		WorkflowStepRunID:        stepID,
		FlowValidationRetryState: &state,
	})
}
