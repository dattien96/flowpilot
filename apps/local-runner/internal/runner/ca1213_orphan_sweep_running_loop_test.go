package runner

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// CA-1213 (live run-262417): the parked-child orphan sweep only ran on the
// BLOCKED-resume path. A continue landing while the loop showed "running"
// (armed cp_lock / dead escalate residue) early-returned into
// redriveQuietFlowLoop — which treats a waiting_user_approval child as
// busy-evidence and no-ops. The spec_align leg stayed frozen ~5 minutes
// until a later continue happened to hit a blocked loop. The sweep is now
// extracted and also runs on the non-blocked continue path.
func TestCA1213_OrphanSweepRevivesParkedChildWithUnfinishedStep(t *testing.T) {
	dir, _ := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	child := newP4ChildRun(svc, "child-spec", parentID, dir, "")
	child.label = "spec_align"
	child.status = RunStatusWaitingUserApr
	child.agentStatus = "waiting_user_approval"
	svc.agentOrchestrator.registerChild(parentID, child.id)

	svc.mu.Lock()
	rs := svc.runs[parentID]
	rs.flowEngineDriven = true
	rs.activeFlowNodes = []agentpack.FlowNode{{ID: "spec_align"}, {ID: "synthesis"}}
	svc.mu.Unlock()

	store := svc.workflowStore.(*fakeWorkflowStore)
	store.seed(parentID, []RuntimeWorkflowStep{
		{ID: "spec_align", NodeID: "spec_align", Status: StepStatusRunning},
		{ID: "synthesis", NodeID: "synthesis", Status: StepStatusPending},
	})

	redrived, _ := svc.redriveParkedFlowOrphans(parentID, nil)
	if !redrived {
		t.Fatal("parked child with an unfinished step row must be re-driven")
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if child.status != RunStatusRunning {
		t.Fatalf("child status = %q, want running after orphan redrive", child.status)
	}
}

// A live parent decision surface owns the park — children stay frozen until
// the card is answered; the sweep must refuse.
func TestCA1213_OrphanSweepRefusesWhileParentSurfaceLive(t *testing.T) {
	dir, _ := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	child := newP4ChildRun(svc, "child-spec", parentID, dir, "")
	child.label = "spec_align"
	child.status = RunStatusWaitingUserApr
	svc.agentOrchestrator.registerChild(parentID, child.id)

	svc.mu.Lock()
	svc.runs[parentID].pendingApprovalID = "appr-1"
	svc.approvals["appr-1"] = &approvalRecord{id: "appr-1", runID: parentID, status: "pending"}
	svc.mu.Unlock()

	redrived, _ := svc.redriveParkedFlowOrphans(parentID, nil)
	if redrived {
		t.Fatal("sweep must not revive children while a parent decision card is live")
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if child.status != RunStatusWaitingUserApr {
		t.Fatalf("child must stay parked, got %q", child.status)
	}
}

// A child parked with its OWN pending card answers the user's decision, not
// the park — it is not an orphan and must not be re-driven.
func TestCA1213_ChildWithOwnCardIsNotOrphan(t *testing.T) {
	dir, _ := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	child := newP4ChildRun(svc, "child-spec", parentID, dir, "")
	child.label = "spec_align"
	child.status = RunStatusWaitingUserApr
	child.pendingApprovalID = "appr-child"
	svc.mu.Lock()
	svc.approvals["appr-child"] = &approvalRecord{id: "appr-child", runID: child.id, status: "pending"}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, child.id)

	store := svc.workflowStore.(*fakeWorkflowStore)
	store.seed(parentID, []RuntimeWorkflowStep{
		{ID: "spec_align", NodeID: "spec_align", Status: StepStatusRunning},
	})

	redrived, _ := svc.redriveParkedFlowOrphans(parentID, nil)
	if redrived {
		t.Fatal("child holding its own pending card is not an orphan")
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if child.status != RunStatusWaitingUserApr {
		t.Fatalf("child must stay parked on its own card, got %q", child.status)
	}
}
