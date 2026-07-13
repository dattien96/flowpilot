package runner

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func TestBuildFirebaseCrashlyticsToolCallRequest(t *testing.T) {
	req, err := buildFirebaseCrashlyticsToolCallRequest(2, "crash-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Method != "tools/call" {
		t.Fatalf("Method = %q, want tools/call", req.Method)
	}
	var params struct {
		Name      string `json:"name"`
		Arguments struct {
			IssueID string `json:"issueId"`
		} `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params.Name != "crashlytics_get_issue" {
		t.Errorf("tool name = %q, want crashlytics_get_issue", params.Name)
	}
	if params.Arguments.IssueID != "crash-123" {
		t.Errorf("issueId = %q, want crash-123", params.Arguments.IssueID)
	}
}

func TestParseMcpToolCallTextResultExtractsText(t *testing.T) {
	raw := []byte(`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"crash detail here"}],"isError":false}}`)
	text, err := parseMcpToolCallTextResult(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if text != "crash detail here" {
		t.Errorf("text = %q, want %q", text, "crash detail here")
	}
}

func TestParseMcpToolCallTextResultReturnsErrorOnMcpError(t *testing.T) {
	raw := []byte(`{"jsonrpc":"2.0","id":2,"error":{"code":-32000,"message":"crash not found"}}`)
	if _, err := parseMcpToolCallTextResult(raw); err == nil || !strings.Contains(err.Error(), "crash not found") {
		t.Fatalf("expected error mentioning 'crash not found', got %v", err)
	}
}

func TestParseMcpToolCallTextResultRejectsMalformedShape(t *testing.T) {
	if _, err := parseMcpToolCallTextResult([]byte(`{"jsonrpc":"2.0","id":2,"result":{}}`)); err == nil {
		t.Fatal("expected error for missing content")
	}
	if _, err := parseMcpToolCallTextResult([]byte(`not json`)); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

// fakeFirebaseToolsMcpServer builds an exec.Cmd for a POSIX shell script that
// reads two JSON-RPC request lines from stdin and replies with two canned
// JSON-RPC response lines — standing in for a real `firebase-tools mcp`
// process so Fetch's spawn/pipe/protocol plumbing is exercised end-to-end
// without needing npx or a live Firebase project.
func fakeFirebaseToolsMcpServer(ctx context.Context, _ string, _ ...string) *exec.Cmd {
	script := `read -r _init
echo '{"jsonrpc":"2.0","id":"1","result":{"protocolVersion":"2025-06-18"}}'
read -r _call
echo '{"jsonrpc":"2.0","id":"2","result":{"content":[{"type":"text","text":"Issue: crash-123\nStatus: open"}]}}'`
	return exec.CommandContext(ctx, "sh", "-c", script)
}

func TestFirebaseToolsMcpAdapterFetchEndToEnd(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	_, err := instance.TriggerIntegrationConnection(context.Background(), "integration-firebase", IntegrationConnectionRequest{
		ProjectID:           "project-alpha",
		ProviderType:        "firebase",
		Action:              "test",
		FirebaseProjectID:   "flowpilot-test",
		FirebaseEnvironment: "production",
		ServiceAccountJSON:  validFirebaseServiceAccountJSON,
	})
	if err != nil {
		t.Fatalf("connect firebase: %v", err)
	}

	adapter := newFirebaseToolsMcpAdapter(instance)
	adapter.commandContextFn = fakeFirebaseToolsMcpServer

	content, err := adapter.Fetch(context.Background(), "crash-123")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(content, "Issue: crash-123") {
		t.Fatalf("expected crash content, got %q", content)
	}
}

func TestFirebaseToolsMcpAdapterFetchFailsWhenNotConnected(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	adapter := newFirebaseToolsMcpAdapter(instance)
	adapter.commandContextFn = fakeFirebaseToolsMcpServer

	if _, err := adapter.Fetch(context.Background(), "crash-123"); err == nil {
		t.Fatal("expected error when Firebase is not connected")
	}
}

func TestFirebaseToolsMcpAdapterFetchRejectsEmptyCrashRef(t *testing.T) {
	instance := &Runner{workspace: t.TempDir(), secretStore: newMemorySecretStore()}
	adapter := newFirebaseToolsMcpAdapter(instance)
	if _, err := adapter.Fetch(context.Background(), "  "); err == nil {
		t.Fatal("expected error for empty crash ref")
	}
}
