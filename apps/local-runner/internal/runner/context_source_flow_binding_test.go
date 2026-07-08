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
// entirely so the default set applies).
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
		"    behavior: context.produce\n" +
		"    outputs:\n" +
		"      main_context: flow_context_package.v1\n"
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

// TestFlowWithoutSourcesUsesDefaultSet verifies a flow that declares no
// `contexts.<name>.sources` resolves to the default built-in set (Task-194 T-3).
func TestFlowWithoutSourcesUsesDefaultSet(t *testing.T) {
	def := parseTestFlow(t, minimalContextFlowYAML(""))
	node := def.Nodes[0]
	ids := resolveEnabledContextSourceIDs(def, node)
	if ids != nil {
		t.Fatalf("expected nil (default set) for a flow with no sources binding, got %v", ids)
	}
}

// TestFlowWithExplicitSourceSubset verifies a flow that declares a source
// subset resolves to exactly that subset (Task-194 T-1/T-4).
func TestFlowWithExplicitSourceSubset(t *testing.T) {
	def := parseTestFlow(t, minimalContextFlowYAML("      - feature.history\n"))
	node := def.Nodes[0]
	ids := resolveEnabledContextSourceIDs(def, node)
	want := []string{"feature.history"}
	if len(ids) != len(want) || ids[0] != want[0] {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
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

// TestRagHarnessDeclaredSourcesMatchDefaultOutput verifies the built-in
// rag-harness flow's explicit `sources:` declaration (added by Task-194 T-5)
// resolves to the same set BuildFlowContextPackage's default already uses —
// documentation-as-config, not a behavior change.
func TestRagHarnessDeclaredSourcesMatchDefaultOutput(t *testing.T) {
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
	ids := resolveEnabledContextSourceIDs(def, contextNode)
	if len(ids) != len(defaultContextSourceIDs) {
		t.Fatalf("declared sources %v do not match default set %v in length", ids, defaultContextSourceIDs)
	}
	declared := make(map[string]bool, len(ids))
	for _, id := range ids {
		declared[id] = true
	}
	for _, id := range defaultContextSourceIDs {
		if !declared[id] {
			t.Errorf("rag-harness declared sources %v missing default id %q", ids, id)
		}
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
