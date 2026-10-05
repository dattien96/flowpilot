package runner

import (
	"context"
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// CA-1219 (live run-295434): the devin hub spawned the adopt-sprint
// contract-planner WITHOUT any provider override, yet the child resolved to
// codex/gpt-5.4 and died at adapter construction — a mirrored
// step_definitions.model row ("gpt-5.4") was carried onto the planner's
// node.Model by recordFromWorkflowRow, CA-616's planner guard only skips the
// step-row *lookup*, so the carried pin still drove providerKeyFromModel.
//
// Two-layer fix:
//  1. resolveFlowNodeModel ignores a planner/scout pin that maps
//     cross-provider from the parent (same-provider tier pins stay valid).
//  2. spawnChildRun gates the RESOLVED provider — any cross-provider route
//     (explicit override, carried node model, agent-def pin) to a provider
//     with no connected account fails before the child row exists.

func plannerNode(model string) agentpack.FlowNode {
	return agentpack.FlowNode{
		ID:       "preflight_contract_plan",
		Behavior: "agent.delegate",
		Agent:    "agents/contract-planner.md",
		Model:    model,
	}
}

func TestPlannerCrossProviderCarriedPinInherits(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat",
		ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	got := svc.resolveFlowNodeModel(context.Background(), parent.RunID, plannerNode("claude-sonnet-4-6"))
	if got != "" {
		t.Fatalf("resolveFlowNodeModel(codex parent, claude pin) = %q, want empty inherit", got)
	}
}

func TestPlannerSameProviderCarriedPinHonored(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat",
		ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	got := svc.resolveFlowNodeModel(context.Background(), parent.RunID, plannerNode("gpt-5.4"))
	if got != "gpt-5.4" {
		t.Fatalf("resolveFlowNodeModel(codex parent, gpt pin) = %q, want gpt-5.4", got)
	}
}

func TestSpawnResolvedPinUnconnectedProviderFailsClosed(t *testing.T) {
	svc, _ := newTestServer(t)
	home := isolateProviderHome(t)
	plantCodexAuth(t, home)

	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat",
		ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	// in.Model routes to claude — no claude account is planted, so the spawn
	// must fail BEFORE a child row exists, with the resolved-pin hint (not
	// the explicit-override hint).
	_, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent: "reviewer", Prompt: "review", Model: "claude-sonnet-4-6",
	})
	if spawnErr == nil {
		t.Fatal("spawnChildRun: expected unconnected-resolved-provider error")
	}
	if !strings.Contains(spawnErr.Error(), `"claude" has no connected local account`) {
		t.Fatalf("error missing provider detail: %v", spawnErr)
	}
	if !strings.Contains(spawnErr.Error(), "model pin") {
		t.Fatalf("error should carry the resolved-pin hint, got: %v", spawnErr)
	}
	svc.mu.Lock()
	for _, rs := range svc.runs {
		if rs.parentRunID == parent.RunID {
			svc.mu.Unlock()
			t.Fatalf("child run created despite unconnected provider gate: %s", rs.id)
		}
	}
	svc.mu.Unlock()
}
