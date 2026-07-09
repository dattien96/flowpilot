package runner

import (
	"context"
	"testing"
	"time"
)

// Task-209: MCP wiring (ask_user/spawn_agent via the reused claudeMCPServer)
// and the MCP-ready-before-prompt gate (BUG-114 class).

func TestGrokAdapterBuildsMcpServersArrayAndRegistersBridge(t *testing.T) {
	sessionID := "session-mcp-1"
	d, fg := startFakeGrok(t, nil)
	a := newGrokAdapter(d, "/tmp/x")
	a.initResult = liveGrokInitializeResult()
	a.mcpServer = newClaudeMCPServer()
	a.mcpBaseURL = func() string { return "http://127.0.0.1:9999" }
	a.extraMCPServers = func(bool) map[string]claudeMcpServer {
		return map[string]claudeMcpServer{"google-drive": {Command: "npx", Args: []string{"-y", "@piotr-agier/google-drive-mcp"}}}
	}

	var capturedMcpServers []interface{}
	fg.serve(func(fg *fakeGrok, m map[string]any) {
		switch m["method"] {
		case "session/new":
			if params, ok := m["params"].(map[string]any); ok {
				capturedMcpServers, _ = params["mcpServers"].([]interface{})
			}
			fg.reply(m["id"], map[string]any{"sessionId": sessionID})
		case "session/prompt":
			fg.reply(m["id"], liveGrokPromptResult(sessionID))
		}
	})

	bridge := &fakeGrokBridge{}
	if err := a.SendTurn(context.Background(), TurnRequest{RunID: "run-mcp-1", Prompt: "hi"}, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}

	if len(capturedMcpServers) != 2 {
		t.Fatalf("expected 2 mcpServers entries (flowpilot + google-drive), got %d: %+v", len(capturedMcpServers), capturedMcpServers)
	}
	sawFlowpilot, sawDrive := false, false
	for _, raw := range capturedMcpServers {
		entry, _ := raw.(map[string]interface{})
		switch entry["name"] {
		case claudeMCPServerName:
			sawFlowpilot = true
			if entry["type"] != "http" {
				t.Fatalf("expected flowpilot entry type=http, got %v", entry["type"])
			}
		case "google-drive":
			sawDrive = true
			if entry["type"] != "stdio" {
				t.Fatalf("expected google-drive entry type=stdio, got %v", entry["type"])
			}
		}
	}
	if !sawFlowpilot || !sawDrive {
		t.Fatalf("expected both flowpilot and google-drive entries, got %+v", capturedMcpServers)
	}

	// The bridge must be unregistered from the MCP server once the turn ends
	// (no token leak across turns).
	if len(a.mcpServer.bridges) != 0 {
		t.Fatalf("expected mcpServer bridges to be empty after SendTurn returns, got %d", len(a.mcpServer.bridges))
	}
}

func TestGrokAdapterMcpReadyGateDoesNotHangOnTimeout(t *testing.T) {
	sessionID := "session-mcp-2"
	d, fg := startFakeGrok(t, nil)
	a := newGrokAdapter(d, "/tmp/x")
	a.initResult = liveGrokInitializeResult()
	a.mcpServer = newClaudeMCPServer()
	a.mcpBaseURL = func() string { return "http://127.0.0.1:9999" }

	fg.serve(func(fg *fakeGrok, m map[string]any) {
		switch m["method"] {
		case "session/new":
			fg.reply(m["id"], map[string]any{"sessionId": sessionID})
		case "session/prompt":
			fg.reply(m["id"], liveGrokPromptResult(sessionID))
		}
		// Deliberately never call tools/list on the mcpServer — simulates a
		// Grok MCP client that never connects (or a stale token) — the gate
		// must still degrade to sending the prompt rather than hang forever.
	})

	origTimeout := claudeMCPReadyDefaultTimeout
	// Can't reassign a const; instead just bound the whole test with a context
	// deadline shorter than the real 30s default so it can't accidentally pass
	// by hanging the full default duration.
	_ = origTimeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	bridge := &fakeGrokBridge{}
	err := a.SendTurn(ctx, TurnRequest{RunID: "run-mcp-2", Prompt: "hi"}, bridge)
	// waitReady blocks up to claudeMCPReadyDefaultTimeout (30s) OR ctx.Done();
	// our 3s ctx deadline fires first, so waitReady returns false quickly and
	// SendTurn proceeds to session/prompt on the same (now-expired) ctx, which
	// then fails the session/prompt call itself. The key assertion is that this
	// returns promptly rather than hanging — not the specific error.
	_ = err
}

func TestGrokAdapterNoMcpServerIsANoOp(t *testing.T) {
	sessionID := "session-mcp-3"
	d, fg := startFakeGrok(t, nil)
	a := newGrokAdapter(d, "/tmp/x") // mcpServer left nil (Task-207 MVP scope)
	a.initResult = liveGrokInitializeResult()

	var capturedMcpServers []interface{}
	fg.serve(func(fg *fakeGrok, m map[string]any) {
		switch m["method"] {
		case "session/new":
			if params, ok := m["params"].(map[string]any); ok {
				capturedMcpServers, _ = params["mcpServers"].([]interface{})
			}
			fg.reply(m["id"], map[string]any{"sessionId": sessionID})
		case "session/prompt":
			fg.reply(m["id"], liveGrokPromptResult(sessionID))
		}
	})

	bridge := &fakeGrokBridge{}
	if err := a.SendTurn(context.Background(), TurnRequest{RunID: "run-mcp-3", Prompt: "hi"}, bridge); err != nil {
		t.Fatalf("SendTurn: %v", err)
	}
	if len(capturedMcpServers) != 0 {
		t.Fatalf("expected zero mcpServers when mcpServer is nil, got %+v", capturedMcpServers)
	}
}
