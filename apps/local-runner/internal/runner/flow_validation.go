package runner

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

const flowValidationArtifactSourceKind = "flow_validation_result"

type FlowValidationResult struct {
	WorkflowRunID         string `json:"workflowRunId"`
	TestingStepRunID      string `json:"testingStepRunId"`
	CodingStepRunID       string `json:"codingStepRunId"`
	ValidationCommand     string `json:"validationCommand"`
	ValidationWorkingDir  string `json:"validationWorkingDir,omitempty"`
	Status                string `json:"status"`
	ExitCode              int    `json:"exitCode"`
	StdoutSummary         string `json:"stdoutSummary,omitempty"`
	StderrSummary         string `json:"stderrSummary,omitempty"`
	FailureSummary        string `json:"failureSummary,omitempty"`
	RetryAttempt          int    `json:"retryAttempt,omitempty"`
	MaxRetries            int    `json:"maxRetries,omitempty"`
	OriginalPlanPackageID string `json:"originalPlanPackageId,omitempty"`
	PreviousCodingTurnID  string `json:"previousCodingTurnId,omitempty"`
}

func runValidationCommand(ctx context.Context, cwd, command string) (stdout string, stderr string, exitCode int, err error) {
	cmd := exec.CommandContext(ctx, "sh", "-lc", command)
	if strings.TrimSpace(cwd) != "" {
		cmd.Dir = cwd
	}
	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	err = cmd.Run()
	stdout = strings.TrimSpace(stdoutBuf.String())
	stderr = strings.TrimSpace(stderrBuf.String())
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	} else if err != nil {
		exitCode = 1
	}
	return stdout, stderr, exitCode, err
}

func summarizeFlowValidationOutput(command, stdout, stderr string, exitCode int) string {
	parts := []string{
		fmt.Sprintf("Command: %s", strings.TrimSpace(command)),
		fmt.Sprintf("Exit code: %d", exitCode),
	}
	if summary := summarizeFlowValidationText(stderr, 12, 1800); summary != "" {
		parts = append(parts, "Stderr:\n"+summary)
	}
	if summary := summarizeFlowValidationText(stdout, 10, 1200); summary != "" {
		parts = append(parts, "Stdout:\n"+summary)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func summarizeFlowValidationText(text string, maxLines, maxBytes int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines = append(lines, "...")
	}
	rendered := strings.Join(lines, "\n")
	if len(rendered) > maxBytes {
		rendered = rendered[:maxBytes] + "\n...[truncated]"
	}
	return rendered
}

func buildValidationRetryPrompt(pkg FlowContextPackage, result FlowValidationResult) string {
	return strings.TrimSpace(strings.Join([]string{
		RenderFlowContextPackage(pkg),
		"",
		"## Validation Feedback",
		summarizeFlowValidationOutput(result.ValidationCommand, result.StdoutSummary, result.StderrSummary, result.ExitCode),
		"",
		"Fix production code unless the user explicitly approves requirement or test changes.",
		"Keep the original Flow Context Package unchanged.",
	}, "\n"))
}
