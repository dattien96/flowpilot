package runner

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// TestResolveEnabledContextSourceIDsStepOverridesFlow verifies Task-196 T-3's
// precedence: a node's own ContextSources (step-definition-level) is used
// when set.
func TestResolveEnabledContextSourceIDsStepOverridesFlow(t *testing.T) {
	def := agentpack.FlowDefinition{ID: "test-flow"}
	node := agentpack.FlowNode{
		ID:             "context",
		ContextSources: []string{"feature.history"},
	}
	got := resolveEnabledContextSourceIDs(def, node)
	if len(got) != 1 || got[0] != "feature.history" {
		t.Fatalf("got %v, want step-level [feature.history]", got)
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

// TestValidateFlowArtifactBindingsRejectsUnknownSourceInContextArtifactConfig
// is the regression test for BUG-269: a context_artifact.v1 binding's
// config_json.sources can carry an id the ContextSourceRegistry doesn't
// recognize (e.g. a typo, or a source retired after the instance was
// configured) — this must fail flow-load with a clear error naming the flow,
// node, and instance, exactly like ValidateFlowContextSources already does
// for the two older tiers, not silently degrade at resolve time.
func TestValidateFlowArtifactBindingsRejectsUnknownSourceInContextArtifactConfig(t *testing.T) {
	def := agentpack.FlowDefinition{
		ID: "test-flow",
		Nodes: []agentpack.FlowNode{
			{
				ID: "context",
				ArtifactBindings: []agentpack.FlowArtifactBinding{
					{
						Direction:          "output",
						ArtifactInstanceID: "history-only-context",
						Required:           true,
						ArtifactTypeID:     ArtifactTypeContext,
						ConfigJSON:         map[string]any{"sources": []any{"feature.history", "totally.unknown.source"}},
					},
				},
			},
		},
	}
	err := ValidateFlowArtifactBindings(def)
	if err == nil {
		t.Fatal("expected error for a context_artifact binding declaring an unknown source id")
	}
	if !strings.Contains(err.Error(), "totally.unknown.source") {
		t.Errorf("error should name the offending source id, got: %v", err)
	}
	if !strings.Contains(err.Error(), "history-only-context") {
		t.Errorf("error should name the offending instance id, got: %v", err)
	}
}

// TestValidateFlowArtifactBindingsAcceptsKnownSourcesInContextArtifactConfig
// verifies a context_artifact.v1 binding whose declared sources are all
// registered passes validation cleanly (mirrors
// TestValidateFlowContextSourcesAcceptsKnownSources for this tier).
func TestValidateFlowArtifactBindingsAcceptsKnownSourcesInContextArtifactConfig(t *testing.T) {
	def := agentpack.FlowDefinition{
		ID: "test-flow",
		Nodes: []agentpack.FlowNode{
			{
				ID: "context",
				ArtifactBindings: []agentpack.FlowArtifactBinding{
					{
						Direction:          "output",
						ArtifactInstanceID: "history-only-context",
						Required:           true,
						ArtifactTypeID:     ArtifactTypeContext,
						ConfigJSON:         map[string]any{"sources": []any{"feature.history"}},
					},
				},
			},
		},
	}
	if err := ValidateFlowArtifactBindings(def); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestValidateFlowArtifactBindingsIgnoresNonContextArtifactConfig verifies
// the new sources check only inspects context_artifact.v1 bindings — a
// file_artifact.v1 binding's unrelated config_json (paths, not sources) must
// never be mistaken for a source list.
func TestValidateFlowArtifactBindingsIgnoresNonContextArtifactConfig(t *testing.T) {
	def := agentpack.FlowDefinition{
		ID: "test-flow",
		Nodes: []agentpack.FlowNode{
			{
				ID: "coder",
				ArtifactBindings: []agentpack.FlowArtifactBinding{
					{
						Direction:          "input",
						ArtifactInstanceID: "spec-file",
						Required:           true,
						ArtifactTypeID:     ArtifactTypeFile,
						ConfigJSON:         map[string]any{"paths": []any{"README.md"}},
					},
				},
			},
		},
	}
	if err := ValidateFlowArtifactBindings(def); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestResolveEnabledContextSourceIDsOldCP44FlowUnaffectedByArtifactValidation
// is a backward-compat guard (Task-203): a flow using neither CP-45 artifact
// bindings nor step-level ContextSources has no ArtifactBindings at all, so
// ValidateFlowArtifactBindings and the D-6 precedence tier are both no-ops —
// resolveEnabledContextSourceIDs falls through to nil, the sentinel
// buildFlowContextPackage reads as "use defaultContextSourceIDs" (Task-194
// T-1/T-3), so the CP-44 default-set behavior is preserved end to end even
// though this function's own contract no longer matches a flow-level
// `contexts.<name>.sources` binding directly (that tier was retired).
func TestResolveEnabledContextSourceIDsOldCP44FlowUnaffectedByArtifactValidation(t *testing.T) {
	def := agentpack.FlowDefinition{ID: "legacy-flow"}
	node := agentpack.FlowNode{ID: "context"}

	if err := ValidateFlowArtifactBindings(def); err != nil {
		t.Fatalf("unexpected error validating a flow with no artifact bindings: %v", err)
	}
	if got := resolveEnabledContextSourceIDs(def, node); got != nil {
		t.Fatalf("got %v, want nil (falls through to defaultContextSourceIDs)", got)
	}
}
