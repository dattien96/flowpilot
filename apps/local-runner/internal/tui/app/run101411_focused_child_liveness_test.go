package app

import (
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-539 — a focused sub-agent must only count as live work while the child run
// is actually working. cmdFocusAgent opens a listen-only StreamLive even for
// completed children (that is how the transcript stays readable), so a bare
// focusStream must NOT keep the Thinking spinner + elapsed clock running —
// run-101411 grok-coder showed the spinner animating forever on a done child.
// Provider-agnostic logic — parameterized over claude/codex/grok anyway.

func run101411Model(pk string) *AppModel {
	m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.connStatus = ConnIdle
	m.mode = ModeFlow
	m.runHandle = &client.RunHandle{RunID: "run-parent", Status: "completed"}
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-parent", AgentName: "main", Role: "main", Status: "completed"},
		{RunID: "run-child", AgentName: "coder", Label: "grok-coder", Status: "completed"},
	}
	return m
}

// focusChild attaches the same listen-only stream cmdFocusAgent uses, so the
// transcript view is active but the child is terminal.
func (m *AppModel) focusCompletedChild() {
	m.focusRunID = "run-child"
	m.focusStream = &orchStreamState{evCh: nil}
}

func TestWorkIsLive_FocusedCompletedChildIsNotLive(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := run101411Model(pk)
			m.focusCompletedChild()
			if m.workIsLive() {
				t.Fatalf("%s: completed focused child must not count as live work", pk)
			}
		})
	}
}

func TestStatusReadyLabel_FocusedCompletedChildStatic(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := run101411Model(pk)
			m.focusCompletedChild()
			m.statusMsg = "flow"
			if got := m.statusReadyLabel(); got != "flow" {
				t.Fatalf("%s: no spinner on completed child: %q", pk, got)
			}
		})
	}
}

func TestWorkIsLive_FocusedRunningChildIsLive(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := run101411Model(pk)
			m.runHandle = &client.RunHandle{RunID: "run-parent", Status: "running"}
			m.agentRuns = []client.AgentRunSummary{
				{RunID: "run-parent", AgentName: "main", Role: "main", Status: "running"},
				{RunID: "run-child", AgentName: "coder", Label: "grok-coder", Status: "running"},
			}
			m.focusRunID = "run-child"
			m.focusStream = &orchStreamState{evCh: nil}
			if !m.workIsLive() {
				t.Fatalf("%s: running focused child must stay live work", pk)
			}
		})
	}
}

func TestShouldPollStepsRuntime_FocusedCompletedChildIdle(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := run101411Model(pk)
			m.focusCompletedChild()
			if m.shouldPollStepsRuntime() {
				t.Fatalf("%s: completed focused child must not keep F2 step poll running", pk)
			}
		})
	}
}

func TestFlowLoopBlocked_CompletedChildFocusStaysBlocked(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := run101411Model(pk)
			m.flowLoopStatus = "blocked"
			m.focusCompletedChild()
			if !m.flowLoopBlocked() {
				t.Fatalf("%s: a completed child transcript must not unblock a parked loop", pk)
			}
		})
	}
}

func TestSettleFlowIfDone_CompletedChildFocusStillSettles(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := run101411Model(pk)
			m.flowLoopStatus = "done"
			m.connStatus = ConnRunning
			m.statusMsg = "flow running…"
			m.focusCompletedChild()
			m.settleFlowIfDone()
			if m.connStatus != ConnIdle || m.statusMsg != "done" {
				t.Fatalf("%s: done flow must settle to ConnIdle/done even with a completed child open", pk)
			}
		})
	}
}

func TestWorkIsLive_FocusedUnknownChildFallsBackToFlow(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := run101411Model(pk)
			// Child not in the hydrate snapshot: liveness falls back to the
			// parent flow state, so a genuinely running flow stays live.
			m.agentRuns = []client.AgentRunSummary{
				{RunID: "run-parent", AgentName: "main", Role: "main", Status: "running"},
			}
			m.focusRunID = "run-child"
			m.focusStream = &orchStreamState{evCh: nil}
			if !m.workIsLive() {
				t.Fatalf("%s: unknown focused child under running parent must stay live", pk)
			}
			m.connStatus = ConnIdle
			m.runHandle = &client.RunHandle{RunID: "run-parent", Status: "completed"}
			m.agentRuns = []client.AgentRunSummary{
				{RunID: "run-parent", AgentName: "main", Role: "main", Status: "completed"},
			}
			if m.workIsLive() {
				t.Fatalf("%s: unknown focused child under completed parent must be idle", pk)
			}
		})
	}
}
