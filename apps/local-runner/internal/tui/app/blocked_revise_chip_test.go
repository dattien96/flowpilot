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
			if !strings.Contains(view, "/continue") {
				t.Fatalf("%s: [Revise] must advertise the /continue affordance:\n%s", pk, view)
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

// Activating [Revise] (mouse or keyboard ring) prefills "/continue " and
// sends nothing — the loop stays blocked until the user hits Enter.
func TestReviseChip_PrefillsComposer(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := reviseBlockedModel(pk)
			x, y, ok := findClickTarget(m, "revise")
			if !ok {
				t.Fatalf("%s: [Revise] must be clickable", pk)
			}
			m2, cmd := m.dispatchMouseClick(x, y)
			am := m2.(*AppModel)
			if cmd != nil {
				t.Fatalf("%s: [Revise] must not issue a cmd (prefill only)", pk)
			}
			if am.inputValue != "/continue " {
				t.Fatalf("%s: composer = %q, want prefilled %q", pk, am.inputValue, "/continue ")
			}
			if am.inputCaretIndex() != len([]rune("/continue ")) {
				t.Fatalf("%s: caret must sit at end of prefill", pk)
			}
			if am.flowLoopStatus != "blocked" {
				t.Fatalf("%s: loop must stay blocked after prefill", pk)
			}
		})
	}
}

// Keyboard ring exposes revise without disturbing Retry/Stop/Allow order.
func TestReviseChip_ActionRingOrder(t *testing.T) {
	m := reviseBlockedModel("codex")
	var targets []string
	for _, it := range m.actionRingItems() {
		targets = append(targets, it.target)
	}
	want := []string{"retry", "stop", "revise"}
	if len(targets) != len(want) {
		t.Fatalf("ring = %v, want %v (appended last, indices stable)", targets, want)
	}
	for i := range want {
		if targets[i] != want[i] {
			t.Fatalf("ring = %v, want %v", targets, want)
		}
	}
}
