package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func scopedTelegramProxyServer(t *testing.T, instance *Runner) *telegramProxyMcpServer {
	t.Helper()
	return &telegramProxyMcpServer{
		botToken:          "test-token",
		chatID:            "chat-1",
		client:            http.DefaultClient,
		runner:            instance,
		workflowRunID:     "run-1",
		workflowStepRunID: "step-1",
		processKey:        "process-1",
	}
}

// TestTelegramProxyCallToolWithScopeCreatesPendingApproval verifies the
// FIRST call for a given (run, step, process, chat, text) tuple always
// comes back "pending" and does not send anything.
func TestTelegramProxyCallToolWithScopeCreatesPendingApproval(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	s := scopedTelegramProxyServer(t, instance)

	params, _ := json.Marshal(map[string]any{"name": "send_message", "arguments": map[string]any{"text": "final summary"}})
	_, err := s.callTool(context.Background(), params)
	if err == nil || !strings.Contains(err.Error(), "MCP_TOOL_APPROVAL_REQUIRED") {
		t.Fatalf("expected MCP_TOOL_APPROVAL_REQUIRED on first call, got %v", err)
	}

	records, err := instance.ListTelegramProxyApprovals("run-1", "step-1", "")
	if err != nil {
		t.Fatalf("list approvals: %v", err)
	}
	if len(records) != 1 || records[0].Status != "pending" || records[0].ChatID != "chat-1" || records[0].Text != "final summary" {
		t.Fatalf("expected one pending record, got %#v", records)
	}
}

// TestTelegramProxyCallToolRetryAfterApprovalSends verifies the identical
// retry after a human approves actually sends and marks the record executed.
func TestTelegramProxyCallToolRetryAfterApprovalSends(t *testing.T) {
	withFakeTelegramAPI(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"result":{"message_id":99}}`))
	})

	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	s := scopedTelegramProxyServer(t, instance)
	params, _ := json.Marshal(map[string]any{"name": "send_message", "arguments": map[string]any{"text": "final summary"}})

	// First call: pending.
	if _, err := s.callTool(context.Background(), params); err == nil {
		t.Fatal("expected pending error on first call")
	}
	records, _ := instance.ListTelegramProxyApprovals("run-1", "step-1", "pending")
	if len(records) != 1 {
		t.Fatalf("expected 1 pending record, got %d", len(records))
	}
	approvalID := records[0].ID

	// User approves.
	if _, err := instance.DecideTelegramProxyApproval(approvalID, TelegramProxyApprovalDecisionRequest{Decision: "approved"}); err != nil {
		t.Fatalf("decide approval: %v", err)
	}

	// Identical retry: should now send for real.
	result, err := s.callTool(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error on approved retry: %v", err)
	}
	content, _ := result["content"].([]any)
	text, _ := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "message_id: 99") {
		t.Fatalf("expected message_id in result, got %q", text)
	}

	executed, _ := instance.ListTelegramProxyApprovals("run-1", "step-1", "executed")
	if len(executed) != 1 || executed[0].ResultMessageID != 99 {
		t.Fatalf("expected one executed record with message id 99, got %#v", executed)
	}

	// A THIRD identical call must not send again — it replays the recorded result.
	result2, err := s.callTool(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error replaying executed result: %v", err)
	}
	content2, _ := result2["content"].([]any)
	text2, _ := content2[0].(map[string]any)["text"].(string)
	if !strings.Contains(text2, "message_id: 99") {
		t.Fatalf("expected replayed message_id, got %q", text2)
	}
}

// TestTelegramProxyCallToolRetryAfterRejectionDoesNotSend verifies a
// rejected approval never sends, even on retry.
func TestTelegramProxyCallToolRetryAfterRejectionDoesNotSend(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	s := scopedTelegramProxyServer(t, instance)
	params, _ := json.Marshal(map[string]any{"name": "send_message", "arguments": map[string]any{"text": "final summary"}})

	if _, err := s.callTool(context.Background(), params); err == nil {
		t.Fatal("expected pending error on first call")
	}
	records, _ := instance.ListTelegramProxyApprovals("run-1", "step-1", "pending")
	if _, err := instance.DecideTelegramProxyApproval(records[0].ID, TelegramProxyApprovalDecisionRequest{Decision: "rejected"}); err != nil {
		t.Fatalf("decide rejection: %v", err)
	}

	result, err := s.callTool(context.Background(), params)
	if err != nil {
		t.Fatalf("unexpected error on rejected retry: %v", err)
	}
	content, _ := result["content"].([]any)
	text, _ := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, "rejected") {
		t.Fatalf("expected rejection notice, got %q", text)
	}
}

// TestTelegramProxyCallToolDifferentTextCreatesSeparateApproval verifies
// approvals are keyed by the exact arguments, mirroring Drive's canonical-
// args-hash approach — a different message needs its own approval.
func TestTelegramProxyCallToolDifferentTextCreatesSeparateApproval(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	s := scopedTelegramProxyServer(t, instance)

	params1, _ := json.Marshal(map[string]any{"name": "send_message", "arguments": map[string]any{"text": "message one"}})
	params2, _ := json.Marshal(map[string]any{"name": "send_message", "arguments": map[string]any{"text": "message two"}})
	_, _ = s.callTool(context.Background(), params1)
	_, _ = s.callTool(context.Background(), params2)

	records, _ := instance.ListTelegramProxyApprovals("run-1", "step-1", "pending")
	if len(records) != 2 {
		t.Fatalf("expected 2 separate pending approvals for different text, got %d", len(records))
	}
}

// TestTelegramProxyCallToolWithoutScopeFallsBackToAutoApprove verifies the
// static autoApprove flag still governs when no run/step/process scope is
// present (e.g. a manual/offline invocation) — no regression from Task-233's
// original behavior.
func TestTelegramProxyCallToolWithoutScopeFallsBackToAutoApprove(t *testing.T) {
	s := &telegramProxyMcpServer{botToken: "tok", chatID: "chat-1", client: http.DefaultClient}
	params, _ := json.Marshal(map[string]any{"name": "send_message", "arguments": map[string]any{"text": "hello"}})
	_, err := s.callTool(context.Background(), params)
	if err == nil || !strings.Contains(err.Error(), "MCP_TOOL_APPROVAL_REQUIRED") {
		t.Fatalf("expected MCP_TOOL_APPROVAL_REQUIRED without scope and autoApprove=false, got %v", err)
	}
}

func TestDecideTelegramProxyApprovalRejectsUnknownID(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	if _, err := instance.DecideTelegramProxyApproval("does-not-exist", TelegramProxyApprovalDecisionRequest{Decision: "approved"}); err == nil {
		t.Fatal("expected error for unknown approval id")
	}
}

func TestDecideTelegramProxyApprovalRejectsInvalidDecision(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	s := scopedTelegramProxyServer(t, instance)
	params, _ := json.Marshal(map[string]any{"name": "send_message", "arguments": map[string]any{"text": "hi"}})
	_, _ = s.callTool(context.Background(), params)
	records, _ := instance.ListTelegramProxyApprovals("run-1", "step-1", "pending")
	if _, err := instance.DecideTelegramProxyApproval(records[0].ID, TelegramProxyApprovalDecisionRequest{Decision: "maybe"}); err == nil {
		t.Fatal("expected error for invalid decision value")
	}
}
