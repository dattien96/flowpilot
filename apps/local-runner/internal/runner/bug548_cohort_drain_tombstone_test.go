package runner

import (
	"testing"
)

// BUG-548 (live run-40950 on :4322): after a cohort is drained — here via
// memberAction=skip on a member_stalled park, but the same applies to a
// normal join — a late-arriving member result (route successor whose turn
// outlived the skip) re-appends into the emptied buffer, and
// ensureCohortExpectedLocked then *re-infers* the expected count from live
// siblings. The barrier re-opens with only the late entry — hasOpenCohort
// stays true forever and the flow parks on hub_stalled.
//
// A drained cohort is closed for appends and inference until a NEW member
// generation registers for the same key (registerCohortMember /
// preRegisterCohort clear the tombstone).
func TestBug548_LateAppendAfterDrainDoesNotReopenBarrier(t *testing.T) {
	svc, runID := clusterFService(t)
	cohortID := "flow-auto-parallel_rollout-attempt-0"

	// Two seats; both delivered via skip-style failed entries.
	mkMember := func(label string) *interactiveRun {
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
		svc.mu.Unlock()
		return c
	}
	mkMember("candidate-a")
	mkMember("candidate-b")

	svc.agentOrchestrator.preRegisterCohort(runID, cohortID, 2)
	svc.agentOrchestrator.appendCohortResult(runID, cohortID, cohortEntry{Label: "candidate-a", Status: "failed", Err: "skipped by user (stalled)"})
	svc.agentOrchestrator.appendCohortResult(runID, cohortID, cohortEntry{Label: "candidate-b", Status: "failed", Err: "skipped by user (stalled)"})
	if !svc.agentOrchestrator.cohortComplete(runID, cohortID) {
		t.Fatal("precondition: barrier must complete on two seat entries")
	}
	svc.agentOrchestrator.drainCohort(runID, cohortID)

	// Late result from the candidate-a successor arrives after delivery —
	// the completion path first heals expected from live siblings, then
	// appends. Both must no-op on a drained cohort.
	svc.ensureCohortExpected(runID, cohortID)
	svc.agentOrchestrator.appendCohortResult(runID, cohortID, cohortEntry{Label: "candidate-a", Status: "completed"})

	if svc.agentOrchestrator.hasOpenCohort(runID) {
		t.Fatal("late append after drain must not re-open the cohort barrier")
	}
	if got := svc.agentOrchestrator.cohortExpectedCount(runID, cohortID); got != 0 {
		t.Fatalf("drained cohort expected count must stay 0, got %d", got)
	}
}

// A drained cohort key may be legitimately reused by a new member
// generation (a fresh parallel_rollout round) — registration clears the
// tombstone and appends then buffer normally.
func TestBug548_NewGenerationRegistrationClearsTombstone(t *testing.T) {
	svc, runID := clusterFService(t)
	cohortID := "flow-auto-parallel_rollout-attempt-0"

	svc.agentOrchestrator.preRegisterCohort(runID, cohortID, 1)
	svc.agentOrchestrator.appendCohortResult(runID, cohortID, cohortEntry{Label: "candidate-a", Status: "completed"})
	svc.agentOrchestrator.drainCohort(runID, cohortID)

	// Fresh generation for the same cohort key: register → append → join.
	svc.agentOrchestrator.registerCohortMember(runID, cohortID)
	svc.agentOrchestrator.appendCohortResult(runID, cohortID, cohortEntry{Label: "candidate-a", Status: "completed"})
	if !svc.agentOrchestrator.cohortComplete(runID, cohortID) {
		t.Fatal("re-registered cohort must complete normally after drain")
	}
}
