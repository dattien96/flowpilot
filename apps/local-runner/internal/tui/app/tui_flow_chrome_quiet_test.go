package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestStepsRuntimeMsg_NoStepNoticeWhileViewingChild(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf", Label: "grok-flow"}
	m.runHandle = &client.RunHandle{RunID: "run-main"}
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-main", AgentName: "main", Role: "main", Status: "running"},
		{RunID: "run-child", AgentName: "coder", Label: "grok-coder", Status: "running"},
	}
	m.focusRunID = "run-child"
	m.messages = []ChatMessage{{Role: "user", Content: "child task"}}
	m2, _ := m.Update(StepsRuntimeMsg{
		RunID: "run-main",
		Steps: []client.WorkflowStepRuntime{
			{StepID: "a", NodeID: "grok-context", Status: "DONE"},
			{StepID: "b", NodeID: "grok-coder", Status: "RUNNING"},
		},
	})
	am := m2.(*AppModel)
	for _, msg := range am.messages {
		if strings.Contains(msg.Content, "[RUNNING]") || msg.FormatHint == "steps" {
			t.Fatalf("step progress must not inject into child view: %q", msg.Content)
		}
	}
	if am.flowStepsActive != "grok-coder" {
		t.Fatalf("panel state still updates: active=%q", am.flowStepsActive)
	}
}

func TestStepsRuntimeMsg_MainStillGetsStepNotice(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf", Label: "grok-flow"}
	m.runHandle = &client.RunHandle{RunID: "run-main"}
	m2, _ := m.Update(StepsRuntimeMsg{
		RunID: "run-main",
		Steps: []client.WorkflowStepRuntime{
			{StepID: "a", NodeID: "grok-context", Status: "RUNNING"},
		},
	})
	am := m2.(*AppModel)
	found := false
	for _, msg := range am.messages {
		if strings.Contains(msg.Content, "[RUNNING] grok-context") {
			found = true
		}
	}
	if !found {
		t.Fatal("main view should still get step progress notices")
	}
}

func TestCopiedMsg_FlashToastNotChatTimeline(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.projectPath = "/repo/myapp"
	m.projectBranch = "main"

	before := len(m.messages)
	m2, cmd := m.Update(CopiedMsg{Kind: "answer"})
	am := m2.(*AppModel)
	if len(am.messages) != before {
		t.Fatalf("Copied must not append chat messages; n=%d", len(am.messages))
	}
	if am.flashToast != "Copied answer." {
		t.Fatalf("flashToast=%q", am.flashToast)
	}
	// Same row as project · branch — not a free-standing line above status.
	projLine := stripANSI(am.renderProjectStatusLine(" | ", 120))
	if !strings.Contains(projLine, "main") || !strings.Contains(projLine, "Copied answer.") {
		t.Fatalf("toast must share project/git status line: %q", projLine)
	}
	if strings.Count(stripANSI(am.renderStatusLine()), "Copied answer.") != 1 {
		t.Fatalf("toast should appear once in status block:\n%s", am.renderStatusLine())
	}
	if cmd == nil {
		t.Fatal("expected toast clear tick")
	}
	m3, _ := am.Update(toastClearMsg{ID: am.flashToastID})
	if m3.(*AppModel).flashToast != "" {
		t.Fatalf("toast should clear: %q", m3.(*AppModel).flashToast)
	}
}

func TestAgentOpen_NoViewingBanner(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.runHandle = &client.RunHandle{RunID: "run-main"}
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-main", AgentName: "main", Role: "main", Status: "running"},
		{RunID: "run-child", AgentName: "coder", Label: "grok-coder", Status: "running"},
	}
	m2, _ := m.handleSlashCommand("/agent grok-coder")
	am := m2.(*AppModel)
	for _, msg := range am.messages {
		if strings.Contains(msg.Content, "Viewing agent:") {
			t.Fatalf("must not show Viewing agent notice: %q", msg.Content)
		}
	}
	if !am.viewingChild() {
		t.Fatal("should focus child")
	}
}
