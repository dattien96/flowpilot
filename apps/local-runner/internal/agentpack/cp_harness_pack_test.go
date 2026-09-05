package agentpack

import (
	"strings"
	"testing"
)

// cpHarnessDefinition loads a CP-58 Task-306 built-in flow by id, failing the
// test when the pack no longer ships it.
func cpHarnessDefinition(t *testing.T, id string) FlowDefinition {
	t.Helper()
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	for _, def := range pack.Flows {
		if def.ID == id {
			return def
		}
	}
	t.Fatalf("%s flow missing from builtin pack", id)
	return FlowDefinition{}
}

// TestCpHarnessPackTopology (Task-306 T-6) pins the slice-only default: 7
// nodes, plan loop only (no code loop), NO freeze/test_signatures/coding
// anywhere, exactly two agent.* writer-shaped nodes (cp_plan_writer +
// task_splitter), acceptance boundary [cp_synthesis, audit], and the
// plan_md/cp_md/task_md file_artifact bindings (Task-307).
func TestCpHarnessPackTopology(t *testing.T) {
	def := cpHarnessDefinition(t, "cp-harness")

	if len(def.Nodes) != 7 {
		t.Fatalf("cp-harness node count = %d, want 7 (slice-only)", len(def.Nodes))
	}
	for _, want := range []string{"preflight_contract_plan", "context", "cp_plan_writer", "cp_reviewer", "cp_synthesis", "task_splitter", "audit"} {
		if !nodeExists(def, want) {
			t.Fatalf("cp-harness missing node %q", want)
		}
	}
	for _, forbidden := range []string{"test_signatures", "implement", "validate", "preflight_contract_freeze", "reviewer", "synthesis"} {
		if nodeExists(def, forbidden) {
			t.Fatalf("slice-only cp-harness must not declare coding-chain node %q (Task-306 T-1)", forbidden)
		}
	}

	wantEdges := [][4]string{
		{"preflight_contract_plan", "context", "done", "forward"},
		{"context", "cp_plan_writer", "done", "forward"},
		{"cp_plan_writer", "cp_reviewer", "done", "forward"},
		{"cp_reviewer", "cp_synthesis", "done", "forward"},
		{"cp_synthesis", "task_splitter", "done", "forward"},
		{"task_splitter", "audit", "done", "forward"},
		{"audit", "done", "done", "forward"},
	}
	for _, w := range wantEdges {
		if !hasEdge(def.Edges, w[0], w[1], w[2], w[3]) {
			t.Fatalf("cp-harness missing edge %s -> %s (when=%s kind=%s)", w[0], w[1], w[2], w[3])
		}
	}

	backEdges := 0
	for _, e := range def.Edges {
		if strings.EqualFold(strings.TrimSpace(e.Kind), "back") &&
			strings.EqualFold(strings.TrimSpace(e.When), "continue") {
			backEdges++
			if e.From != "cp_synthesis" || e.To != "cp_plan_writer" {
				t.Fatalf("cp-harness continue/back edge = %s -> %s, want cp_synthesis -> cp_plan_writer", e.From, e.To)
			}
		}
	}
	if backEdges != 1 {
		t.Fatalf("cp-harness continue/back edge count = %d, want 1 (plan loop only)", backEdges)
	}

	if len(def.AcceptanceNodes) != 2 || !acceptanceIncludes(def, "cp_synthesis") || !acceptanceIncludes(def, "audit") {
		t.Fatalf("cp-harness acceptance_nodes = %v, want exactly [cp_synthesis, audit]", def.AcceptanceNodes)
	}

	// Delegate-shaped nodes are exactly the scout + plan writer + reviewer +
	// splitter (agent.delegate per the documented deviation; no agent.code
	// anywhere in slice-only, so CP-55's writer/freeze contract holds vacuously).
	delegateCount := 0
	for _, n := range def.Nodes {
		if canonical, ok := NormalizeBehaviorID(n.Behavior); ok && canonical == "agent.delegate" {
			delegateCount++
		}
	}
	if delegateCount != 4 {
		t.Fatalf("cp-harness agent.delegate node count = %d, want 4 (scout, cp_plan_writer, cp_reviewer, task_splitter)", delegateCount)
	}

	cpw := findNodeByID(t, def, "cp_plan_writer")
	if cpw.PromptTemplate != "prompts/plan-cp.md" || cpw.Lifecycle != "reinvoke" {
		t.Fatalf("cp_plan_writer prompt/lifecycle = %q/%q, want plan-cp.md/reinvoke", cpw.PromptTemplate, cpw.Lifecycle)
	}
	if out := requiredOutputBinding(t, cpw, "cp_md"); out == nil || out.ArtifactTypeID != "file_artifact.v1" {
		t.Fatalf("cp_plan_writer missing required file_artifact OUTPUT binding for cp_md: %+v", cpw.ArtifactBindings)
	}
	cpr := findNodeByID(t, def, "cp_reviewer")
	if cpr.Cohort != "plan" || cpr.Join != "all" {
		t.Fatalf("cp_reviewer cohort/join = %q/%q, want plan/all", cpr.Cohort, cpr.Join)
	}
	if in := requiredInputBinding(t, cpr, "cp_md"); in == nil {
		t.Fatalf("cp_reviewer missing required INPUT binding for cp_md: %+v", cpr.ArtifactBindings)
	}
	ts := findNodeByID(t, def, "task_splitter")
	if ts.PromptTemplate != "prompts/task-splitter.md" {
		t.Fatalf("task_splitter promptTemplate = %q, want prompts/task-splitter.md", ts.PromptTemplate)
	}
	if in := requiredInputBinding(t, ts, "cp_md"); in == nil {
		t.Fatalf("task_splitter missing required INPUT binding for cp_md: %+v", ts.ArtifactBindings)
	}
	if out := requiredOutputBinding(t, ts, "task_md"); out == nil || out.ArtifactTypeID != "file_artifact.v1" {
		t.Fatalf("task_splitter missing required file_artifact OUTPUT binding for task_md: %+v", ts.ArtifactBindings)
	}
}

// TestCpHarnessSmokePackTopology (Task-306 T-6) pins the opt-in 13-node
// variant: same plan loop + task_splitter plus the full first-Task coding
// chain, two continue/back edges anchored at cp_synthesis and validate, and
// the extended acceptance boundary.
func TestCpHarnessSmokePackTopology(t *testing.T) {
	def := cpHarnessDefinition(t, "cp-harness-smoke")

	if len(def.Nodes) != 13 {
		t.Fatalf("cp-harness-smoke node count = %d, want 13", len(def.Nodes))
	}
	for _, want := range []string{"test_signatures", "implement", "validate", "preflight_contract_freeze", "reviewer", "synthesis", "audit"} {
		if !nodeExists(def, want) {
			t.Fatalf("cp-harness-smoke missing coding-chain node %q", want)
		}
	}

	backEdges := map[string]string{}
	for _, e := range def.Edges {
		if strings.EqualFold(strings.TrimSpace(e.Kind), "back") &&
			strings.EqualFold(strings.TrimSpace(e.When), "continue") {
			backEdges[e.From] = e.To
		}
	}
	if backEdges["cp_synthesis"] != "cp_plan_writer" || backEdges["validate"] != "implement" {
		t.Fatalf("cp-harness-smoke continue/back anchors = %v, want cp_synthesis->cp_plan_writer + validate->implement", backEdges)
	}

	for _, want := range []string{"cp_synthesis", "validate", "synthesis", "audit"} {
		if !acceptanceIncludes(def, want) {
			t.Fatalf("cp-harness-smoke acceptance_nodes %v missing %q", def.AcceptanceNodes, want)
		}
	}

	if err := ValidateFlowDefinition(def); err != nil {
		t.Fatalf("ValidateFlowDefinition(cp-harness-smoke) = %v, want nil", err)
	}
}

// TestCpHarnessSelectableAndCloneable locks the picker contract: cp-harness is
// selectable in flow mode and both harness flows are cloneable.
func TestCpHarnessSelectableAndCloneable(t *testing.T) {
	def := cpHarnessDefinition(t, "cp-harness")
	if !def.Builtin.Cloneable {
		t.Fatal("cp-harness must be cloneable:true (Task-306)")
	}
	selectable := false
	for _, s := range def.Builtin.SelectableIn {
		if s == "flow" {
			selectable = true
		}
	}
	if !selectable {
		t.Fatalf("cp-harness selectableIn = %v, want it to include flow", def.Builtin.SelectableIn)
	}
	smoke := cpHarnessDefinition(t, "cp-harness-smoke")
	if len(smoke.Builtin.SelectableIn) != 0 {
		t.Fatalf("cp-harness-smoke selectableIn = %v, want empty (opt-in only, Task-306 T-3)", smoke.Builtin.SelectableIn)
	}
}

// --- shared helpers (used by task_harness_pack_test.go too) ---

func nodeExists(def FlowDefinition, id string) bool {
	for _, n := range def.Nodes {
		if n.ID == id {
			return true
		}
	}
	return false
}

func acceptanceIncludes(def FlowDefinition, id string) bool {
	for _, a := range def.AcceptanceNodes {
		if a == id {
			return true
		}
	}
	return false
}

func requiredOutputBinding(t *testing.T, node FlowNode, slot string) *FlowArtifactBinding {
	t.Helper()
	for i := range node.ArtifactBindings {
		b := node.ArtifactBindings[i]
		if b.Direction == "output" && b.SlotName == slot && b.Required {
			return &b
		}
	}
	return nil
}

func requiredInputBinding(t *testing.T, node FlowNode, slot string) *FlowArtifactBinding {
	t.Helper()
	for i := range node.ArtifactBindings {
		b := node.ArtifactBindings[i]
		if b.Direction == "input" && b.SlotName == slot && b.Required {
			return &b
		}
	}
	return nil
}
