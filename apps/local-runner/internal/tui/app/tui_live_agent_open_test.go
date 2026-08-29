package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestStepsSuggestChildAgentOpen_OnRunningTransition(t *testing.T) {
	prev := []client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "coder", Status: "PENDING"},
	}
	next := []client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "coder", AgentRef: "coder", Status: "RUNNING"},
	}
	if !stepsSuggestChildAgentOpen(prev, next) {
		t.Fatal("RUNNING agent step should suggest hydrate")
	}
	// No change → no re-hydrate spam.
	if stepsSuggestChildAgentOpen(next, next) {
		t.Fatal("stable steps should not re-trigger")
	}
}

func TestStepsSuggestChildAgentOpen_IgnoresContextOnly(t *testing.T) {
	prev := []client.WorkflowStepRuntime{
		{StepID: "c", NodeID: "grok-context", Status: "PENDING"},
	}
	next := []client.WorkflowStepRuntime{
		{StepID: "c", NodeID: "grok-context", Status: "RUNNING"},
	}
	if stepsSuggestChildAgentOpen(prev, next) {
		t.Fatal("context step without agent signal must not force hydrate")
	}
}

func TestAgentRunsHydrated_ShowsOpenInSidebar(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	enableSidebarForTest(m)
	m.width, m.height = tuiSidebarMinWidth+20, 36
	m.fullWidth = tuiSidebarMinWidth + 20
	m.asciiMode = true
	m.mode = ModeFlow
	m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
	m.width, m.fullWidth = tuiSidebarMinWidth-1, tuiSidebarMinWidth-1 // start narrow
	m.runHandle = &client.RunHandle{RunID: "run-main", Status: "running"}
	m.flowSteps = []client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "my-reviewer", AgentRef: "my-reviewer", Status: "RUNNING"},
	}

	// No agents yet → no [open].
	if strings.Contains(strings.Join(m.flowStepsPanelLines(), "\n"), "[open]") {
		t.Fatal("must not show [open] before agent graph hydrate")
	}
	m.width, m.fullWidth = tuiSidebarMinWidth+20, tuiSidebarMinWidth+20

	next, _ := m.Update(agentRunsHydratedMsg{
		ParentRunID: "run-main",
		Runs: []client.AgentRunSummary{
			{RunID: "run-main", AgentName: "main", Role: "main", Status: "running"},
			{RunID: "run-rev", AgentName: "my-reviewer", Status: "running"},
		},
	})
	am := next.(*AppModel)
	if !am.useRightSidebar() {
		t.Fatal("wide terminal must show the sidebar")
	}
	joined := strings.Join(am.flowStepsPanelLines(), "\n")
	if strings.Contains(joined, "[open]") {
		t.Fatalf("live child must NOT show [open] chip (keyboard-only via /agents):\n%s", joined)
	}
	if !strings.Contains(joined, "my-reviewer") {
		t.Fatalf("child step must render after hydrate:\n%s", joined)
	}
}

func TestAgentGraphUpdated_LiveChildShowsOpen(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	enableSidebarForTest(m)
	m.width, m.fullWidth = tuiSidebarMinWidth+20, tuiSidebarMinWidth+20
	m.runHandle = &client.RunHandle{RunID: "run-p", Status: "running"}
	m.flowSteps = []client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "coder", AgentRef: "coder", Status: "RUNNING"},
	}
	next, cmd := m.Update(EventMsg{Ev: client.ProviderEvent{
		Type: "agent_graph_updated",
		AgentGraph: &client.AgentGraphSnapshot{
			ParentRunID: "run-p",
			Runs: []client.AgentRunSummary{
				{RunID: "run-p", AgentName: "main", Role: "main", Status: "running"},
				{RunID: "run-c", AgentName: "coder", Status: "running"},
			},
		},
	}})
	am := next.(*AppModel)
	if !am.useRightSidebar() {
		t.Fatal("wide terminal must keep the sidebar")
	}
	joined := strings.Join(am.flowStepsPanelLines(), "\n")
	if strings.Contains(joined, "[open]") {
		t.Fatalf("live child must NOT show [open] chip:\n%s", joined)
	}
	if !strings.Contains(joined, "coder") {
		t.Fatalf("child step must render:\n%s", joined)
	}
	// Still refreshes steps (existing CA behavior).
	if cmd == nil {
		t.Fatal("expected steps refresh cmd")
	}
}

func TestStepsRuntimeMsg_TriggersHydrateOnAgentStepStart(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m.runHandle = &client.RunHandle{RunID: "run-1", Status: "running"}
	m.flowSteps = []client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "coder", Status: "PENDING"},
	}
	_, cmd := m.Update(StepsRuntimeMsg{
		RunID: "run-1",
		Steps: []client.WorkflowStepRuntime{
			{StepID: "s1", NodeID: "coder", AgentRef: "coder", Status: "RUNNING"},
		},
	})
	if cmd == nil {
		t.Fatal("agent step RUNNING should return hydrate cmd")
	}
}

func TestHasChildAgentRuns(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.runHandle = &client.RunHandle{RunID: "main"}
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "main", Role: "main", AgentName: "main"},
	}
	if m.hasChildAgentRuns() {
		t.Fatal("main-only graph")
	}
	m.agentRuns = append(m.agentRuns, client.AgentRunSummary{RunID: "c1", AgentName: "coder"})
	if !m.hasChildAgentRuns() {
		t.Fatal("want child")
	}
}
