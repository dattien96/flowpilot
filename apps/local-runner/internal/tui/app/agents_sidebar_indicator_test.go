package app

import (
	"strings"
	"testing"

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
