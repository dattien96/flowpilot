package runner

import (
	"context"
	"strings"
	"testing"
	"time"
)

// CP-57 OC-17 gap closure: the MCP-ready-before-prompt gate exists in
// opencodeAdapter.SendTurn (waitReady between ensureSession and session/prompt,
// BUG-114 class) but had no opencode test. Covers the degrade path (never
// ready → prompt withheld, no hang) and the positive ordering (prompt only
// after the FlowPilot MCP server signals ready). Additive.

func TestOpencodeMcpReadyGateWithholdsPromptWhenNeverReady(t *testing.T) {
	sessionID := "ses_gatewait1"
	d, fg := startFakeOpencode(t, nil)
	promptsSeen := make(chan string, 4)
	fg.serve(func(fg *fakeOpencode, m map[string]any) {
		switch m["method"] {
		case "session/new":
			fg.reply(m["id"], map[string]any{"sessionId": sessionID})
		case "session/set_config_option", "session/set_config":
			fg.reply(m["id"], map[string]any{})
		case "session/prompt":
			promptsSeen <- "prompt"
			fg.reply(m["id"], map[string]any{"stopReason": "end_turn"})
		}
	})
	a := newOpencodeAdapter(d, "/tmp")
	a.mcpServer = newClaudeMCPServer() // never signaled ready
	bridge := &fakeOpencodeBridge{}

	start := time.Now()
	gateCtx, cancelGate := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancelGate()
	err := a.SendTurn(gateCtx, TurnRequest{RunID: "run-gate", Prompt: "hi", Cwd: "/tmp"}, bridge)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expired context must surface as an error while the gate withholds the prompt")
	}
	if elapsed > 5*time.Second {
		t.Fatalf("gate degrade must not hang, took %v", elapsed)
	}
	select {
	case p := <-promptsSeen:
		t.Fatalf("session/prompt must be withheld until MCP ready, saw %q", p)
	default:
	}
}

func TestOpencodeMcpReadyGatePromptOnlyAfterReady(t *testing.T) {
	sessionID := "ses_gateready1"
	d, fg := startFakeOpencode(t, nil)
	promptAt := make(chan time.Time, 4)
	fg.serve(func(fg *fakeOpencode, m map[string]any) {
		switch m["method"] {
		case "session/new":
			fg.reply(m["id"], map[string]any{"sessionId": sessionID})
		case "session/set_config_option", "session/set_config":
			fg.reply(m["id"], map[string]any{})
		case "session/prompt":
			promptAt <- time.Now()
			fg.reply(m["id"], map[string]any{"stopReason": "end_turn"})
		}
	})
	a := newOpencodeAdapter(d, "/tmp")
	mcp := newClaudeMCPServer()
	a.mcpServer = mcp
	a.mcpBaseURL = func() string { return "http://127.0.0.1:0" }

	signaledAt := make(chan time.Time, 1)
	go func() {
		for {
			mcp.mu.Lock()
			var tok string
			for k := range mcp.bridges {
				tok = k
			}
			registered := len(mcp.bridges) > 0
			mcp.mu.Unlock()
			if registered {
				now := time.Now()
				mcp.signalReady(tok)
				signaledAt <- now
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()

	done := make(chan error, 1)
	go func() {
		done <- a.SendTurn(context.Background(), TurnRequest{RunID: "run-gate2", Prompt: "hi", Cwd: "/tmp"}, &fakeOpencodeBridge{})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SendTurn with a ready MCP server: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("SendTurn did not finish after MCP ready signal")
	}
	sig := <-signaledAt
	pr := <-promptAt
	if pr.Before(sig) {
		t.Fatalf("session/prompt must go only after MCP ready: prompt=%v signal=%v", pr, sig)
	}
}

func TestParseOpencodeStatsTable(t *testing.T) {
	out := "" +
		"┌────────────────────────────────────────────────────────┐\n" +
		"│                       OVERVIEW                         │\n" +
		"├────────────────────────────────────────────────────────┤\n" +
		"│Sessions                                             81 │\n" +
		"│Messages                                          5,944 │\n" +
		"│Days                                                 15 │\n" +
		"└────────────────────────────────────────────────────────┘\n" +
		"┌────────────────────────────────────────────────────────┐\n" +
		"│                    COST & TOKENS                       │\n" +
		"├────────────────────────────────────────────────────────┤\n" +
		"│Total Cost                                      $132.11 │\n" +
		"│Avg Cost/Day                                      $8.81 │\n" +
		"│Avg Tokens/Session                                13.1M │\n" +
		"│Median Tokens/Session                              8.8K │\n" +
		"└────────────────────────────────────────────────────────┘\n"
	lines := parseOpencodeStatsTable(out)
	if len(lines) != 4 {
		t.Fatalf("want 4 usage lines, got %d: %+v", len(lines), lines)
	}
	joined := ""
	for _, l := range lines {
		joined += l.Label + "\n"
	}
	for _, want := range []string{"81 sessions", "15 days", "$132.11 total", "$8.81/day", "13.1M avg/session", "8.8K median", "token limit: N/A"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("stats lines missing %q:\n%s", want, joined)
		}
	}
	if lines := parseOpencodeStatsTable("not a stats table"); lines != nil {
		t.Fatalf("garbage input must degrade to nil, got %+v", lines)
	}
}
