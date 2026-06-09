package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Provider-driven MCP test execution functions

const mcpFailureCodeMarker = "MCP_FAILURE_CODE"

// runProviderDrivenMcpTest executes an MCP test using the AI provider CLI
func (r *Runner) runProviderDrivenMcpTest(ctx context.Context, request McpTestRequest) (McpTestResult, error) {
	// Validate provider-driven test fields
	if strings.TrimSpace(request.AIProviderKey) == "" {
		return McpTestResult{}, errors.New("aiProviderKey is required for provider-driven tests")
	}
	if strings.TrimSpace(request.AccountHomePath) == "" {
		return McpTestResult{}, errors.New("accountHomePath is required for provider-driven tests")
	}

	// Run preflight checks
	preflightResult := r.PreflightGoogleDriveMcp(request.AIProviderKey, request.AccountHomePath)
	if preflightResult.ErrorMessage != "" {
		return McpTestResult{
			Status:        "failed",
			ErrorMessage:  preflightResult.ErrorMessage,
			AIProviderKey: request.AIProviderKey,
			AIModelName:   request.AIModelName,
			McpServerName: "google-drive",
			StartedAt:     time.Now().UTC().Format(time.RFC3339Nano),
			CompletedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		}, nil
	}

	// Generate verification prompt with MCP instructions
	verificationPrompt := generateMcpVerificationPrompt(request.AIProviderKey)
	verificationPrompt = InjectRequiredMcpInstructions(verificationPrompt, []string{"google_drive"}, request.AIProviderKey, false, false)

	// Create run directory for artifacts
	runID := newMcpTestRunID()
	runDir := filepath.Join(r.workspace, ".flowpilot", "mcp-tests", runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return McpTestResult{}, err
	}

	// Save artifacts
	promptPath := filepath.Join(runDir, "prompt.txt")
	if err := os.WriteFile(promptPath, []byte(verificationPrompt), 0o644); err != nil {
		return McpTestResult{}, err
	}

	// Execute provider with verification prompt
	startedAt := time.Now().UTC()
	timeout := 10 * time.Minute
	if request.TimeoutMs > 0 {
		timeout = time.Duration(request.TimeoutMs) * time.Millisecond
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute prompt via provider
	execRequest := PromptExecutionRequest{
		ProviderKey:     request.AIProviderKey,
		Prompt:          verificationPrompt,
		AccountHomePath: request.AccountHomePath,
	}

	if request.AIModelName != "" {
		execRequest.ModelName = request.AIModelName
	}

	if request.WorkingDirectory != "" {
		execRequest.WorkingDirectory = request.WorkingDirectory
	}

	execResult, execErr := r.ExecutePrompt(execCtx, execRequest)
	completedAt := time.Now().UTC()

	// Build test result
	result := McpTestResult{
		RunID:         runID,
		AIProviderKey: request.AIProviderKey,
		AIModelName:   request.AIModelName,
		McpServerName: "google-drive",
		StartedAt:     startedAt.Format(time.RFC3339Nano),
		CompletedAt:   completedAt.Format(time.RFC3339Nano),
		ArtifactPaths: []string{promptPath},
		// Populate existing fields if available
		BackendKey:    request.BackendKey,
		ProviderType:  request.ProviderType,
		ProjectID:     request.ProjectID,
		IntegrationID: request.IntegrationID,
	}

	if execErr != nil {
		result.Status = "failed"
		result.ErrorMessage = fmt.Sprintf("Provider execution failed: %v", execErr)
		return result, nil
	}

	// Parse provider output to detect MCP status
	combinedOutput := execResult.StdoutSummary + "\n" + execResult.StderrSummary + "\n" + execResult.OutputMarkdown

	// Save provider output
	stdoutPath := filepath.Join(runDir, "stdout.txt")
	if err := os.WriteFile(stdoutPath, []byte(execResult.StdoutSummary), 0o644); err != nil {
		return result, err
	}
	result.ArtifactPaths = append(result.ArtifactPaths, stdoutPath)

	stderrPath := filepath.Join(runDir, "stderr.txt")
	if err := os.WriteFile(stderrPath, []byte(execResult.StderrSummary), 0o644); err != nil {
		return result, err
	}
	result.ArtifactPaths = append(result.ArtifactPaths, stderrPath)

	outputPath := filepath.Join(runDir, "output.md")
	if err := os.WriteFile(outputPath, []byte(combinedOutput), 0o644); err != nil {
		return result, err
	}
	result.ArtifactPaths = append(result.ArtifactPaths, outputPath)

	// Detect MCP usage and failure codes
	toolUsed := detectMcpToolUsed(combinedOutput)
	failureCode := detectMcpFailureCode(combinedOutput)

	result.McpToolUsed = toolUsed
	result.McpFailureCode = failureCode
	result.StdoutSummary = execResult.StdoutSummary
	result.StderrSummary = execResult.StderrSummary
	result.OutputMarkdown = combinedOutput

	// Determine overall status
	if failureCode != "" {
		result.Status = "failed"
		result.ErrorMessage = fmt.Sprintf("MCP failure detected: %s", failureCode)
	} else if toolUsed != "" {
		result.Status = "success"
		result.ErrorMessage = "" // Clear error on success
	} else {
		result.Status = "failed"
		result.ErrorMessage = "No MCP tool usage detected in provider output"
	}

	// Save result
	resultPath := filepath.Join(runDir, "result.json")
	if err := writeJSONFile(resultPath, result); err != nil {
		return result, err
	}
	result.ArtifactPaths = append(result.ArtifactPaths, resultPath)

	// Save request for reference
	requestPath := filepath.Join(runDir, "request.json")
	if err := writeJSONFile(requestPath, request); err != nil {
		// Log but don't fail on request save error
		_ = err
	} else {
		result.ArtifactPaths = append(result.ArtifactPaths, requestPath)
	}

	return result, nil
}

// generateMcpVerificationPrompt creates a prompt that requests MCP tool usage
func generateMcpVerificationPrompt(providerKey string) string {
	var sb strings.Builder

	sb.WriteString("# Google Drive MCP Verification\n\n")
	sb.WriteString("You have access to Google Drive MCP tools. Verify that the connection works by:\n\n")
	sb.WriteString("1. Use `authGetStatus` or equivalent diagnostic tool to check authentication status\n")
	sb.WriteString("2. If auth fails, report the error clearly\n")
	sb.WriteString("3. If auth succeeds, list 1-3 files from Google Drive using `search` or `listFolder`\n")
	sb.WriteString("4. Include the file name and file ID for each file\n\n")
	sb.WriteString("Respond with a structured summary:\n")
	sb.WriteString("- MCP Server: google-drive\n")
	sb.WriteString("- Tool Used: [tool name]\n")
	sb.WriteString("- Status: [success/error]\n")
	sb.WriteString("- Details: [error message or file listing]\n")

	return sb.String()
}

// detectMcpToolUsed extracts the first MCP tool used from provider output
func detectMcpToolUsed(output string) string {
	// Look for common patterns that indicate tool usage
	patterns := []string{
		`(?i)calling.*?(authGetStatus|search|listFolder|readGoogleDoc|list_contents|read_file|auth_status)`,
		`(?i)(authGetStatus|search|listFolder|readGoogleDoc|list_contents|read_file|auth_status)\s*\(`,
		`(?i)used\s+(authGetStatus|search|listFolder|readGoogleDoc|list_contents|read_file|auth_status)`,
		`(?i)tool\s*[:\s]+\s*(authGetStatus|search|listFolder|readGoogleDoc|list_contents|read_file|auth_status)`,
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(output)
		if len(matches) > 1 {
			return strings.TrimSpace(matches[1])
		}
	}

	// No explicit tool usage found
	return ""
}

// detectMcpFailureCode detects failure codes in provider output
func detectMcpFailureCode(output string) string {
	failureCodes := strings.Join([]string{
		"mcp_unavailable",
		"mcp_auth_required",
		"mcp_write_approval_required",
		"mcp_tool_blocked",
		"mcp_tool_failed",
		"drive_content_not_found",
	}, "|")
	explicitMarkerPattern := regexp.MustCompile(`(?i)(?:^|.*\s)` + regexp.QuoteMeta(mcpFailureCodeMarker) + `\s*[:=-]\s*(` + failureCodes + `)\s*$`)
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)^\s*(?:error|failure|tool execution failed|mcp failure detected)\s*[:=-]?\s*(` + failureCodes + `)\b`),
		regexp.MustCompile(`(?i)^\s*(` + failureCodes + `)(?:\b|[:\s-])`),
	}

	rawLines := make([]string, 0)
	normalizedLines := make([]string, 0)
	for _, line := range strings.Split(output, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		rawLines = append(rawLines, trimmed)
		normalized := strings.TrimSpace(strings.Trim(trimmed, "`\"'"))
		normalizedLines = append(normalizedLines, normalized)
	}

	if len(rawLines) > 0 {
		lastLine := rawLines[len(rawLines)-1]
		matches := explicitMarkerPattern.FindStringSubmatch(lastLine)
		if len(matches) > 1 {
			return strings.ToLower(strings.TrimSpace(matches[1]))
		}
	}

	for _, normalized := range normalizedLines {
		for _, pattern := range patterns {
			matches := pattern.FindStringSubmatch(normalized)
			if len(matches) > 1 {
				return strings.ToLower(strings.TrimSpace(matches[1]))
			}
		}
	}

	return ""
}
