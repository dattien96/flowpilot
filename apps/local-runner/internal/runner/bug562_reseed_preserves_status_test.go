package runner

// BUG-562 — live run-69320 (PrivateVault Task-024 sprint, 2026-10-02): every
// sub-flow re-resolve (owner-debate mount, sprint re-resolve after debate)
// called reseedFlowStepRuntime, which REPLACED the whole step list with fresh
// PENDING rows. The timeline dropped all settled statuses ("0/10 pending"),
// and a legitimate child completion then landed on a hub row that looked
// never-dispatched — CA-1087's stale-done guard consumed it as
// flow_control_stale_hub_done, so the coder leg was never spawned.
//
// Contract: a re-resolve merges, not replaces — rows the flow already owns
// (non-empty NodeID) keep status + timestamps + rejection note; node ids new
// to the topology seed PENDING; prior flow rows no longer present in the
// resolved node list stay as historical rows. Catalog-seeded rows (empty
// NodeID, the workflow-picker shape) are still replaced on first reseed.

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

func bug562SprintNodes() []agentpack.FlowNode {
	return []agentpack.FlowNode{
		{ID: "context", Behavior: "context.produce"},
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
		{ID: "reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md"},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
}

func TestBug562_ReseedPreservesSettledStatuses(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	nodes := bug562SprintNodes()
	svc.markFlowEngineDriven(runID)
	svc.reseedFlowStepRuntime(runID, nodes)

	svc.setFlowStepStatus(context.Background(), runID, "context", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), runID, "coder", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), runID, "reviewer", StepStatusRunning)

	// Re-resolve the SAME topology (the path a sprint re-resolve after a
	// mounted debate takes) — settled statuses must survive.
	svc.reseedFlowStepRuntime(runID, nodes)

	steps, err := svc.workflowStore.LoadRunSteps(context.Background(), runID)
	if err != nil {
		t.Fatalf("LoadRunSteps: %v", err)
	}
	if st, _ := stepByID(steps, "context"); st.Status != StepStatusDone {
		t.Fatalf("context = %q after re-resolve, want DONE (status wiped — BUG-562)", st.Status)
	}
	if st, _ := stepByID(steps, "coder"); st.Status != StepStatusDone {
		t.Fatalf("coder = %q after re-resolve, want DONE", st.Status)
	}
	if st, _ := stepByID(steps, "reviewer"); st.Status != StepStatusRunning {
		t.Fatalf("reviewer = %q after re-resolve, want RUNNING (in-flight leg must not regress to PENDING)", st.Status)
	}
	if st, _ := stepByID(steps, "context"); st.FinishedAt == "" {
		t.Fatal("context lost its FinishedAt timestamp on re-resolve")
	}
}

func TestBug562_ReseedKeepsSubflowRowsAsHistory(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	svc.markFlowEngineDriven(runID)
	svc.reseedFlowStepRuntime(runID, bug562SprintNodes())
	svc.setFlowStepStatus(context.Background(), runID, "synthesis", StepStatusRunning)

	// A mounted owner-debate sub-flow resolves its own nodes onto the same
	// run (vibe remediation flow). Sprint rows must survive the foreign
	// reseed — they are real executed history, not stale seeds.
	debateNodes := []agentpack.FlowNode{
		{ID: "debate_trigger", Behavior: "hub.inline"},
		{ID: "owner_1", Behavior: "agent.delegate"},
		{ID: "owner_2", Behavior: "agent.delegate"},
		{ID: "debate_synthesis", Behavior: "hub.inline"},
	}
	svc.reseedFlowStepRuntime(runID, debateNodes)
	svc.setFlowStepStatus(context.Background(), runID, "owner_1", StepStatusDone)

	// The sprint graph re-resolves after the debate settles.
	svc.reseedFlowStepRuntime(runID, bug562SprintNodes())

	steps, err := svc.workflowStore.LoadRunSteps(context.Background(), runID)
	if err != nil {
		t.Fatalf("LoadRunSteps: %v", err)
	}
	if st, _ := stepByID(steps, "synthesis"); st.Status != StepStatusRunning {
		t.Fatalf("synthesis = %q after sprint re-resolve, want RUNNING (debate reseed wiped it)", st.Status)
	}
	if st, _ := stepByID(steps, "owner_1"); st.Status != StepStatusDone {
		t.Fatalf("owner_1 history row = %q, want DONE kept as historical row", st.Status)
	}
}

func TestBug562_ReseedStillSeedsNewNodesPending(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	svc.markFlowEngineDriven(runID)
	svc.reseedFlowStepRuntime(runID, bug562SprintNodes())

	// Re-resolve with an appended node — new ids still seed PENDING.
	nodes := append(append([]agentpack.FlowNode{}, bug562SprintNodes()...),
		agentpack.FlowNode{ID: "audit", Behavior: "artifact.audit_draft"})
	svc.reseedFlowStepRuntime(runID, nodes)

	steps, _ := svc.workflowStore.LoadRunSteps(context.Background(), runID)
	if st, _ := stepByID(steps, "audit"); st.Status != StepStatusPending {
		t.Fatalf("audit = %q, want PENDING for a newly resolved node", st.Status)
	}
}
