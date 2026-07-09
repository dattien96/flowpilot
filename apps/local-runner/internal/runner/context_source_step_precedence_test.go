package runner

import (
	"strings"
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

// TestValidateFlowArtifactBindingsFailsOnMissingRequiredInstance verifies
// Task-203's core DOD: a REQUIRED binding whose instance no longer resolves
// (ArtifactTypeID left empty by recordFromWorkflowRow on a stale FK — SD-23
// F-1) fails flow-load with a clear, actionable error, never a silent no-op.
func TestValidateFlowArtifactBindingsFailsOnMissingRequiredInstance(t *testing.T) {
	def := agentpack.FlowDefinition{
		ID: "test-flow",
		Nodes: []agentpack.FlowNode{
			{
				ID: "context",
				ArtifactBindings: []agentpack.FlowArtifactBinding{
					{Direction: "output", ArtifactInstanceID: "deleted-instance", Required: true, ArtifactTypeID: ""},
				},
			},
		},
	}
	err := ValidateFlowArtifactBindings(def)
	if err == nil {
		t.Fatal("expected error for a required binding to a missing artifact instance")
	}
	if !strings.Contains(err.Error(), "deleted-instance") {
		t.Fatalf("expected error to name the dangling instance id, got: %v", err)
	}
}

// TestValidateFlowArtifactBindingsDegradesOnMissingOptionalInstance verifies
// the other half of SD-23 F-1: an OPTIONAL binding whose instance no longer
// resolves must NOT fail flow-load — it degrades softly at resolve time
// instead (resolveArtifactBoundContextSources / resolveInputArtifactPrompt
// both already skip an empty-type binding).
func TestValidateFlowArtifactBindingsDegradesOnMissingOptionalInstance(t *testing.T) {
	def := agentpack.FlowDefinition{
		ID: "test-flow",
		Nodes: []agentpack.FlowNode{
			{
				ID: "coder",
				ArtifactBindings: []agentpack.FlowArtifactBinding{
					{Direction: "input", ArtifactInstanceID: "deleted-instance", Required: false, ArtifactTypeID: ""},
				},
			},
		},
	}
	if err := ValidateFlowArtifactBindings(def); err != nil {
		t.Fatalf("expected an optional missing binding to pass validation (soft degrade), got: %v", err)
	}
}

// TestValidateFlowArtifactBindingsAcceptsResolvedBindings verifies a flow
// with fully-resolved bindings (the common case) never fails validation.
func TestValidateFlowArtifactBindingsAcceptsResolvedBindings(t *testing.T) {
	def := agentpack.FlowDefinition{
		ID: "test-flow",
		Nodes: []agentpack.FlowNode{
			{
				ID: "context",
				ArtifactBindings: []agentpack.FlowArtifactBinding{
					{Direction: "output", ArtifactInstanceID: "ok-instance", Required: true, ArtifactTypeID: ArtifactTypeContext},
				},
			},
		},
	}
	if err := ValidateFlowArtifactBindings(def); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestResolveEnabledContextSourceIDsOldCP44FlowUnaffectedByArtifactValidation
// is a backward-compat guard (Task-203): a flow using ONLY the pre-CP-45
// mechanisms (flow-level contexts.sources, step-level ContextSources, or
// neither) has no ArtifactBindings at all, so ValidateFlowArtifactBindings
// and the D-6 precedence tier are both no-ops — the CP-44 fallback chain
// resolves exactly as it always did.
func TestResolveEnabledContextSourceIDsOldCP44FlowUnaffectedByArtifactValidation(t *testing.T) {
	def := agentpack.FlowDefinition{
		ID: "legacy-flow",
		Contexts: map[string]agentpack.FlowContextBinding{
			"main_context": {Ref: "contexts/flow-context-package.yaml", Sources: []string{"feature.history", "chat.summary", "source.excerpt"}},
		},
	}
	node := agentpack.FlowNode{ID: "context", Outputs: map[string]string{"main_context": "flow_context_package.v1"}}

	if err := ValidateFlowArtifactBindings(def); err != nil {
		t.Fatalf("unexpected error validating a flow with no artifact bindings: %v", err)
	}
	got := resolveEnabledContextSourceIDs(def, node)
	want := []string{"feature.history", "chat.summary", "source.excerpt"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i, id := range want {
		if got[i] != id {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}
