package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// Guide-F UX follow-up (operator): spawned sub-agents were invisible in the
// main view — /agents required an explicit command. The right sidebar now
// shows a live "agents" section (styled like the flow "steps" section) while
// the current run's agent graph has entries.

func agentsSidebarModel(t *testing.T) *AppModel {
	t.Helper()
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:9")
	m.authPhase = AuthNone
	m.sessionDefaultsLoaded = true
	m.width = 160
	m.height = 30
	m.mode = ModeChat
	// The sidebar only engages with session content (useRightSidebar).
	m.sessionPanel.RunnerURL = "http://127.0.0.1:9"
	return m
}

func TestAgentsSidebar_ShowsLiveSubAgentRows(t *testing.T) {
	m := agentsSidebarModel(t)
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-a", AgentName: "helper", Label: "helper", Status: "running"},
		{RunID: "run-b", AgentName: "reviewer", Status: "completed"},
		{RunID: "run-c", AgentName: "writer", Status: "failed"},
	}
	lines := m.renderRightSidebar(m.height)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(stripANSI(joined), "\nagents") && !strings.HasPrefix(stripANSI(joined), "agents") && !strings.Contains(stripANSI(joined), "agents") {
		t.Fatalf("sidebar must contain an agents section, got:\n%s", joined)
	}
	for _, want := range []string{"helper", "reviewer", "writer"} {
		if !strings.Contains(stripANSI(joined), want) {
			t.Fatalf("sidebar missing sub-agent %q, got:\n%s", want, joined)
		}
	}
	if !strings.Contains(stripANSI(joined), "RUNNING") {
		t.Fatal("running sub-agent must show its status")
	}
	if len(lines) != m.height {
		t.Fatalf("sidebar must stay height-padded: %d lines != %d", len(lines), m.height)
	}
}

func TestAgentsSidebar_HiddenWithoutAgentRuns(t *testing.T) {
	m := agentsSidebarModel(t)
	lines := m.renderRightSidebar(m.height)
	for _, l := range lines {
		if strings.Contains(stripANSI(l), "agents") {
			t.Fatalf("sidebar must not show an agents section without agent runs, got %q", stripANSI(l))
		}
	}
}

// BUG-336 (operator report: agents rows flickered and re-ordered constantly):
// display order is deterministic — main pinned first, the rest by spawn time
// (CreatedAt), RunID tiebreak — so the same set of runs always renders in the
// same order no matter which snapshot/hydrate delivered it.
func TestAgentsSidebar_StableMainFirstThenSpawnOrder(t *testing.T) {
	m := agentsSidebarModel(t)
	// Deliberately shuffled: snapshot delivered main LAST and reviewers mid-list.
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-3", AgentName: "reviewer", Role: "reviewer", Status: "completed", CreatedAt: "2026-08-30T10:05:00Z"},
		{RunID: "run-1", AgentName: "main", Role: "main", Status: "completed", CreatedAt: "2026-08-30T10:00:00Z"},
		{RunID: "run-4", AgentName: "implement", Role: "coder", Status: "completed", CreatedAt: "2026-08-30T10:03:00Z"},
		{RunID: "run-2", AgentName: "test_signatures", Role: "coder", Status: "completed", CreatedAt: "2026-08-30T10:01:00Z"},
	}
	want := []string{"main", "test_signatures", "implement", "reviewer"}
	got := stripANSI(strings.Join(m.agentRunsSectionLines(10), "\n"))
	pos := -1
	for _, w := range want {
		next := strings.Index(got, w)
		if next < 0 {
			t.Fatalf("missing agent %q in section:\n%s", w, got)
		}
		if next < pos {
			t.Fatalf("agents out of time order: %q appears before the previous row\n%s", w, got)
		}
		pos = next
	}
	// Same set in a different delivery order must render identically.
	m2 := agentsSidebarModel(t)
	m2.agentRuns = []client.AgentRunSummary{
		{RunID: "run-2", AgentName: "test_signatures", Role: "coder", Status: "completed", CreatedAt: "2026-08-30T10:01:00Z"},
		{RunID: "run-4", AgentName: "implement", Role: "coder", Status: "completed", CreatedAt: "2026-08-30T10:03:00Z"},
		{RunID: "run-1", AgentName: "main", Role: "main", Status: "completed", CreatedAt: "2026-08-30T10:00:00Z"},
		{RunID: "run-3", AgentName: "reviewer", Role: "reviewer", Status: "completed", CreatedAt: "2026-08-30T10:05:00Z"},
	}
	if strings.Join(m2.agentRunsSectionLines(10), "|") != strings.Join(m.agentRunsSectionLines(10), "|") {
		t.Fatal("identical run sets must render identically regardless of delivery order (flicker fix)")
	}
}

func TestAgentsSidebar_RowsTruncatedToHeight(t *testing.T) {
	m := agentsSidebarModel(t)
	m.height = 10
	for i := 0; i < 30; i++ {
		m.agentRuns = append(m.agentRuns, client.AgentRunSummary{RunID: "run-x", AgentName: "agent-x", Status: "completed"})
	}
	lines := m.renderRightSidebar(m.height)
	if len(lines) != m.height {
		t.Fatalf("sidebar must stay height-padded: %d != %d", len(lines), m.height)
	}
	if !strings.Contains(stripANSI(strings.Join(lines, "\n")), "more") {
		t.Fatal("overflow agents must collapse into a … +N more row")
	}
}

// BUG-336 UX: flow mode must NOT render the agents section — steps already
// name their agent inline. Chat mode keeps the section (existing tests).
func TestAgentsSidebar_HiddenInFlowMode(t *testing.T) {
	m := agentsSidebarModel(t)
	m.mode = ModeFlow
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-a", AgentName: "reviewer", Role: "reviewer", Status: "completed"},
	}
	lines := m.renderRightSidebar(m.height)
	for _, l := range lines {
		if strings.Contains(stripANSI(l), "agents") || strings.Contains(stripANSI(l), "reviewer") {
			t.Fatalf("flow-mode sidebar must not show the agents section, got %q", stripANSI(l))
		}
	}
}

// BUG-336 UX: in flow mode a step executed by a spawned agent carries an
// inline "· agent: <name>" label on its row.
func TestFlowSteps_AgentBackedStepNamesItsAgent(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	m := agentsSidebarModel(t)
	m.mode = ModeFlow
	m.flowSteps = []client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "reviewer", StepType: "review", Status: "done"},
		{StepID: "s2", NodeID: "context", StepType: "context", Status: "done"},
	}
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-child-1", AgentName: "reviewer-1", Label: "reviewer", Role: "reviewer", Status: "completed"},
	}
	lines := m.flowStepsPanelLinesMax(20)
	joined := stripANSI(strings.Join(lines, "\n"))
	if !strings.Contains(joined, "agent: reviewer-1") {
		t.Fatalf("agent-backed step must name its agent inline, got:\n%s", joined)
	}
	if strings.Count(joined, "agent: reviewer-1") != 1 {
		t.Fatalf("the agent-less step must not carry the chip:\n%s", joined)
	}
}
