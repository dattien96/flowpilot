package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestApprovalTranscriptWaitsInsteadOfInvitingClick(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.yolo = false
	m.asciiMode = false

	m2, cmd := m.handleEvent(client.ProviderEvent{
		Type:          "permission_required",
		ApprovalID:    "appr-158015",
		WorkflowRunID: "run-1",
	})
	am := m2.(*AppModel)
	if cmd != nil {
		t.Fatal("YOLO=off must wait, not auto-approve")
	}
	view := am.View()
	if !strings.Contains(view, "[APPROVAL] appr-158015") {
		t.Fatalf("missing approval id:\n%s", view)
	}
	if !strings.Contains(view, "⏳") {
		t.Fatalf("missing waiting icon:\n%s", view)
	}
	if !strings.Contains(view, "Waiting user") {
		t.Fatalf("missing waiting copy:\n%s", view)
	}
	if strings.Contains(view, "click Approve or Deny") {
		t.Fatalf("transcript must not invite click on the response line:\n%s", view)
	}
	if !strings.Contains(view, "Approve") || !strings.Contains(view, "Deny") {
		t.Fatalf("input bar must still offer Approve/Deny:\n%s", view)
	}
}

func TestApprovalTranscriptAsciiWaitingIcon(t *testing.T) {
	got := formatApprovalWaitingLine("appr-1", true)
	if !strings.HasPrefix(got, "... [APPROVAL] appr-1 ") {
		t.Fatalf("ascii line=%q", got)
	}
	if strings.Contains(got, "click") || strings.Contains(got, "Approve or Deny") {
		t.Fatalf("ascii line still looks clickable: %q", got)
	}
}
