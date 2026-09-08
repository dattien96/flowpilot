package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestFormatStepChatNotices_HidesSyntheticChatRunning(t *testing.T) {
	next := []client.WorkflowStepRuntime{{
		StepID: "chat-run-211960", StepType: "chat", Status: "RUNNING",
	}}
	got := formatStepChatNotices(nil, next, "", "chat", "")
	for _, line := range got {
		if strings.Contains(strings.ToLower(line), "chat") {
			t.Fatalf("must not print [RUNNING] chat, got %v", got)
		}
	}
}

func TestActiveStepName_IgnoresSyntheticChat(t *testing.T) {
	if got := activeStepName([]client.WorkflowStepRuntime{
		{StepID: "chat-run-1", StepType: "chat", Status: "RUNNING"},
	}); got != "" {
		t.Fatalf("got %q want blank", got)
	}
	if got := activeStepName([]client.WorkflowStepRuntime{
		{StepID: "chat-run-1", StepType: "chat", Status: "RUNNING"},
		{NodeID: "ingest_reader", Status: "RUNNING"},
	}); got != "ingest_reader" {
		t.Fatalf("got %q want ingest_reader", got)
	}
}

func TestFlowStepsPanel_BlankWhenOnlySyntheticChat(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.flowSteps = []client.WorkflowStepRuntime{
		{StepID: "chat-run-1", StepType: "chat", Status: "RUNNING"},
	}
	if lines := m.flowStepsPanelLines(); len(lines) != 0 {
		t.Fatalf("want blank sidebar, got %v", lines)
	}
}
