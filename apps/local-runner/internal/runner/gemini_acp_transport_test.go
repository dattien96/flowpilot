package runner

import (
	"reflect"
	"testing"
)

func TestGeminiACPRequestPayloads(t *testing.T) {
	init := geminiACPInitializeParams()
	if got := init["protocolVersion"]; got != 1 {
		t.Fatalf("protocolVersion = %#v, want 1", got)
	}
	clientInfo, ok := init["clientInfo"].(map[string]interface{})
	if !ok {
		t.Fatalf("clientInfo has type %T, want map", init["clientInfo"])
	}
	if clientInfo["name"] != "flowpilot" || clientInfo["version"] != "1.0" {
		t.Fatalf("clientInfo = %#v, want flowpilot/1.0", clientInfo)
	}
	if _, ok := init["capabilities"].(map[string]interface{}); !ok {
		t.Fatalf("capabilities has type %T, want empty map", init["capabilities"])
	}

	sessionNew := geminiACPSessionNewParams("/tmp/work")
	wantSessionNew := map[string]interface{}{
		"cwd":        "/tmp/work",
		"mcpServers": []interface{}{},
	}
	if !reflect.DeepEqual(sessionNew, wantSessionNew) {
		t.Fatalf("session/new params = %#v, want %#v", sessionNew, wantSessionNew)
	}
	sessionNewWithMCP := geminiACPSessionNewParamsWithMCP("/tmp/work", geminiACPFlowPilotMCPServers("http://127.0.0.1:9999/", "tok"))
	servers, ok := sessionNewWithMCP["mcpServers"].([]interface{})
	if !ok || len(servers) != 1 {
		t.Fatalf("session/new mcpServers = %#v, want one server", sessionNewWithMCP["mcpServers"])
	}
	server, ok := servers[0].(map[string]interface{})
	if !ok {
		t.Fatalf("server = %#v, want map", servers[0])
	}
	if server["type"] != "http" || server["name"] != claudeMCPServerName || server["url"] != "http://127.0.0.1:9999"+ClaudeMCPPath+"?token=tok" {
		t.Fatalf("server = %#v, want FlowPilot HTTP MCP server", server)
	}

	prompt := geminiACPPromptParams("gemini-session", "hello")
	wantPrompt := map[string]interface{}{
		"sessionId": "gemini-session",
		"prompt": []map[string]string{
			{"type": "text", "text": "hello"},
		},
	}
	if !reflect.DeepEqual(prompt, wantPrompt) {
		t.Fatalf("session/prompt params = %#v, want %#v", prompt, wantPrompt)
	}
}

func TestGeminiACPExtractStreamedText(t *testing.T) {
	msg := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "session/update",
		"params": map[string]interface{}{
			"sessionId": "gemini-session",
			"update": map[string]interface{}{
				"sessionUpdate": "agent_message_chunk",
				"content": map[string]interface{}{
					"type": "text",
					"text": "chunk",
				},
			},
		},
	}

	if got := extractGeminiACPText(msg); got != "chunk" {
		t.Fatalf("extractGeminiACPText = %q, want chunk", got)
	}
}

func TestGeminiACPExtractStreamedTextIgnoresMalformedOrUnsupportedMessages(t *testing.T) {
	cases := []map[string]interface{}{
		nil,
		{"method": "other"},
		{"method": "session/update"},
		{"method": "session/update", "params": map[string]interface{}{"update": "bad"}},
		{"method": "session/update", "params": map[string]interface{}{"update": map[string]interface{}{"sessionUpdate": "tool_call"}}},
		{"method": "session/update", "params": map[string]interface{}{"update": map[string]interface{}{"sessionUpdate": "agent_message_chunk", "content": map[string]interface{}{"type": "image", "text": "ignored"}}}},
		{"method": "session/update", "params": map[string]interface{}{"update": map[string]interface{}{"sessionUpdate": "agent_message_chunk", "content": map[string]interface{}{"type": "text", "text": 123}}}},
	}

	for i, tc := range cases {
		if got := extractGeminiACPText(tc); got != "" {
			t.Fatalf("case %d extractGeminiACPText = %q, want empty", i, got)
		}
	}
}

func TestGeminiACPResultTextPrefersTextThenContentBlocks(t *testing.T) {
	if got := geminiACPResultText(map[string]interface{}{
		"text": "final text",
		"content": []interface{}{
			map[string]interface{}{"type": "text", "text": "content"},
		},
	}); got != "final text" {
		t.Fatalf("geminiACPResultText text result = %q, want final text", got)
	}

	if got := geminiACPResultText(map[string]interface{}{
		"content": []interface{}{
			map[string]interface{}{"type": "text", "text": "hello "},
			map[string]interface{}{"type": "image", "text": "ignored"},
			map[string]interface{}{"type": "text", "text": "world"},
		},
	}); got != "hello world" {
		t.Fatalf("geminiACPResultText content result = %q, want hello world", got)
	}
}

func TestGeminiACPResultTextIgnoresMalformedContent(t *testing.T) {
	cases := []map[string]interface{}{
		nil,
		{"content": "bad"},
		{"content": []interface{}{"bad", map[string]interface{}{"type": "image", "text": "ignored"}}},
		{"content": []interface{}{map[string]interface{}{"type": "text", "text": 123}}},
	}

	for i, tc := range cases {
		if got := geminiACPResultText(tc); got != "" {
			t.Fatalf("case %d geminiACPResultText = %q, want empty", i, got)
		}
	}
}

func TestGeminiACPResponseSessionID(t *testing.T) {
	msg := map[string]interface{}{
		"result": map[string]interface{}{
			"sessionId": " gemini-session ",
		},
	}
	if got := geminiACPResponseSessionID(msg); got != "gemini-session" {
		t.Fatalf("geminiACPResponseSessionID = %q, want gemini-session", got)
	}

	if got := geminiACPResponseSessionID(map[string]interface{}{"result": map[string]interface{}{"sessionId": 123}}); got != "" {
		t.Fatalf("numeric session id = %q, want empty", got)
	}
	if got := geminiACPResponseSessionID(map[string]interface{}{"result": "bad"}); got != "" {
		t.Fatalf("malformed result session id = %q, want empty", got)
	}
}

func TestJsonRpcErrorMessageHandlesGeminiACPShapes(t *testing.T) {
	if got := jsonRpcErrorMessage(map[string]interface{}{"error": map[string]interface{}{"message": "bad initialize"}}); got != "bad initialize" {
		t.Fatalf("object error = %q, want bad initialize", got)
	}
	if got := jsonRpcErrorMessage(map[string]interface{}{"error": "bad prompt"}); got != "bad prompt" {
		t.Fatalf("string error = %q, want bad prompt", got)
	}
	if got := jsonRpcErrorMessage(map[string]interface{}{"error": map[string]interface{}{"message": "   "}}); got != "" {
		t.Fatalf("blank object error = %q, want empty", got)
	}
}
