package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func withFakeTelegramAPI(t *testing.T, handler http.HandlerFunc) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	original := telegramBotAPIBase
	telegramBotAPIBase = server.URL
	t.Cleanup(func() { telegramBotAPIBase = original })
}

func TestTelegramProxyToolsListReturnsOnlySendMessage(t *testing.T) {
	s := &telegramProxyMcpServer{botToken: "tok", chatID: "chat-1", client: http.DefaultClient}
	tools := s.tools()
	if len(tools) != 1 || tools[0].Name != "send_message" {
		t.Fatalf("expected exactly one tool named send_message, got %#v", tools)
	}
}

func TestTelegramProxySendMessageReturnsMessageID(t *testing.T) {
	withFakeTelegramAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/bottest-token/sendMessage") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["chat_id"] != "chat-1" || body["text"] != "hello" {
			t.Fatalf("unexpected body: %#v", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":42}}`))
	})

	s := &telegramProxyMcpServer{botToken: "test-token", chatID: "chat-1", client: http.DefaultClient}
	id, err := s.sendMessage(context.Background(), "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 42 {
		t.Fatalf("message id = %d, want 42", id)
	}
}

func TestTelegramProxySendMessageFailsOnAPIError(t *testing.T) {
	withFakeTelegramAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":false,"description":"chat not found"}`))
	})

	s := &telegramProxyMcpServer{botToken: "test-token", chatID: "bad-chat", client: http.DefaultClient}
	if _, err := s.sendMessage(context.Background(), "hello"); err == nil || !strings.Contains(err.Error(), "chat not found") {
		t.Fatalf("expected 'chat not found' error, got %v", err)
	}
}

func TestTelegramProxyCallToolReturnsMessageIDInText(t *testing.T) {
	withFakeTelegramAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":7}}`))
	})

	s := &telegramProxyMcpServer{botToken: "test-token", chatID: "chat-1", client: http.DefaultClient, autoApprove: true}
	params, _ := json.Marshal(map[string]any{"name": "send_message", "arguments": map[string]any{"text": "final summary"}})
	result, err := s.callTool(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	content, _ := result["content"].([]any)
	if len(content) != 1 {
		t.Fatalf("expected 1 content block, got %#v", result)
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "message_id: 7") {
		t.Fatalf("expected message_id in tool result text, got %q", text)
	}
}

// TestTelegramProxyCallToolRequiresAutoApprove verifies Task-233's default
// posture: sending is refused (not silently allowed) until autoApprove is
// explicitly enabled for the connected integration.
func TestTelegramProxyCallToolRequiresAutoApprove(t *testing.T) {
	s := &telegramProxyMcpServer{botToken: "tok", chatID: "chat-1", client: http.DefaultClient}
	params, _ := json.Marshal(map[string]any{"name": "send_message", "arguments": map[string]any{"text": "hello"}})
	_, err := s.callTool(context.Background(), params)
	if err == nil || !strings.Contains(err.Error(), "MCP_TOOL_APPROVAL_REQUIRED") {
		t.Fatalf("expected MCP_TOOL_APPROVAL_REQUIRED error when autoApprove is false, got %v", err)
	}
}

func TestTelegramProxyCallToolRejectsUnknownTool(t *testing.T) {
	s := &telegramProxyMcpServer{botToken: "tok", chatID: "chat-1", client: http.DefaultClient, autoApprove: true}
	params, _ := json.Marshal(map[string]any{"name": "delete_message", "arguments": map[string]any{}})
	if _, err := s.callTool(context.Background(), params); err == nil {
		t.Fatal("expected error for unknown tool")
	}
}

func TestTelegramProxyCallToolRejectsEmptyText(t *testing.T) {
	s := &telegramProxyMcpServer{botToken: "tok", chatID: "chat-1", client: http.DefaultClient, autoApprove: true}
	params, _ := json.Marshal(map[string]any{"name": "send_message", "arguments": map[string]any{"text": "  "}})
	if _, err := s.callTool(context.Background(), params); err == nil {
		t.Fatal("expected error for empty text")
	}
}

func TestTelegramProxyHandleRequestInitializeAndToolsList(t *testing.T) {
	s := &telegramProxyMcpServer{botToken: "tok", chatID: "chat-1", client: http.DefaultClient}
	initResp := s.handleRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage(`1`), Method: "initialize"})
	if initResp.Error != nil {
		t.Fatalf("unexpected error on initialize: %#v", initResp.Error)
	}
	listResp := s.handleRequest(context.Background(), mcpRequest{JSONRPC: "2.0", ID: json.RawMessage(`2`), Method: "tools/list"})
	result, _ := listResp.Result.(map[string]any)
	tools, _ := result["tools"].([]proxyMcpTool)
	if len(tools) != 1 || tools[0].Name != "send_message" {
		t.Fatalf("expected exactly one send_message tool, got %#v", result)
	}
}
