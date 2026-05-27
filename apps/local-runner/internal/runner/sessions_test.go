package runner

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

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
}

func TestStartSessionResumesProviderSessionID(t *testing.T) {
	// Mock the command so it doesn't try to run a real process
	originalCmdCtx := commandContextFn
	defer func() { commandContextFn = originalCmdCtx }()
	commandContextFn = func(ctx context.Context, name string, arg ...string) *exec.Cmd {
		// Return a valid JSON-RPC initialization response so StartSession succeeds
		return exec.CommandContext(ctx, "powershell", "-NoProfile", "-Command", "Write-Output '{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}'")
	}

	r, _ := New(".")
	
	resumeID := "old-codex-thread-123"
	
	req := AiSessionStartRequest{
		ProviderKey: "codex",
		ModelName:   "codex-mcp",
		WorkingDirectory: ".",
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
		r.sessionsMu.Lock()
		delete(r.sessions, *handle.ProcessKey)
		r.sessionsMu.Unlock()
	}
}
