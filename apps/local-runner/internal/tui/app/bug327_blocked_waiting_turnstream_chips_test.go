package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// BUG-327: When a flow is parked awaiting user approval (escalate/cap),
// a stale or open turnStream must NOT prevent flowLoopBlocked() from being true,
// must NOT keep the Thinking timer spinning (workIsLive must be false),
// and must render the [Continue] [Stop] action chips.
func TestBUG327_BlockedWaitingTurnStreamRendersChipsAndStopsThinking(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.width = 100
			m.height = 30
			m.asciiMode = true
			m.mode = ModeFlow
			m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf", Label: pk + "-flow"}
			m.runHandle = &client.RunHandle{RunID: "run-218125", Status: "running"}
			m.flowLoopStatus = "blocked"
			m.flowBlockReason = "escalate"
			m.agentRuns = []client.AgentRunSummary{
				{RunID: "run-218125", AgentName: "main", Role: "main", Status: "running"},
				{RunID: "run-218126", AgentName: "implement", Role: "coder", Status: "waiting_user_approval"},
			}
			// Simulate turnStream remaining open during park
			m.turnStream = &turnStreamState{}

			// 1. flowLoopBlocked must be true
			if !m.flowLoopBlocked() {
				t.Fatalf("%s: flowLoopBlocked must be true even with turnStream open during park", pk)
			}

			// 2. workIsLive must be false (Thinking timer off)
			if m.workIsLive() {
				t.Fatalf("%s: workIsLive must be false when parked (Thinking must be off)", pk)
			}

			// 3. Chat rows must render [Continue] and [Stop] chips
			rows := m.chatRows()
			foundContinue, foundStop := false, false
			for _, r := range rows {
				text := stripANSI(r.Text)
				if strings.Contains(text, "[Continue]") {
					foundContinue = true
				}
				if strings.Contains(text, "[Stop]") {
					foundStop = true
				}
			}
			if !foundContinue || !foundStop {
				t.Fatalf("%s: expected [Continue] and [Stop] chips in chat timeline", pk)
			}

			// 4. Click targeting must resolve continue and stop
			xCont, yCont, okCont := findClickTarget(m, "continue")
			if !okCont {
				t.Fatalf("%s: continue chip must be clickable", pk)
			}
			if target := m.clickTargetAt(xCont, yCont); target != "continue" {
				t.Fatalf("%s: expected target continue, got %q", pk, target)
			}

			xStop, yStop, okStop := findClickTarget(m, "stop")
			if !okStop {
				t.Fatalf("%s: stop chip must be clickable", pk)
			}
			if target := m.clickTargetAt(xStop, yStop); target != "stop" {
				t.Fatalf("%s: expected target stop, got %q", pk, target)
			}
		})
	}
}

func TestBUG327_FocusedParkedChildDoesNotHideChipsOrTurnThinkingOn(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.width = 100
			m.height = 30
			m.asciiMode = true
			m.mode = ModeFlow
			m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf", Label: pk + "-flow"}
			m.runHandle = &client.RunHandle{RunID: "run-218125", Status: "running"}
			m.flowLoopStatus = "blocked"
			m.flowBlockReason = "escalate"
			m.agentRuns = []client.AgentRunSummary{
				{RunID: "run-218125", AgentName: "main", Role: "main", Status: "running"},
				{RunID: "run-218126", AgentName: "implement", Role: "coder", Status: "waiting_user_approval"},
			}
			// User opens F2 and focuses the parked implement child
			m.focusRunID = "run-218126"
			m.focusStream = &orchStreamState{}

			if m.focusedChildLive() {
				t.Fatalf("%s: focusedChildLive must be false when focused child is parked (waiting_user_approval)", pk)
			}
			if !m.flowLoopBlocked() {
				t.Fatalf("%s: flowLoopBlocked must be true when focusing a parked child", pk)
			}
			if m.workIsLive() {
				t.Fatalf("%s: workIsLive must be false when focusing a parked child (Thinking off)", pk)
			}
		})
	}
}
