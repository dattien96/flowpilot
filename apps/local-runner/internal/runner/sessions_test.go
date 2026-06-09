package runner

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type nopWriteCloser struct {
	io.Writer
}

func (n nopWriteCloser) Close() error {
	return nil
}

func TestSweepIdleSessions(t *testing.T) {
	r, _ := New(".")

	r.sessionsMu.Lock()
	r.sessions["test-proc"] = &LiveSession{
		SessionID:  "test-sess",
		Provider:   "codex",
		Model:      "codex-mcp",
		LastUsedAt: time.Now().UTC().Add(-2 * time.Hour),
		IdleTTL:    1 * time.Hour,
		Status:     "active",
	}
	r.sessionsMu.Unlock()

	r.sweepIdleSessions()

	r.sessionsMu.Lock()
	_, exists := r.sessions["test-proc"]
	r.sessionsMu.Unlock()

	if exists {
		t.Errorf("Expected session to be swept, but it still exists")
	}
}

func TestDetermineProviderSessionID(t *testing.T) {
	// Test Gemini fallback
	geminiID := DetermineProviderSessionID("gemini_acp", "gemini", "123", "", nil)
	if geminiID != "gemini_acp_session_123" {
		t.Errorf("Expected gemini_acp_session_123, got %s", geminiID)
	}

	// Test Claude fallback
	claudeID := DetermineProviderSessionID("claude_stream_json", "claude", "456", "", nil)
	if claudeID != "claude_stream_session_456" {
		t.Errorf("Expected claude_stream_session_456, got %s", claudeID)
	}

	// Test Codex resume
	resumeID := "old-codex-thread"
	codexID := DetermineProviderSessionID("codex_mcp", "codex", "789", "current-thread", &resumeID)
	if codexID != "old-codex-thread" {
		t.Errorf("Expected old-codex-thread, got %s", codexID)
	}

	// Test Gemini ignore resume
	geminiIgnoreID := DetermineProviderSessionID("gemini_acp", "gemini", "123", "current-thread", &resumeID)
	if geminiIgnoreID != "current-thread" {
		t.Errorf("Expected current-thread, got %s", geminiIgnoreID)
	}

	// Test Claude resume
	claudeResumeID := DetermineProviderSessionID("claude_stream_json", "claude", "456", "", &resumeID)
	if claudeResumeID != "old-codex-thread" {
		t.Errorf("Expected old-codex-thread, got %s", claudeResumeID)
	}
}

func TestStartSessionResumesProviderSessionID(t *testing.T) {
	// Mock the command so it doesn't try to run a real process
	originalCmdCtx := commandContextFn
	defer func() { commandContextFn = originalCmdCtx }()
	commandContextFn = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		// Keep the mock process alive long enough for the Codex MCP handshake to finish.
		script := "$null = [Console]::In.ReadLine(); Write-Output '{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}'; $null = [Console]::In.ReadLine(); Start-Sleep -Seconds 2"
		return exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script)
	}

	r, _ := New(".")

	resumeID := "old-codex-thread-123"

	req := AiSessionStartRequest{
		ProviderKey:             "codex",
		ModelName:               "codex-mcp",
		WorkingDirectory:        ".",
		ResumeProviderSessionID: &resumeID,
	}

	handle, err := r.StartSession(context.Background(), req)

	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	if handle.ProviderSessionID != "old-codex-thread-123" {
		t.Errorf("Expected StartSession to seed handle with old-codex-thread-123, got %s", handle.ProviderSessionID)
	}

	// Clean up
	if handle.ProcessKey != nil {
		if err := r.CloseSession(context.Background(), handle); err != nil {
			t.Fatalf("CloseSession failed: %v", err)
		}
	}
}

func TestStartSessionGeminiACPUsesCurrentHandshake(t *testing.T) {
	originalCmdCtx := commandContextFn
	defer func() { commandContextFn = originalCmdCtx }()
	var startedArgs []string
	commandContextFn = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		startedArgs = append([]string{}, arg...)
		return exec.CommandContext(
			ctx,
			"powershell",
			"-NoProfile",
			"-Command",
			"Write-Output '{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"protocolVersion\":1}}'; Write-Output '{\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{\"sessionId\":\"gemini-live-session\"}}'; Start-Sleep -Seconds 2",
		)
	}

	r, _ := New(".")

	handle, err := r.StartSession(context.Background(), AiSessionStartRequest{
		ProviderKey:      "gemini",
		ModelName:        "gemini-flash",
		WorkingDirectory: ".",
	})
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	if handle.ProviderSessionID != "gemini-live-session" {
		t.Fatalf("expected Gemini provider session id to come from ACP session/new, got %q", handle.ProviderSessionID)
	}

	if !strings.Contains(strings.Join(startedArgs, " "), "--model gemini-2.5-flash") {
		t.Fatalf("expected Gemini ACP session to normalize legacy model alias, got args %v", startedArgs)
	}

	if handle.ProcessKey != nil {
		if err := r.CloseSession(context.Background(), handle); err != nil {
			t.Fatalf("CloseSession failed: %v", err)
		}
	}
}

func TestStartSessionCodexMcpUsesRequestedModelConfig(t *testing.T) {
	originalCmdCtx := commandContextFn
	defer func() { commandContextFn = originalCmdCtx }()
	var startedArgs []string
	commandContextFn = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		startedArgs = append([]string{}, arg...)
		return exec.CommandContext(
			ctx,
			"powershell",
			"-NoProfile",
			"-Command",
			"Write-Output '{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}'; Start-Sleep -Seconds 2",
		)
	}

	r, _ := New(".")
	reasoningEffort := "medium"

	handle, err := r.StartSession(context.Background(), AiSessionStartRequest{
		ProviderKey:      "codex",
		ModelName:        "gpt-5.4-mini",
		ReasoningEffort:  &reasoningEffort,
		WorkingDirectory: ".",
	})
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	if got := strings.Join(startedArgs, " "); !strings.Contains(got, "-c model=\"gpt-5.4-mini\"") {
		t.Fatalf("expected Codex MCP session to pass requested model config, got args %v", startedArgs)
	}
	if got := strings.Join(startedArgs, " "); !strings.Contains(got, "-c model_reasoning_effort=medium") {
		t.Fatalf("expected Codex MCP session to pass requested reasoning effort config, got args %v", startedArgs)
	}

	if handle.ProcessKey != nil {
		if err := r.CloseSession(context.Background(), handle); err != nil {
			t.Fatalf("CloseSession failed: %v", err)
		}
	}
}

func TestStartSessionInjectsProcessKeyIntoProviderEnv(t *testing.T) {
	originalCmdCtx := commandContextFn
	defer func() { commandContextFn = originalCmdCtx }()

	envCaptureDir := t.TempDir()
	envCapturePath := filepath.Join(envCaptureDir, "process-key.txt")
	commandContextFn = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		script := "$env:FLOWPILOT_PROCESS_KEY | Set-Content -LiteralPath '" + strings.ReplaceAll(envCapturePath, "'", "''") + "'; Write-Output '{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}'; Start-Sleep -Seconds 2"
		return exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script)
	}

	r, _ := New(".")
	handle, err := r.StartSession(context.Background(), AiSessionStartRequest{
		ProviderKey:      "codex",
		ModelName:        "gpt-5.4-mini",
		WorkingDirectory: ".",
	})
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	if handle.ProcessKey == nil {
		t.Fatal("expected process key to be populated")
	}

	raw, err := os.ReadFile(envCapturePath)
	if err != nil {
		t.Fatalf("failed to read captured process key: %v", err)
	}
	got := strings.TrimSpace(string(raw))
	if got != *handle.ProcessKey {
		t.Fatalf("expected captured process key %q, got %q", *handle.ProcessKey, got)
	}

	if err := r.CloseSession(context.Background(), handle); err != nil {
		t.Fatalf("CloseSession failed: %v", err)
	}
}

func TestStartSessionGeminiACPRejectsInitializeErrors(t *testing.T) {
	originalCmdCtx := commandContextFn
	defer func() { commandContextFn = originalCmdCtx }()
	commandContextFn = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		return exec.CommandContext(
			ctx,
			"powershell",
			"-NoProfile",
			"-Command",
			"Write-Output '{\"jsonrpc\":\"2.0\",\"id\":1,\"error\":{\"message\":\"bad initialize\"}}'",
		)
	}

	r, _ := New(".")

	_, err := r.StartSession(context.Background(), AiSessionStartRequest{
		ProviderKey:      "gemini",
		ModelName:        "gemini-flash",
		WorkingDirectory: ".",
	})
	if err == nil {
		t.Fatal("expected StartSession to fail when Gemini initialize returns an error")
	}
	if !strings.Contains(err.Error(), "failed Gemini ACP initialize: bad initialize") {
		t.Fatalf("expected initialize error to be surfaced, got %v", err)
	}
}

func TestStartSessionClaudeUsesVirtualSessionState(t *testing.T) {
	originalCmdCtx := commandContextFn
	originalLookPath := lookPathFn
	defer func() {
		commandContextFn = originalCmdCtx
		lookPathFn = originalLookPath
	}()

	commandStarted := false
	commandContextFn = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		commandStarted = true
		return exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", "Write-Output unexpected")
	}
	lookPathFn = func(file string) (string, error) {
		return file, nil
	}

	r, _ := New(".")
	resumeID := "claude-session-old"
	handle, err := r.StartSession(context.Background(), AiSessionStartRequest{
		ProviderKey:             "claude",
		ModelName:               "claude-haiku",
		WorkingDirectory:        ".",
		ResumeProviderSessionID: &resumeID,
	})
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	if commandStarted {
		t.Fatal("expected Claude StartSession to avoid starting a long-lived process")
	}
	if handle.ProviderSessionID != "claude-session-old" {
		t.Fatalf("expected Claude StartSession to seed the resume session id, got %q", handle.ProviderSessionID)
	}
	if handle.ProcessPid != nil {
		t.Fatalf("expected virtual Claude session to have no pid, got %v", *handle.ProcessPid)
	}
	if handle.ProcessKey == nil {
		t.Fatal("expected Claude StartSession to return a process key")
	}
}

func TestSendMessageGeminiACPUsesContentBlocksAndStreamsText(t *testing.T) {
	r, _ := New(".")

	var stdin bytes.Buffer
	processKey := "gemini-proc"
	r.sessions[processKey] = &LiveSession{
		SessionID:         "session-1",
		Provider:          "gemini",
		Model:             "gemini-flash",
		TransportType:     "gemini_acp",
		ProviderSessionID: "gemini-live-session",
		Stdin:             nopWriteCloser{Writer: &stdin},
		StdoutScanner: bufio.NewScanner(strings.NewReader(strings.Join([]string{
			`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"gemini-live-session","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"Hello "}}}}`,
			`{"jsonrpc":"2.0","method":"session/update","params":{"sessionId":"gemini-live-session","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"world"}}}}`,
			`{"jsonrpc":"2.0","id":3,"result":{"stopReason":"end_turn"}}`,
		}, "\n"))),
		LastUsedAt: time.Now().UTC(),
		IdleTTL:    time.Hour,
		Status:     "active",
	}

	result, err := r.SendMessage(context.Background(), AiSessionMessageRequest{
		Session: AiSessionHandle{
			TransportType:     "gemini_acp",
			ProviderSessionID: "gemini-live-session",
			ProcessKey:        &processKey,
		},
		Prompt: "Reply with just OK",
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if result.OutputMarkdown != "Hello world" {
		t.Fatalf("expected streamed Gemini output to be collected, got %q", result.OutputMarkdown)
	}

	written := stdin.String()
	if !strings.Contains(written, `"method":"session/prompt"`) {
		t.Fatalf("expected Gemini prompt request to be written, got %q", written)
	}
	if !strings.Contains(written, `"prompt":[{"text":"Reply with just OK","type":"text"}]`) {
		t.Fatalf("expected Gemini prompt payload to use ACP content blocks, got %q", written)
	}
}

func TestSendMessageRequiredGoogleDriveMcpPreflightFailsBeforeProviderCall(t *testing.T) {
	workspace := t.TempDir()
	r := &Runner{
		workspace: workspace,
		sessions:  make(map[string]*LiveSession),
	}
	writeValidGoogleDriveWorkspaceConfig(t, workspace)

	var stdin bytes.Buffer
	processKey := "codex-proc"
	accountHomePath := t.TempDir()
	r.sessions[processKey] = &LiveSession{
		SessionID:         "session-1",
		Provider:          "codex",
		Model:             "codex-mcp",
		AccountHomePath:   accountHomePath,
		TransportType:     "codex_mcp",
		ProviderSessionID: "codex_mcp_session_session-1",
		Stdin:             nopWriteCloser{Writer: &stdin},
		StdoutScanner:     bufio.NewScanner(strings.NewReader("")),
		LastUsedAt:        time.Now().UTC(),
		IdleTTL:           time.Hour,
		Status:            "active",
	}

	_, err := r.SendMessage(context.Background(), AiSessionMessageRequest{
		Session: AiSessionHandle{
			TransportType:     "codex_mcp",
			ProviderSessionID: "codex_mcp_session_session-1",
			ProcessKey:        &processKey,
		},
		Prompt:          "List the Drive root",
		RequiredMcps:    []string{"google_drive"},
		AllowWrite:      false,
		AccountHomePath: accountHomePath,
	})
	if err == nil {
		t.Fatal("expected preflight error when provider config is missing")
	}
	if !strings.Contains(err.Error(), "not configured with the google-drive MCP server") {
		t.Fatalf("expected provider config error, got %v", err)
	}
	if stdin.Len() != 0 {
		t.Fatalf("expected no provider request to be written on preflight failure, got %q", stdin.String())
	}
}

func TestSendMessageInjectsRequiredGoogleDriveInstructionsIntoActualPrompt(t *testing.T) {
	workspace := t.TempDir()
	r := &Runner{
		workspace: workspace,
		sessions:  make(map[string]*LiveSession),
	}
	writeValidGoogleDriveWorkspaceConfig(t, workspace)

	accountHomePath := t.TempDir()
	_, err := r.EnsureGoogleDriveMcpProviderConfig(GoogleDriveMcpProviderConfigRequest{
		ProviderKey:     "codex",
		AccountHomePath: accountHomePath,
		Scope:           "account",
		Mode:            "read_only",
	})
	if err != nil {
		t.Fatalf("ensure provider config: %v", err)
	}

	var stdin bytes.Buffer
	processKey := "codex-proc"
	r.sessions[processKey] = &LiveSession{
		SessionID:         "session-1",
		Provider:          "codex",
		Model:             "gpt-5.4-mini",
		ReasoningEffort:   "low",
		AccountHomePath:   accountHomePath,
		TransportType:     "codex_mcp",
		ProviderSessionID: "codex_mcp_session_session-1",
		Stdin:             nopWriteCloser{Writer: &stdin},
		StdoutScanner: bufio.NewScanner(strings.NewReader(
			`{"jsonrpc":"2.0","id":3,"result":{"threadId":"thread-1","content":[{"text":"ok"}]}}`,
		)),
		LastUsedAt: time.Now().UTC(),
		IdleTTL:    time.Hour,
		Status:     "active",
	}

	result, err := r.SendMessage(context.Background(), AiSessionMessageRequest{
		Session: AiSessionHandle{
			TransportType:     "codex_mcp",
			ProviderSessionID: "codex_mcp_session_session-1",
			ProcessKey:        &processKey,
		},
		Prompt:          "Summarize the roadmap doc.",
		RequiredMcps:    []string{"google_drive"},
		AllowWrite:      false,
		AccountHomePath: accountHomePath,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if !strings.Contains(result.ActualPromptText, "## Required MCP Usage") {
		t.Fatalf("expected injected MCP section in actual prompt, got %q", result.ActualPromptText)
	}
	if !strings.Contains(result.ActualPromptText, "google-drive") {
		t.Fatalf("expected server name in actual prompt, got %q", result.ActualPromptText)
	}
	if !strings.Contains(stdin.String(), "google-drive") {
		t.Fatalf("expected provider request to contain injected prompt, got %q", stdin.String())
	}
	if !strings.Contains(stdin.String(), `"model":"gpt-5.4-mini"`) {
		t.Fatalf("expected initial Codex MCP tool call to include selected model, got %q", stdin.String())
	}
	if !strings.Contains(stdin.String(), `"model_reasoning_effort":"low"`) {
		t.Fatalf("expected initial Codex MCP tool call to include selected reasoning effort, got %q", stdin.String())
	}
}

func TestSendMessageMarksResultFailedWhenProviderReportsMcpFailureCode(t *testing.T) {
	workspace := t.TempDir()
	r := &Runner{
		workspace: workspace,
		sessions:  make(map[string]*LiveSession),
	}
	writeValidGoogleDriveWorkspaceConfig(t, workspace)

	accountHomePath := t.TempDir()
	_, err := r.EnsureGoogleDriveMcpProviderConfig(GoogleDriveMcpProviderConfigRequest{
		ProviderKey:     "codex",
		AccountHomePath: accountHomePath,
		Scope:           "account",
		Mode:            "read_only",
	})
	if err != nil {
		t.Fatalf("ensure provider config: %v", err)
	}

	processKey := "codex-proc"
	r.sessions[processKey] = &LiveSession{
		SessionID:         "session-1",
		Provider:          "codex",
		Model:             "codex-mcp",
		AccountHomePath:   accountHomePath,
		TransportType:     "codex_mcp",
		ProviderSessionID: "codex_mcp_session_session-1",
		Stdin:             nopWriteCloser{Writer: &bytes.Buffer{}},
		StdoutScanner: bufio.NewScanner(strings.NewReader(
			`{"jsonrpc":"2.0","id":3,"result":{"threadId":"thread-1","content":[{"text":"MCP_AUTH_REQUIRED"}]}}`,
		)),
		LastUsedAt: time.Now().UTC(),
		IdleTTL:    time.Hour,
		Status:     "active",
	}

	result, err := r.SendMessage(context.Background(), AiSessionMessageRequest{
		Session: AiSessionHandle{
			TransportType:     "codex_mcp",
			ProviderSessionID: "codex_mcp_session_session-1",
			ProcessKey:        &processKey,
		},
		Prompt:          "Summarize the roadmap doc.",
		RequiredMcps:    []string{"google_drive"},
		AllowWrite:      false,
		AccountHomePath: accountHomePath,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if result.Status != "failed" {
		t.Fatalf("expected failed status, got %q", result.Status)
	}
	if !strings.Contains(result.ErrorMessage, "mcp_auth_required") {
		t.Fatalf("expected MCP failure code in error message, got %q", result.ErrorMessage)
	}
}

func TestSendMessageClaudeRespawnsPrintCommandPerTurnAndResumesSession(t *testing.T) {
	originalCmdCtx := commandContextFn
	originalLookPath := lookPathFn
	defer func() {
		commandContextFn = originalCmdCtx
		lookPathFn = originalLookPath
	}()

	lookPathFn = func(file string) (string, error) {
		return file, nil
	}

	var startedArgs [][]string
	envCaptureDir := t.TempDir()
	commandContextFn = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		startedArgs = append(startedArgs, append([]string{name}, arg...))
		output := strings.Join([]string{
			`{"type":"system","subtype":"init","session_id":"claude-real-session"}`,
			`{"type":"assistant","message":{"content":[{"type":"text","text":"ignored"}]}}`,
			`{"type":"result","subtype":"success","is_error":false,"result":"first reply","session_id":"claude-real-session"}`,
		}, "\n")
		if len(startedArgs) == 2 {
			output = strings.Join([]string{
				`{"type":"system","subtype":"init","session_id":"claude-real-session"}`,
				`{"type":"result","subtype":"success","is_error":false,"result":"second reply","session_id":"claude-real-session"}`,
			}, "\n")
		}
		envCapturePath := filepath.Join(envCaptureDir, fmt.Sprintf("claude-env-%d.txt", len(startedArgs)))
		script := "$env:FLOWPILOT_WORKFLOW_RUN_ID + '|' + $env:FLOWPILOT_WORKFLOW_STEP_RUN_ID + '|' + $env:FLOWPILOT_PROCESS_KEY | Set-Content -LiteralPath '" + strings.ReplaceAll(envCapturePath, "'", "''") + "'; $input | Out-Null; Write-Output '" + output + "'"
		return exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script)
	}

	r, _ := New(".")
	handle, err := r.StartSession(context.Background(), AiSessionStartRequest{
		ProviderKey:      "claude",
		ModelName:        "claude-haiku",
		WorkingDirectory: ".",
		CustomEnv: map[string]string{
			googleDriveProxyWorkflowRunIDEnv:  "run-claude",
			googleDriveProxyWorkflowStepIDEnv: "step-claude",
		},
	})
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	resultOne, err := r.SendMessage(context.Background(), AiSessionMessageRequest{
		Session: handle,
		Prompt:  "First prompt",
	})
	if err != nil {
		t.Fatalf("first Claude SendMessage failed: %v", err)
	}

	resultTwo, err := r.SendMessage(context.Background(), AiSessionMessageRequest{
		Session: handle,
		Prompt:  "Second prompt",
	})
	if err != nil {
		t.Fatalf("second Claude SendMessage failed: %v", err)
	}

	if resultOne.OutputMarkdown != "first reply" {
		t.Fatalf("expected first Claude output, got %q", resultOne.OutputMarkdown)
	}
	if resultTwo.OutputMarkdown != "second reply" {
		t.Fatalf("expected second Claude output, got %q", resultTwo.OutputMarkdown)
	}
	if resultTwo.ProviderSessionID != "claude-real-session" {
		t.Fatalf("expected persisted Claude session id, got %q", resultTwo.ProviderSessionID)
	}
	if len(startedArgs) != 2 {
		t.Fatalf("expected two Claude print invocations, got %d", len(startedArgs))
	}
	if !strings.Contains(strings.Join(startedArgs[0], " "), "--output-format stream-json") {
		t.Fatalf("expected Claude stream-json output mode, got args %v", startedArgs[0])
	}
	if !strings.Contains(strings.Join(startedArgs[0], " "), "--model haiku") {
		t.Fatalf("expected Claude model name to be normalized for CLI, got args %v", startedArgs[0])
	}
	if strings.Contains(strings.Join(startedArgs[0], " "), "--resume claude-real-session") {
		t.Fatalf("expected first Claude turn to avoid resume with synthetic session id, got args %v", startedArgs[0])
	}
	if !strings.Contains(strings.Join(startedArgs[1], " "), "--resume claude-real-session") {
		t.Fatalf("expected second Claude turn to resume the real session id, got args %v", startedArgs[1])
	}
	for idx := 1; idx <= 2; idx++ {
		raw, err := os.ReadFile(filepath.Join(envCaptureDir, fmt.Sprintf("claude-env-%d.txt", idx)))
		if err != nil {
			t.Fatalf("failed to read Claude env capture %d: %v", idx, err)
		}
		expected := "run-claude|step-claude|" + *handle.ProcessKey
		if strings.TrimSpace(string(raw)) != expected {
			t.Fatalf("expected Claude env capture %d to equal %q, got %q", idx, expected, strings.TrimSpace(string(raw)))
		}
	}
}

func TestCloseSessionKillsMatchingOrphanProcessByPID(t *testing.T) {
	originalLookupProcessName := lookupProcessNameFn
	originalKillProcessByPID := killProcessByPIDFn
	defer func() {
		lookupProcessNameFn = originalLookupProcessName
		killProcessByPIDFn = originalKillProcessByPID
	}()

	lookupProcessNameFn = func(ctx context.Context, pid int) (string, error) {
		return "codex.exe", nil
	}

	killedPID := 0
	killProcessByPIDFn = func(pid int) error {
		killedPID = pid
		return nil
	}

	r, _ := New(".")
	processKey := "missing-proc"
	processPid := 4321

	err := r.CloseSession(context.Background(), AiSessionHandle{
		TransportType:     "codex_mcp",
		ProviderSessionID: "thread-1",
		ProcessKey:        &processKey,
		ProcessPid:        &processPid,
	})
	if err != nil {
		t.Fatalf("CloseSession failed: %v", err)
	}

	if killedPID != 4321 {
		t.Fatalf("expected orphan PID 4321 to be killed, got %d", killedPID)
	}
}

func TestCloseSessionDoesNotKillMismatchedOrphanProcessByPID(t *testing.T) {
	originalLookupProcessName := lookupProcessNameFn
	originalKillProcessByPID := killProcessByPIDFn
	defer func() {
		lookupProcessNameFn = originalLookupProcessName
		killProcessByPIDFn = originalKillProcessByPID
	}()

	lookupProcessNameFn = func(ctx context.Context, pid int) (string, error) {
		return "powershell.exe", nil
	}

	killed := false
	killProcessByPIDFn = func(pid int) error {
		killed = true
		return nil
	}

	r, _ := New(".")
	processKey := "missing-proc"
	processPid := 4321

	err := r.CloseSession(context.Background(), AiSessionHandle{
		TransportType:     "codex_mcp",
		ProviderSessionID: "thread-1",
		ProcessKey:        &processKey,
		ProcessPid:        &processPid,
	})
	if err != nil {
		t.Fatalf("CloseSession failed: %v", err)
	}

	if killed {
		t.Fatal("expected mismatched orphan process to be left untouched")
	}
}

func TestCloseSessionKillsActiveClaudeCommand(t *testing.T) {
	originalCmdCtx := commandContextFn
	originalLookPath := lookPathFn
	defer func() {
		commandContextFn = originalCmdCtx
		lookPathFn = originalLookPath
	}()

	lookPathFn = func(file string) (string, error) {
		return file, nil
	}

	commandContextFn = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		script := "Start-Sleep -Seconds 10"
		return exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script)
	}

	r, _ := New(".")
	handle, err := r.StartSession(context.Background(), AiSessionStartRequest{
		ProviderKey:      "claude",
		ModelName:        "claude-haiku",
		WorkingDirectory: ".",
	})
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	errChan := make(chan error, 1)
	go func() {
		_, err := r.SendMessage(context.Background(), AiSessionMessageRequest{
			Session: handle,
			Prompt:  "simulate message execution",
		})
		errChan <- err
	}()

	time.Sleep(200 * time.Millisecond)

	startTime := time.Now()
	err = r.CloseSession(context.Background(), handle)
	if err != nil {
		t.Fatalf("CloseSession failed: %v", err)
	}

	select {
	case sendErr := <-errChan:
		if sendErr == nil {
			t.Fatal("expected SendMessage to fail after CloseSession killed it")
		}
		if !strings.Contains(sendErr.Error(), "session_terminated:") {
			t.Fatalf("expected intentional shutdown error, got %v", sendErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SendMessage did not exit within 3 seconds after CloseSession")
	}

	if time.Since(startTime) > 2*time.Second {
		t.Errorf("expected CloseSession to finish quickly, took %v", time.Since(startTime))
	}
}

func TestCleanupSessionsKillsActiveClaudeCommand(t *testing.T) {
	originalCmdCtx := commandContextFn
	originalLookPath := lookPathFn
	defer func() {
		commandContextFn = originalCmdCtx
		lookPathFn = originalLookPath
	}()

	lookPathFn = func(file string) (string, error) {
		return file, nil
	}

	commandContextFn = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		script := "Start-Sleep -Seconds 10"
		return exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script)
	}

	r, _ := New(".")
	handle, err := r.StartSession(context.Background(), AiSessionStartRequest{
		ProviderKey:      "claude",
		ModelName:        "claude-haiku",
		WorkingDirectory: ".",
	})
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	errChan := make(chan error, 1)
	go func() {
		_, err := r.SendMessage(context.Background(), AiSessionMessageRequest{
			Session: handle,
			Prompt:  "simulate message execution",
		})
		errChan <- err
	}()

	time.Sleep(200 * time.Millisecond)

	r.CleanupSessions()

	select {
	case sendErr := <-errChan:
		if sendErr == nil {
			t.Fatal("expected SendMessage to fail after CleanupSessions")
		}
		if !strings.Contains(sendErr.Error(), "session_terminated:") {
			t.Fatalf("expected intentional shutdown error, got %v", sendErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SendMessage did not exit within 3 seconds after CleanupSessions")
	}
}

func TestCloseSessionMarksInFlightCodexBootstrapSessionTerminated(t *testing.T) {
	originalCmdCtx := commandContextFn
	originalLookPath := lookPathFn
	defer func() {
		commandContextFn = originalCmdCtx
		lookPathFn = originalLookPath
	}()

	lookPathFn = func(file string) (string, error) {
		return file, nil
	}

	commandContextFn = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		script := "$null = [Console]::In.ReadLine(); Write-Output '{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}'; $null = [Console]::In.ReadLine(); Start-Sleep -Seconds 10"
		return exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script)
	}

	r, _ := New(".")
	handle, err := r.StartSession(context.Background(), AiSessionStartRequest{
		ProviderKey:      "codex",
		ModelName:        "codex-mcp",
		WorkingDirectory: ".",
	})
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	if !strings.HasPrefix(handle.ProviderSessionID, "codex_mcp_session_") {
		t.Fatalf("expected synthetic bootstrap provider session id, got %q", handle.ProviderSessionID)
	}

	errChan := make(chan error, 1)
	go func() {
		_, err := r.SendMessage(context.Background(), AiSessionMessageRequest{
			Session: handle,
			Prompt:  "simulate first codex prompt",
		})
		errChan <- err
	}()

	time.Sleep(200 * time.Millisecond)

	err = r.CloseSession(context.Background(), handle)
	if err != nil {
		t.Fatalf("CloseSession failed: %v", err)
	}

	select {
	case sendErr := <-errChan:
		if sendErr == nil {
			t.Fatal("expected SendMessage to fail after CloseSession killed bootstrap codex session")
		}
		if !strings.Contains(sendErr.Error(), "session_terminated:") {
			t.Fatalf("expected intentional shutdown error, got %v", sendErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("SendMessage did not exit within 3 seconds after CloseSession")
	}
}

func TestCloseSessionRacingWithClaudeStart(t *testing.T) {
	originalCmdCtx := commandContextFn
	originalLookPath := lookPathFn
	defer func() {
		commandContextFn = originalCmdCtx
		lookPathFn = originalLookPath
	}()

	lookPathFn = func(file string) (string, error) {
		return file, nil
	}

	commandContextFn = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		script := "Start-Sleep -Seconds 10"
		return exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", script)
	}

	r, _ := New(".")
	handle, err := r.StartSession(context.Background(), AiSessionStartRequest{
		ProviderKey:      "claude",
		ModelName:        "claude-haiku",
		WorkingDirectory: ".",
	})
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	err = r.CloseSession(context.Background(), handle)
	if err != nil {
		t.Fatalf("CloseSession failed: %v", err)
	}

	_, err = r.SendMessage(context.Background(), AiSessionMessageRequest{
		Session: handle,
		Prompt:  "hello",
	})
	if err == nil || !strings.Contains(err.Error(), "session_dead") {
		t.Fatalf("expected session_dead error, got: %v", err)
	}
}

func TestCloseSessionRetiresProcessBoundGoogleDriveApprovals(t *testing.T) {
	r, _ := New(t.TempDir())

	processKey := "proc-approval"
	r.sessions[processKey] = &LiveSession{
		SessionID:  "session-1",
		ProcessKey: processKey,
		Status:     "active",
		LastUsedAt: time.Now().UTC(),
		IdleTTL:    time.Hour,
	}
	if err := r.saveGoogleDriveProxyApprovalState(googleDriveProxyApprovalState{
		Version: 1,
		Records: map[string]googleDriveProxyApprovalRecord{
			"approval-1": {
				ID:                "approval-1",
				WorkflowRunID:     "run-1",
				WorkflowStepRunID: "step-1",
				ProcessKey:        processKey,
				ToolName:          "createFolder",
				CanonicalArgsJSON: "{}",
				ArgumentsHash:     "hash-1",
				Status:            "pending",
				RequestedAt:       time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339Nano),
				ExpiresAt:         time.Now().UTC().Add(30 * time.Minute).Format(time.RFC3339Nano),
			},
			"approval-2": {
				ID:                "approval-2",
				WorkflowRunID:     "run-1",
				WorkflowStepRunID: "step-1",
				ProcessKey:        "other-proc",
				ToolName:          "createFolder",
				CanonicalArgsJSON: "{}",
				ArgumentsHash:     "hash-2",
				Status:            "pending",
				RequestedAt:       time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339Nano),
				ExpiresAt:         time.Now().UTC().Add(30 * time.Minute).Format(time.RFC3339Nano),
			},
		},
	}); err != nil {
		t.Fatalf("saveGoogleDriveProxyApprovalState() failed: %v", err)
	}

	if err := r.CloseSession(context.Background(), AiSessionHandle{ProcessKey: &processKey}); err != nil {
		t.Fatalf("CloseSession failed: %v", err)
	}

	state, err := r.loadGoogleDriveProxyApprovalState()
	if err != nil {
		t.Fatalf("loadGoogleDriveProxyApprovalState() failed: %v", err)
	}
	if got := state.Records["approval-1"].Status; got != "expired" {
		t.Fatalf("expected closed-session approval to be expired, got %q", got)
	}
	if got := state.Records["approval-2"].Status; got != "pending" {
		t.Fatalf("expected unrelated approval to remain pending, got %q", got)
	}
}

func TestCloseSessionRetiresProcessBoundApprovalsWhenSessionStateIsMissing(t *testing.T) {
	r, _ := New(t.TempDir())

	processKey := "proc-missing"
	if err := r.saveGoogleDriveProxyApprovalState(googleDriveProxyApprovalState{
		Version: 1,
		Records: map[string]googleDriveProxyApprovalRecord{
			"approval-missing": {
				ID:                "approval-missing",
				WorkflowRunID:     "run-1",
				WorkflowStepRunID: "step-1",
				ProcessKey:        processKey,
				ToolName:          "createFolder",
				CanonicalArgsJSON: "{}",
				ArgumentsHash:     "hash-missing",
				Status:            "pending",
				RequestedAt:       time.Now().UTC().Add(-1 * time.Minute).Format(time.RFC3339Nano),
				ExpiresAt:         time.Now().UTC().Add(30 * time.Minute).Format(time.RFC3339Nano),
			},
		},
	}); err != nil {
		t.Fatalf("saveGoogleDriveProxyApprovalState() failed: %v", err)
	}

	if err := r.CloseSession(context.Background(), AiSessionHandle{ProcessKey: &processKey}); err != nil {
		t.Fatalf("CloseSession failed: %v", err)
	}

	state, err := r.loadGoogleDriveProxyApprovalState()
	if err != nil {
		t.Fatalf("loadGoogleDriveProxyApprovalState() failed: %v", err)
	}
	if got := state.Records["approval-missing"].Status; got != "expired" {
		t.Fatalf("expected missing-session approval to be expired, got %q", got)
	}
}

func TestSweepIdleSessionsSkipsInFlightSession(t *testing.T) {
	r, _ := New(".")
	handle, err := r.StartSession(context.Background(), AiSessionStartRequest{
		ProviderKey:      "claude",
		ModelName:        "claude-haiku",
		WorkingDirectory: ".",
	})
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	r.sessionsMu.Lock()
	sess := r.sessions[*handle.ProcessKey]
	r.sessionsMu.Unlock()

	sess.Mu.Lock()
	sess.InFlight = true
	sess.LastUsedAt = time.Now().Add(-2 * time.Hour)
	sess.IdleTTL = 1 * time.Hour
	sess.Mu.Unlock()

	r.sweepIdleSessions()

	r.sessionsMu.Lock()
	_, exists := r.sessions[*handle.ProcessKey]
	r.sessionsMu.Unlock()

	if !exists {
		t.Fatal("expected session to not be swept because InFlight is true")
	}

	sess.Mu.Lock()
	sess.InFlight = false
	sess.Mu.Unlock()

	r.sweepIdleSessions()

	r.sessionsMu.Lock()
	_, exists = r.sessions[*handle.ProcessKey]
	r.sessionsMu.Unlock()

	if exists {
		t.Fatal("expected session to be swept because InFlight is false")
	}
}
