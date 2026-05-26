package runner

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"slices"
	"testing"
)

func TestSessionsStartMessageAndClose(t *testing.T) {
	// Set up path lookups to run powershell or sh as the mock provider binary
	originalLookPath := lookPathFn
	originalCommandContext := commandContextFn
	t.Cleanup(func() {
		lookPathFn = originalLookPath
		commandContextFn = originalCommandContext
	})

	var mockCmd string
	var mockArgs []string
	if runtime.GOOS == "windows" {
		mockCmd = "powershell"
		mockArgs = []string{
			"-Command",
			`while ($true) { ` +
				`$line = [Console]::ReadLine(); ` +
				`if ($line -eq $null) { break }; ` +
				`Write-Output '{"jsonrpc":"2.0","id":1,"result":{"threadId":"thread-abc","sessionId":"sess-xyz","text":"hello","content":[{"text":"hello"}],"session_id":"sess-123"}}' ` +
				`}`,
		}
	} else {
		mockCmd = "/bin/sh"
		mockArgs = []string{
			"-c",
			`while read line; do echo '{"jsonrpc":"2.0","id":1,"result":{"threadId":"thread-abc","sessionId":"sess-xyz","text":"hello","content":[{"text":"hello"}],"session_id":"sess-123"}}'; done`,
		}
	}

	lookPathFn = func(file string) (string, error) {
		return mockCmd, nil
	}

	commandContextFn = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		// Replace arguments with our mock shell script args to read stdin and echo json
		return exec.CommandContext(ctx, name, mockArgs...)
	}

	instance := &Runner{
		workspace: t.TempDir(),
		sessions:  make(map[string]*LiveSession),
	}

	// Test StartSession
	effort := "medium"
	handle, err := instance.StartSession(context.Background(), AiSessionStartRequest{
		ProviderKey:      "claude",
		ModelName:        "claude-sonnet",
		ReasoningEffort:  &effort,
		WorkingDirectory: instance.workspace,
		AllowWrite:       true,
	})
	if err != nil {
		t.Fatalf("start session: %v", err)
	}

	if handle.TransportType != "claude_stream_json" {
		t.Fatalf("expected transport type claude_stream_json, got %q", handle.TransportType)
	}
	if handle.ProcessKey == nil || *handle.ProcessKey == "" {
		t.Fatal("expected process key to be returned")
	}

	// Test SendMessage
	result, err := instance.SendMessage(context.Background(), AiSessionMessageRequest{
		Session:  handle,
		Prompt:   "hello",
		SkillIds: []string{},
	})
	if err != nil {
		t.Fatalf("send message: %v", err)
	}

	if result.Status != "success" {
		t.Fatalf("expected status success, got %q", result.Status)
	}
	if result.OutputMarkdown != "hello" {
		t.Fatalf("expected output markdown 'hello', got %q", result.OutputMarkdown)
	}

	// Verify locking works: concurrency test
	errChan := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func(id int) {
			_, err := instance.SendMessage(context.Background(), AiSessionMessageRequest{
				Session:  handle,
				Prompt:   fmt.Sprintf("hello %d", id),
				SkillIds: []string{},
			})
			errChan <- err
		}(i)
	}

	for i := 0; i < 2; i++ {
		if err := <-errChan; err != nil {
			t.Errorf("concurrent send message failed: %v", err)
		}
	}

	// Test CloseSession
	err = instance.CloseSession(context.Background(), handle)
	if err != nil {
		t.Fatalf("close session: %v", err)
	}

	// Verify session is deleted from registry
	instance.sessionsMu.Lock()
	_, exists := instance.sessions[*handle.ProcessKey]
	instance.sessionsMu.Unlock()
	if exists {
		t.Fatal("expected session to be removed from registry after close")
	}
}

func TestResolveBinaryAndArgs(t *testing.T) {
	tests := []struct {
		provider      string
		model         string
		effort        string
		expectedBin   string
		expectedArgs  []string
		expectedTrans string
	}{
		{"codex", "", "", "codex", []string{"mcp-server"}, "codex_mcp"},
		{"claude", "claude-sonnet", "high", "claude", []string{"-p", "--input-format", "stream-json", "--output-format", "stream-json", "--verbose", "--tools", "default", "--model", "claude-sonnet", "--effort", "high"}, "claude_stream_json"},
		{"gemini", "gemini-pro", "", "gemini", []string{"--acp", "--model", "gemini-pro"}, "gemini_acp"},
	}

	for _, tc := range tests {
		bin, args, trans := resolveBinaryAndArgs(tc.provider, tc.model, tc.effort)
		if bin != tc.expectedBin {
			t.Errorf("resolveBinaryAndArgs(%q, %q, %q) binary: expected %q, got %q", tc.provider, tc.model, tc.effort, tc.expectedBin, bin)
		}
		if trans != tc.expectedTrans {
			t.Errorf("resolveBinaryAndArgs(%q, %q, %q) transport: expected %q, got %q", tc.provider, tc.model, tc.effort, tc.expectedTrans, trans)
		}
		if !slices.Equal(args, tc.expectedArgs) {
			t.Errorf("resolveBinaryAndArgs(%q, %q, %q) args: expected %v, got %v", tc.provider, tc.model, tc.effort, tc.expectedArgs, args)
		}
	}
}
