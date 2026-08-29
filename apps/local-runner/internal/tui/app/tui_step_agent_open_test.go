package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func testFlowWithChildAgents() *AppModel {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.width, m.height = 120, 36
	m.asciiMode = true
	m.mode = ModeFlow
	m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
	enableSidebarForTest(m)
	m.runHandle = &client.RunHandle{RunID: "run-main", Status: "running"}
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-main", AgentName: "main", Role: "main", Status: "running"},
		{RunID: "run-rev", AgentName: "my-reviewer", Status: "running"},
	}
	m.flowSteps = []client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "coder", Status: "DONE"},
		{StepID: "s2", NodeID: "my-reviewer", AgentRef: "my-reviewer", Status: "RUNNING"},
	}
	m.flowStepsActive = "my-reviewer"
	return m
}

func TestStepPanel_ChildStepHasNoOpenChip(t *testing.T) {
	m := testFlowWithChildAgents()
	joined := strings.Join(m.flowStepsPanelLines(), "\n")
	if !strings.Contains(joined, "my-reviewer") {
		t.Fatalf("child step must render:\n%s", joined)
	}
	if strings.Contains(joined, "[open]") {
		t.Fatalf("child step must NOT show [open] (keyboard-only via /agents):\n%s", joined)
	}
	if strings.Contains(joined, "[back]") {
		t.Fatalf("child step must NOT show [back]:\n%s", joined)
	}
}

func TestStepPanel_NoOpenWithoutChildRun(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.flowSteps = []client.WorkflowStepRuntime{
		{StepID: "b", NodeID: "grok-context", Status: "RUNNING"},
	}
	joined := strings.Join(m.flowStepsPanelLines(), "\n")
	if strings.Contains(joined, "[open]") {
		t.Fatalf("no [open] when agent graph has no child: %q", joined)
	}
}

func TestStepPanel_FocusChildViaAgentsShowsTealMarker(t *testing.T) {
	m := testFlowWithChildAgents()
	m2, cmd := m.handleSlashCommand("/agents my-reviewer")
	if cmd == nil {
		t.Fatal("expected focus cmd")
	}
	am := m2.(*AppModel)
	if !am.viewingChild() || am.focusRunID != "run-rev" {
		t.Fatalf("focusRunID=%q viewingChild=%v", am.focusRunID, am.viewingChild())
	}
	// The focused child step row uses the teal agent style + a marker (no chip).
	joined := stripANSI(strings.Join(am.flowStepsPanelLines(), "\n"))
	if !strings.Contains(joined, "my-reviewer") {
		t.Fatalf("child step must render:\n%s", joined)
	}
	if !strings.Contains(joined, ">") && !strings.Contains(joined, "▸") {
		t.Fatalf("focused child step must show a selection marker: %s", joined)
	}
	if strings.Contains(joined, "[open]") || strings.Contains(joined, "[back]") {
		t.Fatalf("no clickable chips on the focused row:\n%s", joined)
	}
}

func TestStepPanel_EscFromChildReturnsMain(t *testing.T) {
	m := testFlowWithChildAgents()
	m.mainTranscript = []ChatMessage{{Role: "user", Content: "hello"}}
	m.focusRunID = "run-rev"
	joined := stripANSI(strings.Join(m.flowStepsPanelLines(), "\n"))
	if strings.Contains(joined, "[back]") {
		t.Fatalf("focused step row must not hold [back]:\n%s", joined)
	}
	if strings.Contains(joined, "[open]") {
		t.Fatalf("focused step row must not hold [open]:\n%s", joined)
	}
	// stepsSectionTitle carries no [back] chip (switching is /agents + Esc).
	if strings.Contains(m.stepsSectionTitle(), "[back]") {
		t.Fatalf("steps header must not host [back]: %q", m.stepsSectionTitle())
	}
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEscape})
	am := m2.(*AppModel)
	if am.viewingChild() || am.focusRunID != "" {
		t.Fatalf("Esc must return to main; focusRunID=%q", am.focusRunID)
	}
}

func TestStepPanel_OpenBackNotOnStatus(t *testing.T) {
	m := testFlowWithChildAgents()
	_ = m.View()
	line0 := stripANSI(m.renderStatusLine0(" | ", 120))
	if !strings.Contains(line0, "agent:main") {
		t.Fatalf("status want agent:main: %q", line0)
	}
	if strings.Contains(line0, "my-reviewer") && strings.Contains(line0, "[open]") {
		t.Fatalf("status must not host open chrome: %q", line0)
	}
	panel := strings.Join(m.flowStepsPanelLines(), "\n")
	if strings.Contains(panel, "[open]") || strings.Contains(panel, "[back]") {
		t.Fatalf("panel must not host click chips:\n%q", panel)
	}
}

func TestAgentsSlash_PickerListsChildren(t *testing.T) {
	m := testFlowWithChildAgents()
	m.inputValue = "/agents"
	items := m.collectSuggestions()
	if len(items) < 2 {
		t.Fatalf("picker rows=%d want >=2", len(items))
	}
	var names []string
	for _, it := range items {
		if it.kind != "agent" {
			t.Fatalf("kind=%q", it.kind)
		}
		names = append(names, it.value)
	}
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, "main") || !strings.Contains(joined, "my-reviewer") {
		t.Fatalf("picker names=%v", names)
	}
	if got := suggestionAcceptValue(items[1]); got != "/agent my-reviewer" && !strings.HasPrefix(got, "/agent ") {
		// order is main-first; child is not index 0
		found := false
		for _, it := range items {
			if suggestionAcceptValue(it) == "/agent my-reviewer" {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing /agent my-reviewer accept; items=%v accept=%q", names, got)
		}
	}
}

func TestAgentsSlash_NamedArgOpensChild(t *testing.T) {
	m := testFlowWithChildAgents()
	m2, cmd := m.handleSlashCommand("/agents my-reviewer")
	if cmd == nil {
		t.Fatal("expected focus cmd")
	}
	am := m2.(*AppModel)
	if am.focusRunID != "run-rev" {
		t.Fatalf("focusRunID=%q", am.focusRunID)
	}
}

func TestAgentsSlash_ExactWithoutRunsStillToggles(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.inputValue = "/agents"
	items := m.collectSuggestions()
	for _, it := range items {
		if it.kind == "agent" {
			t.Fatalf("must not trap /agents enter when no runs: %+v", items)
		}
	}
	m2, _ := m.handleSlashCommand("/agents")
	var blob strings.Builder
	for _, msg := range m2.(*AppModel).messages {
		blob.WriteString(msg.Content)
		blob.WriteByte('\n')
	}
	if !strings.Contains(blob.String(), "Agents focus: ON") {
		t.Fatalf("toggle missing: %+v", m2.(*AppModel).messages)
	}
}
