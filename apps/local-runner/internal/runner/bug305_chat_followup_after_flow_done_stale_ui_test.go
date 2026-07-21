package runner

import (
	"context"
	"testing"
	"time"
)

// BUG-305: a follow-up turn admitted onto an already-"done" flow loop (allowed
// by BUG-302) had its TurnCompleted event permanently deferred behind the
// post-turn flow gate. That gate cannot run for a "done" loop —
// runFlowGateAtEpoch's gateEpochStillValid check returns false for status
// "done" and it force-returns block — so the real TurnCompleted was never
// broadcast to live subscribers. The desktop's turn stream (which only ends on
// TurnCompleted for its turn id) therefore hung forever: the thinking row, the
// Stop button, and the history spinner all stayed stuck until the chat was
// reopened (which re-reads the correctly-settled durable state). Confirmed live
// against run-18997 / run-18371 (CP-51 A10): runner.log showed
// "[settle] gate-block disposition ... revision is stale" with NO "[gate]" line
// — the tell-tale of the early gateEpochStillValid block — and no live
// TurnCompleted reached the UI.
//
// cross-provider-parity: Case 1, provider-agnostic. The deferral
// (emitLocked), the post-turn gate (runTurn / runFlowGateAtEpoch), and the
// startTurn admission flag all take no `providerKey` and never branch on one —
// confirmed by reading each site. One representative provider (Codex, matching
// this file's sibling bug302_chat_followup_after_flow_done_test.go) is
// sufficient.
//
// additive-tests-only: this file only adds new tests; no existing test file is
// modified.
func TestChatFollowUpAfterFlowDoneBroadcastsLiveCompletion(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				// Plain prose reply, no submit_review_outcome — a genuine follow-up.
				b.Emit(ProviderEvent{Type: EventTurnCompleted, ProviderTurnID: req.ProviderTurnID, FinalMessage: "I fixed the Add offset bug."})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	stepID := "chat-" + parent.RunID
	if _, apiErr := svc.startTurn(parent.RunID, TurnInput{StepID: stepID, Prompt: "fix bug 1+1 != 2"}, "", ""); apiErr != nil {
		t.Fatalf("first startTurn: %s", apiErr.msg)
	}
	waitLoop(t, "first turn completes", time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		rs := svc.runs[parent.RunID]
		return !rs.turnInFlight && !rs.pendingFlowGateSettle && rs.postTurnGateCancel == nil
	})

	// Exact run-18997 precondition: flow-engine-driven Review Loop, loop done.
	// A non-empty workspaceCwd is required to reproduce the bug: runFlowGateAtEpoch
	// returns pass immediately when cwd == "", so only a real cwd reaches the
	// gateEpochStillValid check that force-blocks a "done" loop (that check runs
	// before any git operation, so the dir contents are irrelevant here).
	cwd := t.TempDir()
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.workspaceCwd = cwd
	if rs.runKind == "" {
		rs.runKind = "chat"
	}
	lastSeqBeforeFollowUp := rs.seq
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "done", Round: 1, Cap: 3, Mode: "explicit"})

	// Subscribe like the desktop's live turn stream BEFORE sending the follow-up,
	// so we observe exactly what a connected client would receive.
	subID, ch, _, ok := svc.subscribe(parent.RunID, lastSeqBeforeFollowUp)
	if !ok {
		t.Fatal("subscribe failed")
	}
	defer svc.unsubscribe(parent.RunID, subID)

	followUpTurnID, apiErr := svc.startTurn(parent.RunID, TurnInput{StepID: stepID, Prompt: "hello bug này fix gì vậy"}, "", "")
	if apiErr != nil {
		t.Fatalf("follow-up startTurn after loop done was rejected: %s (code=%s)", apiErr.msg, apiErr.code)
	}

	// Core assertion: the follow-up's TurnCompleted MUST be broadcast live to the
	// subscriber. Without the fix it is deferred behind the gate and never
	// broadcast, so this drain times out (the exact desktop-hang symptom).
	deadline := time.After(3 * time.Second)
	gotLiveCompletion := false
	for !gotLiveCompletion {
		select {
		case ev, alive := <-ch:
			if !alive {
				t.Fatal("event channel closed before the follow-up TurnCompleted was broadcast")
			}
			if ev.Type == EventTurnCompleted && ev.ProviderTurnID == followUpTurnID {
				gotLiveCompletion = true
			}
		case <-deadline:
			t.Fatal("follow-up TurnCompleted was never broadcast live — desktop turn stream would hang (BUG-305 not fixed)")
		}
	}

	// Durable/live state must be a clean terminal completion, not a lingering
	// deferred-gate state.
	waitLoop(t, "follow-up turn settles", time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		r := svc.runs[parent.RunID]
		return !r.turnInFlight && !r.pendingFlowGateSettle
	})
	svc.mu.Lock()
	finalStatus := svc.runs[parent.RunID].status
	svc.mu.Unlock()
	if finalStatus != RunStatusCompleted {
		t.Fatalf("run status after post-done follow-up = %q, want %q", finalStatus, RunStatusCompleted)
	}
	// The loop must remain terminal "done" (a plain follow-up must not revive it).
	if snap := svc.agentOrchestrator.loopStateFor(parent.RunID); snap.Status != "done" {
		t.Fatalf("loop status after plain follow-up = %q, want unchanged %q", snap.Status, "done")
	}
}

// Guard the discriminator directly (non-racy): the per-turn flag that BUG-305's
// carve-out keys off must be false when the loop is still ACTIVE at admission
// and true only when it is already "done". This proves the carve-out cannot leak
// into a normal flow turn (whose loop is running/synthesizing at admission), so
// those turns keep their exact deferral + post-turn-gate behavior. The flag is
// set once at startTurn admission and stable until the next startTurn, so it can
// be read deterministically after each call.
func TestTurnStartedAfterLoopDoneFlagReflectsAdmissionLoopState(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, ProviderTurnID: req.ProviderTurnID, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	stepID := "chat-" + parent.RunID
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	if rs.runKind == "" {
		rs.runKind = "chat"
	}
	svc.mu.Unlock()

	settle := func(label string) {
		waitLoop(t, label, 2*time.Second, func() bool {
			svc.mu.Lock()
			defer svc.mu.Unlock()
			r := svc.runs[parent.RunID]
			return !r.turnInFlight
		})
	}
	readFlag := func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return svc.runs[parent.RunID].turnStartedAfterLoopDone
	}

	// Loop ACTIVE at admission → flag must be false (normal flow turn: still gated).
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Round: 0, Cap: 3, Mode: "explicit"})
	if _, apiErr := svc.startTurn(parent.RunID, TurnInput{StepID: stepID, Prompt: "fix bug 1+1 != 2"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn (loop running): %s", apiErr.msg)
	}
	if readFlag() {
		t.Fatal("turnStartedAfterLoopDone must be FALSE when the loop is running at admission (carve-out leaked into a normal flow turn)")
	}
	settle("running-turn settles")

	// Loop DONE at admission → flag must be true (plain follow-up: carve-out active).
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "done", Round: 1, Cap: 3, Mode: "explicit"})
	if _, apiErr := svc.startTurn(parent.RunID, TurnInput{StepID: stepID, Prompt: "hello follow-up"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn (loop done): %s", apiErr.msg)
	}
	if !readFlag() {
		t.Fatal("turnStartedAfterLoopDone must be TRUE when the loop is already done at admission")
	}
	settle("done-turn settles")
}
