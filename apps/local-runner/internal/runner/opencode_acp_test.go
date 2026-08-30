package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOpencodeACPTypesRoundTrip(t *testing.T) {
	// initialize result round-trip via typed struct + generic map
	raw, err := os.ReadFile(filepath.Join("testdata", "opencode_acp", "initialize_result.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var typed OpencodeInitializeResult
	if err := json.Unmarshal(raw, &typed); err != nil {
		t.Fatalf("unmarshal typed: %v", err)
	}
	b, err := json.Marshal(typed)
	if err != nil {
		t.Fatalf("marshal typed: %v", err)
	}
	var round map[string]any
	if err := json.Unmarshal(b, &round); err != nil {
		t.Fatalf("unmarshal round: %v", err)
	}
	if round["protocolVersion"] == nil {
		t.Fatal("expected protocolVersion after round-trip")
	}

	// session/new result
	raw2, err := os.ReadFile(filepath.Join("testdata", "opencode_acp", "session_new_result.json"))
	if err != nil {
		t.Fatalf("read fixture2: %v", err)
	}
	var typed2 OpencodeSessionNewResult
	if err := json.Unmarshal(raw2, &typed2); err != nil {
		t.Fatalf("unmarshal typed2: %v", err)
	}
	b2, _ := json.Marshal(typed2)
	var round2 map[string]any
	_ = json.Unmarshal(b2, &round2)
	if typed2.SessionID == "" {
		t.Fatal("expected sessionId")
	}

	// permission request
	raw3, err := os.ReadFile(filepath.Join("testdata", "opencode_acp", "permission_request.json"))
	if err != nil {
		t.Fatalf("read fixture3: %v", err)
	}
	var generic map[string]any
	if err := json.Unmarshal(raw3, &generic); err != nil {
		t.Fatalf("unmarshal generic: %v", err)
	}
	// Re-marshal as OpencodePermissionRequest via inner params
	params, _ := generic["params"].(map[string]any)
	pb, _ := json.Marshal(params)
	var typed3 OpencodePermissionRequest
	if err := json.Unmarshal(pb, &typed3); err != nil {
		t.Fatalf("unmarshal permission typed: %v", err)
	}
	if typed3.SessionID == "" || len(typed3.Options) != 3 {
		t.Fatalf("unexpected permission typed: %+v", typed3)
	}

	// prompt result
	raw4, _ := os.ReadFile(filepath.Join("testdata", "opencode_acp", "session_prompt_result.json"))
	var typed4 OpencodePromptResult
	if err := json.Unmarshal(raw4, &typed4); err != nil {
		t.Fatalf("unmarshal prompt result: %v", err)
	}
	if typed4.StopReason != "end_turn" {
		t.Fatalf("expected end_turn, got %s", typed4.StopReason)
	}
}

func TestOpencodeACPResponseSessionIDHandlesMetaFallback(t *testing.T) {
	top := map[string]any{"result": map[string]any{"sessionId": "ses_top"}}
	if got := opencodeACPResponseSessionID(top); got != "ses_top" {
		t.Fatalf("expected ses_top, got %s", got)
	}
	meta := map[string]any{"result": map[string]any{"_meta": map[string]any{"sessionId": "ses_meta"}}}
	if got := opencodeACPResponseSessionID(meta); got != "ses_meta" {
		t.Fatalf("expected ses_meta, got %s", got)
	}
	empty := map[string]any{"result": map[string]any{}}
	if got := opencodeACPResponseSessionID(empty); got != "" {
		t.Fatalf("expected empty, got %s", got)
	}
}

func TestOpencodeACPSessionNewParamsBuildsCorrectJSON(t *testing.T) {
	mcp := []interface{}{map[string]interface{}{"type": "http", "name": "flowpilot", "url": "http://localhost:0/mcp"}}
	perm := []interface{}{map[string]interface{}{"permission": "question", "pattern": "*", "action": "deny"}}
	params := opencodeACPSessionNewParams("/tmp", mcp, perm, "opencode/muse-spark-1.2-contributor-free", "high")
	if params["cwd"] != "/tmp" {
		t.Fatalf("cwd: %v", params["cwd"])
	}
	if _, ok := params["mcpServers"]; !ok {
		t.Fatal("missing mcpServers")
	}
	if _, ok := params["permission"]; !ok {
		t.Fatal("missing permission")
	}
	if params["model"] != "opencode/muse-spark-1.2-contributor-free" {
		t.Fatalf("model: %v", params["model"])
	}
	if params["variant"] != "high" {
		t.Fatalf("variant: %v", params["variant"])
	}
	// omitted when empty
	params2 := opencodeACPSessionNewParams("/tmp", nil, nil, "", "")
	if _, ok := params2["model"]; ok {
		t.Fatal("model should be omitted when empty")
	}
	if _, ok := params2["variant"]; ok {
		t.Fatal("variant should be omitted when empty")
	}
	// permission nil should be omitted? Spec says when nil, still mcpServers empty; permission omitted is okay
	if mcpServers, ok := params2["mcpServers"].([]interface{}); !ok || len(mcpServers) != 0 {
		t.Fatalf("expected empty mcpServers, got %v", params2["mcpServers"])
	}
}

func TestOpencodeACPSessionLoadParamsBuildsCorrectJSON(t *testing.T) {
	mcp := []interface{}{map[string]interface{}{"type": "http", "name": "flowpilot"}}
	params := opencodeACPSessionLoadParams("ses_123", "/tmp", mcp)
	if params["sessionId"] != "ses_123" {
		t.Fatalf("sessionId: %v", params["sessionId"])
	}
	if params["cwd"] != "/tmp" {
		t.Fatalf("cwd: %v", params["cwd"])
	}
	if _, ok := params["mcpServers"]; !ok {
		t.Fatal("missing mcpServers")
	}
	// Ensure trimming
	params2 := opencodeACPSessionLoadParams("  ses_123  ", "  /tmp  ", nil)
	if params2["sessionId"] != "ses_123" {
		t.Fatalf("trim: %v", params2["sessionId"])
	}
}

func TestOpencodeACPPromptParamsBuildsCorrectJSON(t *testing.T) {
	params := opencodeACPPromptParams("ses_123", "hello")
	if params["sessionId"] != "ses_123" {
		t.Fatalf("sessionId: %v", params["sessionId"])
	}
	prompt, ok := params["prompt"].([]map[string]string)
	if !ok || len(prompt) != 1 || prompt[0]["text"] != "hello" || prompt[0]["type"] != "text" {
		t.Fatalf("prompt: %v", params["prompt"])
	}
}

func TestOpencodeACPExtractTextHandlesAllUpdateShapes(t *testing.T) {
	// agent_message_chunk via full message
	msg := map[string]any{
		"method": "session/update",
		"params": map[string]any{
			"sessionId": "ses_1",
			"update": map[string]any{
				"sessionUpdate": "agent_message_chunk",
				"content":       map[string]any{"type": "text", "text": "Hello World"},
			},
		},
	}
	if got := extractOpencodeACPText(msg); got != "Hello World" {
		t.Fatalf("expected Hello World, got %q", got)
	}
	// non-message chunk should return ""
	msg2 := map[string]any{
		"method": "session/update",
		"params": map[string]any{
			"sessionId": "ses_1",
			"update": map[string]any{
				"sessionUpdate": "tool_call",
				"toolCallId":    "call_1",
			},
		},
	}
	if got := extractOpencodeACPText(msg2); got != "" {
		t.Fatalf("expected empty for tool_call, got %q", got)
	}
	// direct update map shape
	update := map[string]any{
		"sessionUpdate": "agent_message_chunk",
		"content":       map[string]any{"type": "text", "text": "Direct"},
	}
	if got := opencodeACPExtractText(update); got != "Direct" {
		t.Fatalf("expected Direct, got %q", got)
	}
	// usage_update should not extract text
	update2 := map[string]any{
		"sessionUpdate": "usage_update",
		"used":          123,
	}
	if got := opencodeACPExtractText(update2); got != "" {
		t.Fatalf("expected empty for usage_update, got %q", got)
	}
}

func TestOpencodeACPIsPermissionRequest(t *testing.T) {
	if !opencodeACPIsPermissionRequest("session/request_permission") {
		t.Fatal("expected true for session/request_permission")
	}
	if !opencodeACPIsPermissionRequest("question") {
		t.Fatal("expected true for question")
	}
	if !opencodeACPIsPermissionRequest("permission") {
		t.Fatal("expected true for permission")
	}
	if opencodeACPIsPermissionRequest("session/update") {
		t.Fatal("expected false for session/update")
	}
}

func TestOpencodeACPFlowPilotMCPServers(t *testing.T) {
	servers := opencodeACPFlowPilotMCPServers("http://localhost:8080", "tok123")
	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}
	entry, ok := servers[0].(map[string]interface{})
	if !ok || entry["type"] != "http" || entry["name"] != claudeMCPServerName {
		t.Fatalf("unexpected entry: %v", servers[0])
	}
	if len(opencodeACPFlowPilotMCPServers("", "tok")) != 0 {
		t.Fatal("expected empty when base empty")
	}
	if len(opencodeACPFlowPilotMCPServers("http://localhost", "")) != 0 {
		t.Fatal("expected empty when token empty")
	}
}

func TestOpencodeACPNameValueList(t *testing.T) {
	m := map[string]string{"B": "2", "A": "1"}
	list := opencodeACPNameValueList(m)
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
	first, _ := list[0].(map[string]interface{})
	if first["name"] != "A" {
		t.Fatalf("expected sorted A first, got %v", first)
	}
}

func TestRedactOpencodeFrameForLogStripsCredentialShapedFields(t *testing.T) {
	raw := `{"jsonrpc":"2.0","method":"session/new","params":{"mcpServers":[{"name":"google-drive","headers":{"Authorization":"Bearer super-secret"},"env":{"REFRESH_TOKEN":"abc123"}}],"cwd":"/tmp"}}`
	redacted := redactOpencodeFrameForLog(raw)
	if strings.Contains(redacted, "super-secret") || strings.Contains(redacted, "abc123") {
		t.Fatalf("credential leaked: %s", redacted)
	}
	if !strings.Contains(redacted, "[redacted]") {
		t.Fatalf("expected redaction marker, got: %s", redacted)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(redacted), &parsed); err != nil {
		t.Fatalf("redacted not valid JSON: %v", err)
	}
}

func TestRedactOpencodeFrameHandlesMalformed(t *testing.T) {
	if redactOpencodeFrameForLog("not json") == "" {
		t.Fatal("expected placeholder for malformed input")
	}
}
