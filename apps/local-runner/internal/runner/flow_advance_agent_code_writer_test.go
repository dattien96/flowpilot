package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// Task-293: tryAdvanceFlowFromNode must spawn a forward-done agent.code target
// (test_signatures -> implement) through the frozen-writer path with the
// writer prompt — never the review handoff. New file — no pre-existing test
// is modified.

func tddAdvanceFixture() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	edges := []agentpack.FlowEdge{
		{From: "freeze", To: "test_signatures", When: "done", Kind: "forward"},
		{From: "test_signatures", To: "implement", When: "done", Kind: "forward"},
		{From: "implement", To: "validate", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "freeze", Behavior: "contract.freeze"},
		{ID: "test_signatures", Behavior: "agent.code", Agent: "agents/tester.md", PromptTemplate: "prompts/test-signatures.md"},
		{ID: "implement", Behavior: "agent.code", Agent: "agents/coder.md", PromptTemplate: "prompts/implement-complete-tests.md"},
		{ID: "validate", Behavior: "command.validate"},
	}
	return edges, nodes
}

func TestTryAdvanceSpawnsAgentCodeWriterWithWriterPrompt(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := tddAdvanceFixture()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	freezeNode, _ := findFlowNode(nodes, "freeze")

	// Freeze binds both writers, then spawns the first writer (test_signatures).
	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
		t.Fatal("expected freeze to be handled")
	}
	waitLoop(t, "test_signatures child spawned after freeze", 3*time.Second, func() bool {
		return countChildrenWithLabel(svc, runID, "test_signatures") == 1
	})

	// test_signatures completes -> implement must spawn via the writer path.
	if !svc.tryAdvanceFlowFromNode(runID, "test_signatures", "signatures written") {
		t.Fatal("expected tryAdvanceFlowFromNode(test_signatures) to spawn implement")
	}
	waitLoop(t, "implement child spawned after test_signatures completion", 3*time.Second, func() bool {
		return countChildrenWithLabel(svc, runID, "implement") == 1
	})

	svc.mu.Lock()
	var implPrompt string
	for _, run := range svc.runs {
		if run.parentRunID == runID && run.label == "implement" {
			implPrompt = run.lastFullPrompt
		}
	}
	svc.mu.Unlock()

	if implPrompt == "" {
		t.Fatal("implement child prompt not captured")
	}
	if strings.Contains(implPrompt, "Review this result") {
		t.Fatalf("implement must NOT receive the review handoff prompt: %q", implPrompt)
	}
	if !strings.Contains(implPrompt, "[FlowPilot implement step]") {
		t.Fatalf("implement prompt missing the implement static step prompt: %q", implPrompt)
	}
	if !strings.Contains(implPrompt, "fix rounding") {
		t.Fatalf("implement prompt missing the frozen contract intent: %q", implPrompt)
	}
}

func TestTryAdvanceBlocksAgentCodeWriterWithoutContract(t *testing.T) {
	dir, _ := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := tddAdvanceFixture()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, "")

	// No freeze ran -> implement has no frozen contract; the spawn must be
	// blocked, not silently spawned unbound.
	if svc.tryAdvanceFlowFromNode(runID, "test_signatures", "signatures written") {
		t.Fatal("expected tryAdvance to block an agent.code target with no frozen contract")
	}
	svc.mu.Lock()
	spawned := 0
	for _, run := range svc.runs {
		if run.parentRunID == runID && run.label == "implement" {
			spawned++
		}
	}
	svc.mu.Unlock()
	if spawned != 0 {
		t.Fatalf("implement must not spawn without a frozen contract, found %d", spawned)
	}
}
