package runner

import (
	"context"
	"testing"
)

// live-039 residual (run-183756): the owner-debate overlay parked
// debate_trigger WAITING_USER_APPROVAL mid-debate; the debate later concluded
// (debate_synthesis DONE) and restoreVibeFlowAfterDebate reseeded the parked
// sprint topology — but mergeReseedSteps preserves every prior row, so the
// overlay's WAITING row survived untouched. Ledger readers (monitor's waiting
// list, step-transitions consumers) see a gate that is still parked forever.
// On restore the overlay has concluded: any of its rows still non-terminal
// are stale by definition and must be finalized.
func TestLive039_RestoreFinalizesOverlaySteps(t *testing.T) {
	svc, store, runID := bug594SeedClaimedDebate(t)
	// Live residue shape: trigger parked WAITING when the debate concluded.
	// ID mirrors flowStepRowsFromNodes — real flow rows carry ID == NodeID and
	// ApplyStepTransition keys on ID.
	store.seed(runID, []RuntimeWorkflowStep{
		{ID: "debate_trigger", NodeID: "debate_trigger", Status: StepStatusWaitingUserApr},
		{ID: "owner_1", NodeID: "owner_1", Status: StepStatusDone},
		{ID: "owner_2", NodeID: "owner_2", Status: StepStatusDone},
		{ID: "debate_synthesis", NodeID: "debate_synthesis", Status: StepStatusDone},
	})

	if !svc.restoreVibeFlowAfterDebate(runID) {
		t.Fatal("restoreVibeFlowAfterDebate must restore the parked sprint")
	}

	steps, err := store.LoadRunSteps(context.Background(), runID)
	if err != nil {
		t.Fatalf("LoadRunSteps: %v", err)
	}
	byNode := map[string]RuntimeWorkflowStep{}
	for _, st := range steps {
		byNode[st.NodeID] = st
	}
	if got := byNode["debate_trigger"].Status; got != StepStatusSkipped {
		t.Fatalf("debate_trigger stale after overlay restore: status=%q, want skipped (overlay concluded)", got)
	}
	if got := byNode["debate_synthesis"].Status; got != StepStatusDone {
		t.Fatalf("debate_synthesis DONE row must survive finalize, got %q", got)
	}
	if got := byNode["owner_1"].Status; got != StepStatusDone {
		t.Fatalf("owner_1 DONE row must survive finalize, got %q", got)
	}
}

// An overlay node still RUNNING with a live leg is real work, not residue —
// the finalize must not stamp over it.
func TestLive039_RestoreSkipsLiveOverlayWork(t *testing.T) {
	svc, store, runID := bug594SeedClaimedDebate(t)
	store.seed(runID, []RuntimeWorkflowStep{
		{ID: "debate_trigger", NodeID: "debate_trigger", Status: StepStatusDone},
		{ID: "owner_1", NodeID: "owner_1", Status: StepStatusRunning},
		{ID: "owner_2", NodeID: "owner_2", Status: StepStatusDone},
		{ID: "debate_synthesis", NodeID: "debate_synthesis", Status: StepStatusDone},
	})
	// A live owner_1 leg: child run labelled owner_1 still mid-turn.
	child := &interactiveRun{
		id:          "run-live-owner",
		parentRunID: runID,
		label:       "owner_1",
		status:      RunStatusRunning,
	}
	svc.mu.Lock()
	svc.runs[child.id] = child
	svc.mu.Unlock()

	if !svc.restoreVibeFlowAfterDebate(runID) {
		t.Fatal("restoreVibeFlowAfterDebate must restore the parked sprint")
	}
	steps, _ := store.LoadRunSteps(context.Background(), runID)
	for _, st := range steps {
		if st.NodeID == "owner_1" && st.Status != StepStatusRunning {
			t.Fatalf("live owner_1 leg stamped %q — live work must win over finalize", st.Status)
		}
	}
}
