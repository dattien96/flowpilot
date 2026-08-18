package app

import (
	"testing"

	"flowpilot-runner/internal/tui/client"
)

// CA-544 (run-107774) — after a flow loop reaches "done", a follow-up chat turn
// is plain hub chat, not a flow restart. The TUI must not flash the chrome to
// "done" in the send→stream window (a steps poll landing before
// turnStreamOpenedMsg used to settle the finished loop), must not relabel the
// turn as "flow running…", and must settle cleanly on completion.

// flowModelForDoneFollowUp builds a finished-flow model in the exact state
// processInput leaves before cmdSendTurn opens the SSE stream: ConnRunning +
// "thinking…" and no turnStream yet.
func flowModelForDoneFollowUp() *AppModel {
	m := flowModelForSettleTest()
	m.flowLoopStatus = "done"
	m.flowSteps = []client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "grok-context", Status: "DONE"},
		{StepID: "s2", NodeID: "grok-coder", Status: "DONE"},
		{StepID: "s3", NodeID: "grok-review", Status: "DONE"},
		{StepID: "s4", NodeID: "grok-synthesis", Status: "DONE"},
	}
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-189839", AgentName: "main", Role: "main", Status: "completed"},
	}
	m.connStatus = ConnRunning
	m.statusMsg = "thinking…"
	m.turnStream = nil
	m.turnSendPending = true
	return m
}

// TestPostDoneFollowUp_StepsPollInSendGapDoesNotSettle: a steps poll landing
// between processInput (ConnRunning/"thinking…") and turnStreamOpenedMsg must
// NOT settle the done loop — the follow-up turn is still in flight.
func TestPostDoneFollowUp_StepsPollInSendGapDoesNotSettle(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := flowModelForDoneFollowUp()

			m2, _ := m.Update(StepsRuntimeMsg{
				RunID: "run-189839",
				Steps: m.flowSteps,
			})
			am := m2.(*AppModel)
			if am.connStatus != ConnRunning {
				t.Fatalf("%s: send-gap steps poll must not settle (connStatus=%v)", pk, am.connStatus)
			}
			if am.statusMsg != "thinking…" {
				t.Fatalf("%s: statusMsg=%q want thinking…", pk, am.statusMsg)
			}
			if am.runHandle.Status != "running" {
				t.Fatalf("%s: handle must stay running, got %q", pk, am.runHandle.Status)
			}
			if !am.turnIsActive() {
				t.Fatalf("%s: in-flight follow-up must keep [stop] armed", pk)
			}
		})
	}
}

// TestPostDoneFollowUp_StreamOpenedStaysChatChrome: opening the follow-up turn
// stream after loop done must keep the chat label, not claim "flow running…".
func TestPostDoneFollowUp_StreamOpenedStaysChatChrome(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := flowModelForDoneFollowUp()
			m.provider = pk

			m2, _ := m.Update(turnStreamOpenedMsg{
				EvCh:  make(chan client.ProviderEvent),
				ErrCh: make(chan error),
			})
			am := m2.(*AppModel)
			if am.connStatus != ConnRunning {
				t.Fatalf("%s: connStatus=%v want ConnRunning", pk, am.connStatus)
			}
			if am.statusMsg != "thinking…" {
				t.Fatalf("%s: stream-opened statusMsg=%q want thinking… (not flow running…)", pk, am.statusMsg)
			}
		})
	}
}

// TestPostDoneFollowUp_TurnStartedUsesChatLabel: turn_started on a done loop
// must label the follow-up as plain chat ("turn running…"), not "flow running…".
func TestPostDoneFollowUp_TurnStartedUsesChatLabel(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := flowModelForDoneFollowUp()
			m.provider = pk

			m2, _ := m.Update(EventMsg{Ev: client.ProviderEvent{Type: "turn_started", Prompt: "chat tiếp"}})
			am := m2.(*AppModel)
			if am.statusMsg != "turn running…" {
				t.Fatalf("%s: turn_started statusMsg=%q want turn running…", pk, am.statusMsg)
			}
		})
	}
}

// TestPostDoneFollowUp_MessageDeltaStreams: a done-loop follow-up streams its
// reply with "streaming…" like plain chat instead of staying muted.
func TestPostDoneFollowUp_MessageDeltaStreams(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := flowModelForDoneFollowUp()
			m.provider = pk

			m2, _ := m.Update(EventMsg{Ev: client.ProviderEvent{Type: "message_delta", Text: "ok"}})
			am := m2.(*AppModel)
			if am.statusMsg != "streaming…" {
				t.Fatalf("%s: message_delta statusMsg=%q want streaming…", pk, am.statusMsg)
			}
		})
	}
}

// TestPostDoneFollowUp_TurnCompletedSettlesDone: a completed follow-up turn on a
// done loop settles to ConnIdle "done" (with TurnDoneMsg) instead of re-entering
// "flow running…", even when an orchestration stream is attached.
func TestPostDoneFollowUp_TurnCompletedSettlesDone(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := flowModelForDoneFollowUp()
			m.provider = pk
			m.orchStream = &orchStreamState{}

			m2, cmd := m.Update(EventMsg{Ev: client.ProviderEvent{Type: "turn_completed", FinalMessage: "reply"}})
			am := m2.(*AppModel)
			if am.connStatus != ConnIdle {
				t.Fatalf("%s: connStatus=%v want ConnIdle", pk, am.connStatus)
			}
			if am.statusMsg != "done" {
				t.Fatalf("%s: statusMsg=%q want done", pk, am.statusMsg)
			}
			if cmd == nil {
				t.Fatalf("%s: turn_completed on done loop must return TurnDoneMsg", pk)
			}
			if _, ok := cmd().(TurnDoneMsg); !ok {
				t.Fatalf("%s: cmd=%T want TurnDoneMsg", pk, cmd())
			}
		})
	}
}

// TestFlowNotDone_TurnCompletedKeepsFlowChrome: near-miss — an in-progress loop
// with an orchestration stream still parks on "flow running…" after a turn
// completes (CA-515 behavior preserved).
func TestFlowNotDone_TurnCompletedKeepsFlowChrome(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := flowModelForDoneFollowUp()
			m.provider = pk
			m.flowLoopStatus = "running"
			m.flowSteps = []client.WorkflowStepRuntime{
				{StepID: "s1", NodeID: "grok-context", Status: "DONE"},
				{StepID: "s2", NodeID: "grok-synthesis", Status: "RUNNING"},
			}
			m.flowStepsActive = "grok-synthesis"
			m.orchStream = &orchStreamState{}

			m2, _ := m.Update(EventMsg{Ev: client.ProviderEvent{Type: "turn_completed", FinalMessage: "step done"}})
			am := m2.(*AppModel)
			if am.connStatus != ConnWaiting {
				t.Fatalf("%s: live flow must park ConnWaiting, got %v", pk, am.connStatus)
			}
			if am.statusMsg != "flow running…" {
				t.Fatalf("%s: live flow statusMsg=%q want flow running…", pk, am.statusMsg)
			}
		})
	}
}

// TestPostDoneFollowUp_SettleStillWorksWhenIdle: near-miss — the ConnRunning
// guard must not block the real finished-flow settle when chrome is idle
// (CA-515). A done loop at ConnWaiting still settles to done.
func TestPostDoneFollowUp_SettleStillWorksWhenIdle(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := flowModelForSettleTest()
			m.provider = pk
			m.flowLoopStatus = "done"
			m.flowSteps = []client.WorkflowStepRuntime{
				{StepID: "s1", NodeID: "grok-context", Status: "DONE"},
				{StepID: "s2", NodeID: "grok-synthesis", Status: "DONE"},
			}
			m.agentRuns = []client.AgentRunSummary{
				{RunID: "run-189839", AgentName: "main", Role: "main", Status: "completed"},
			}
			m.connStatus = ConnWaiting
			m.statusMsg = "flow running…"

			m2, _ := m.Update(StepsRuntimeMsg{RunID: "run-189839", Steps: m.flowSteps})
			am := m2.(*AppModel)
			if am.connStatus != ConnIdle || am.statusMsg != "done" {
				t.Fatalf("%s: idle done loop must still settle (status=%v %q)", pk, am.connStatus, am.statusMsg)
			}
		})
	}
}
