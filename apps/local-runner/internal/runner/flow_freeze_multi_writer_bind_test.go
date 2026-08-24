package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
)

// Task-293: a freeze node binds the same preflight contract to EVERY
// agent.code writer in the flow (rag-harness: test_signatures AND implement),
// not only the first writer resolveFreezeWriterTarget finds. New file — no
// pre-existing test is modified.

func tddFreezeFixture() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	edges := []agentpack.FlowEdge{
		{From: "freeze", To: "context", When: "done", Kind: "forward"},
		{From: "context", To: "test_signatures", When: "done", Kind: "forward"},
		{From: "test_signatures", To: "implement", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "freeze", Behavior: "contract.freeze"},
		{ID: "context", Behavior: "context.produce"},
		{ID: "test_signatures", Behavior: "agent.code", Agent: "agents/tester.md"},
		{ID: "implement", Behavior: "agent.code", Agent: "agents/coder.md"},
	}
	return edges, nodes
}

func TestRunContractFreezeNodeBindsEveryWriter(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := tddFreezeFixture()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	freezeNode, _ := findFlowNode(nodes, "freeze")

	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
		t.Fatal("expected runContractFreezeNode to return true (handled)")
	}

	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, writer := range []string{"test_signatures", "implement"} {
		rec, ok, err := store.GetFrozenForStep(runID, writer)
		if err != nil || !ok {
			t.Fatalf("expected a frozen record for (runID, %s): ok=%v err=%v", writer, ok, err)
		}
		if rec.FeatureKey != "calc-core" {
			t.Fatalf("%s contract feature_key = %q, want calc-core", writer, rec.FeatureKey)
		}
		if rec.CoderStepID != writer {
			t.Fatalf("%s contract CoderStepID = %q, want %q", writer, rec.CoderStepID, writer)
		}
	}
	// The two records are distinct contracts (different CoderStepID => different
	// ContractID), not the same record duplicated.
	first, _, _ := store.GetFrozenForStep(runID, "test_signatures")
	second, _, _ := store.GetFrozenForStep(runID, "implement")
	if first.ContractID == second.ContractID {
		t.Fatalf("sibling writers must get distinct ContractIDs, both = %q", first.ContractID)
	}
}

func TestRunContractFreezeNodeReuseBindsMissingSibling(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := tddFreezeFixture()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	freezeNode, _ := findFlowNode(nodes, "freeze")

	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
		t.Fatal("expected first freeze to be handled")
	}
	// Second delivery (recovery/dup): must reuse the first writer's contract
	// AND still bind the sibling if it was lost.
	store, _ := changecontract.NewFrozenStore(dir)
	firstRec, ok, _ := store.GetFrozenForStep(runID, "test_signatures")
	if !ok {
		t.Fatal("first writer contract missing after first freeze")
	}
	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
		t.Fatal("expected duplicate freeze delivery to be handled")
	}
	reused, ok, _ := store.GetFrozenForStep(runID, "test_signatures")
	if !ok || reused.ContractID != firstRec.ContractID {
		t.Fatalf("duplicate freeze must reuse contract %q, got %q (ok=%v)", firstRec.ContractID, reused.ContractID, ok)
	}
	impl, ok, _ := store.GetFrozenForStep(runID, "implement")
	if !ok {
		t.Fatal("duplicate freeze must still bind the missing sibling writer implement")
	}
	if impl.ContractID == reused.ContractID {
		t.Fatal("sibling contract must be a distinct ContractID")
	}
}

func TestFlowAgentCodeWriterNodesFiltersByBehavior(t *testing.T) {
	nodes := []agentpack.FlowNode{
		{ID: "test_signatures", Behavior: "agent.code"},
		{ID: "implement", Behavior: "agent.code"},
		{ID: "reviewer", Behavior: "agent.delegate"},
		{ID: "validate", Behavior: "command.validate"},
	}
	got := flowAgentCodeWriterNodes(nodes)
	if len(got) != 2 || got[0].ID != "test_signatures" || got[1].ID != "implement" {
		t.Fatalf("flowAgentCodeWriterNodes = %+v, want [test_signatures implement]", got)
	}
}
