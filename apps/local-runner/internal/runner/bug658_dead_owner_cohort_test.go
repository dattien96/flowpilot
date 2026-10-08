package runner

import (
	"context"
	"testing"
	"time"

	"flowpilot-runner/internal/workingmode"
)

// BUG-658 (live run-523131): both debate owner legs failed on
// provider_limit; debate_synthesis kept a RUNNING/WAITING stamp while
// re-evaluating the dead cohort — every agent-loop/continue re-drove the
// same "both owners dead" blocked verdict and re-parked. Nothing respawned
// the cohort after the transient quota window passed.
//
// Fix: when BOTH owner seats are terminal-failed the synthesis stamps stop
// shielding the owner-fail settle ladder (a live in-flight verdict still
// shields — its settle re-fires the ladder). Continue on the dead cohort
// routes to the ladder: transient failures re-mount a bounded retry, a
// capped cohort escalates/parks instead of re-evaluating.

func bug658SeedDeadCohort(t *testing.T, retries int) (*InteractiveService, *fakeWorkflowStore, string) {
	t.Helper()
	store := newFakeWorkflowStore()
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)
	runID := "run-bug658-dead-cohort"
	// The escalate-park shape observed live: owners FAILED, the synthesis
	// step parked WAITING after its own dead-cohort verdict, loop blocked.
	store.seed(runID, []RuntimeWorkflowStep{
		{NodeID: "debate_trigger", Status: StepStatusDone},
		{NodeID: "owner_1", Status: StepStatusFailed},
		{NodeID: "owner_2", Status: StepStatusFailed},
		{NodeID: "debate_synthesis", Status: StepStatusWaitingUserApr},
	})
	rs := &interactiveRun{
		id:                runID,
		projectID:         "proj",
		status:            RunStatusWaitingUserApr,
		agentStatus:       string(RunStatusWaitingUserApr),
		workingMode:       workingmode.Vibe,
		chatFlowRef:       workingmode.PackPrefix + vibeOwnerDebateFlowID,
		autoOrchestrate:   true,
		flowEngineDriven:  true,
		activeFlowNodes:   bug567DebateGraph(),
		vibeParkedNodes:   bug567SprintGraph(),
		vibeParkedFlowRef: workingmode.PackPrefix + vibeSprintFlowID,
		vibeTaskPlan:      []string{"task-a"},
		vibeSprintIndex:   0,
		vibeOwnerFailRetries: retries,
	}
	// The dead owner legs, as the live run recorded them.
	for _, label := range []string{"owner_1", "owner_2"} {
		child := &interactiveRun{
			id:          "run-bug658-" + label,
			parentRunID: runID,
			label:       label,
			status:      RunStatusFailed,
			agentStatus: string(RunStatusFailed),
			legState:    LegStateClosed,
		}
		svc.runs[child.id] = child
		svc.agentOrchestrator.registerChild(runID, child.id)
	}
	svc.runs[runID] = rs
	// The dead-cohort verdict park: loop blocked with the escalate surface.
	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = "escalate"
		st.GateReason = "both owners dead, reset at 02:01Z"
		return st
	})
	return svc, store, runID
}

// Transient all-fail + escalate park → the ladder respawns the owner cohort
// (bounded retry), un-blocking the loop it just re-armed.
func TestBug658_AllOwnersQuotaFailRespawnsAfterReset(t *testing.T) {
	svc, store, runID := bug658SeedDeadCohort(t, 0)

	if !svc.maybeSettleVibeOwnerDebate(runID) {
		t.Fatal("dead-cohort settle ladder refused an all-owners-failed escalate park")
	}

	svc.mu.Lock()
	retries := svc.runs[runID].vibeOwnerFailRetries
	svc.mu.Unlock()
	if retries != 1 {
		t.Fatalf("retry cohort not armed: vibeOwnerFailRetries=%d want 1", retries)
	}
	st := svc.agentOrchestrator.loopStateFor(runID)
	if st.Status != "running" {
		t.Fatalf("dead-cohort escalate block must lift for the retry, got loop=%q reason=%q", st.Status, st.BlockReason)
	}
	// The re-mounted debate re-seeds owner steps — the FAILED seats must be
	// cleared back to a dispatchable state for the fresh cohort.
	waitLoop(t, "owner seats re-seeded for retry cohort", 5*time.Second, func() bool {
		steps, err := store.LoadRunSteps(context.Background(), runID)
		if err != nil {
			return false
		}
		for _, st := range steps {
			if st.NodeID == "owner_1" && st.Status != StepStatusFailed {
				return true
			}
		}
		return false
	})
}

// Permanent failure after the retry budget is the ladder's terminal surface:
// blocked/cap + human park — never another silent re-eval.
func TestBug658_PermanentMemberFailureEscalates(t *testing.T) {
	svc, _, runID := bug658SeedDeadCohort(t, maxVibeOwnerFailRetries)

	if !svc.maybeSettleVibeOwnerDebate(runID) {
		t.Fatal("capped dead-cohort settle must still handle (escalate/park)")
	}
	st := svc.agentOrchestrator.loopStateFor(runID)
	// The cap surface is bounded either way: flag-on escalates into a
	// tournament rescue child (loop leaves "blocked" by design), flag-off
	// parks blocked/cap for a human decision. Both are terminal surfaces —
	// what must NOT happen is another silent re-eval round.
	isTerminalSurface := (st.Status == "blocked" && st.BlockReason == "cap") ||
		st.Status == LoopStatusTournamentEscalation
	if !isTerminalSurface {
		t.Fatalf("permanent cohort failure must surface blocked/cap or tournament escalation, got status=%q reason=%q", st.Status, st.BlockReason)
	}
}
