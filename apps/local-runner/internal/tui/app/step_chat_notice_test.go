package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestFormatStepChatNotices_CurrentOnly(t *testing.T) {
	prev := []client.WorkflowStepRuntime{
		{StepID: "a", NodeID: "grok-context", Status: "DONE"},
		{StepID: "b", NodeID: "grok-coder", Status: "RUNNING"},
		{StepID: "c", NodeID: "grok-reviewer", Status: "PENDING"},
	}
	next := []client.WorkflowStepRuntime{
		{StepID: "a", NodeID: "grok-context", Status: "DONE"},
		{StepID: "b", NodeID: "grok-coder", Status: "FAILED"},
		{StepID: "c", NodeID: "grok-reviewer", Status: "PENDING"},
	}
	next[1].RejectionNote = "tool timed out"
	got := formatStepChatNotices(prev, next, "grok-coder", "", "")
	if len(got) != 1 {
		t.Fatalf("notices=%v want 1 FAILED line", got)
	}
	if !strings.Contains(got[0], "[FAILED]") || !strings.Contains(got[0], "grok-coder") {
		t.Fatalf("line=%q", got[0])
	}
	if !strings.Contains(got[0], "reason: tool timed out") {
		t.Fatalf("missing failure reason: %q", got[0])
	}
	if strings.Contains(got[0], "grok-reviewer") || strings.Contains(strings.Join(got, "\n"), "Flow steps") {
		t.Fatalf("must not dump full step list: %v", got)
	}
}

func TestFormatStepChatNotices_FailedUsesFallbackError(t *testing.T) {
	prev := []client.WorkflowStepRuntime{
		{StepID: "b", NodeID: "grok-coder", Status: "RUNNING"},
	}
	next := []client.WorkflowStepRuntime{
		{StepID: "b", NodeID: "grok-coder", Status: "FAILED"},
	}
	got := formatStepChatNotices(prev, next, "grok-coder", "", "provider rate limited")
	if len(got) != 1 || !strings.Contains(got[0], "reason: provider rate limited") {
		t.Fatalf("got=%v", got)
	}
}

func TestFormatStepChatNotices_AnnouncesNewRunning(t *testing.T) {
	prev := []client.WorkflowStepRuntime{
		{StepID: "a", NodeID: "grok-context", Status: "RUNNING"},
		{StepID: "b", NodeID: "grok-coder", Status: "PENDING"},
	}
	next := []client.WorkflowStepRuntime{
		{StepID: "a", NodeID: "grok-context", Status: "DONE"},
		{StepID: "b", NodeID: "grok-coder", Status: "RUNNING"},
	}
	got := formatStepChatNotices(prev, next, "grok-context", "grok-coder", "")
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "[RUNNING] grok-coder") {
		t.Fatalf("missing running notice: %v", got)
	}
	// DONE for previous active is OK; must not list PENDING-only noise as a full banner.
	for _, line := range got {
		if strings.Contains(line, "Flow steps (") {
			t.Fatalf("unexpected full banner: %q", line)
		}
	}
}

func TestStepsRuntimeMsg_ChatShowsCurrentNotFullList(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.runHandle = &client.RunHandle{RunID: "run-1"}
	m.launch = LaunchArm{WorkflowID: "wf-1", Label: "grok-flow", Mode: ModeFlow}
	m.mode = ModeFlow
	enableSidebarForTest(m)
	m2, _ := m.Update(StepsRuntimeMsg{
		RunID: "run-1",
		Steps: []client.WorkflowStepRuntime{
			{StepID: "a", NodeID: "grok-context", Status: "RUNNING"},
			{StepID: "b", NodeID: "grok-coder", Status: "PENDING"},
			{StepID: "c", NodeID: "grok-reviewer", Status: "PENDING"},
			{StepID: "d", NodeID: "grok-tester", Status: "PENDING"},
			{StepID: "e", NodeID: "grok-done", Status: "PENDING"},
		},
	})
	am := m2.(*AppModel)
	var chatBlob strings.Builder
	for _, msg := range am.messages {
		chatBlob.WriteString(msg.Content)
		chatBlob.WriteByte('\n')
	}
	chat := chatBlob.String()
	if strings.Contains(chat, "Flow steps (5)") {
		t.Fatalf("chat must not dump full banner:\n%s", chat)
	}
	if !strings.Contains(chat, "[RUNNING] grok-context") {
		t.Fatalf("chat missing current step:\n%s", chat)
	}
	// Full list still available in the wide sidebar (Task-311).
	enableSidebarForTest(am)
	am.width, am.fullWidth = tuiSidebarMinWidth+10, tuiSidebarMinWidth+10
	if !strings.Contains(strings.Join(am.renderRightSidebar(30), "\n"), "grok-reviewer") {
		t.Fatalf("sidebar should still list other steps")
	}
}

func TestDispatchImageCommand_PasteNotFilePath(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.provider = "codex"
	_, cmd := m.dispatchImageCommand([]string{"paste"})
	if cmd == nil {
		t.Fatal("expected clipboard cmd, not file-path attach")
	}
	// Must not have errored with read paste…
	for _, msg := range m.messages {
		if strings.Contains(msg.Content, "read paste") {
			t.Fatalf("treated paste as path: %q", msg.Content)
		}
	}
}
