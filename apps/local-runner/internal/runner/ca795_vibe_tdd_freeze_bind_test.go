package runner

import (
	"context"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
)

// CA-795: vibe-sprint freeze→context→tdd(agent.code)→coder matches harness
// test_signatures→implement order. freezeWriterBinding takes no providerKey.
// New file — CA-785 fixture tests (tdd still delegate) stay untouched.

func vibeSprintWriterChain() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	edges := []agentpack.FlowEdge{
		{From: "preflight_contract_freeze", To: "context", When: "done", Kind: "forward"},
		{From: "context", To: "tdd", When: "done", Kind: "forward"},
		{From: "tdd", To: "coder", When: "done", Kind: "forward"},
		{From: "coder", To: "validate", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "preflight_contract_freeze", Behavior: "contract.freeze"},
		{ID: "context", Behavior: "context.produce"},
		{ID: "tdd", Behavior: "agent.code", Agent: "agents/tester.md", PromptTemplate: "prompts/test-signatures.md"},
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md", PromptTemplate: "prompts/implement-complete-tests.md"},
		{ID: "validate", Behavior: "command.validate"},
	}
	return edges, nodes
}

func TestFreezeWriterBinding_VibeSprintPackBindsTdd(t *testing.T) {
	pack, err := agentpack.LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	var def agentpack.FlowDefinition
	for _, f := range pack.Flows {
		if f.ID == "vibe-sprint" {
			def = f
			break
		}
	}
	if def.ID == "" {
		t.Fatal("missing vibe-sprint")
	}
	w, _, direct, ok := freezeWriterBinding(def.Edges, def.Nodes, "preflight_contract_freeze")
	if !ok {
		t.Fatal("freeze must bind a writer")
	}
	if !direct || w.ID != "tdd" {
		t.Fatalf("writer=%q direct=%v, want tdd direct", w.ID, direct)
	}
}

func TestFreezeWriterBinding_VibeChainBindsTddNotCoder(t *testing.T) {
	edges, nodes := vibeSprintWriterChain()
	w, path, direct, ok := freezeWriterBinding(edges, nodes, "preflight_contract_freeze")
	if !ok || !direct || w.ID != "tdd" {
		t.Fatalf("writer=%q direct=%v ok=%v, want tdd direct", w.ID, direct, ok)
	}
	if len(path) != 1 || path[0].ID != "context" {
		t.Fatalf("path=%v, want [context]", path)
	}
}

func TestVibeSprintFreezeSpawnsTddThenCoder(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := vibeSprintWriterChain()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	freezeNode, _ := findFlowNode(nodes, "preflight_contract_freeze")

	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
		t.Fatal("expected freeze to be handled")
	}
	waitLoop(t, "tdd child spawned after freeze", 3*time.Second, func() bool {
		return countChildrenWithLabel(svc, runID, "tdd") == 1
	})
	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, writer := range []string{"tdd", "coder"} {
		if _, ok, _ := store.GetFrozenForStep(runID, writer); !ok {
			t.Fatalf("missing frozen contract for %s", writer)
		}
	}

	// Fake adapter may auto-complete tdd and event-advance coder; then skip
	// tryAdvance. Order is still freeze→tdd first (wait above). Direct
	// tryAdvance is TestTryAdvanceSpawnsAgentCodeWriterWithWriterPrompt.
	if countChildrenWithLabel(svc, runID, "coder") == 0 {
		if !svc.tryAdvanceFlowFromNode(runID, "tdd", "signatures written") {
			t.Fatal("expected tryAdvanceFlowFromNode(tdd) to spawn coder")
		}
	}
	waitLoop(t, "coder child spawned after tdd", 3*time.Second, func() bool {
		return countChildrenWithLabel(svc, runID, "coder") == 1
	})
}

func TestVibeSprintAdvanceCoderBlockedWithoutContract(t *testing.T) {
	dir, _ := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := vibeSprintWriterChain()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, "")

	if svc.tryAdvanceFlowFromNode(runID, "tdd", "signatures written") {
		t.Fatal("expected tryAdvance to block coder with no frozen contract")
	}
	if n := countChildrenWithLabel(svc, runID, "coder"); n != 0 {
		t.Fatalf("unbound coder children=%d", n)
	}
}
