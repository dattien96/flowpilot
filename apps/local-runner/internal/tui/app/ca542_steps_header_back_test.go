package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-542 — the step [back] chip moved from the focused step row to the "steps"
// section header so it never occupies the [open] pixel column. Clicking [open]
// and then clicking again at the same coordinates can no longer flip back to the
// main view (the reported duplicate-click bug). The redundant "Steps N:" count
// line is gone too — the sidebar/overlay already render a "steps" section title.

// TestStepsHeader_HostsBackWhileViewingChild: while a child is focused the steps
// section title carries [back]; the focused step row keeps highlight but no chip.
func TestStepsHeader_HostsBackWhileViewingChild(t *testing.T) {
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
			if !strings.Contains(title, "[back]") {
				t.Fatalf("%s: steps header must host [back]: %q", pk, title)
			}
			if !strings.Contains(m.stepsSectionTitle(), styleStepAgentAction.Render("[back]")) {
				t.Fatalf("%s: header [back] must use the step-agent action style", pk)
			}
			rows := strings.Join(m.flowStepsPanelLines(), "\n")
			if strings.Contains(rows, "[back]") {
				t.Fatalf("%s: focused step row must not hold [back]:\n%s", pk, rows)
			}
			if strings.Contains(rows, "[open]") {
				t.Fatalf("%s: focused step row must not hold [open]:\n%s", pk, rows)
			}
			// The sidebar renders the header with [back].
			side := strings.Join(m.renderRightSidebar(m.height), "\n")
			if !strings.Contains(side, "[back]") {
				t.Fatalf("%s: right sidebar must render header [back]:\n%s", pk, side)
			}
		})
	}
}

// TestStepsHeader_NoCountLine: the redundant "Steps N:" line is removed.
func TestStepsHeader_NoCountLine(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := sidebarFlowModel(pk, 120)
			joined := strings.Join(m.flowStepsPanelLines(), "\n")
			if strings.Contains(joined, "Steps 2:") {
				t.Fatalf("%s: 'Steps N:' count line must be gone (CA-542):\n%s", pk, joined)
			}
			if !strings.Contains(joined, "[open]") {
				t.Fatalf("%s: child step must still expose [open]:\n%s", pk, joined)
			}
		})
	}
}

// TestOpenClick_DoubleClickDoesNotFlipBack locks the reported bug: clicking [open]
// focuses the child, and a second click at the exact same coordinates must NOT
// return to the main view (the old [back] chip used to appear at that pixel).
func TestOpenClick_DoubleClickDoesNotFlipBack(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := sidebarFlowModel(pk, 120)
			x, y, ok := findClickTarget(m, "agent-open:run-rev")
			if !ok {
				t.Fatalf("%s: expected [open] hit", pk)
			}
			m2, _ := m.dispatchMouseClick(x, y)
			am := m2.(*AppModel)
			if !am.viewingChild() || am.focusRunID != "run-rev" {
				t.Fatalf("%s: first click should focus child: focusRunID=%q", pk, am.focusRunID)
			}
			// Second click at the identical pixel: the row no longer hosts [back],
			// so it must not flip back to main.
			m3, _ := am.dispatchMouseClick(x, y)
			am2 := m3.(*AppModel)
			if !am2.viewingChild() || am2.focusRunID != "run-rev" {
				t.Fatalf("%s: double-click at [open] pixel flipped back to main: focusRunID=%q",
					pk, am2.focusRunID)
			}
		})
	}
}

// TestStepsHeaderBack_ClickReturnsMain: clicking [back] on the steps header in the
// sidebar returns to the main view.
func TestStepsHeaderBack_ClickReturnsMain(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := sidebarFlowModel(pk, 120)
			m.mainTranscript = []ChatMessage{{Role: "user", Content: "hello"}}
			m.focusRunID = "run-rev"
			x, y, ok := findClickTarget(m, "agent-back")
			if !ok {
				t.Fatalf("%s: expected [back] hit on steps header", pk)
			}
			m2, _ := m.dispatchMouseClick(x, y)
			am := m2.(*AppModel)
			if am.viewingChild() || am.focusRunID != "" {
				t.Fatalf("%s: header [back] must return main: focusRunID=%q", pk, am.focusRunID)
			}
		})
	}
}

// TestStepsHeaderBack_ReactivePaths (Task-311): the [back] chip lives in the
// reactive sidebar. A narrow terminal shows no panel surface at all — chat
// column only; wide terminals render the sidebar with a hittable [back].
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
			// Wide → sidebar renders the steps header with [back] and is hittable.
			enableSidebarForTest(m)
			m.width, m.fullWidth = tuiSidebarMinWidth+20, tuiSidebarMinWidth+20
			side := strings.Join(m.renderRightSidebar(30), "\n")
			if !strings.Contains(side, "[back]") {
				t.Fatalf("%s: wide sidebar must render header [back]:\n%s", pk, side)
			}
			if _, _, ok := findClickTarget(m, "agent-back"); !ok {
				t.Fatalf("%s: sidebar header [back] must be hittable:\n%s", pk, side)
			}
		})
	}
}
