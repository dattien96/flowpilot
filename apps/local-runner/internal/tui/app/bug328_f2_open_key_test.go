package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestBug328_F2BracketOpenStep(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.authPhase = AuthNone
	enableSidebarForTest(m)
	m.width, m.fullWidth = tuiSidebarMinWidth+10, tuiSidebarMinWidth+10
	m.flowSteps = []client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "plan", Status: "DONE"},
		{StepID: "s2", NodeID: "coder", Status: "RUNNING", AgentRef: "coder"},
	}
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-main", AgentName: "main", Status: "running"},
		{RunID: "run-child", AgentName: "coder", Status: "running"},
	}
	m.runHandle = &client.RunHandle{RunID: "run-main"}
	runs := m.openableStepRunIDs()
	if len(runs) == 0 {
		t.Fatal("expected at least one openable step")
	}
	m2, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if cmd == nil {
		t.Fatal("o must open highlighted sidebar step")
	}
	if m2 == nil {
		t.Fatal("must return model")
	}
}

func TestBug328_F2BracketNoOpWhenSidebarHidden(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.authPhase = AuthNone
	m.width, m.fullWidth = tuiSidebarMinWidth-1, tuiSidebarMinWidth-1
	m.flowSteps = []client.WorkflowStepRuntime{{StepID: "s1", NodeID: "plan", Status: "RUNNING"}}
	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if cmd != nil {
		t.Fatal("o must be no-op when the sidebar is hidden")
	}
}