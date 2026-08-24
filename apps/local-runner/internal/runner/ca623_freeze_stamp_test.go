package runner

import (
	"context"
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// ragHarnessLikeNodes returns a minimal rag-harness topology sufficient for
// freeze-escalate tests: planner not needed (freeze is entry for this test),
// freeze -> context -> coder -> validate -> audit.
func ragHarnessLikeNodes() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	edges := []agentpack.FlowEdge{
		{From: "preflight_contract_freeze", To: "context", When: "done", Kind: "forward"},
		{From: "context", To: "implement", When: "done", Kind: "forward"},
		{From: "implement", To: "validate", When: "done", Kind: "forward"},
		{From: "validate", To: "audit", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "preflight_contract_freeze", Behavior: "contract.freeze"},
		{ID: "context", Behavior: "context.produce"},
		{ID: "implement", Behavior: "agent.code", Agent: "agents/coder.md"},
		{ID: "validate", Behavior: "command.validate"},
		{ID: "audit", Behavior: "artifact.audit_draft"},
	}
	return edges, nodes
}

func TestCA623_FreezeInvalid_StampsFreezeNotAudit(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := ragHarnessLikeNodes()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	// Seed steps PENDING for all nodes
	svc.reseedFlowStepRuntime(runID, nodes)

	freezeNode, _ := findFlowNode(nodes, "preflight_contract_freeze")
	invalid := `not valid json at all`

	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, invalid) {
		t.Fatal("expected escalated true")
	}
	steps, _ := svc.workflowStore.LoadRunSteps(context.Background(), runID)
	statusByID := map[string]string{}
	for _, s := range steps {
		statusByID[s.ID] = string(s.Status)
	}
	if got := statusByID["preflight_contract_freeze"]; got != string(StepStatusWaitingUserApr) {
		t.Fatalf("freeze status = %q want WAITING, steps=%v", got, statusByID)
	}
	if got := statusByID["audit"]; got == string(StepStatusWaitingUserApr) {
		t.Fatalf("audit must NOT be WAITING when freeze escalated (audit=%q) steps=%v", got, statusByID)
	}
	if got := statusByID["audit"]; got != string(StepStatusPending) {
		t.Fatalf("audit should stay PENDING, got %q", got)
	}
	// lastEscalated must be freeze
	svc.mu.Lock()
	esc := svc.runs[runID].lastEscalatedInlineNodeID
	svc.mu.Unlock()
	if esc != "preflight_contract_freeze" {
		t.Fatalf("lastEscalatedInlineNodeID=%q want freeze", esc)
	}
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.Status != "blocked" || loop.BlockReason != "escalate" {
		t.Fatalf("loop status=%q reason=%q want blocked/escalate", loop.Status, loop.BlockReason)
	}
	if !strings.Contains(loop.GateReason, "Contract freeze blocked") {
		t.Fatalf("GateReason %q must contain freeze reason", loop.GateReason)
	}
	// Agnostic confirmation: no ProviderKey branch in freeze escalate path
	// (grep checked: runContractFreezeNode never takes ProviderKey)
}

func TestCA623_FreezeInvalid_ContinueDoesNotSpawnParentAuditStillPending(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := ragHarnessLikeNodes()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	svc.reseedFlowStepRuntime(runID, nodes)
	freezeNode, _ := findFlowNode(nodes, "preflight_contract_freeze")
	invalid := `{"feature_key":"x"}` // missing intent/paths => invalid

	svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, invalid)

	// Continue with empty feedback (user just clicks Continue)
	snap, err := svc.resumeFlowWithFeedback(runID, "")
	if err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}
	_ = snap

	// tryAdvance is async (go routine)
	waitLoop(t, "re-escalate after Continue", 2_000_000_000, func() bool {
		return svc.agentOrchestrator.loopStateFor(runID).Status == "blocked"
	})

	steps, _ := svc.workflowStore.LoadRunSteps(context.Background(), runID)
	for _, s := range steps {
		if s.ID == "audit" && s.Status == StepStatusWaitingUserApr {
			t.Fatalf("audit must remain not-WAITING after Continue on freeze")
		}
	}
	// No second child for audit or implement should exist; parent must not have been reinvoked
	if got := countChildrenWithLabel(svc, runID, "audit"); got != 0 {
		t.Fatalf("audit child must not be spawned on freeze Continue, got %d", got)
	}
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.Status != "blocked" {
		t.Fatalf("after Continue with empty draft loop still blocked, got %q", loop.Status)
	}
}

func TestCA623_AuditRunning_EscalateStillStampsAudit(t *testing.T) {
	// When audit itself is RUNNING and escalates (e.g. no validate command), it
	// must still stamp audit WAITING — the guard only skips duplicate audit,
	// not the primary audit case.
	svc, _ := newTestServer(t)
	run, _ := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	svc.markFlowEngineDriven(run.RunID)
	nodes := []agentpack.FlowNode{
		{ID: "context", Behavior: "context.produce"},
		{ID: "implement", Behavior: "agent.delegate"},
		{ID: "validate", Behavior: "command.validate"},
		{ID: "audit", Behavior: "artifact.audit_draft"},
	}
	svc.mu.Lock()
	svc.runs[run.RunID].activeFlowNodes = nodes
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(run.RunID, nodes)
	svc.setFlowStepStatus(context.Background(), run.RunID, "context", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), run.RunID, "implement", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), run.RunID, "validate", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), run.RunID, "audit", StepStatusRunning)

	svc.setFlowStepAwaitingUser(context.Background(), run.RunID)
	steps, _ := svc.workflowStore.LoadRunSteps(context.Background(), run.RunID)
	for _, s := range steps {
		if s.ID == "audit" && s.Status != StepStatusWaitingUserApr {
			t.Fatalf("audit RUNNING must become WAITING, got %q", s.Status)
		}
	}
}

func TestCA623_GuardAlreadyWaiting_DoesNotStampAudit(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := ragHarnessLikeNodes()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	svc.reseedFlowStepRuntime(runID, nodes)
	freezeNode, _ := findFlowNode(nodes, "preflight_contract_freeze")
	invalid := `not valid json`
	svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, invalid)

	// Now freeze is already WAITING; calling setFlowStepAwaitingUser again must not
	// add audit WAITING
	svc.setFlowStepAwaitingUser(context.Background(), runID)
	steps, _ := svc.workflowStore.LoadRunSteps(context.Background(), runID)
	auditWaiting := false
	freezeWaiting := false
	for _, s := range steps {
		if s.ID == "audit" && s.Status == StepStatusWaitingUserApr {
			auditWaiting = true
		}
		if s.ID == "preflight_contract_freeze" && s.Status == StepStatusWaitingUserApr {
			freezeWaiting = true
		}
	}
	if !freezeWaiting {
		t.Fatal("freeze must remain WAITING")
	}
	if auditWaiting {
		t.Fatal("audit must NOT become WAITING when freeze already WAITING")
	}
}

func TestCA623_ChainEscalate_StampsChainNode(t *testing.T) {
	// When freeze chain fails (context produce), it should stamp context, not audit.
	// We simulate by calling escalate via advanceFlowThroughFreezeChain path:
	// easier: call runContractFreezeNode with valid draft but break writer target
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	// No edges to writer => resolve fails => escalate
	edges := []agentpack.FlowEdge{}
	nodes := []agentpack.FlowNode{{ID: "preflight_contract_freeze", Behavior: "contract.freeze"}}
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	svc.reseedFlowStepRuntime(runID, nodes)
	freezeNode, _ := findFlowNode(nodes, "preflight_contract_freeze")
	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
		t.Fatal("expected escalate")
	}
	steps, _ := svc.workflowStore.LoadRunSteps(context.Background(), runID)
	for _, s := range steps {
		if s.ID == "preflight_contract_freeze" && s.Status != StepStatusWaitingUserApr {
			t.Fatalf("chain-failed freeze must be WAITING got %q", s.Status)
		}
	}
}
