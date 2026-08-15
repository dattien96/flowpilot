package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-524 — F2 right sidebar (OpenCode-style) + todo-list step styling.

func sidebarFlowModel(pk string, width int) *AppModel {
	m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
	m.width, m.height = width, 30
	m.asciiMode = true
	m.mode = ModeFlow
	m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
	m.sessionPanel.Collapsed = false
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

// TestRightSidebar_WideExpandedEngages: a wide (>=100) expanded F2 panel renders a
// full-height right sidebar (panelH=0, sideActive) instead of the top overlay.
func TestRightSidebar_WideExpandedEngages(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := sidebarFlowModel(pk, 120)
			if !m.useRightSidebar() {
				t.Fatalf("%s: wide expanded must engage right sidebar", pk)
			}
			c := m.tuiChrome()
			if !c.sideActive {
				t.Fatalf("%s: tuiChrome sideActive=false", pk)
			}
			if c.panelH != 0 {
				t.Fatalf("%s: sidebar must not reserve top overlay rows (panelH=%d)", pk, c.panelH)
			}
			// Sidebar column present in the rendered view.
			view := m.View()
			if !strings.Contains(view, "[collapse]") {
				t.Fatalf("%s: view missing sidebar [collapse]:\n%s", pk, view)
			}
		})
	}
}

// TestRightSidebar_NarrowUsesOverlay: a narrow (<100) expanded F2 panel keeps the
// legacy top-right overlay (no sidebar column).
func TestRightSidebar_NarrowUsesOverlay(t *testing.T) {
	m := sidebarFlowModel("codex", 80)
	if m.useRightSidebar() {
		t.Fatal("narrow terminal must not engage right sidebar")
	}
	c := m.tuiChrome()
	if c.sideActive {
		t.Fatal("narrow terminal must not set sideActive")
	}
	if c.panelH == 0 {
		t.Fatal("narrow terminal must keep the top-right overlay (panelH>0)")
	}
}

// TestRightSidebar_CollapsedWideShowsChip: a wide but collapsed F2 panel shows only
// the [info] chip overlay (no sidebar column).
func TestRightSidebar_CollapsedWideShowsChip(t *testing.T) {
	m := sidebarFlowModel("codex", 120)
	m.sessionPanel.Collapsed = true
	if m.useRightSidebar() {
		t.Fatal("collapsed panel must not engage right sidebar")
	}
	c := m.tuiChrome()
	if c.sideActive {
		t.Fatal("collapsed panel must not set sideActive")
	}
	view := stripANSI(m.View())
	if strings.Contains(view, "[collapse]") {
		t.Fatal("collapsed panel must not render sidebar [collapse]")
	}
	if !strings.Contains(view, "[info]") {
		t.Fatal("collapsed panel must render the [info] chip")
	}
}

// TestRightSidebar_ClickOpenFocusesChild: clicking [open] on a child step row in the
// right sidebar focuses that child agent.
func TestRightSidebar_ClickOpenFocusesChild(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := sidebarFlowModel(pk, 120)
			x, y, ok := findClickTarget(m, "agent-open:run-rev")
			if !ok {
				t.Fatalf("%s: expected clickable [open] in right sidebar", pk)
			}
			m2, _ := m.dispatchMouseClick(x, y)
			am := m2.(*AppModel)
			if !am.viewingChild() || am.focusRunID != "run-rev" {
				t.Fatalf("%s: focusRunID=%q viewingChild=%v", pk, am.focusRunID, am.viewingChild())
			}
		})
	}
}

// TestRightSidebar_ClickCollapseFolds: clicking [collapse] in the right sidebar
// collapses the F2 panel back to the [info] chip.
func TestRightSidebar_ClickCollapseFolds(t *testing.T) {
	m := sidebarFlowModel("codex", 120)
	x, y, ok := findClickTarget(m, "sidebar-collapse")
	if !ok {
		t.Fatalf("expected clickable [collapse] in right sidebar")
	}
	m2, _ := m.dispatchMouseClick(x, y)
	am := m2.(*AppModel)
	if !am.sessionPanel.Collapsed {
		t.Fatal("clicking [collapse] must fold the F2 panel")
	}
}

// TestRightSidebar_ClickSessionToggles: clicking the sidebar "session" header toggles
// the F2 panel collapsed.
func TestRightSidebar_ClickSessionToggles(t *testing.T) {
	m := sidebarFlowModel("codex", 120)
	x, y, ok := findClickTarget(m, "session")
	if !ok {
		t.Fatalf("expected clickable session header in right sidebar")
	}
	m2, _ := m.dispatchMouseClick(x, y)
	am := m2.(*AppModel)
	if !am.sessionPanel.Collapsed {
		t.Fatal("clicking session header must toggle F2 collapsed")
	}
}

// TestStepTodoRestyle_KeepsLegacyTokens: the todo-list step restyle still emits the
// tokens the legacy suite asserts ("Steps N:", "RUNNING", "Now: <name>") plus the
// OpenCode todo glyphs ([+]/[•]).
func TestStepTodoRestyle_KeepsLegacyTokens(t *testing.T) {
	m := sidebarFlowModel("codex", 120)
	joined := strings.Join(m.flowStepsPanelLines(), "\n")
	if !strings.Contains(joined, "Steps 2:") {
		t.Fatalf("must keep 'Steps N:' header:\n%s", joined)
	}
	if !strings.Contains(joined, "RUNNING") {
		t.Fatalf("running step must keep RUNNING token:\n%s", joined)
	}
	if !strings.Contains(joined, "Now: my-reviewer") {
		t.Fatalf("must keep 'Now:' line:\n%s", joined)
	}
	if !strings.Contains(joined, "[+]") {
		t.Fatalf("DONE step must use [+] glyph:\n%s", joined)
	}
	if !strings.Contains(joined, "[•]") {
		t.Fatalf("RUNNING step must use [•] glyph:\n%s", joined)
	}
}

// TestStepTodoRestyle_FailedGlyph: a FAILED step uses the [x] glyph and keeps its
// rejection note.
func TestStepTodoRestyle_FailedGlyph(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.flowSteps = []client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "reviewer", Status: "FAILED", RejectionNote: "incomplete"},
	}
	joined := strings.Join(m.flowStepsPanelLines(), "\n")
	if !strings.Contains(joined, "[x]") {
		t.Fatalf("FAILED step must use [x] glyph:\n%s", joined)
	}
	if !strings.Contains(joined, "incomplete") {
		t.Fatalf("FAILED step must keep rejection note:\n%s", joined)
	}
}
