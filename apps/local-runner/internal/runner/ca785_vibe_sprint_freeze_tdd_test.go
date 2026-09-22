package runner

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
)

func vibeSprintFreezeFixture() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	edges := []agentpack.FlowEdge{
		{From: "preflight_contract_freeze", To: "context", When: "done", Kind: "forward"},
		{From: "context", To: "tdd", When: "done", Kind: "forward"},
		{From: "tdd", To: "coder", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "preflight_contract_freeze", Behavior: "contract.freeze"},
		{ID: "context", Behavior: "context.produce"},
		{ID: "tdd", Behavior: "agent.delegate", Agent: "agents/tester.md"},
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
	}
	return edges, nodes
}

func TestFreezeWriterBinding_VibeSprintTddBeforeCoder(t *testing.T) {
	edges, nodes := vibeSprintFreezeFixture()
	if _, _, ok := resolveFreezeWriterTarget(edges, nodes, "preflight_contract_freeze", flowInlineChainHopLimit); ok {
		t.Fatal("direct freeze-chain must still fail closed through TDD agent.delegate")
	}
	w, path, direct, ok := freezeWriterBinding(edges, nodes, "preflight_contract_freeze")
	if !ok {
		t.Fatal("coder must bind even when TDD sits between freeze and writer")
	}
	if w.ID != "coder" || direct || len(path) != 0 {
		t.Fatalf("writer=%q direct=%v path=%d", w.ID, direct, len(path))
	}
}

func TestFreezeWriterBinding_StillFailsWithoutAgentCode(t *testing.T) {
	edges := []agentpack.FlowEdge{
		{From: "freeze", To: "tdd", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "freeze", Behavior: "contract.freeze"},
		{ID: "tdd", Behavior: "agent.delegate", Agent: "agents/tester.md"},
	}
	if _, _, _, ok := freezeWriterBinding(edges, nodes, "freeze"); ok {
		t.Fatal("no agent.code must still fail closed")
	}
}
