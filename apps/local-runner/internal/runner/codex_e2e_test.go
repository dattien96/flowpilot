package runner

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

// TestCodexAskUserEndToEnd is the live-flow regression test for the Codex ask_user fix: a REAL
// `codex app-server` process driven through the production dispatcher + adapter. It proves the
// model can discover and call the ask_user dynamicTool and that the call round-trips through
// handleInbound/handleDynamicToolCall → bridge.AskQuestion → DynamicToolCallResponse.
//
// Before the fix this was impossible: ask_user was registered as an (ignored) inline
// mcpServers entry, initialize did not request the experimentalApi capability (so dynamicTools
// is rejected), and item/tool/call had no handler.
//
// Skipped by default: needs the `codex` binary, a logged-in account, network, and a little
// quota. Run with:
//
//	FLOWPILOT_CODEX_E2E=1 go test ./internal/runner/ -run TestCodexAskUserEndToEnd -v
func TestCodexAskUserEndToEnd(t *testing.T) {
	if os.Getenv("FLOWPILOT_CODEX_E2E") == "" {
		t.Skip("set FLOWPILOT_CODEX_E2E=1 to run the live codex ask_user e2e test")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, codexBinaryName(), "app-server", "--listen", "stdio://")
	cmd.Env = os.Environ()
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start codex app-server: %v", err)
	}
	defer func() { _ = cmd.Process.Kill() }()

	d := newCodexDispatcher(stdin, nil)
	d.start(stdout)

	// Handshake via the production param builder (now requesting experimentalApi).
	ictx, icancel := context.WithTimeout(ctx, 30*time.Second)
	defer icancel()
	if _, err := d.call(ictx, "initialize", codexInitializeParams()); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	_ = d.notify("initialized", map[string]any{})

	// newCodexAdapter wires d.setInbound(adapter.handleInbound) — the real inbound path.
	adapter := newCodexAdapter(d, os.TempDir())
	bridge := &captureBridge{approveWith: "approve", askAnswer: []string{"Go"}}

	req := TurnRequest{
		RunID:    "codex-e2e",
		StepID:   "s1",
		Prompt:   "Call the ask_user tool now to ask me whether to use Python or Go. You MUST call the tool, do not answer in plain text.",
		YoloMode: true, // permissive sandbox/approval so the turn runs unattended
		Cwd:      os.TempDir(),
	}
	if err := adapter.SendTurn(ctx, req, bridge); err != nil {
		t.Fatalf("SendTurn failed: %v", err)
	}

	bridge.mu.Lock()
	seen, prompt := bridge.askCallSeen, bridge.askPrompt
	bridge.mu.Unlock()
	if !seen {
		t.Fatalf("model never called the ask_user dynamicTool (handleDynamicToolCall not reached)")
	}
	t.Logf("SUCCESS: model called ask_user via item/tool/call with prompt=%q", prompt)
}
