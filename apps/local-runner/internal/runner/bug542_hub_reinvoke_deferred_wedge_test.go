package runner

import (
	"context"
	"testing"
	"time"
)

// BUG-542 live repro (run-18354, tournament on :4322):
// candidate-a completed while hub turn-18356 was in flight waiting on exec
// approval appr-18519. The cohort join reached maybeAutoReinvokeHubWithNote
// with cohort_note_len=0 (the child result rode the provider tool channel, not
// pendingAgentContext) — so pendingHubReinvoke was NEVER armed (it is gated on
// len(pendingAgentContext) > 0, a contract TestAutoReinvokeHubNoPendingContextNoDefer
// pins for the spurious-duplicate case). The hub turn later finished with a
// prose answer, emitted no deterministic flow signal, and the loop sat at
// waiting_review forever: candidate-b never spawned, no hub_stalled surfaced.
//
// The arm site cannot distinguish "join consumed" from "join carried no note",
// so the fix is drain-side: when the hub goes idle and the loop still sits at
// waiting_review with nothing queued to drive it, notifyTurnIdle owes the hub
// exactly the review turn the join meant to schedule — bounded so a prose-only
// hub surfaces hub_stalled instead of looping.

// D1: idle parent at waiting_review with no queued intent owes the hub a
// review reinvoke — for BOTH loop flavors. The live wedge (run-18354) had
// autoOrchestrate=false but flowEngineDriven=true; a gate on the legacy flag
// alone would reproduce the exact wedge.
func TestBug542_IdleWaitingReviewOwesHubReinvoke(t *testing.T) {
	for _, tc := range []struct {
		name          string
		autoOrchestr  bool
		flowEngineDrv bool
		wantDue       bool
	}{
		{"explicit flow-engine loop (live run-18354 shape)", false, true, true},
		{"legacy auto-orchestrate loop", true, false, true},
		{"plain chat with no loop driver", false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := bug289Service(t)
			parentID := "run-542p"
			svc.mu.Lock()
			rs := &interactiveRun{
				id:               parentID,
				flowEngineDriven: tc.flowEngineDrv,
				autoOrchestrate:  tc.autoOrchestr,
				status:           RunStatusRunning,
				// turnInFlight=false, pendingAgentContext empty — the live wedge shape.
				subs: map[int64]chan ProviderEvent{},
			}
			svc.runs[parentID] = rs
			svc.mu.Unlock()
			svc.agentOrchestrator.mutateLoop(parentID, func(st AgentLoopState) AgentLoopState {
				st.Status = "waiting_review"
				st.Cap = 3
				return st
			})

			svc.mu.Lock()
			due := svc.waitingReviewDrainDueLocked(rs)
			n := svc.runs[parentID].waitingReviewReinvokes
			svc.mu.Unlock()
			if due != tc.wantDue {
				t.Fatalf("waiting_review + idle hub due=%v, want %v", due, tc.wantDue)
			}
			if tc.wantDue && n != 1 {
				t.Fatalf("waitingReviewReinvokes = %d, want 1 after first due", n)
			}
		})
	}
}

// D1b: the drain is bounded — after the cap, waiting_review stops firing
// reinvokes so a prose-only hub surfaces hub_stalled instead of looping.
func TestBug542_WaitingReviewDrainIsBounded(t *testing.T) {
	svc := bug289Service(t)
	parentID := "run-542b"

	svc.mu.Lock()
	rs := &interactiveRun{
		id:               parentID,
		flowEngineDriven: true,
		autoOrchestrate:  true,
		status:           RunStatusRunning,
		subs:             map[int64]chan ProviderEvent{},
	}
	svc.runs[parentID] = rs
	svc.mu.Unlock()
	svc.agentOrchestrator.mutateLoop(parentID, func(st AgentLoopState) AgentLoopState {
		st.Status = "waiting_review"
		st.Cap = 3
		return st
	})

	fired := 0
	for i := 0; i < waitingReviewReinvokeDrainCap+2; i++ {
		svc.mu.Lock()
		if svc.waitingReviewDrainDueLocked(svc.runs[parentID]) {
			fired++
		}
		svc.mu.Unlock()
	}
	if fired != waitingReviewReinvokeDrainCap {
		t.Fatalf("waiting_review drain fired %d times, want cap %d", fired, waitingReviewReinvokeDrainCap)
	}
}

// D1c: the bound resets once the loop leaves waiting_review — a new join earns
// a fresh budget.
func TestBug542_WaitingReviewBoundResetsOnAdvance(t *testing.T) {
	svc := bug289Service(t)
	parentID := "run-542c"

	svc.mu.Lock()
	rs := &interactiveRun{
		id:               parentID,
		flowEngineDriven: true,
		autoOrchestrate:  true,
		status:           RunStatusRunning,
		subs:             map[int64]chan ProviderEvent{},
	}
	svc.runs[parentID] = rs
	svc.mu.Unlock()
	svc.agentOrchestrator.mutateLoop(parentID, func(st AgentLoopState) AgentLoopState {
		st.Status = "waiting_review"
		st.Cap = 3
		return st
	})
	for i := 0; i < waitingReviewReinvokeDrainCap; i++ {
		svc.mu.Lock()
		svc.waitingReviewDrainDueLocked(svc.runs[parentID])
		svc.mu.Unlock()
	}
	svc.agentOrchestrator.mutateLoop(parentID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		return st
	})
	svc.mu.Lock()
	svc.waitingReviewDrainDueLocked(svc.runs[parentID]) // non-review status resets
	svc.mu.Unlock()
	svc.agentOrchestrator.mutateLoop(parentID, func(st AgentLoopState) AgentLoopState {
		st.Status = "waiting_review"
		return st
	})
	svc.mu.Lock()
	due := svc.waitingReviewDrainDueLocked(svc.runs[parentID])
	svc.mu.Unlock()
	if !due {
		t.Fatal("drain budget must reset after the loop advanced out of waiting_review")
	}
}

// D1d: end-to-end through the real drain — notifyTurnIdle on an idle parent at
// waiting_review must actually dispatch a hub turn (not just set a flag).
func TestBug542_NotifyTurnIdleDispatchesWaitingReviewReinvoke(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "done"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	handle, _ := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	runID := handle.RunID

	svc.mu.Lock()
	svc.runs[runID].autoOrchestrate = true
	svc.mu.Unlock()
	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Status = "waiting_review"
		st.Cap = 3
		return st
	})

	svc.notifyTurnIdle(runID)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		fired := svc.runs[runID].reinvokeInFlight || svc.runs[runID].turnInFlight || svc.runs[runID].turnCount > 0
		svc.mu.Unlock()
		if fired {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("notifyTurnIdle on waiting_review idle hub dispatched no reinvoke — wedge persists")
}

// D2: the hub stall watchdog must keep watching across a live approval card.
// On run-18354 the watchdog fired once while the run held a live exec approval
// (waiting_approval + pendingApprovalID), returned early WITHOUT re-arming, and
// was never scheduled again — so when the approved turn later ended without a
// flow signal, no hub_stalled card ever surfaced. Silent 45+ min wedge.
func TestBug542_WatchdogRearmsWhileLiveCardPending(t *testing.T) {
	svc := bug289Service(t)
	runID := "run-542w"

	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:                runID,
		flowEngineDriven:  true,
		status:            RunStatusWaitingApproval,
		pendingApprovalID: "appr-542",
		hubLastProgressAt: time.Now().UTC().Add(-10 * time.Minute),
		stallTimeout:      time.Second,
		subs:              map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Status = "waiting_review"
		return st
	})

	if svc.checkAndBlockStalledHub(runID) {
		t.Fatal("a live approval card is actionable — hub_stalled must not fire")
	}
	hubStallTimerMu.Lock()
	_, rearmed := hubStallTimers[hubStallTimerKey(svc, runID)]
	hubStallTimerMu.Unlock()
	if !rearmed {
		t.Fatal("watchdog disarmed permanently while a live card was pending — " +
			"nothing re-arms after the card resolves (run-18354 silent wedge)")
	}
}

// D2b: same defect on the waiting_question path.
func TestBug542_WatchdogRearmsWhileLiveQuestionPending(t *testing.T) {
	svc := bug289Service(t)
	runID := "run-542q"

	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:                runID,
		flowEngineDriven:  true,
		status:            RunStatusWaitingQuestion,
		pendingQuestionID: "q-542",
		hubLastProgressAt: time.Now().UTC().Add(-10 * time.Minute),
		stallTimeout:      time.Second,
		subs:              map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Status = "waiting_review"
		return st
	})

	if svc.checkAndBlockStalledHub(runID) {
		t.Fatal("a live question card is actionable — hub_stalled must not fire")
	}
	hubStallTimerMu.Lock()
	_, rearmed := hubStallTimers[hubStallTimerKey(svc, runID)]
	hubStallTimerMu.Unlock()
	if !rearmed {
		t.Fatal("watchdog disarmed permanently while a live question card was pending")
	}
}

// D2c: end-to-end — after the card resolves and the run goes idle still
// waiting_review, the re-armed watchdog fires hub_stalled (bounded, surfaced).
func TestBug542_WatchdogSurfacesAfterCardResolves(t *testing.T) {
	svc := bug289Service(t)
	runID := "run-542z"

	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:                runID,
		flowEngineDriven:  true,
		status:            RunStatusWaitingApproval,
		pendingApprovalID: "appr-542z",
		hubLastProgressAt: time.Now().UTC(),
		stallTimeout:      50 * time.Millisecond,
		subs:              map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Status = "waiting_review"
		st.Cap = 3
		return st
	})
	// Simulate the approval resolving: card gone, run back to running, loop
	// still waiting_review, and the deferred drain budget already spent (the
	// prose hub burned it) — the watchdog must surface hub_stalled, not wedge.
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.pendingApprovalID = ""
	rs.status = RunStatusRunning
	rs.waitingReviewReinvokes = waitingReviewReinvokeDrainCap
	svc.mu.Unlock()

	svc.checkAndBlockStalledHub(runID) // re-arms (busy or not, it schedules)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		st := svc.agentOrchestrator.loopStateFor(runID)
		if st.Status == "blocked" && st.BlockReason == "hub_stalled" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	st := svc.agentOrchestrator.loopStateFor(runID)
	t.Fatalf("loop never surfaced hub_stalled after card resolution — status=%q reason=%q", st.Status, st.BlockReason)
}
