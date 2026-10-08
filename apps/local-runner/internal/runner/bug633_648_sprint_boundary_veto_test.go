package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// BUG-633 (live run-297984 / run-306526): the audit settle vetoed the sprint
// boundary on a still-RUNNING straggler leg — correctly — but returned false,
// so the caller fell through to flow_run_complete and sealed the run at task
// 3/6. A veto on TRANSIENT evidence must defer: audit back to PENDING, and
// the straggler's terminal settle re-drives the flow (re-dispatching the
// audit for a fresh evaluation).
//
// BUG-648 (live run-523131): a sprint whose tdd leg FAILED still closed
// done — the evidence gate only checked agent.code legs, so a failed
// scaffold leg never vetoed, and even if it had, the fall-through sealed
// the run. Write-path legs (agent.scaffold/agent.code/command.validate)
// must all be DONE; a failed leg with nothing in flight escalates instead
// of silently checkpointing a task that produced no verified output.

func TestBug633_StragglerDefersBoundary(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	nodes := []agentpack.FlowNode{
		{ID: "validate", Behavior: "command.validate"},
		{ID: "synthesis", Behavior: "hub.inline"},
		{ID: "coder", Behavior: "agent.code"},
		{ID: "audit", Behavior: "artifact.audit_draft"},
	}
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.activeFlowNodes = nodes
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "synthesis", To: "audit", When: "done", Kind: "forward"},
	}
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(runID, nodes)
	svc.setFlowStepStatus(context.Background(), runID, "coder", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), runID, "synthesis", StepStatusDone)
	// The straggler: a genuinely in-flight validate leg, NOT an audit
	// done-edge ancestor — the ~9s window that sealed run-297984.
	svc.setFlowStepStatus(context.Background(), runID, "validate", StepStatusRunning)
	// validate is on the write/verify path and RUNNING — not DONE — so the
	// evidence gate vetoes; the RUNNING step then takes the defer branch.
	svc.setFlowStepStatus(context.Background(), runID, "audit", StepStatusRunning)

	if !svc.maybeAutoAdvanceVibeSprintBoundary(context.Background(), runID, "audit") {
		t.Fatal("straggler veto must own the outcome (defer), not fall through to flow_run_complete")
	}
	if got := svc.lookupFlowStepStatus(runID, "audit"); got != StepStatusPending {
		t.Fatalf("deferred audit must return to PENDING for the straggler-settle re-drive, got %q", got)
	}
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.Status == "done" {
		t.Fatal("deferred boundary must never seal the run done (BUG-633: sealed at task 3/6)")
	}
	svc.mu.Lock()
	armed := svc.runs[runID].vibeSprintBoundaryPending
	svc.mu.Unlock()
	if armed {
		t.Fatal("a deferred veto must not arm the boundary flag — nothing was verified")
	}
}

func TestBug648_FailedTddKeepsSprintOpen(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	nodes := []agentpack.FlowNode{
		{ID: "tdd", Behavior: "agent.scaffold"},
		{ID: "coder", Behavior: "agent.code"},
		{ID: "validate", Behavior: "command.validate"},
		{ID: "synthesis", Behavior: "hub.inline"},
		{ID: "audit", Behavior: "artifact.audit_draft"},
	}
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.activeFlowNodes = nodes
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "synthesis", To: "audit", When: "done", Kind: "forward"},
	}
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(runID, nodes)
	// run-523131 shape: tdd leg FAILED on provider limit mid-turn; the rest
	// of the write path never ran. No straggler — nothing is RUNNING.
	svc.setFlowStepStatus(context.Background(), runID, "tdd", StepStatusFailed)
	svc.setFlowStepStatus(context.Background(), runID, "synthesis", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), runID, "audit", StepStatusRunning)

	if !svc.maybeAutoAdvanceVibeSprintBoundary(context.Background(), runID, "audit") {
		t.Fatal("failed-leg veto must own the outcome (escalate), not fall through to flow_run_complete")
	}
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.Status != "blocked" {
		t.Fatalf("failed sprint must hold the run blocked for the remediation decision, got %q", loop.Status)
	}
	if got := svc.lookupFlowStepStatus(runID, "audit"); got != StepStatusWaitingUserApr {
		t.Fatalf("audit must park WAITING_USER_APPROVAL on the escalation, got %q", got)
	}
	svc.mu.Lock()
	idx, pending := svc.runs[runID].vibeSprintIndex, svc.runs[runID].vibeSprintBoundaryPending
	svc.mu.Unlock()
	if idx != 1 {
		t.Fatalf("task-chain checkpoint must NOT advance on a failed sprint, index=%d want 1", idx)
	}
	if pending {
		t.Fatal("an escalation veto must not arm the boundary flag")
	}
}

func TestBug648_CleanSprintStillAdvances(t *testing.T) {
	// Guard against over-blocking: a sprint whose whole write path is DONE
	// still auto-advances (the CA-1093 contract is unchanged).
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)
	nodes := []agentpack.FlowNode{
		{ID: "tdd", Behavior: "agent.scaffold"},
		{ID: "coder", Behavior: "agent.code"},
		{ID: "validate", Behavior: "command.validate"},
		{ID: "synthesis", Behavior: "hub.inline"},
		{ID: "audit", Behavior: "artifact.audit_draft"},
	}
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.activeFlowNodes = nodes
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "synthesis", To: "audit", When: "done", Kind: "forward"},
	}
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(runID, nodes)
	svc.setFlowStepStatus(context.Background(), runID, "tdd", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), runID, "coder", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), runID, "validate", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), runID, "synthesis", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), runID, "audit", StepStatusRunning)

	if !svc.maybeAutoAdvanceVibeSprintBoundary(context.Background(), runID, "audit") {
		t.Fatal("clean sprint with full write-path evidence must auto-advance")
	}
	svc.mu.Lock()
	idx := svc.runs[runID].vibeSprintIndex
	svc.mu.Unlock()
	if idx != 2 {
		t.Fatalf("clean sprint must take the next task, index=%d want 2", idx)
	}
}
