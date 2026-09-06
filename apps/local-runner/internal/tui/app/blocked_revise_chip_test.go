package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// Task-325 UX (live-found run-594636): nobody knew human feedback rides
// "/continue <note>" — approve/retry need no text so the affordance was
// invisible. The blocked bar gains a [Revise] chip that prefills the
// composer with "/continue " (type the note + Enter); the chip itself sends
// nothing and the loop stays blocked.

func reviseBlockedModel(pk string) *AppModel {
	m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.asciiMode = true
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf", Label: pk + "-flow"}
	m.runHandle = &client.RunHandle{RunID: "run-1", Status: "running"}
	m.flowLoopStatus = "blocked"
	m.flowBlockReason = "plan_approval"
	m.flowGateReason = "Plan revised after review (writer round 1)"
	m.agentRuns = []client.AgentRunSummary{{RunID: "run-1", AgentName: "main", Role: "main", Status: "completed"}}
	return m
}

// Bar shows [Revise] with its self-documenting desc on a blocked flow.
func TestBlockedBar_RendersReviseChip(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			view := stripANSI(reviseBlockedModel(pk).View())
			if !strings.Contains(view, "[Revise]") {
				t.Fatalf("%s: blocked view must render [Revise] chip:\n%s", pk, view)
			}
			if !strings.Contains(view, "feedback") {
				t.Fatalf("%s: [Revise] must advertise the feedback affordance:\n%s", pk, view)
			}
			// Existing chips untouched.
			for _, want := range []string{"[Retry]", "[Stop]"} {
				if !strings.Contains(view, want) {
					t.Fatalf("%s: blocked view must keep %s:\n%s", pk, want, view)
				}
			}
		})
	}
}

// No [Revise] when the flow is not blocked.
func TestBlockedBar_NoReviseWhenNotBlocked(t *testing.T) {
	for _, st := range []string{"running", "done"} {
		m := reviseBlockedModel("codex")
		m.flowLoopStatus = st
		if view := stripANSI(m.View()); strings.Contains(view, "[Revise]") {
			t.Fatalf("%s: must not render [Revise]:\n%s", st, view)
		}
	}
}

// Activating [Revise] (mouse or keyboard ring) clears the composer for a
// plain-text note and sends nothing — parked plain text IS the feedback, so
// no "/continue" prefix is needed. The loop stays blocked until Enter.
func TestReviseChip_PrefillsComposer(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := reviseBlockedModel(pk)
			m.inputValue = "stale draft"
			x, y, ok := findClickTarget(m, "revise")
			if !ok {
				t.Fatalf("%s: [Revise] must be clickable", pk)
			}
			m2, _ := m.dispatchMouseClick(x, y)
			am := m2.(*AppModel)
			if am.inputValue != "" {
				t.Fatalf("%s: composer = %q, want cleared for the note", pk, am.inputValue)
			}
			if am.inputCaretIndex() != 0 {
				t.Fatalf("%s: caret must sit at start", pk)
			}
			if am.flowLoopStatus != "blocked" {
				t.Fatalf("%s: loop must stay blocked after prefill", pk)
			}
		})
	}
}

// Tab cycling reaches [Revise] with a visible highlight — without drift it
// sits at ring index 2 (live-found: hardcoded 3 made Tab appear dead on
// [Stop]); with drift (Allow shown) it shifts to 3.
func TestReviseChip_TabHighlightFollowsRingOrder(t *testing.T) {
	m := reviseBlockedModel("codex")
	m.actionRingFocus = true
	m.actionRingIdx = 2
	bar := stripANSI(m.renderBlockedBar())
	if !strings.Contains(bar, " [Revise] ") {
		t.Fatalf("Revise must highlight at ring idx 2 without Allow:\n%s", bar)
	}

	drift := reviseBlockedModel("codex")
	drift.flowGateReason = "flow scope drift: wrote outside the frozen contract's declared paths: calc_test.go"
	drift.actionRingFocus = true
	drift.actionRingIdx = 3
	bar = stripANSI(drift.renderBlockedBar())
	if !strings.Contains(bar, "[Allow]") {
		t.Fatalf("drift reason must show Allow:\n%s", bar)
	}
	if !strings.Contains(bar, " [Revise] ") {
		t.Fatalf("Revise must highlight at ring idx 3 with Allow:\n%s", bar)
	}
}
