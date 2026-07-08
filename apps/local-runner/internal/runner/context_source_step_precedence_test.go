package runner

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// TestResolveEnabledContextSourceIDsStepOverridesFlow verifies Task-196 T-3's
// precedence: a node's own ContextSources (step-definition-level) wins over
// the flow-level contexts.<name>.sources binding.
func TestResolveEnabledContextSourceIDsStepOverridesFlow(t *testing.T) {
	def := agentpack.FlowDefinition{
		ID: "test-flow",
		Contexts: map[string]agentpack.FlowContextBinding{
			"main_context": {Ref: "contexts/flow-context-package.yaml", Sources: []string{"chat.summary"}},
		},
	}
	node := agentpack.FlowNode{
		ID:             "context",
		Outputs:        map[string]string{"main_context": "flow_context_package.v1"},
		ContextSources: []string{"feature.history"},
	}
	got := resolveEnabledContextSourceIDs(def, node)
	if len(got) != 1 || got[0] != "feature.history" {
		t.Fatalf("got %v, want step-level [feature.history] to win over flow-level", got)
	}
}

// TestResolveEnabledContextSourceIDsFallsBackToFlowWhenStepUnset verifies the
// second precedence tier: no step-level selection falls back to the flow
// binding (Task-194 behavior, unchanged by Task-196).
func TestResolveEnabledContextSourceIDsFallsBackToFlowWhenStepUnset(t *testing.T) {
	def := agentpack.FlowDefinition{
		ID: "test-flow",
		Contexts: map[string]agentpack.FlowContextBinding{
			"main_context": {Ref: "contexts/flow-context-package.yaml", Sources: []string{"chat.summary"}},
		},
	}
	node := agentpack.FlowNode{
		ID:      "context",
		Outputs: map[string]string{"main_context": "flow_context_package.v1"},
	}
	got := resolveEnabledContextSourceIDs(def, node)
	if len(got) != 1 || got[0] != "chat.summary" {
		t.Fatalf("got %v, want flow-level [chat.summary]", got)
	}
}

// TestResolveEnabledContextSourceIDsFallsBackToDefaultWhenNeitherSet verifies
// the third precedence tier: neither step nor flow selects → nil (default set).
func TestResolveEnabledContextSourceIDsFallsBackToDefaultWhenNeitherSet(t *testing.T) {
	def := agentpack.FlowDefinition{ID: "test-flow"}
	node := agentpack.FlowNode{ID: "context"}
	got := resolveEnabledContextSourceIDs(def, node)
	if got != nil {
		t.Fatalf("got %v, want nil (default set)", got)
	}
}

// TestValidateFlowContextSourcesRejectsUnknownNodeLevelSource verifies
// Task-196's fail-fast requirement for step-definition-level sources, mirroring
// the existing flow-level check (Task-194 T-2).
func TestValidateFlowContextSourcesRejectsUnknownNodeLevelSource(t *testing.T) {
	def := agentpack.FlowDefinition{
		ID: "test-flow",
		Nodes: []agentpack.FlowNode{
			{ID: "context", ContextSources: []string{"totally.unknown.source"}},
		},
	}
	err := ValidateFlowContextSources(def)
	if err == nil {
		t.Fatal("expected error for unknown node-level context source id")
	}
}

// TestValidateFlowContextSourcesAcceptsKnownNodeLevelSources verifies a node
// whose declared ContextSources are all registered passes validation.
func TestValidateFlowContextSourcesAcceptsKnownNodeLevelSources(t *testing.T) {
	def := agentpack.FlowDefinition{
		ID: "test-flow",
		Nodes: []agentpack.FlowNode{
			{ID: "context", ContextSources: []string{"feature.history", "chat.summary"}},
		},
	}
	if err := ValidateFlowContextSources(def); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
