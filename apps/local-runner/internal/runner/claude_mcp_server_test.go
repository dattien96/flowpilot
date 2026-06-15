package runner

import (
	"bytes"
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
	_, list := postMCP(t, srv, "", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/list"})
	lr, _ := list["result"].(map[string]any)
	tools, _ := lr["tools"].([]any)
	if len(tools) != 2 {
		t.Fatalf("tools/list = %+v", list)
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
	tok := srv.register(b)
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
	tok := srv.register(&fakeClaudeBridge{approval: "approve"})
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
	a, b := srv.register(&fakeClaudeBridge{}), srv.register(&fakeClaudeBridge{})
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
	path, cleanup, err := writeClaudeMCPConfig("http://127.0.0.1:9999", "tABC")
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
