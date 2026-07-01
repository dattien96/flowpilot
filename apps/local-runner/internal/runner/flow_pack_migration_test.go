package runner

import (
	"context"
	"testing"
)

// TestArbitraryNodeStepTypeIsInertWithoutRunnerChange proves the CP-42 claim
// that a Flow Mode step using a step type unrelated to any known behavior
// alias runs through the generic path untouched — adding a new, unrelated
// step name requires a pack alias entry, not a runner code change
// (Task-180 T-6).
func TestArbitraryNodeStepTypeIsInertWithoutRunnerChange(t *testing.T) {
	if isCodingStepType("", "totally-custom-node-xyz") {
		t.Fatal("an unrelated step type must not be classified as a Coding step")
	}
	if isPlanStepType("", "totally-custom-node-xyz") {
		t.Fatal("an unrelated step type must not be classified as a Plan step")
	}

	workspace, _ := fcpFixture(t)
	store := newFakeWorkflowStore()
	store.seed("run-custom", []RuntimeWorkflowStep{
		{ID: "step-custom", StepType: "totally-custom-node-xyz", Status: StepStatusPending},
	})
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	rs := &interactiveRun{id: "run-custom", workspaceCwd: workspace}

	prompt := "do the custom thing"
	out := svc.injectFlowContextIfCoding(context.Background(), rs, "step-custom", prompt, prompt)
	if out != prompt {
		t.Errorf("custom step type must leave the prompt untouched, got: %q", out)
	}
}

// TestGenericFlowStepTypeClassifiesByBehaviorID is the regression test for
// BUG-NOTE-CP42 #7: a CP-42 generic flow node's step_type is one of the
// reusable dispatch categories seeded by the add_flow_engine_attrs_to_
// workflows migration (e.g. "flow-agent-delegate"), which
// NormalizeBehaviorID's alias table does not recognize at all — classifying
// by step_type alone left every UI-authored generic flow's coding/plan steps
// unclassifiable, disconnecting them from the Flow Mode context-handoff path.
// This proves a step with that step_type but a real BehaviorID set is
// correctly classified as a Coding step.
func TestGenericFlowStepTypeClassifiesByBehaviorID(t *testing.T) {
	if isCodingStepType("", "flow-agent-delegate") {
		t.Fatal("step_type alone (\"flow-agent-delegate\") must NOT classify as a Coding step — it's a dispatch category, not a behavior alias")
	}
	if !isCodingStepType("agent.delegate", "flow-agent-delegate") {
		t.Fatal("a generic flow step with behavior_id=agent.delegate must classify as a Coding step even though its step_type is the generic dispatch category")
	}
	if !isPlanStepType("context.produce", "flow-context-produce") {
		t.Fatal("a generic flow step with behavior_id=context.produce must classify as a Plan step even though its step_type is the generic dispatch category")
	}
}

// TestIsCoderRunMatchesIsAgentRoleForCoder proves the isCoderRun shim
// (Task-180 T-3: consolidate the four separate isAgentRole(_, "coder") call
// sites into one named function) is behavior-identical to the inline check
// it replaced.
func TestIsCoderRunMatchesIsAgentRoleForCoder(t *testing.T) {
	cases := []struct {
		name string
		rs   *interactiveRun
	}{
		{"nil run", nil},
		{"matches by role", &interactiveRun{role: "coder"}},
		{"matches by agentName", &interactiveRun{agentName: "Coder Agent"}},
		{"reviewer does not match", &interactiveRun{role: "reviewer"}},
		{"empty run does not match", &interactiveRun{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want := isAgentRole(tc.rs, "coder")
			got := isCoderRun(tc.rs)
			if got != want {
				t.Errorf("isCoderRun = %v, want %v (matching isAgentRole)", got, want)
			}
		})
	}
}

// TestLegacyPlanCodingStepTypesStillResolveThroughAliasTable proves the
// legacy CP-41 step-type strings keep working through the alias table
// without a literal string check in the active path (Task-180 T-6 migration
// coverage: old CP-41 Plan/Coding definitions still resolve).
func TestLegacyPlanCodingStepTypesStillResolveThroughAliasTable(t *testing.T) {
	for _, legacy := range []string{"plan", "planning", "design"} {
		if !isPlanStepType("", legacy) {
			t.Errorf("legacy step type %q must still resolve as a Plan step", legacy)
		}
	}
	for _, legacy := range []string{"coding", "implementation", "code"} {
		if !isCodingStepType("", legacy) {
			t.Errorf("legacy step type %q must still resolve as a Coding step", legacy)
		}
	}
}
