package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// Unit tests for the runner-hosted Claude permission MCP server (07): JSON-RPC dispatch
// over HTTP + per-turn token routing to the bridge, against the validated contract.

func postMCP(t *testing.T, srv *claudeMCPServer, token string, payload map[string]any) (int, map[string]any) {
	t.Helper()
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, ClaudeMCPPath+"?token="+token, bytes.NewReader(body))
	req.RemoteAddr = "127.0.0.1:54321" // loopback-only handler
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Body.Len() == 0 {
		return rec.Code, nil
	}
	var resp map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	return rec.Code, resp
}

func mcpResultDecision(t *testing.T, resp map[string]any) map[string]any {
	t.Helper()
	result, _ := resp["result"].(map[string]any)
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("no content in result: %+v", resp)
	}
	block, _ := content[0].(map[string]any)
	text, _ := block["text"].(string)
	var dec map[string]any
	if err := json.Unmarshal([]byte(text), &dec); err != nil {
		t.Fatalf("decision text not JSON (%q): %v", text, err)
	}
	return dec
}

func TestClaudeMCPInitializeAndList(t *testing.T) {
	srv := newClaudeMCPServer()
	_, init := postMCP(t, srv, "", map[string]any{"jsonrpc": "2.0", "id": 0, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-11-25"}})
	res, _ := init["result"].(map[string]any)
	if res["protocolVersion"] != "2025-11-25" || res["serverInfo"] == nil {
		t.Fatalf("initialize result = %+v", init)
	}
	// BUG-NOTE-CP42 #24: submit_review_outcome is only advertised for a
	// registered token whose turn is actually a flow hub. This request uses
	// no token ("") at all, so only the 3 always-on tools (approve, ask_user,
	// spawn_agent) should appear.
	_, list := postMCP(t, srv, "", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
	lr, _ := list["result"].(map[string]any)
	tools, _ := lr["tools"].([]any)
	if len(tools) != 3 {
		t.Fatalf("tools/list = %+v", list)
	}
}

// TestClaudeMCPToolsListIncludesReviewOutcomeOnlyWhenAllowed is the
// regression test for BUG-NOTE-CP42 #24: submit_review_outcome used to be
// unconditionally advertised to every turn on every provider, so a model in
// ordinary normal_chat could call it and mutate that run's loop state.
func TestClaudeMCPToolsListIncludesReviewOutcomeOnlyWhenAllowed(t *testing.T) {
	srv := newClaudeMCPServer()

	normalChatTok := srv.register(&fakeClaudeBridge{}, false)
	defer srv.unregister(normalChatTok)
	_, list := postMCP(t, srv, normalChatTok, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
	lr, _ := list["result"].(map[string]any)
	tools, _ := lr["tools"].([]any)
	for _, def := range tools {
		if m, ok := def.(map[string]any); ok && m["name"] == "submit_review_outcome" {
			t.Fatalf("submit_review_outcome must not be advertised for a normal_chat (non-hub) turn, got %+v", tools)
		}
	}

	hubTok := srv.register(&fakeClaudeBridge{}, true)
	defer srv.unregister(hubTok)
	_, list2 := postMCP(t, srv, hubTok, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list"})
	lr2, _ := list2["result"].(map[string]any)
	tools2, _ := lr2["tools"].([]any)
	found := false
	for _, def := range tools2 {
		if m, ok := def.(map[string]any); ok && m["name"] == "submit_review_outcome" {
			found = true
		}
	}
	if !found {
		t.Fatalf("submit_review_outcome must be advertised for a flow-hub turn, got %+v", tools2)
	}
}

// TestClaudeMCPSubmitReviewOutcomeRejectedWhenNotAllowed proves the defense-
// in-depth check at tools/call time: even if a model somehow calls
// submit_review_outcome despite it not being listed, the call must be
// rejected, not silently mutate the run's loop state via applyFlowControl.
func TestClaudeMCPSubmitReviewOutcomeRejectedWhenNotAllowed(t *testing.T) {
	srv := newClaudeMCPServer()
	tok := srv.register(&fakeClaudeBridge{}, false)
	defer srv.unregister(tok)

	_, resp := postMCP(t, srv, tok, map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "submit_review_outcome", "arguments": map[string]any{"status": "approved"}},
	})
	if _, hasError := resp["error"]; !hasError {
		t.Fatalf("expected an error rejecting submit_review_outcome for a non-hub turn, got %+v", resp)
	}
}

// TestClaudeMCPGetOpensSSEStream guards the primary live-flow fix: claude's Streamable-HTTP
// client opens a GET SSE stream and only marks the server "connected" once it succeeds.
// Declining GET with 405 left the server "pending" so ask_user was never exposed to the model.
// GET must now return 200 text/event-stream with the open-stream prelude.
func TestClaudeMCPGetOpensSSEStream(t *testing.T) {
	srv := newClaudeMCPServer()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, ClaudeMCPPath, nil).WithContext(ctx)
	req.RemoteAddr = "127.0.0.1:54321" // loopback-only handler
	rec := httptest.NewRecorder()
	cancel() // so serveSSE writes the prelude then returns instead of blocking on keepalive
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET must open an SSE stream with 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Fatalf("GET must be text/event-stream, got %q", ct)
	}
	if !strings.Contains(rec.Body.String(), ": connected") {
		t.Fatalf("SSE stream must emit the open-stream prelude, got %q", rec.Body.String())
	}
}

func TestClaudeMCPNotificationReturns202(t *testing.T) {
	srv := newClaudeMCPServer()
	code, _ := postMCP(t, srv, "", map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	if code != http.StatusAccepted {
		t.Fatalf("notification should be 202, got %d", code)
	}
}

func TestClaudeMCPApproveRoutesToBridgeDeny(t *testing.T) {
	srv := newClaudeMCPServer()
	b := &fakeClaudeBridge{approval: "deny"}
	tok := srv.register(b, true)
	defer srv.unregister(tok)

	_, resp := postMCP(t, srv, tok, map[string]any{
		"jsonrpc": "2.0", "id": 2, "method": "tools/call",
		"params": map[string]any{"name": "approve", "arguments": map[string]any{
			"tool_name": "Write", "input": map[string]any{"file_path": "/w/x.txt", "content": "hi"}, "tool_use_id": "tu1",
		}},
	})
	if dec := mcpResultDecision(t, resp); dec["behavior"] != "deny" {
		t.Fatalf("expected deny, got %+v", dec)
	}
	if len(b.approvalCalls) != 1 || b.approvalCalls[0].Command != "/w/x.txt" {
		t.Fatalf("bridge approval not routed correctly: %+v", b.approvalCalls)
	}
}

func TestClaudeMCPApproveAllow(t *testing.T) {
	srv := newClaudeMCPServer()
	tok := srv.register(&fakeClaudeBridge{approval: "approve"}, true)
	_, resp := postMCP(t, srv, tok, map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": "tools/call",
		"params": map[string]any{"name": "approve", "arguments": map[string]any{"tool_name": "Bash", "input": map[string]any{"command": "echo hi"}}},
	})
	if dec := mcpResultDecision(t, resp); dec["behavior"] != "allow" {
		t.Fatalf("expected allow, got %+v", dec)
	}
}

func TestClaudeMCPRejectsNonLoopback(t *testing.T) {
	srv := newClaudeMCPServer()
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
	req := httptest.NewRequest(http.MethodPost, ClaudeMCPPath, bytes.NewReader(body))
	req.RemoteAddr = "203.0.113.7:5555" // non-loopback
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("non-loopback caller must be 403, got %d", rec.Code)
	}
}

func TestClaudeMCPTokensAreRandom(t *testing.T) {
	srv := newClaudeMCPServer()
	a, b := srv.register(&fakeClaudeBridge{}, true), srv.register(&fakeClaudeBridge{}, true)
	if a == b || len(a) < 16 || a == "t1" {
		t.Fatalf("tokens must be unguessable + unique: %q %q", a, b)
	}
}

func TestClaudeMCPNoTokenIsError(t *testing.T) {
	srv := newClaudeMCPServer()
	_, resp := postMCP(t, srv, "bogus", map[string]any{
		"jsonrpc": "2.0", "id": 4, "method": "tools/call",
		"params": map[string]any{"name": "approve", "arguments": map[string]any{"tool_name": "Write"}},
	})
	if resp["error"] == nil {
		t.Fatalf("unknown token must yield a JSON-RPC error, got %+v", resp)
	}
}

func TestWriteClaudeMCPConfigShape(t *testing.T) {
	path, cleanup, err := writeClaudeMCPConfig("http://127.0.0.1:9999", "tABC", nil)
	if err != nil {
		t.Fatalf("writeClaudeMCPConfig: %v", err)
	}
	defer cleanup()
	rawb, _ := os.ReadFile(path)
	raw := string(rawb)
	if !strings.Contains(raw, `"type":"http"`) ||
		!strings.Contains(raw, ClaudeMCPPath+"?token=tABC") {
		t.Fatalf("mcp-config shape wrong: %s", raw)
	}
}

// TestWriteClaudeMCPConfigMergesExtraServers guards the Google Drive fix: a FlowPilot-managed
// server (e.g. google-drive) must be merged next to the flowpilot permission server, since
// --strict-mcp-config makes claude ignore .claude.json mcpServers.
func TestWriteClaudeMCPConfigMergesExtraServers(t *testing.T) {
	extra := map[string]claudeMcpServer{
		"google-drive": {Type: "stdio", Command: "flowpilot", Args: []string{"google-drive-mcp"}, Env: map[string]string{"X": "1"}},
		"flowpilot":    {Type: "stdio", Command: "should-be-ignored"}, // must not shadow the permission route
	}
	path, cleanup, err := writeClaudeMCPConfig("http://127.0.0.1:9999", "tABC", extra)
	if err != nil {
		t.Fatalf("writeClaudeMCPConfig: %v", err)
	}
	defer cleanup()

	rawb, _ := os.ReadFile(path)
	var cfg struct {
		McpServers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(rawb, &cfg); err != nil {
		t.Fatalf("config not JSON: %v\n%s", err, rawb)
	}
	// flowpilot must remain the HTTP permission route, not the shadowing extra.
	fp := cfg.McpServers["flowpilot"]
	if fp["type"] != "http" {
		t.Fatalf("flowpilot must stay the http permission route, got %v", fp)
	}
	// google-drive must be present as the stdio server.
	gd, ok := cfg.McpServers["google-drive"]
	if !ok {
		t.Fatalf("google-drive extra server not merged: %s", rawb)
	}
	if gd["command"] != "flowpilot" {
		t.Fatalf("google-drive command wrong: %v", gd)
	}
}
