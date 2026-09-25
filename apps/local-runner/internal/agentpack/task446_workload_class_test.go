package agentpack

import (
	"strings"
	"testing"
	"testing/fstest"
)

// Task-446 (CP-87 P-2): workloadClass schema on FlowNode — three values only,
// provider-backed nodes only, fail-closed on misuse.

func flowWithNodeClass(behavior, class string) FlowDefinition {
	return FlowDefinition{ID: "f", Nodes: []FlowNode{{
		ID: "n1", Behavior: behavior, WorkloadClass: WorkloadClass(class),
	}}}
}

func TestTask446_WorkloadClassValidation(t *testing.T) {
	for _, class := range []string{"scan", "high_reasoning", "coding"} {
		if err := ValidateFlowDefinition(flowWithNodeClass("agent.delegate", class)); err != nil {
			t.Fatalf("provider node with class %q rejected: %v", class, err)
		}
	}
	if err := ValidateFlowDefinition(flowWithNodeClass("agent.delegate", "banana")); err == nil {
		t.Fatal("unknown workloadClass value accepted")
	}
	for _, behavior := range []string{"hub.inline", "context.produce", "user.confirm", ""} {
		if err := ValidateFlowDefinition(flowWithNodeClass(behavior, "scan")); err == nil {
			t.Fatalf("workloadClass on non-provider node behavior %q accepted", behavior)
		}
	}
	// Backward compat: flows authored before CP-87 (stored/user-authored rows
	// resolve through ValidateFlowDefinition too) may omit the class on a
	// provider-backed node — the routing gate, not the loader, is where a
	// missing class blocks auto-rotation.
	if err := ValidateFlowDefinition(flowWithNodeClass("agent.delegate", "")); err != nil {
		t.Fatalf("provider node without class rejected by shared validator: %v", err)
	}
}

func TestTask446_WorkloadClassParsesFromYAML(t *testing.T) {
	fsys := fstest.MapFS{
		"flows/f.yaml": &fstest.MapFile{Data: []byte(`
id: f
nodes:
  - id: coder
    behavior: agent.delegate
    workloadClass: coding
`)},
	}
	def, err := LoadFlowFS(fsys, "flows/f.yaml")
	if err != nil {
		t.Fatalf("LoadFlowFS: %v", err)
	}
	if len(def.Nodes) != 1 || def.Nodes[0].WorkloadClass != WorkloadCoding {
		t.Fatalf("node.WorkloadClass = %+v, want coding", def.Nodes)
	}
}

func TestTask446_AllProviderNodesClassified(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	if len(pack.Flows) == 0 {
		t.Fatal("builtin pack has no flows")
	}
	var providerNodes int
	for _, flow := range pack.Flows {
		for _, node := range flow.Nodes {
			canonical, known := NormalizeBehaviorID(node.Behavior)
			if ProviderBackedBehavior(canonical) {
				providerNodes++
				if !ValidWorkloadClass(node.WorkloadClass) {
					t.Fatalf("builtin flow %q provider node %q missing/invalid workloadClass %q", flow.ID, node.ID, node.WorkloadClass)
				}
			} else if node.WorkloadClass != "" {
				t.Fatalf("builtin flow %q non-provider node %q (behavior %q, known=%v) declares workloadClass %q", flow.ID, node.ID, node.Behavior, known, node.WorkloadClass)
			}
		}
	}
	if providerNodes == 0 {
		t.Fatal("no provider-backed nodes found — test would be vacuous")
	}
}

func TestTask446_BuiltinPackRequiresProviderClass(t *testing.T) {
	// The strict check is pack-author-facing, not shared-validator-facing:
	// ValidateFlowWorkloadClasses fails a flow whose provider node omits the
	// class, but stays silent on a correctly-annotated one.
	bare := FlowDefinition{ID: "f", Nodes: []FlowNode{{ID: "coder", Behavior: "agent.code"}}}
	if err := ValidateFlowWorkloadClasses(bare); err == nil || !strings.Contains(err.Error(), "workloadClass") {
		t.Fatalf("missing class on provider node, err = %v", err)
	}
	annotated := FlowDefinition{ID: "f", Nodes: []FlowNode{{ID: "coder", Behavior: "agent.code", WorkloadClass: WorkloadCoding}}}
	if err := ValidateFlowWorkloadClasses(annotated); err != nil {
		t.Fatalf("annotated provider node rejected: %v", err)
	}
	// reproduce is a provider turn too (BehaviorScopeDelegate alias family).
	repro := FlowDefinition{ID: "f", Nodes: []FlowNode{{ID: "r", Behavior: "agent.reproduce"}}}
	if err := ValidateFlowWorkloadClasses(repro); err == nil {
		t.Fatal("agent.reproduce without class accepted")
	}
}
