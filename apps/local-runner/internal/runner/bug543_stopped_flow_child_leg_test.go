package runner

import (
	"testing"
)

// BUG-543 (live run-18660 / run-23156 / run-28790 residue): completed or
// park-cancelled flow children were observed with leg_state=active long after
// their last turn. Assessment (2026-09-28, R13): the leg claim is deliberate —
// a cancelled/waiting/completed child stays re-drivable through
// reinvokeMatchingFlowChild (label match), resumePendingLoopWork intent flush,
// and the orphan-cure path; closing the leg on park/stop would break
// resume-after-stop and review-loop reuse (the reverted broad close proved
// exactly that — ~56 suite failures).
//
// Terminal ownership transfers only through explicit seams, all implemented:
// reconcileChildRunsOnFlowDone (LegClosedReasonFlowDone), member_action skip
// (LegClosedReasonMemberSkipped), dispatch failure
// (LegClosedReasonDispatchFailed), candidate sweep
// (LegClosedReasonWorktreeSwept). These tests pin that contract: Stop must
// terminalize children without stripping their claims, so a later
// resume/reinvoke still has a leg to dispatch on.
func TestBug543_StopKeepsChildLegClaim(t *testing.T) {
	svc, runID := clusterFService(t)

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
	child.flowCohortId = "flow-auto-parallel_rollout-attempt-0"
	child.status = RunStatusWaitingUserApr
	child.agentStatus = string(RunStatusWaitingUserApr)
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(runID, handle.RunID)
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running"})

	if _, e := svc.stopAgentLoop(runID); e != nil {
		t.Fatalf("stopAgentLoop: %v", e)
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if child.status != RunStatusCancelled {
		t.Fatalf("stopped flow must terminalize the child, got %q", child.status)
	}
	if child.legState != LegStateActive {
		t.Fatalf("Stop must not strip the child's leg claim — a resume/reinvoke would hit leg_closed; got %q", child.legState)
	}
}

// The flow-done reconcile is the only generic leg-close seam for settled
// children: waiting/running children of a DONE flow are forced completed and
// lose their claim (flow_done) — the barrier already proved every member
// finished, so no re-drive can legitimately need the leg.
func TestBug543_FlowDoneReconcileClosesLegs(t *testing.T) {
	svc, runID := clusterFService(t)

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
	child.label = "candidate-b"
	child.flowCohortId = "flow-auto-parallel_rollout-attempt-0"
	child.status = RunStatusWaitingUserApr
	child.agentStatus = string(RunStatusWaitingUserApr)
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(runID, handle.RunID)

	svc.reconcileChildRunsOnFlowDone(runID)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if child.status != RunStatusCompleted {
		t.Fatalf("flow-done reconcile must settle the child, got %q", child.status)
	}
	if child.legState != LegStateClosed || child.legClosedReason != LegClosedReasonFlowDone {
		t.Fatalf("flow-done reconcile must close the leg flow_done, got %q/%q", child.legState, child.legClosedReason)
	}
}
