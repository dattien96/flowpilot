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
	"strconv"
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
	geminiID := DetermineProviderSessionID("gemini_agy", "gemini", "123", "", nil)
	if geminiID != "gemini_agy_session_123" {
		t.Errorf("Expected gemini_agy_session_123, got %s", geminiID)
	}

	// Test Gemini keeps explicit real session ids
	realGemini := "gemini-real-123"
	geminiRealID := DetermineProviderSessionID("gemini_agy", "gemini", "123", realGemini, &realGemini)
	if geminiRealID != "gemini-real-123" {
		t.Errorf("Expected gemini-real-123, got %s", geminiRealID)
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
	geminiResumeID := DetermineProviderSessionID("gemini_agy", "gemini", "123", "current-thread", &resumeID)
	if geminiResumeID != "old-codex-thread" {
		t.Errorf("Expected old-codex-thread, got %s", geminiResumeID)
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
		script := shellReadLine() + shellOutputLine(`{"jsonrpc":"2.0","id":1,"result":{}}`) + shellReadLine() + "sleep 2\n"
		return testShellCommand(ctx, script)
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

func TestStartSessionGeminiUsesVirtualAgySession(t *testing.T) {
	originalCmdCtx := commandContextFn
	defer func() { commandContextFn = originalCmdCtx }()
	commandStarted := false
	commandContextFn = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		commandStarted = true
		return testShellCommand(ctx, "printf unexpected")
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
	if commandStarted {
		t.Fatal("expected Gemini StartSession to avoid starting a long-lived process")
	}
	if handle.ProviderSessionID != "gemini_agy_session_"+r.sessions[*handle.ProcessKey].SessionID {
		t.Fatalf("expected synthetic Gemini agy session id, got %q", handle.ProviderSessionID)
	}
	if handle.ProcessPid != nil {
		t.Fatalf("expected virtual Gemini session to have no pid, got %v", *handle.ProcessPid)
	}
}

func TestStartSessionGeminiSeedsExplicitResumeID(t *testing.T) {
	r, _ := New(".")
	resumeID := "real-gemini-session"

	handle, err := r.StartSession(context.Background(), AiSessionStartRequest{
		ProviderKey:             "gemini",
		ModelName:               "gemini-flash",
		WorkingDirectory:        ".",
		ResumeProviderSessionID: &resumeID,
	})
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}
	if handle.ProviderSessionID != resumeID {
		t.Fatalf("expected Gemini StartSession to keep explicit resume id, got %q", handle.ProviderSessionID)
	}
}

func TestStartSessionCodexMcpUsesRequestedModelConfig(t *testing.T) {
	originalCmdCtx := commandContextFn
	defer func() { commandContextFn = originalCmdCtx }()
	var startedArgs []string
	commandContextFn = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		startedArgs = append([]string{}, arg...)
		return testShellCommand(ctx, shellOutputLine(`{"jsonrpc":"2.0","id":1,"result":{}}`)+"sleep 2\n")
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
		script := "printf %s \"$FLOWPILOT_PROCESS_KEY\" > " + strconv.Quote(envCapturePath) + "; printf '%s\\n' '{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}'; sleep 2"
		return exec.CommandContext(ctx, "sh", "-c", script)
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

func TestStartSessionPreservesProvidedProcessKeyInProviderEnv(t *testing.T) {
	originalCmdCtx := commandContextFn
	defer func() { commandContextFn = originalCmdCtx }()

	envCaptureDir := t.TempDir()
	envCapturePath := filepath.Join(envCaptureDir, "process-key.txt")
	commandContextFn = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		script := "printf %s \"$FLOWPILOT_PROCESS_KEY\" > " + strconv.Quote(envCapturePath) + "; printf '%s\\n' '{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}'; sleep 2"
		return exec.CommandContext(ctx, "sh", "-c", script)
	}

	r, _ := New(".")
	handle, err := r.StartSession(context.Background(), AiSessionStartRequest{
		ProviderKey:      "codex",
		ModelName:        "gpt-5.4-mini",
		WorkingDirectory: ".",
		CustomEnv: map[string]string{
			googleDriveProxyProcessKeyEnv: "workflow-run-123-step-step-456",
		},
	})
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	if handle.ProcessKey == nil || *handle.ProcessKey != "workflow-run-123-step-step-456" {
		t.Fatalf("expected provided process key in handle, got %#v", handle.ProcessKey)
	}

	raw, err := os.ReadFile(envCapturePath)
	if err != nil {
		t.Fatalf("failed to read captured process key: %v", err)
	}
	if got := strings.TrimSpace(string(raw)); got != "workflow-run-123-step-step-456" {
		t.Fatalf("expected provided process key in env, got %q", got)
	}

	if err := r.CloseSession(context.Background(), handle); err != nil {
		t.Fatalf("CloseSession failed: %v", err)
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
		return testShellCommand(ctx, shellOutputLine("unexpected"))
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

func TestSendMessageGeminiUsesAgyPrint(t *testing.T) {
	r, _ := New(".")
	homePath := t.TempDir()
	workspace := t.TempDir()
	projectsDir := filepath.Join(homePath, ".gemini", "config", "projects")
	if err := os.MkdirAll(projectsDir, 0o755); err != nil {
		t.Fatalf("mkdir projects dir: %v", err)
	}
	projectID := "d77f0d2e-2e78-4adf-bf19-93599129d476"
	payload, err := buildGeminiProjectConfigPayload(projectID, "flowpilot", workspace)
	if err != nil {
		t.Fatalf("build project config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectsDir, projectID+".json"), payload, 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	processKey := "gemini-proc"
	r.sessions[processKey] = &LiveSession{
		SessionID:         "session-1",
		Provider:          "gemini",
		Model:             "gemini-flash",
		AccountHomePath:   homePath,
		TransportType:     "gemini_agy",
		ProviderSessionID: "flowpilot-gemini-run-1",
		BinaryPath:        "agy",
		WorkingDirectory:  workspace,
		LastUsedAt:        time.Now().UTC(),
		IdleTTL:           time.Hour,
		Status:            "active",
	}
	originalCmdCtx := commandContextFn
	defer func() { commandContextFn = originalCmdCtx }()
	var startedArgs []string
	commandContextFn = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		startedArgs = append([]string{}, arg...)
		return testShellCommand(ctx, "printf 'Hello world\\n'")
	}

	result, err := r.SendMessage(context.Background(), AiSessionMessageRequest{
		Session: AiSessionHandle{
			TransportType:     "gemini_agy",
			ProviderSessionID: "flowpilot-gemini-run-1",
			ProcessKey:        &processKey,
		},
		Prompt: "Reply with just OK",
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if result.OutputMarkdown != "Hello world" {
		t.Fatalf("expected Gemini output to be collected, got %q", result.OutputMarkdown)
	}
	got := strings.Join(startedArgs, " ")
	for _, want := range []string{"--print", "--project " + projectID, "--sandbox", "Reply with just OK"} {
		if !strings.Contains(got, want) {
			t.Fatalf("agy send args = %q, missing %q", got, want)
		}
	}
	if strings.Contains(got, "--new-project") {
		t.Fatalf("agy send args = %q, should not pass --new-project when project config already exists; --project UUID is sufficient", got)
	}
	if len(startedArgs) < 2 || startedArgs[len(startedArgs)-2] != "--print" || startedArgs[len(startedArgs)-1] != "Reply with just OK" {
		t.Fatalf("agy send args = %q, want --print followed by actual prompt at the end", got)
	}
	if strings.Contains(got, "--continue") {
		t.Fatalf("agy send args = %q, should not continue from a legacy synthetic project id", got)
	}
	if r.sessions[processKey].ProviderSessionID != projectID {
		t.Fatalf("session provider id = %q, want AGY project id %q", r.sessions[processKey].ProviderSessionID, projectID)
	}
}

func TestSendMessageRequiredGoogleDriveMcpPreflightFailsBeforeProviderCall(t *testing.T) {
	workspace := t.TempDir()
	r := &Runner{
		workspace:   workspace,
		secretStore: newMemorySecretStore(),
		sessions:    make(map[string]*LiveSession),
	}
	writeValidGoogleDriveWorkspaceConfig(t, r)

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
	if !strings.Contains(err.Error(), "not configured with the flowpilot_drive MCP server") {
		t.Fatalf("expected provider config error, got %v", err)
	}
	if stdin.Len() != 0 {
		t.Fatalf("expected no provider request to be written on preflight failure, got %q", stdin.String())
	}
}

func TestSendMessageInjectsRequiredGoogleDriveInstructionsIntoActualPrompt(t *testing.T) {
	workspace := t.TempDir()
	r := &Runner{
		workspace:   workspace,
		secretStore: newMemorySecretStore(),
		sessions:    make(map[string]*LiveSession),
	}
	writeValidGoogleDriveWorkspaceConfig(t, r)

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
	if !strings.Contains(result.ActualPromptText, "flowpilot_drive") {
		t.Fatalf("expected server name in actual prompt, got %q", result.ActualPromptText)
	}
	if !strings.Contains(stdin.String(), "flowpilot_drive") {
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
		workspace:   workspace,
		secretStore: newMemorySecretStore(),
		sessions:    make(map[string]*LiveSession),
	}
	writeValidGoogleDriveWorkspaceConfig(t, r)

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
		script := "printf '%s|%s|%s\\n' \"$FLOWPILOT_WORKFLOW_RUN_ID\" \"$FLOWPILOT_WORKFLOW_STEP_RUN_ID\" \"$FLOWPILOT_PROCESS_KEY\" > " + strconv.Quote(envCapturePath) + "\n" +
			"cat >/dev/null\n" +
			shellOutputLines(strings.Split(output, "\n")...)
		return testShellCommand(ctx, script)
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

func TestSendMessageClaudeStaleUsageLimitMetadataStillRunsCommand(t *testing.T) {
	originalCmdCtx := commandContextFn
	originalLookPath := lookPathFn
	defer func() {
		commandContextFn = originalCmdCtx
		lookPathFn = originalLookPath
	}()

	lookPathFn = func(file string) (string, error) {
		return file, nil
	}

	commandStarted := false
	commandContextFn = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		commandStarted = true
		output := strings.Join([]string{
			`{"type":"system","subtype":"init","session_id":"claude-real-session"}`,
			`{"type":"result","subtype":"success","is_error":false,"result":"ok after reset","session_id":"claude-real-session"}`,
		}, "\n")
		script := "cat >/dev/null\n" + shellOutputLines(strings.Split(output, "\n")...)
		return testShellCommand(ctx, script)
	}

	accountHomePath := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(accountHomePath, ".claude.json"),
		[]byte(`{"oauthAccount":{"emailAddress":"user@example.com"},"cachedExtraUsageDisabledReason":"out_of_credits"}`),
		0o600,
	); err != nil {
		t.Fatalf("write auth: %v", err)
	}

	r, _ := New(".")
	handle, err := r.StartSession(context.Background(), AiSessionStartRequest{
		ProviderKey:      "claude",
		ModelName:        "claude-haiku",
		WorkingDirectory: ".",
		AccountHomePath:  accountHomePath,
	})
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	result, err := r.SendMessage(context.Background(), AiSessionMessageRequest{
		Session:         handle,
		Prompt:          "First prompt",
		AccountHomePath: accountHomePath,
	})
	if err != nil {
		t.Fatalf("expected stale usage metadata not to block Claude command, got %v", err)
	}
	if !commandStarted {
		t.Fatal("expected Claude SendMessage to run the print command")
	}
	if result.OutputMarkdown != "ok after reset" {
		t.Fatalf("output markdown = %q, want reset response", result.OutputMarkdown)
	}
}

func TestSendMessageClaudeResultLimitDoesNotReportLogin(t *testing.T) {
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
		output := strings.Join([]string{
			`{"type":"system","subtype":"init","session_id":"claude-real-session"}`,
			`{"type":"result","subtype":"error_during_execution","is_error":true,"result":"Not logged in · Please run /login","rate_limit":{"cachedExtraUsageDisabledReason":"out_of_credits"}}`,
		}, "\n")
		script := "cat >/dev/null\n" + shellOutputLines(strings.Split(output, "\n")...)
		return testShellCommand(ctx, script)
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

	_, err = r.SendMessage(context.Background(), AiSessionMessageRequest{
		Session: handle,
		Prompt:  "First prompt",
	})
	if err == nil {
		t.Fatal("expected Claude SendMessage to fail")
	}
	if strings.Contains(err.Error(), "/login") {
		t.Fatalf("expected normalized usage-limit error, got %q", err.Error())
	}
	if got := err.Error(); got != "provider error: Claude usage limit reached. Switch Claude account or wait for quota reset." {
		t.Fatalf("usage limit error = %q", got)
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
		return testShellCommand(ctx, "sleep 10\n")
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
		return testShellCommand(ctx, "sleep 10\n")
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
		script := shellReadLine() + shellOutputLine(`{"jsonrpc":"2.0","id":1,"result":{}}`) + shellReadLine() + "sleep 10\n"
		return testShellCommand(ctx, script)
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
		return testShellCommand(ctx, "sleep 10\n")
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
