package runner

import (
	"context"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestClaudeAskUserEndToEnd is the live-flow regression test for the ask_user defect: a REAL
// `claude` process, the REAL runner-hosted MCP server (serveSSE + signalReady), and the REAL
// SendTurn prompt-gate (waitReady). It proves the model can actually discover and call
// mcp__flowpilot__ask_user — which failed before the fix because (1) the MCP server declined
// the GET SSE probe (server stuck "pending", tools never exposed) and (2) the prompt was
// delivered before the async MCP connection completed.
//
// Skipped by default: needs the `claude` binary, network, and burns a little quota. Run with
//
//	FLOWPILOT_CLAUDE_E2E=1 go test ./internal/runner/ -run TestClaudeAskUserEndToEnd -v
func TestClaudeAskUserEndToEnd(t *testing.T) {
	if os.Getenv("FLOWPILOT_CLAUDE_E2E") == "" {
		t.Skip("set FLOWPILOT_CLAUDE_E2E=1 to run the live claude ask_user e2e test")
	}

	// Real spawn (other tests override commandContextFn via mockClaude; restore after).
	origCmd := commandContextFn
	commandContextFn = exec.CommandContext
	defer func() { commandContextFn = origCmd }()

	// Real runner-hosted MCP server behind a loopback httptest listener.
	mcpSrv := newClaudeMCPServer()
	httpSrv := httptest.NewServer(mcpSrv)
	defer httpSrv.Close()

	bridge := &fakeClaudeBridge{answer: []string{"Go"}} // AskQuestion replies "Go"

	a := newClaudeAdapter(newClaudeProcessPool(), ".", "e2e-account", nil, "")
	a.mcpServer = mcpSrv
	a.mcpBaseURL = func() string { return httpSrv.URL }
	// Plain prompt (no reinforcement noise) — assert tool discovery on its own merits.
	a.promptPrep = func(req TurnRequest) string { return req.Prompt }

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()

	req := TurnRequest{
		RunID:    "e2e-run",
		StepID:   "s1",
		Prompt:   "Call the ask_user tool now to ask me whether to use Python or Go. You MUST call the tool, do not ask in plain text.",
		YoloMode: true, // bypassPermissions: no approval gate, ask_user via MCP
		ModelName: "haiku",
	}

	if err := a.SendTurn(ctx, req, bridge); err != nil {
		t.Fatalf("SendTurn failed: %v", err)
	}

	bridge.mu.Lock()
	prompt := bridge.questionPrompt
	bridge.mu.Unlock()
	if strings.TrimSpace(prompt) == "" {
		t.Fatalf("model never called mcp__flowpilot__ask_user (AskQuestion not routed) — ask_user was not exposed to the model")
	}
	t.Logf("SUCCESS: model called ask_user with prompt=%q", prompt)
}
