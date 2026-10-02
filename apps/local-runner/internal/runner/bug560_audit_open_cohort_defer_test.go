package runner

// BUG-560 (live run-60899): audit evaluated blocked_missing_feature_key and
// escalated while the reviewer cohort (flow-auto-validate-round-1) still had
// an in-flight member — the escalate's park cancelled that member mid-turn
// ("interrupted by user") and drained its barrier as cancelled, so nothing
// could rejoin on resume. A not-ready verdict while an upstream cohort is
// still open is provisional: the audit must defer (node back to PENDING,
// loop keeps running) until the barrier resolves.
//
// New file; no pre-existing test is modified.

import (
	"context"
	"testing"
)

func TestBug560_AuditDefersWhileCohortOpen(t *testing.T) {
	svc, runID, nodes, edges, auditNode := ca1096Setup(t, 3, true)

	// A reviewer leg still in flight: barrier expects 1 member, none joined.
	svc.agentOrchestrator.preRegisterCohort(runID, "flow-auto-validate-round-1", 1)
	if !svc.agentOrchestrator.hasOpenCohort(runID) {
		t.Fatal("fixture must have an open cohort")
	}

	if !svc.runAuditNode(context.Background(), runID, edges, nodes, auditNode, "task partial") {
		t.Fatal("runAuditNode must handle the dispatch")
	}

	if loop := svc.agentOrchestrator.loopStateFor(runID); loop.Status == "blocked" {
		t.Fatalf("audit must defer while a cohort is open, not escalate/park: %+v", loop)
	}
	if got := flowStepStatus(t, svc, runID, "audit"); got != StepStatusPending {
		t.Fatalf("deferred audit step = %v, want PENDING (re-dispatched when the barrier joins)", got)
	}
}

// Once the barrier resolves, the same not-ready verdict escalates as before —
// deferral is a pause, not a swallow.
func TestBug560_AuditEscalatesAfterCohortResolves(t *testing.T) {
	svc, runID, nodes, edges, auditNode := ca1096Setup(t, 3, true)

	cid := "flow-auto-validate-round-1"
	svc.agentOrchestrator.preRegisterCohort(runID, cid, 1)
	svc.runAuditNode(context.Background(), runID, edges, nodes, auditNode, "task partial")
	if loop := svc.agentOrchestrator.loopStateFor(runID); loop.Status == "blocked" {
		t.Fatalf("still must defer first: %+v", loop)
	}

	// Member completes → join drains the barrier.
	svc.agentOrchestrator.appendCohortResult(runID, cid, cohortEntry{Label: "reviewer", Status: "completed"})
	svc.agentOrchestrator.drainCohort(runID, cid)

	if !svc.runAuditNode(context.Background(), runID, edges, nodes, auditNode, "task partial") {
		t.Fatal("runAuditNode must handle the redispatch")
	}
	if loop := svc.agentOrchestrator.loopStateFor(runID); loop.Status != "blocked" {
		t.Fatalf("after the cohort resolves the same not-ready audit must escalate: %+v", loop)
	}
}

// A closed (delivered/drained or never-open) barrier must not defer — the
// existing CA-1096 escalate contract is untouched when nothing is in flight.
func TestBug560_NoDeferWithoutOpenCohort(t *testing.T) {
	svc, runID, nodes, edges, auditNode := ca1096Setup(t, 3, true)

	svc.runAuditNode(context.Background(), runID, edges, nodes, auditNode, "task partial")

	if loop := svc.agentOrchestrator.loopStateFor(runID); loop.Status != "blocked" {
		t.Fatalf("no open cohort → audit must escalate as before: %+v", loop)
	}
}
