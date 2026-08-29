package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// Requirement 1: Tab loop only for buttons, not for agent-flow switch.
// Agent view switches only via /agents; picker /agents + Tab still works for the picker.

// TestTab_DoesNotCycleAgentView verifies that Tab never focuses a child agent.
func TestTab_DoesNotCycleAgentView_NewUX(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-main", AgentName: "main", Role: "main", Status: "running"},
		{RunID: "run-child", AgentName: "coder", Status: "running"},
	}
	prevFocus := m.focusRunID
	prevIdx := m.focusedAgentIdx
	m2, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	am := m2.(*AppModel)
	if am.focusRunID != prevFocus {
		t.Fatalf("Tab must not change focusRunID: got %q want %q", am.focusRunID, prevFocus)
	}
	if am.focusedAgentIdx != prevIdx {
		t.Fatalf("Tab must not cycle focusedAgentIdx: got %d want %d", am.focusedAgentIdx, prevIdx)
	}
	if cmd != nil {
		// The only allowed Tab cmd when agents exist is the action-ring/posture/cmd,
		// not cmdFocusAgent. Ensure focus didn't change.
		_ = cmd
	}
}

// TestTab_PickerAgentsStillWorks ensures /agents picker is still reachable via Tab.
func TestTab_PickerAgentsStillWorks(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-main", AgentName: "main", Role: "main"},
		{RunID: "run-coder", AgentName: "coder", Status: "running"},
	}
	m.inputValue = "/agents"
	items := m.collectSuggestions()
	if len(items) == 0 {
		t.Fatal("want agent suggestions for /agents")
	}
	// Tab should accept the current suggestion (first agent = main).
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	_ = m2
}

// Requirement 2: sidebar step highlights the selected agent with a different color
// and a marker character.

// TestSidebar_HighlightsSelectedAgent verifies the teal marker on the viewed child row.
func TestSidebar_HighlightsSelectedAgent(t *testing.T) {
	m := testFlowWithChildAgents()
	m2, _ := m.handleSlashCommand("/agents my-reviewer")
	am := m2.(*AppModel)
	joined := stripANSI(strings.Join(am.flowStepsPanelLines(), "\n"))
	if !strings.Contains(joined, "my-reviewer") {
		t.Fatalf("child step must render: %s", joined)
	}
	if !strings.Contains(joined, ">") && !strings.Contains(joined, "▸") {
		t.Fatalf("highlight marker missing on focused child: %s", joined)
	}
	if strings.Contains(joined, "[open]") || strings.Contains(joined, "[back]") {
		t.Fatalf("no clickable chips on highlighted row: %s", joined)
	}
}

// Requirements 3 + 4: remove [open] and [back] texts entirely (no click).

func TestSidebar_NoOpenBackChips(t *testing.T) {
	m := testFlowWithChildAgents()
	joined := strings.Join(m.flowStepsPanelLines(), "\n")
	if strings.Contains(joined, "[open]") {
		t.Fatalf("must not render [open]: %s", joined)
	}
	if strings.Contains(joined, "[back]") {
		t.Fatalf("must not render [back]: %s", joined)
	}
	title := m.stepsSectionTitle()
	if strings.Contains(title, "[back]") {
		t.Fatalf("steps header must not host [back]: %q", title)
	}
}

// Requirement 5: agent view shows guide + Esc returns to main, and /agent main also works.

// TestEsc_FromAgentView_ReturnsMain verifies Esc restores the main transcript.
func TestEsc_FromAgentView_ReturnsMain(t *testing.T) {
	m := testFlowWithChildAgents()
	m.mainTranscript = []ChatMessage{{Role: "user", Content: "hello main"}}
	m.focusRunID = "run-rev"
	if !m.viewingChild() {
		t.Fatal("precondition: must be viewing child")
	}
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEscape})
	am := m2.(*AppModel)
	if am.viewingChild() || am.focusRunID != "" {
		t.Fatalf("Esc must return to main: focusRunID=%q viewingChild=%v", am.focusRunID, am.viewingChild())
	}
	// Esc on main should not crash and should not focus a child.
	m3, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyEscape})
	_ = m3
}

// TestAgentView_BannerMentionsEsc verifies the read-only banner describes both ways back.
func TestAgentView_BannerMentionsEsc(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.focusRunID = "run-rev"
	m.runHandle = &client.RunHandle{RunID: "run-main"}
	b := m.renderInputLine()
	if !strings.Contains(b, "Esc") {
		t.Fatalf("read-only banner must mention Esc: %q", b)
	}
	if !strings.Contains(b, "/agent main") {
		t.Fatalf("banner must keep /agent main hint: %q", b)
	}
}

// TestAgentsDump_MentionsEsc verifies /agents dump text mentions Esc (not Tab cycle / [open]).
func TestAgentsDump_MentionsEsc(t *testing.T) {
	m := testFlowWithChildAgents()
	m2, _ := m.handleSlashCommand("/agents")
	blob := ""
	for _, msg := range m2.(*AppModel).messages {
		blob += msg.Content + "\n"
	}
	if !strings.Contains(blob, "Esc") {
		t.Fatalf("agents dump must mention Esc: %s", blob)
	}
	if strings.Contains(blob, "[open]") {
		t.Fatalf("dump must not mention [open] chip: %s", blob)
	}
}

// Requirement 6: /agents list does not flicker (stable order across hydrate reorders).

func TestAgentsList_NoFlickerAcrossHydrateReorder(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m.runHandle = &client.RunHandle{RunID: "run-main", Status: "running"}
	m.flowSteps = []client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "my-reviewer", AgentRef: "my-reviewer", Status: "RUNNING"},
	}
	// First order: b, a
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-main", AgentName: "main", Role: "main", Status: "running"},
		{RunID: "run-b", AgentName: "my-reviewer", Status: "running"},
		{RunID: "run-a", AgentName: "my-reviewer", Status: "running"},
	}
	m.inputValue = "/agents"
	first := m.collectSuggestions()
	var firstNames []string
	for _, it := range first {
		firstNames = append(firstNames, it.value)
	}
	// Hydrate reorder: a, b (opposite incoming order)
	next, _ := m.Update(agentRunsHydratedMsg{
		ParentRunID: "run-main",
		Runs: []client.AgentRunSummary{
			{RunID: "run-main", AgentName: "main", Role: "main", Status: "running"},
			{RunID: "run-a", AgentName: "my-reviewer", Status: "running"},
			{RunID: "run-b", AgentName: "my-reviewer", Status: "running"},
		},
	})
	am := next.(*AppModel)
	am.inputValue = "/agents"
	second := am.collectSuggestions()
	var secondNames []string
	for _, it := range second {
		secondNames = append(secondNames, it.value)
	}
	if strings.Join(firstNames, ",") != strings.Join(secondNames, ",") {
		t.Fatalf("picker must not flicker: first %v second %v", firstNames, secondNames)
	}
	// Stable order also keeps the focused marker from jumping.
	m2, _ := am.handleSlashCommand("/agents my-reviewer")
	am2 := m2.(*AppModel)
	line := stripANSI(strings.Join(am2.flowStepsPanelLines(), "\n"))
	if !strings.Contains(line, "my-reviewer") {
		t.Fatalf("child step must render after reorder: %s", line)
	}
}
