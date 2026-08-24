package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

func TestContractFreeze_ProseWrappedPlannerDraft(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges := []agentpack.FlowEdge{
		{From: "plan", To: "freeze", When: "done", Kind: "forward"},
		{From: "freeze", To: "coder", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "plan", Behavior: "agent.delegate"},
		{ID: "freeze", Behavior: "contract.freeze"},
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
	}
	parentID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	// Prose wrapped draft (like Grok's output in run-214858)
	prosePlannerOutput := "I'll locate `src/calc.go` so the contract can list exact paths only. " +
		`{"feature_key":"calc-core","intent":"fix rounding","declared_paths":["src/calc.go"]}`

	freezeNode, _ := findFlowNode(nodes, "freeze")
	handled := svc.runContractFreezeNode(context.Background(), parentID, edges, nodes, freezeNode, prosePlannerOutput)
	if !handled {
		t.Fatal("expected runContractFreezeNode to handle freeze")
	}

	snap := svc.agentGraphSnapshot(parentID)
	if snap.LoopState.Status == "escalate" {
		t.Fatalf("freeze node should not have escalated on prose-wrapped draft, got status %s", snap.LoopState.Status)
	}
}

func TestContractFreeze_ContinueFlowUsesChildPlannerResult(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges := []agentpack.FlowEdge{
		{From: "plan", To: "freeze", When: "done", Kind: "forward"},
		{From: "freeze", To: "coder", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "plan", Behavior: "agent.delegate"},
		{ID: "freeze", Behavior: "contract.freeze"},
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
	}
	parentID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	// 1. Create predecessor planner child run
	plannerChildID := "child-planner-1"
	svc.mu.Lock()
	svc.runs[plannerChildID] = &interactiveRun{
		id:          plannerChildID,
		parentRunID: parentID,
		label:       "plan",
		events: []ProviderEvent{
			{
				Type:         EventTurnCompleted,
				FinalMessage: `{"feature_key":"calc-core","intent":"fix rounding","declared_paths":["src/calc.go"]}`,
			},
		},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, plannerChildID)

	// 2. Invoke freeze node with plannerResult = "/continue" (as sent when user clicks [Continue])
	freezeNode, _ := findFlowNode(nodes, "freeze")
	handled := svc.runContractFreezeNode(context.Background(), parentID, edges, nodes, freezeNode, "/continue")
	if !handled {
		t.Fatal("expected runContractFreezeNode to handle freeze")
	}

	snap := svc.agentGraphSnapshot(parentID)
	if snap.LoopState.Status == "escalate" {
		t.Fatalf("freeze node should have resolved planner draft from child run on /continue, got status: %s", snap.LoopState.Status)
	}
}
