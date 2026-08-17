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

func TestAgentRunsHydrated_ExpandsF2AndShowsOpen(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.width, m.height = 120, 36
	m.asciiMode = true
	m.mode = ModeFlow
	m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
	m.sessionPanel.Collapsed = true // start folded
	m.runHandle = &client.RunHandle{RunID: "run-main", Status: "running"}
	m.flowSteps = []client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "my-reviewer", AgentRef: "my-reviewer", Status: "RUNNING"},
	}

	// No agents yet → no [open].
	if strings.Contains(strings.Join(m.flowStepsPanelLines(), "\n"), "[open]") {
		t.Fatal("must not show [open] before agent graph hydrate")
	}

	next, _ := m.Update(agentRunsHydratedMsg{
		ParentRunID: "run-main",
		Runs: []client.AgentRunSummary{
			{RunID: "run-main", AgentName: "main", Role: "main", Status: "running"},
			{RunID: "run-rev", AgentName: "my-reviewer", Status: "running"},
		},
	})
	am := next.(*AppModel)
	if am.sessionPanel.Collapsed {
		t.Fatal("F2 panel must expand when child agents hydrate")
	}
	joined := strings.Join(am.flowStepsPanelLines(), "\n")
	if !strings.Contains(joined, "[open]") {
		t.Fatalf("expected live [open] after hydrate:\n%s", joined)
	}
	if _, _, ok := findClickTarget(am, "agent-open:run-rev"); !ok {
		t.Fatalf("F2 [open] must be hittable while panel expanded:\n%s",
			strings.Join(am.renderSessionPanelOverlay(), "\n"))
	}
}

func TestAgentGraphUpdated_ExpandsF2ForLiveChild(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m.sessionPanel.Collapsed = true
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
	if am.sessionPanel.Collapsed {
		t.Fatal("agent_graph_updated must expand F2 when children present")
	}
	if !strings.Contains(strings.Join(am.flowStepsPanelLines(), "\n"), "[open]") {
		t.Fatal("live child step must show [open]")
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
