package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// TestProviderKeyFromModelMatchesPackTable pins Task-320's single source of
// truth: the runner's providerKeyFromModel must agree with
// agentpack.ModelProviderKey on every known prefix family and stay
// fail-closed on unknown/empty input.
func TestProviderKeyFromModelMatchesPackTable(t *testing.T) {
	cases := []struct {
		model string
		want  ProviderKey
		ok    bool
	}{
		{"gpt-5.4-mini", ProviderKeyCodex, true},
		{"gemini-2.5-flash", ProviderKeyGemini, true},
		{"auto-gemini-2.5", ProviderKeyGemini, true},
		{"claude-sonnet-4-5", ProviderKeyClaude, true},
		{"grok-4-5", ProviderKeyGrok, true},
		{"grok-build", ProviderKeyGrok, true},
		{"opencode/grok-code-fast-1", ProviderKeyOpencode, true},
		{"opencode-go/claude-opus-4-6", ProviderKeyOpencode, true},
		{"CLAUDE-SONNET-4-5", ProviderKeyClaude, true},
		{"mystery-model-1", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		got, ok := providerKeyFromModel(tc.model)
		if got != tc.want || ok != tc.ok {
			t.Errorf("providerKeyFromModel(%q) = (%q, %v), want (%q, %v)", tc.model, got, ok, tc.want, tc.ok)
		}
		if pk, pok := agentpack.ModelProviderKey(tc.model); string(tc.want) != pk || tc.ok != pok {
			t.Errorf("parity: agentpack.ModelProviderKey(%q) = (%q, %v), runner says (%q, %v)", tc.model, pk, pok, tc.want, tc.ok)
		}
	}
}

// TestResolveFlowNodeModelFallsBackToYamlPackDefault pins Task-320 T-5: a
// delegate node with a pack-YAML model and no step row resolves the YAML
// model (previously: "" → inherit, with no way to tier via the pack).
func TestResolveFlowNodeModelFallsBackToYamlPackDefault(t *testing.T) {
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())

	got := svc.resolveFlowNodeModel(context.Background(), "", agentpack.FlowNode{
		ID: "plan_writer", Behavior: "agent.delegate", Agent: "agents/coder.md", Model: "grok-4-5",
	})
	if got != "grok-4-5" {
		t.Fatalf("resolveFlowNodeModel = %q, want YAML pack default grok-4-5", got)
	}
}

// TestResolveFlowNodeModelDbRowBeatsYamlPackDefault pins Task-320 T-1: the
// admin's per-installation step row still wins over the pack default for
// non-planner delegate nodes.
func TestResolveFlowNodeModelDbRowBeatsYamlPackDefault(t *testing.T) {
	catalog := newInteractiveCatalog()
	catalog.steps["wf-feature"] = append(catalog.steps["wf-feature"],
		Step{
			ID:       "flowpilot_core_flow_pack__task_harness__plan_writer",
			Name:     "Task: Plan Writer",
			NodeID:   "plan_writer",
			AgentRef: "agents/coder.md",
			Model:    "claude-sonnet-4-5",
		},
	)
	svc := newInteractiveService(DefaultProviderRegistry(), catalog, newFakeWorkflowStore())

	got := svc.resolveFlowNodeModel(context.Background(), "", agentpack.FlowNode{
		ID: "plan_writer", Behavior: "agent.delegate", Agent: "agents/coder.md", Model: "grok-4-5",
	})
	if got != "claude-sonnet-4-5" {
		t.Fatalf("resolveFlowNodeModel = %q, want admin row claude-sonnet-4-5 over YAML grok-4-5", got)
	}
}

// TestResolveFlowNodeModelPlannerHonorsYamlIgnoresRow pins Task-320's scoped
// CA-616 guard: the planner honors an explicit pack-YAML tier, but a (stale,
// seeded) DB step row must never pin it — empty YAML still inherits ("").
func TestResolveFlowNodeModelPlannerHonorsYamlIgnoresRow(t *testing.T) {
	catalog := newInteractiveCatalog()
	catalog.steps["wf-feature"] = append(catalog.steps["wf-feature"],
		Step{
			ID:       "flowpilot_core_flow_pack__task_harness__preflight_contract_plan",
			Name:     "Task: Scout",
			NodeID:   "preflight_contract_plan",
			AgentRef: "agents/contract-planner.md",
			Model:    "gpt-5.4",
		},
	)
	svc := newInteractiveService(DefaultProviderRegistry(), catalog, newFakeWorkflowStore())

	got := svc.resolveFlowNodeModel(context.Background(), "", agentpack.FlowNode{
		ID: "preflight_contract_plan", Behavior: "agent.delegate", Agent: "agents/contract-planner.md", Model: "grok-4-1-fast",
	})
	if got != "grok-4-1-fast" {
		t.Fatalf("planner resolveFlowNodeModel = %q, want YAML tier grok-4-1-fast (row gpt-5.4 must be skipped)", got)
	}
	got = svc.resolveFlowNodeModel(context.Background(), "", agentpack.FlowNode{
		ID: "preflight_contract_plan", Behavior: "agent.delegate", Agent: "agents/contract-planner.md",
	})
	if got != "" {
		t.Fatalf("planner resolveFlowNodeModel = %q, want empty (inherit) despite legacy row gpt-5.4 (CA-616)", got)
	}
}

// TestRecordFromWorkflowRowCarriesNodeModel pins Task-320 T-6: a DB-backed
// (cloned) flow's admin-set step model lands on FlowNode.Model so resolution
// treats it exactly like a pack-YAML default.
func TestRecordFromWorkflowRowCarriesNodeModel(t *testing.T) {
	nodeID, behavior, agent, model := "plan_writer", "agent.delegate", "agents/coder.md", "claude-sonnet-4-5"
	row := dbWorkflowRow{
		ID: "wf-clone-1", Name: "cloned task-harness",
		WorkflowSteps: []dbWorkflowStepRow{
			{
				StepType:   "clone__plan_writer",
				OrderIndex: 0,
				StepDefinition: dbStepDefinitionRow{
					StepType: "clone__plan_writer", NodeID: &nodeID,
					BehaviorID: &behavior, AgentRef: &agent, Model: &model,
				},
			},
		},
	}
	rec := recordFromWorkflowRow(row)
	if len(rec.Definition.Nodes) != 1 {
		t.Fatalf("node count = %d, want 1", len(rec.Definition.Nodes))
	}
	if got := rec.Definition.Nodes[0].Model; got != "claude-sonnet-4-5" {
		t.Fatalf("node.Model = %q, want carried DB model claude-sonnet-4-5", got)
	}
}
