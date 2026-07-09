package runner

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"

	"flowpilot-runner/internal/agentpack"
)

// minimalContextFlowYAML builds a tiny valid flow YAML with one inline
// context.produce entry node whose `contexts.main_context` binding declares
// sourcesYAML (a pre-formatted YAML list block, or "" to omit the field
// entirely). This binding is validated by ValidateFlowContextSources
// (Task-194 T-2) but — since the retirement of the node-`outputs:`-to-
// `contexts.<name>` matching tier (superseded by CP-45's artifact-binding
// tier) — no longer affects what resolveEnabledContextSourceIDs returns for
// this flow's context node; see context_sources_builtin.go's doc comment.
func minimalContextFlowYAML(sourcesYAML string) string {
	contextsBlock := "contexts:\n  main_context:\n    ref: contexts/flow-context-package.yaml\n"
	if sourcesYAML != "" {
		contextsBlock += "    sources:\n" + sourcesYAML
	}
	return "id: test-context-flow\n" +
		"version: 1\n" +
		"mode: flow\n" +
		contextsBlock +
		"nodes:\n" +
		"  - id: context\n" +
		"    run: inline\n" +
		"    lifecycle: once\n" +
		"    behavior: context.produce\n"
}

func parseTestFlow(t *testing.T, yamlContent string) agentpack.FlowDefinition {
	t.Helper()
	fsys := fstest.MapFS{"flow.yaml": &fstest.MapFile{Data: []byte(yamlContent)}}
	def, err := agentpack.LoadFlowFS(fsys, "flow.yaml")
	if err != nil {
		t.Fatalf("LoadFlowFS: %v", err)
	}
	return def
}

// TestUnknownSourceIDFailsFlowLoad verifies a flow declaring a source id the
// registry doesn't know about fails validation with a clear error rather than
// silently running without it (Task-194 T-2).
func TestUnknownSourceIDFailsFlowLoad(t *testing.T) {
	def := parseTestFlow(t, minimalContextFlowYAML("      - totally.unknown.source\n"))
	err := ValidateFlowContextSources(def)
	if err == nil {
		t.Fatal("expected error for unknown context source id")
	}
	if !strings.Contains(err.Error(), "totally.unknown.source") {
		t.Errorf("error should name the offending source id, got: %v", err)
	}
	if !strings.Contains(err.Error(), "main_context") {
		t.Errorf("error should name the offending context binding, got: %v", err)
	}
}

// TestValidateFlowContextSourcesAcceptsKnownSources verifies a flow whose
// declared sources are all registered passes validation cleanly.
func TestValidateFlowContextSourcesAcceptsKnownSources(t *testing.T) {
	def := parseTestFlow(t, minimalContextFlowYAML("      - feature.history\n      - chat.summary\n      - source.excerpt\n"))
	if err := ValidateFlowContextSources(def); err != nil {
		t.Fatalf("unexpected error for known source ids: %v", err)
	}
}

// TestRagHarnessContextNodeFallsThroughToDefaultSet verifies the built-in
// rag-harness flow's context node — which declares no step-level
// ContextSources and no artifact binding — resolves to nil from
// resolveEnabledContextSourceIDs, the sentinel buildFlowContextPackage reads
// as "use defaultContextSourceIDs" (Task-194 T-1/T-3). The flow's own
// `contexts.main_context.sources:` declaration still passes
// ValidateFlowContextSources but no longer feeds resolution (retired tier).
func TestRagHarnessContextNodeFallsThroughToDefaultSet(t *testing.T) {
	pack, err := agentpack.LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	var def agentpack.FlowDefinition
	found := false
	for _, f := range pack.Flows {
		if f.ID == "rag-harness" {
			def = f
			found = true
			break
		}
	}
	if !found {
		t.Fatal("rag-harness flow not found in embedded pack")
	}
	if err := ValidateFlowContextSources(def); err != nil {
		t.Fatalf("rag-harness declared sources failed validation: %v", err)
	}

	var contextNode agentpack.FlowNode
	for _, n := range def.Nodes {
		if n.ID == "context" {
			contextNode = n
			break
		}
	}
	if ids := resolveEnabledContextSourceIDs(def, contextNode); ids != nil {
		t.Fatalf("resolveEnabledContextSourceIDs = %v, want nil (falls through to defaultContextSourceIDs)", ids)
	}
}

// TestBehaviorContextProduceRespectsExplicitContextSourceIDs verifies the
// behavior handler actually restricts retrieval to the caller-supplied
// source list, not just the default set (Task-194 T-4 end-to-end).
func TestBehaviorContextProduceRespectsExplicitContextSourceIDs(t *testing.T) {
	workspace, _ := fcpFixture(t)
	out, err := behaviorContextProduce(context.Background(), BehaviorInput{
		WorkspaceCwd:     workspace,
		WorkflowRunID:    "run-194",
		StepRunID:        "plan-194",
		Prompt:           "agent-flow-engine",
		ContextSourceIDs: []string{"feature.history"},
	})
	if err != nil {
		t.Fatalf("behaviorContextProduce: %v", err)
	}
	pkg, ok := out.Payload["package"].(FlowContextPackage)
	if !ok {
		t.Fatal("expected FlowContextPackage in payload")
	}
	if pkg.HistoryBlock == "" {
		t.Error("expected HistoryBlock populated (feature.history was enabled)")
	}
	for _, s := range pkg.Sections {
		if ContextSourceID(s.SourceType) != ContextSourceFeatureHistory {
			t.Errorf("expected only feature.history section, also found %q", s.SourceType)
		}
	}
	if len(pkg.Sections) != 1 {
		t.Errorf("expected exactly 1 section (feature.history only), got %d: %#v", len(pkg.Sections), pkg.Sections)
	}
}
