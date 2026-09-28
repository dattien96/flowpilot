package runner

import (
	"net/http"
	"testing"
)

// BUG-544 (live run-25217/run-26015): a spawned cohort member whose FIRST
// turn admission hits the quota veto must hold its cohort seat while the
// quota_route_required card is pending — the member is parked awaiting a
// route decision, not failed. Treating the veto as a dispatch failure
// appended a `failed` cohort entry, letting the barrier complete with the
// card still unanswered: the tournament arbitrated, marked the flow done,
// and swept the vetoed member's worktree — every later answer then failed
// `worktree ... no longer exists (swept)` and the card could never resolve.
func TestBug544_QuotaVetoedSpawnedMemberHoldsCohortSeat(t *testing.T) {
	svc, runID := clusterFService(t)
	cohortID := "flow-auto-parallel_rollout-attempt-0"
	svc.agentOrchestrator.preRegisterCohort(runID, cohortID, 2)

	// Spawned-child shape exactly as spawnChildRun leaves it: leg active,
	// cohort tag set, card already parked by emitQuotaRouteCard (the veto
	// surfaces the card BEFORE startTurn returns quota_route_required).
	handle, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if apiErr != nil {
		t.Fatalf("createRun child: %v", apiErr)
	}
	svc.mu.Lock()
	child := svc.runs[handle.RunID]
	child.parentRunID = runID
	child.agentStatus = "spawned"
	child.legState = LegStateActive
	child.stepID = handle.StepID
	child.label = "candidate-a"
	child.flowCohortId = cohortID
	child.status = RunStatusWaitingQuestion
	child.agentStatus = string(RunStatusWaitingQuestion)
	child.pendingQuestionID = "q-vetoed"
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(runID, handle.RunID)

	svc.handleSpawnedChildTurnFailure(handle.RunID, runID, "spawn first-turn prompt", handle.StepID,
		newAPIErr(http.StatusConflict, "quota_route_required", "pinned account is unusable; resolve the quota route card"))

	svc.mu.Lock()
	if child.status == RunStatusFailed {
		t.Fatal("quota-parked member must not be marked failed while its card is pending")
	}
	if child.status != RunStatusWaitingQuestion {
		t.Fatalf("quota-parked member must keep waiting_question, got %q", child.status)
	}
	if child.legState != LegStateActive {
		t.Fatalf("vetoed member must keep its leg claim until the card resolves, got %q", child.legState)
	}
	svc.mu.Unlock()

	// The barrier must NOT count the vetoed member: sibling completes alone
	// and the cohort join must still be waiting on the held seat.
	svc.agentOrchestrator.appendCohortResult(runID, cohortID, cohortEntry{
		Label: "candidate-b", Provider: "devin", Status: "completed",
	})
	if svc.agentOrchestrator.cohortComplete(runID, cohortID) {
		t.Fatal("cohort join completed while a member was parked on an unanswered quota card")
	}
}

// The sibling card answers: `stop` must convert the held seat into a failed
// cohort entry (so the barrier can complete) and close the member's leg —
// otherwise the join waits forever on a member nobody will ever run.
func TestBug544_QuotaStopAnswerReleasesSeat(t *testing.T) {
	svc, runID := clusterFService(t)
	cohortID := "flow-auto-parallel_rollout-attempt-0"
	svc.agentOrchestrator.preRegisterCohort(runID, cohortID, 2)

	handle, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if apiErr != nil {
		t.Fatalf("createRun child: %v", apiErr)
	}
	svc.mu.Lock()
	child := svc.runs[handle.RunID]
	child.parentRunID = runID
	child.legState = LegStateActive
	child.stepID = handle.StepID
	child.label = "candidate-a"
	child.flowCohortId = cohortID
	child.status = RunStatusWaitingQuestion
	child.pendingQuestionID = "q-vetoed"
	svc.mu.Unlock()

	if e := svc.applyQuotaRouteAnswer(child, "q-vetoed", "stop"); e != nil {
		t.Fatalf("stop answer failed: %v", e)
	}
	svc.mu.Lock()
	if child.legState != LegStateClosed {
		t.Fatalf("stop must close the vetoed member's leg claim, got %q", child.legState)
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.appendCohortResult(runID, cohortID, cohortEntry{
		Label: "candidate-b", Provider: "devin", Status: "completed",
	})
	if !svc.agentOrchestrator.cohortComplete(runID, cohortID) {
		t.Fatal("stop answer must release the held seat — barrier should complete with a failed member entry")
	}
}

// The routed successor inherits the vetoed member's seat — it must NOT bump
// the cohort's expected count or the barrier can never complete (expected
// grows to 3 while only sibling+successor ever append).
func TestBug544_RespawnSuccessorInheritsSeat(t *testing.T) {
	svc, runID := clusterFService(t)
	cohortID := "flow-auto-parallel_rollout-attempt-0"
	svc.agentOrchestrator.preRegisterCohort(runID, cohortID, 2)

	handle, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if apiErr != nil {
		t.Fatalf("createRun child: %v", apiErr)
	}
	svc.mu.Lock()
	child := svc.runs[handle.RunID]
	child.parentRunID = runID
	child.legState = LegStateActive
	child.stepID = handle.StepID
	child.label = "candidate-a"
	child.flowCohortId = cohortID
	child.status = RunStatusWaitingQuestion
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(runID, handle.RunID)

	if err := svc.respawnChildOnRoute(t.Context(), child, &RouteCandidate{
		ProviderKey: ProviderKeyCodex, AccountID: "", Model: "m",
	}, ""); err != nil {
		t.Fatalf("respawnChildOnRoute: %v", err)
	}
	svc.mu.Lock()
	if child.legState != LegStateClosed {
		t.Fatalf("vetoed leg must close after the successor provisioned, got %q", child.legState)
	}
	svc.mu.Unlock()

	// Sibling + successor completions must satisfy the barrier: expected
	// stays 2 (seat inherited), not 3 (successor over-registered).
	svc.agentOrchestrator.appendCohortResult(runID, cohortID, cohortEntry{
		Label: "candidate-b", Provider: "devin", Status: "completed",
	})
	var successorID string
	for _, cid := range svc.agentOrchestrator.listChildren(runID) {
		svc.mu.Lock()
		c := svc.runs[cid]
		svc.mu.Unlock()
		if c != nil && c.label == "candidate-a" && c.id != handle.RunID {
			successorID = c.id
		}
	}
	if successorID == "" {
		t.Fatal("no successor spawned for the vetoed seat")
	}
	svc.mu.Lock()
	succ := svc.runs[successorID]
	if succ == nil {
		svc.mu.Unlock()
		t.Fatal("successor run missing")
	}
	if succ.flowCohortId != cohortID {
		svc.mu.Unlock()
		t.Fatalf("successor lost the cohort tag %q → %q", cohortID, succ.flowCohortId)
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.appendCohortResult(runID, cohortID, cohortEntry{
		Label: "candidate-a", Provider: "devin", Status: "completed",
	})
	if !svc.agentOrchestrator.cohortComplete(runID, cohortID) {
		t.Fatal("barrier never completed — successor must inherit the seat, not bump expected")
	}
}
