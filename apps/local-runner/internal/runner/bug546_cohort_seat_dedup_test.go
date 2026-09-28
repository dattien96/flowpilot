package runner

import (
	"testing"
)

// BUG-546 (live run-34296 on :4322): a quota-vetoed cohort member and its
// route-committed successor are TWO physical runs holding ONE logical seat
// (same label). Expected-count inference counted physical runs, so after a
// restart/rebuild (or any expected-count heal) the barrier expected 3 while
// the label-deduped buffer can only ever hold 2 entries — the join never
// fires and the flow parks on hub_stalled.
//
// Seats are the logical identity: label when set, run id otherwise.
func TestBug546_ExpectedCountsSeatsNotRuns(t *testing.T) {
	svc, runID := clusterFService(t)
	cohortID := "flow-auto-parallel_rollout-attempt-0"

	mkMember := func(label string, status RunStatus) *interactiveRun {
		handle, apiErr := svc.createRun(StartRunInput{
			ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		})
		if apiErr != nil {
			t.Fatalf("createRun: %v", apiErr)
		}
		svc.mu.Lock()
		c := svc.runs[handle.RunID]
		c.parentRunID = runID
		c.label = label
		c.flowCohortId = cohortID
		c.status = status
		svc.mu.Unlock()
		return c
	}

	// candidate-a: vetoed original (closed leg) + successor — one seat.
	mkMember("candidate-a", RunStatusCompleted)
	mkMember("candidate-a", RunStatusCompleted)
	// candidate-b: one seat.
	mkMember("candidate-b", RunStatusCompleted)

	svc.ensureCohortExpected(runID, cohortID)
	if got := svc.agentOrchestrator.cohortExpectedCount(runID, cohortID); got != 2 {
		t.Fatalf("expected count must be 2 logical seats, got %d (physical runs)", got)
	}

	svc.agentOrchestrator.appendCohortResult(runID, cohortID, cohortEntry{Label: "candidate-a", Status: "completed"})
	svc.agentOrchestrator.appendCohortResult(runID, cohortID, cohortEntry{Label: "candidate-b", Status: "completed"})
	if !svc.agentOrchestrator.cohortComplete(runID, cohortID) {
		t.Fatal("join must complete once every logical seat has a buffered result")
	}
}

// Members without a label keep the old per-run counting semantics.
func TestBug546_UnlabeledMembersCountPerRun(t *testing.T) {
	svc, runID := clusterFService(t)
	cohortID := "cohort-unlabeled"

	for i := 0; i < 3; i++ {
		handle, apiErr := svc.createRun(StartRunInput{
			ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		})
		if apiErr != nil {
			t.Fatalf("createRun: %v", apiErr)
		}
		svc.mu.Lock()
		c := svc.runs[handle.RunID]
		c.parentRunID = runID
		c.flowCohortId = cohortID
		svc.mu.Unlock()
	}

	svc.ensureCohortExpected(runID, cohortID)
	if got := svc.agentOrchestrator.cohortExpectedCount(runID, cohortID); got != 3 {
		t.Fatalf("unlabeled members must keep per-run counting, got %d", got)
	}
}
