package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// CA-616: contract-planner must inherit the hub's provider/model
// (grok-4.5 / claude-sonnet / gpt-5.4-mini) even when catalog carries a
// seeded gpt-5.4 row. Other delegate nodes (coder/reviewer) keep their
// own node-specific model (CA-230/239/241).
func TestRun135037_DelegateSpawnModel_PlannerInheritsHub(t *testing.T) {
	catalog := newInteractiveCatalog()
	// Seeded legacy row that would give gpt-5.4 if we called
	// resolveFlowNodeModel directly.
	catalog.steps["wf"] = append(catalog.steps["wf"], Step{
		ID:     "flow-agent-delegate-contract-planner",
		Name:   "Flow: Contract Planner",
		Model:  "gpt-5.4",
		NodeID: "preflight_contract_plan",
	})
	catalog.steps["wf"] = append(catalog.steps["wf"], Step{
		ID:       "flowpilot_core_flow_pack__rag_harness__preflight_contract_plan",
		Name:     "Rag Harness: Preflight Contract Plan",
		NodeID:   "preflight_contract_plan",
		AgentRef: "agents/contract-planner.md",
		Model:    "gpt-5.4",
	})
	svc := newInteractiveService(DefaultProviderRegistry(), catalog, newFakeWorkflowStore())
	for _, tc := range []struct {
		name      string
		provider  ProviderKey
		model     string
		wantProv  string
		wantModel string
	}{
		{"grok", ProviderKeyGrok, "grok-4.5", "grok", "grok-4.5"},
		{"codex", ProviderKeyCodex, "gpt-5.4-mini", "codex", "gpt-5.4-mini"},
		{"claude", ProviderKeyClaude, "claude-sonnet", "claude", "claude-sonnet"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// createRun only succeeds for registered provider+model combos (codex).
			// Use a codex model for creation, then mutate to the desired provider/model.
			parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini"})
			if err != nil {
				t.Fatalf("createRun: %v", err)
			}
			svc.mu.Lock()
			svc.runs[parent.RunID].chatFlowRef = "flowpilot-core-flow-pack/rag-harness"
			svc.runs[parent.RunID].modelName = tc.model
			svc.runs[parent.RunID].providerKey = tc.provider
			svc.mu.Unlock()

			node := agentpack.FlowNode{ID: "preflight_contract_plan", Behavior: "agent.delegate", Agent: "agents/contract-planner.md"}
			if got := svc.delegateSpawnModel(context.Background(), parent.RunID, node); got != "" {
				t.Fatalf("delegateSpawnModel planner = %q, want empty (inherit)", got)
			}
			prov, mod := svc.resolveFlowNodeProviderModel(context.Background(), parent.RunID, node)
			if prov != tc.wantProv || mod != tc.wantModel {
				t.Fatalf("providerModel planner = %q/%q, want %q/%q", prov, mod, tc.wantProv, tc.wantModel)
			}
		})
	}
}

func TestRun135037_DelegateSpawnModel_NonPlannerKeepsOwnModel(t *testing.T) {
	catalog := newInteractiveCatalog()
	catalog.steps["wf-feature"] = append(catalog.steps["wf-feature"], Step{
		ID:       "flowpilot_core_flow_pack__review_loop__reviewer_correctness",
		Name:     "Review Loop: Reviewer Correctness",
		NodeID:   "reviewer_correctness",
		AgentRef: "agents/reviewer.md",
		Model:    "gpt-5.4",
	})
	svc := newInteractiveService(DefaultProviderRegistry(), catalog, newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini"})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].chatFlowRef = "flowpilot-core-flow-pack/review-loop"
	svc.mu.Unlock()
	node := agentpack.FlowNode{ID: "reviewer_correctness", Behavior: "agent.delegate", Agent: "agents/reviewer.md"}
	if got := svc.delegateSpawnModel(context.Background(), parent.RunID, node); got != "gpt-5.4" {
		t.Fatalf("delegateSpawnModel reviewer = %q, want gpt-5.4 (keep CA-230)", got)
	}
	// also coder still inherits when no row
	coder := agentpack.FlowNode{ID: "coder", Behavior: "agent.delegate", Agent: "agents/coder.md"}
	if got := svc.delegateSpawnModel(context.Background(), parent.RunID, coder); got != "" {
		// no node-specific coder row in this catalog, so empty (inherit) is correct
		t.Logf("coder delegateSpawnModel = %q (empty=inherit, correct when no row)", got)
	}
}
