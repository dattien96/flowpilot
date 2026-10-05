package runner

import (
	"context"
	"testing"
	"time"
)

// ageStepStartedAt back-dates a step row's RUNNING stamp — the
// setFlowStepStatus helper always writes now(), so aged-dispatch shapes need
// a second transition patch.
func ageStepStartedAt(t *testing.T, svc *InteractiveService, runID, nodeID string, age time.Duration) {
	t.Helper()
	past := time.Now().UTC().Add(-age).Format(time.RFC3339Nano)
	if err := svc.workflowStore.ApplyStepTransition(context.Background(), runID, WorkflowStepTransition{
		StepID: nodeID,
		Patch:  WorkflowStepPatch{StartedAt: &past},
	}); err != nil {
		t.Fatalf("age %s: %v", nodeID, err)
	}
}

// live-039 (run-225691 round-4): the debate mount swapped the overlay in and
// stamped debate_trigger RUNNING, but the trigger's hub turn never dispatched
// owner legs — the mount goroutine exited, the flag cleared, and the wedge
// shape matched no heal path: ownersStarved required stTrig != RUNNING
// (BUG-624 mount-window guard), maybeResolveZombieVibeDebate required owners
// DONE/RUNNING, and the hub_stalled watchdog shields a mounted debate. A
// trigger that has been RUNNING far past any settle window with zero owner
// children and no in-flight hub work is a dead dispatch — the settle ladder
// must re-drive it.
func TestLive039_DeadDebateTriggerReinvokesHub(t *testing.T) {
	svc, runID := seedVibeDebateStarvedRun(t)
	svc.setFlowStepStatus(context.Background(), runID, "debate_trigger", StepStatusRunning)
	ageStepStartedAt(t, svc, runID, "debate_trigger", 3*time.Minute)
	if !svc.maybeSettleVibeOwnerDebate(runID) {
		t.Fatal("dead debate_trigger dispatch must re-drive the debate, not wedge")
	}
	svc.mu.Lock()
	armed := svc.runs[runID].reinvokeInFlight || svc.runs[runID].pendingHubReinvoke
	svc.mu.Unlock()
	if !armed {
		t.Fatal("dead trigger must re-drive the hub (reinvoke or armed pending)")
	}
}

// Same shape while the trigger's hub turn is genuinely in flight is live
// work, not a wedge — the BUG-624/CA-1088 mount-window contract holds.
func TestLive039_TriggerTurnInFlightIsNotWedged(t *testing.T) {
	svc, runID := seedVibeDebateStarvedRun(t)
	svc.setFlowStepStatus(context.Background(), runID, "debate_trigger", StepStatusRunning)
	ageStepStartedAt(t, svc, runID, "debate_trigger", 3*time.Minute)
	svc.mu.Lock()
	svc.runs[runID].turnInFlight = true
	svc.mu.Unlock()
	if svc.maybeSettleVibeOwnerDebate(runID) {
		t.Fatal("an in-flight trigger turn must not be re-driven")
	}
}

// A freshly stamped RUNNING trigger is still inside the legitimate dispatch
// window (mount flag cleared, spawn/settle mid-flight) — only an aged stamp
// with no live work is a dead dispatch.
func TestLive039_FreshTriggerRunningIsNotWedged(t *testing.T) {
	svc, runID := seedVibeDebateStarvedRun(t)
	svc.setFlowStepStatus(context.Background(), runID, "debate_trigger", StepStatusRunning)
	if svc.maybeSettleVibeOwnerDebate(runID) {
		t.Fatal("fresh RUNNING trigger is in-flight dispatch, not a wedge")
	}
}

// The wedge re-drive is bounded by the same retry budget — on exhaustion the
// settle parks visibly instead of reinvoking forever.
func TestLive039_DeadDebateTriggerCapParks(t *testing.T) {
	svc, runID := seedVibeDebateStarvedRun(t)
	svc.setFlowStepStatus(context.Background(), runID, "debate_trigger", StepStatusRunning)
	ageStepStartedAt(t, svc, runID, "debate_trigger", 3*time.Minute)
	svc.mu.Lock()
	svc.runs[runID].vibeOwnerFailRetries = maxVibeOwnerFailRetries
	svc.mu.Unlock()
	if !svc.maybeSettleVibeOwnerDebate(runID) {
		t.Fatal("wedge at cap must still act (park), not silently wedge")
	}
	st := svc.agentOrchestrator.loopStateFor(runID)
	// Cap resolution is blocked/cap — the tournament rescue seam may lift
	// Status to tournament_escalation first; the contract is the cap reason.
	if st.BlockReason != "cap" {
		t.Fatalf("wedge cap must surface blocked/cap, got %+v", st)
	}
}
