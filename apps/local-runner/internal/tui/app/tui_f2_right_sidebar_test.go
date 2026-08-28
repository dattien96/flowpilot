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

// TestRightSidebar_WideEngages: a wide (>= tuiSidebarMinWidth) terminal renders
// the full-height right sidebar column (panelH=0, sideActive).
func TestRightSidebar_WideEngages(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := sidebarFlowModel(pk, 120)
			if !m.useRightSidebar() {
				t.Fatalf("%s: wide terminal must engage right sidebar", pk)
			}
			c := m.tuiChrome()
			if !c.sideActive {
				t.Fatalf("%s: tuiChrome sideActive=false", pk)
			}
			if c.panelH != 0 {
				t.Fatalf("%s: sidebar must not reserve top overlay rows (panelH=%d)", pk, c.panelH)
			}
			// Sidebar column present in the rendered view – now only session+steps (no status)
			view := m.View()
			if !strings.Contains(view, "session") {
				t.Fatalf("%s: view missing sidebar session section:\n%s", pk, view)
			}
			if !strings.Contains(view, "steps") {
				t.Fatalf("%s: view missing sidebar steps section:\n%s", pk, view)
			}
		})
	}
}

// TestRightSidebar_NarrowKeepsChatOnly: a narrow (< tuiSidebarMinWidth)
// terminal shows no sidebar and no overlay — chat column only (Task-311).
func TestRightSidebar_NarrowKeepsChatOnly(t *testing.T) {
	m := sidebarFlowModel("codex", 80)
	if m.useRightSidebar() {
		t.Fatal("narrow terminal must not engage right sidebar")
	}
	c := m.tuiChrome()
	if c.sideActive {
		t.Fatal("narrow terminal must not set sideActive")
	}
	if len(c.panelLines) != 0 {
		t.Fatal("narrow terminal must have no overlay rows (Task-311)")
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

// TestRightSidebar_ClickCollapseIsNoOp: the [collapse] click target is gone —
// width is the only switch (Task-311).
func TestRightSidebar_ClickCollapseIsNoOp(t *testing.T) {
	m := sidebarFlowModel("codex", 120)
	m2, _ := m.activateClickTarget("sidebar-collapse")
	if len(m2.(*AppModel).messages) != 0 {
		t.Fatal("sidebar-collapse click must be a no-op")
	}
}

// TestRightSidebar_ClickSessionIsNoOp: clicking the sidebar "session" header no
// longer toggles anything (Task-311).
func TestRightSidebar_ClickSessionIsNoOp(t *testing.T) {
	m := sidebarFlowModel("codex", 120)
	m2, _ := m.activateClickTarget("session")
	if len(m2.(*AppModel).messages) != 0 {
		t.Fatal("session click must be a no-op")
	}
}

// TestStepTodoRestyle_KeepsLegacyTokens: the todo-list step restyle still emits the
// tokens the legacy suite asserts ("RUNNING", "Now: <name>") plus the
// OpenCode todo glyphs ([+]/[x]) and, for the RUNNING step, the animated spinner
// glyph (CA-537) instead of the legacy [•]. The redundant "Steps N:" count line
// is gone (CA-542) — the sidebar renders a "steps" section title instead.
func TestStepTodoRestyle_KeepsLegacyTokens(t *testing.T) {
	m := sidebarFlowModel("codex", 120)
	joined := strings.Join(m.flowStepsPanelLines(), "\n")
	if strings.Contains(joined, "Steps 2:") {
		t.Fatalf("'Steps N:' count line must be gone (CA-542):\n%s", joined)
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
	if !strings.Contains(joined, "[|]") {
		t.Fatalf("RUNNING step must use the ascii spinner glyph (frame 0) instead of [•]:\n%s", joined)
	}
	if !strings.Contains(strings.Join(m.renderRightSidebar(m.height), "\n"), "steps") {
		t.Fatal("sidebar must keep the 'steps' section title")
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
