package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// Selected step row must use the SAME filled-chip selection language as the
// action ring / mode-setup tab row (styleStepSelected: bold white on bg 62) —
// the old teal text (styleStatusAgent) did not stand out against the dim
// unselected rows (user feedback on the sidebar steps view).
//
// Assertions compare against styleStepSelected.Render(...) so they hold under
// any lipgloss color profile (in the no-color test profile the render is the
// plain text; with color it carries the 62 background fill).

func TestSelectedStepRow_UsesFilledChipStyle(t *testing.T) {
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

			rows := m.flowStepsPanelLines()

			// The focused step row must be rendered through styleStepSelected
			// (chip padding inside the fill, like renderActionRingChip).
			glyph := thinkingSpinner(m.thinkingFrame, m.asciiMode)
			want := styleStepSelected.Render(" [" + glyph + "] > my-reviewer RUNNING ")
			found := false
			for _, r := range rows {
				if strings.Contains(r, want) {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("%s: focused step row must use styleStepSelected fill:\nwant: %q\ngot:  %s", pk, want, strings.Join(rows, "\n"))
			}
		})
	}
}

func TestSelectedStepRow_NoFocus_NoSelectionMarker(t *testing.T) {
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
				{StepID: "s2", NodeID: "my-reviewer", AgentRef: "my-reviewer", Status: "RUNNING"},
			}
			m.flowStepsActive = "my-reviewer"
			// No focusRunID: nothing is selected.

			rows := m.flowStepsPanelLines()
			for _, r := range rows {
				plain := stripANSI(r)
				if strings.Contains(plain, ">") || strings.Contains(plain, "▸") {
					t.Fatalf("%s: no row may show the selection marker without focus: %q", pk, plain)
				}
			}
		})
	}
}
