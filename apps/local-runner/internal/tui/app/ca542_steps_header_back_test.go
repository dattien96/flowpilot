package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-542 follow-up — the step row no longer hosts [open]/[back] chips. The
// focused child is highlighted with a teal marker (styleStatusAgent) in the step
// row; switching agents is keyboard-only via /agents and Esc returns to main
// (user request). The redundant "Steps N:" count line is still gone.

// TestStepsHeader_NoBackChipWhileViewingChild: while a child is focused the steps
// section title carries NO [back] chip; the focused step row shows the teal
// selection marker and no [open] chip.
func TestStepsHeader_NoBackChipWhileViewingChild(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
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
			m.focusRunID = "run-rev"

			title := stripANSI(m.stepsSectionTitle())
			if strings.Contains(title, "[back]") {
				t.Fatalf("%s: steps header must NOT host [back]: %q", pk, title)
			}
			rows := stripANSI(strings.Join(m.flowStepsPanelLines(), "\n"))
			if strings.Contains(rows, "[back]") {
				t.Fatalf("%s: focused step row must not hold [back]:\n%s", pk, rows)
			}
			if strings.Contains(rows, "[open]") {
				t.Fatalf("%s: focused step row must not hold [open]:\n%s", pk, rows)
			}
			if !strings.Contains(rows, ">") && !strings.Contains(rows, "▸") {
				t.Fatalf("%s: focused child must show selection marker: %s", pk, rows)
			}
			// The sidebar renders the header without [back].
			side := strings.Join(m.renderRightSidebar(m.height), "\n")
			if strings.Contains(side, "[back]") {
				t.Fatalf("%s: right sidebar must NOT render header [back]:\n%s", pk, side)
			}
		})
	}
}

// TestStepsHeader_NoCountLine: the redundant "Steps N:" line is removed and the
// child step never exposes an [open] chip.
func TestStepsHeader_NoCountLine(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := sidebarFlowModel(pk, 120)
			joined := strings.Join(m.flowStepsPanelLines(), "\n")
			if strings.Contains(joined, "Steps 2:") {
				t.Fatalf("%s: 'Steps N:' count line must be gone (CA-542):\n%s", pk, joined)
			}
			if strings.Contains(joined, "[open]") {
				t.Fatalf("%s: child step must NOT expose [open]:\n%s", pk, joined)
			}
		})
	}
}

// TestFocusChild_StableAcrossDoubleFocus locks the reported bug: focusing the
// child via /agents, then focusing it again, must NOT flip back to main.
func TestFocusChild_StableAcrossDoubleFocus(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := sidebarFlowModel(pk, 120)
			m2, _ := m.handleSlashCommand("/agents my-reviewer")
			am := m2.(*AppModel)
			if !am.viewingChild() || am.focusRunID != "run-rev" {
				t.Fatalf("%s: focus must target child: focusRunID=%q", pk, am.focusRunID)
			}
			m3, _ := am.handleSlashCommand("/agents my-reviewer")
			am2 := m3.(*AppModel)
			if !am2.viewingChild() || am2.focusRunID != "run-rev" {
				t.Fatalf("%s: second focus flipped back to main: focusRunID=%q", pk, am2.focusRunID)
			}
		})
	}
}

// TestStepsHeader_EscReturnsMain: Esc from a focused child returns to main.
func TestStepsHeader_EscReturnsMain(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := sidebarFlowModel(pk, 120)
			m.mainTranscript = []ChatMessage{{Role: "user", Content: "hello"}}
			m.focusRunID = "run-rev"
			m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEscape})
			am := m2.(*AppModel)
			if am.viewingChild() || am.focusRunID != "" {
				t.Fatalf("%s: Esc must return main: focusRunID=%q", pk, am.focusRunID)
			}
		})
	}
}

// TestStepsHeaderBack_ReactivePaths (Task-311): a narrow terminal shows no panel
// surface at all — chat column only; wide terminals render the sidebar.
func TestStepsHeaderBack_ReactivePaths(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.width, m.height = 80, 24
			m.asciiMode = true
			m.mode = ModeFlow
			m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
			m.runHandle = &client.RunHandle{RunID: "run-main", Status: "running"}
			m.agentRuns = []client.AgentRunSummary{
				{RunID: "run-main", AgentName: "main", Role: "main", Status: "running"},
				{RunID: "run-rev", AgentName: "my-reviewer", Status: "running"},
			}
			m.flowSteps = []client.WorkflowStepRuntime{
				{StepID: "s2", NodeID: "my-reviewer", AgentRef: "my-reviewer", Status: "RUNNING"},
			}
			m.flowStepsActive = "my-reviewer"
			m.focusRunID = "run-rev"
			if m.useRightSidebar() {
				t.Fatal("80-col terminal must not show the sidebar")
			}
			if c := m.tuiChrome(); len(c.panelLines) != 0 {
				t.Fatalf("%s: narrow terminal must have no panel overlay rows (Task-311): %v", pk, c.panelLines)
			}
			// Wide → sidebar renders the focused child with the teal marker.
			enableSidebarForTest(m)
			m.width, m.fullWidth = tuiSidebarMinWidth+20, tuiSidebarMinWidth+20
			side := strings.Join(m.renderRightSidebar(30), "\n")
			if strings.Contains(side, "[back]") {
				t.Fatalf("%s: wide sidebar must NOT render header [back]:\n%s", pk, side)
			}
			if !strings.Contains(side, "my-reviewer") {
				t.Fatalf("%s: wide sidebar must render the child step:\n%s", pk, side)
			}
		})
	}
}
