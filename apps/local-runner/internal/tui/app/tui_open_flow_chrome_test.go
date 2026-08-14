package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestApplyOpenedRunFlowChrome_WorkflowRestoresModeAndLaunch(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.mode = ModeChat
	m.applyOpenedRunFlowChrome(client.RunHandle{
		RunID:      "run-98153",
		RunKind:    "workflow",
		WorkflowID: "9ecadf22-0a0e-4963-a45e-22812f0f9700",
		FlowRef:    "9ecadf22-0a0e-4963-a45e-22812f0f9700",
	}, client.RunHistoryItem{})
	if m.mode != ModeFlow {
		t.Fatalf("mode=%v want ModeFlow", m.mode)
	}
	if !m.launch.IsCatalogWorkflow() {
		t.Fatalf("launch not catalog: %+v", m.launch)
	}
	// Poll requires a run handle (live workflow).
	m.runHandle = &client.RunHandle{RunID: "run-98153"}
	if !m.shouldPollStepsRuntime() {
		t.Fatal("expected steps poll after open workflow")
	}
}

func TestApplyOpenedRunFlowChrome_HistoryMetaFallback(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.applyOpenedRunFlowChrome(client.RunHandle{RunID: "run-1"}, client.RunHistoryItem{
		RunID:      "run-1",
		RunKind:    "workflow",
		WorkflowID: "wf-1",
	})
	if m.mode != ModeFlow || m.launch.WorkflowID != "wf-1" {
		t.Fatalf("mode=%v launch=%+v", m.mode, m.launch)
	}
}

func TestApplyOpenedRunFlowChrome_PlainChatClearsFlow(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf-1", Label: "old"}
	m.flowSteps = []client.WorkflowStepRuntime{{StepID: "s1"}}
	m.applyOpenedRunFlowChrome(client.RunHandle{RunID: "run-chat", RunKind: "chat"}, client.RunHistoryItem{})
	if m.mode != ModeChat {
		t.Fatalf("mode=%v want ModeChat", m.mode)
	}
	if m.launch.IsArmed() {
		t.Fatalf("launch still armed: %+v", m.launch)
	}
	if len(m.flowSteps) != 0 {
		t.Fatal("flow steps should clear on chat open")
	}
}

func TestChatOpenedMsg_WorkflowArmsStepsPoll(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.flowWorkflows = []client.Workflow{{ID: "wf-grok", Name: "grok-flow"}}
	m2, cmd := m.Update(ChatOpenedMsg{
		Handle: client.RunHandle{
			RunID: "run-98153", RunKind: "workflow",
			WorkflowID: "wf-grok", FlowRef: "wf-grok", Status: "running",
		},
		Messages: []ChatMessage{{Role: "user", Content: "fix bug 1+1 != 2"}},
	})
	am := m2.(*AppModel)
	if am.mode != ModeFlow {
		t.Fatalf("mode=%v", am.mode)
	}
	if am.launch.Label != "grok-flow" {
		t.Fatalf("launch.Label=%q want grok-flow (human name, not id)", am.launch.Label)
	}
	if !strings.Contains(am.View(), "Opened flow") {
		t.Fatalf("view missing flow open label:\n%s", am.View())
	}
	_ = cmd
	if !am.shouldPollStepsRuntime() {
		t.Fatal("should poll steps after workflow open")
	}
}

func TestResolveFlowDisplayName_PrefersCatalogName(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.flowWorkflows = []client.Workflow{
		{ID: "9ecadf22-0a0e-4963-a45e-22812f0f9700", Name: "grok-flow"},
	}
	got := m.resolveFlowDisplayName("9ecadf22-0a0e-4963-a45e-22812f0f9700", "9ecadf22-0a0e-4963-a45e-22812f0f9700")
	if got != "grok-flow" {
		t.Fatalf("got %q want grok-flow", got)
	}
}

func TestAgentRunsHydratedMsg_EnablesAgentPicker(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.runHandle = &client.RunHandle{RunID: "run-98153"}
	m.mode = ModeFlow
	m2, _ := m.Update(agentRunsHydratedMsg{
		ParentRunID: "run-98153",
		Runs: []client.AgentRunSummary{
			{RunID: "run-98153", AgentName: "main", Role: "main", Status: "completed"},
			{RunID: "run-98485", AgentName: "reviewer-agent", Label: "my-reviewer", Status: "completed"},
		},
	})
	am := m2.(*AppModel)
	if len(am.agentRuns) != 2 {
		t.Fatalf("agentRuns=%d", len(am.agentRuns))
	}
	am.inputValue = "/agents "
	items := am.collectSuggestions()
	if len(items) < 2 {
		t.Fatalf("picker rows=%d want >=2: %+v", len(items), items)
	}
	found := false
	for _, it := range items {
		if it.value == "my-reviewer" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected my-reviewer in picker: %+v", items)
	}
	runID, name, ok := am.resolveAgentFocusTarget("my-reviewer")
	if !ok || runID != "run-98485" || name != "my-reviewer" {
		t.Fatalf("resolve my-reviewer = %s %s ok=%v", runID, name, ok)
	}
}
