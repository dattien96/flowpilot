package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// CA-1077: agent.scaffold is a spawnable delegate child (behaviorAgentDelegate,
// behavior_registry_builtin.go) and pack-load validation has accepted
// `model:` on it since CP-67 — but resolveFlowNodeModel's allowlist only
// admitted {agent.delegate, agent.code}, so both the Settings step row AND
// the pack pin (vibe-sprint tdd → claude-sonnet-4-5) were silently dropped
// and the scaffold always inherited the run model.

// DB step row wins for agent.scaffold, same as agent.code/agent.delegate.
func TestCA1077_ResolveFlowNodeModelScaffoldHonorsStepRow(t *testing.T) {
	catalog := newInteractiveCatalog()
	catalog.steps["wf"] = []Step{{
		ID:       "flowpilot_core_flow_pack_vibe_sprint_tdd",
		Name:     "Tdd",
		NodeID:   "tdd",
		AgentRef: "agents/scaffold-architect.md",
		Model:    "grok-4.6",
	}}
	svc := newInteractiveService(DefaultProviderRegistry(), catalog, newFakeWorkflowStore())
	got := svc.resolveFlowNodeModel(context.Background(), "", agentpack.FlowNode{
		ID: "tdd", Behavior: "agent.scaffold", Agent: "agents/scaffold-architect.md",
		Model: "claude-sonnet-4-5",
	})
	if got != "grok-4.6" {
		t.Fatalf("resolveFlowNodeModel(agent.scaffold tdd) = %q, want step row grok-4.6", got)
	}
}

// With no step row, the pack YAML pin must apply — it is declared on
// agent.scaffold today (vibe-sprint tdd → claude-sonnet-4-5) and the pack
// loader treats it as a live contract (CP-67 B-5), not decoration.
func TestCA1077_ResolveFlowNodeModelScaffoldHonorsPackPin(t *testing.T) {
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	got := svc.resolveFlowNodeModel(context.Background(), "", agentpack.FlowNode{
		ID: "tdd", Behavior: "agent.scaffold", Agent: "agents/scaffold-architect.md",
		Model: "claude-sonnet-4-5",
	})
	if got != "claude-sonnet-4-5" {
		t.Fatalf("resolveFlowNodeModel(agent.scaffold tdd, pack pin) = %q, want claude-sonnet-4-5", got)
	}
}

// Empty everywhere → inherit (unchanged contract).
func TestCA1077_ResolveFlowNodeModelScaffoldEmptyInherits(t *testing.T) {
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	got := svc.resolveFlowNodeModel(context.Background(), "", agentpack.FlowNode{
		ID: "tdd", Behavior: "agent.scaffold", Agent: "agents/scaffold-architect.md",
	})
	if got != "" {
		t.Fatalf("resolveFlowNodeModel(agent.scaffold, no row/pin) = %q, want empty inherit", got)
	}
}
