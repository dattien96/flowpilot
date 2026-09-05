package agentpack

import (
	"strings"
	"testing"
)

// taskHarnessDefinition loads the CP-58 Task-305 built-in task-harness flow,
// failing the test when the pack no longer ships it.
func taskHarnessDefinition(t *testing.T) FlowDefinition {
	t.Helper()
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	for _, def := range pack.Flows {
		if def.ID == "task-harness" {
			return def
		}
	}
	t.Fatal("task-harness flow missing from builtin pack")
	return FlowDefinition{}
}

// TestTaskHarnessPackTopology (Task-305 T-6) pins the 12-node topology: two
// continue/back edges anchored at different nodes (plan loop + code loop),
// acceptance boundary including plan_synthesis, the plan_reviewer cohort
// shape, exactly two hub.inline nodes, and the plan loop wiring from Scout
// through draft context to the freeze scope lock.
func TestTaskHarnessPackTopology(t *testing.T) {
	def := taskHarnessDefinition(t)

	if len(def.Nodes) != 12 {
		t.Fatalf("task-harness node count = %d, want 12", len(def.Nodes))
	}

	var continueBack []FlowEdge
	for _, e := range def.Edges {
		if strings.EqualFold(strings.TrimSpace(e.Kind), "back") &&
			strings.EqualFold(strings.TrimSpace(e.When), "continue") {
			continueBack = append(continueBack, e)
		}
	}
	if len(continueBack) != 2 {
		t.Fatalf("task-harness continue/back edge count = %d (%+v), want 2", len(continueBack), continueBack)
	}
	froms := map[string]bool{}
	for _, e := range continueBack {
		froms[e.From] = true
	}
	if !froms["plan_synthesis"] || !froms["validate"] {
		t.Fatalf("continue/back anchors = %v, want plan_synthesis and validate", froms)
	}
	if continueBack[0].From == continueBack[1].From {
		t.Fatal("both continue/back edges anchor the same node; the plan and code loops must stay independent")
	}

	for _, want := range []string{"plan_synthesis", "validate", "synthesis", "audit"} {
		found := false
		for _, id := range def.AcceptanceNodes {
			if id == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("acceptance_nodes %v missing %q", def.AcceptanceNodes, want)
		}
	}

	planWriter := findNodeByID(t, def, "plan_writer")
	if planWriter.PromptTemplate != "prompts/plan-task.md" {
		t.Fatalf("plan_writer promptTemplate = %q, want prompts/plan-task.md", planWriter.PromptTemplate)
	}
	if planWriter.Lifecycle != "reinvoke" {
		t.Fatalf("plan_writer lifecycle = %q, want reinvoke (plan-loop session reuse)", planWriter.Lifecycle)
	}

	planReviewer := findNodeByID(t, def, "plan_reviewer")
	if planReviewer.Cohort != "plan" || planReviewer.Join != "all" {
		t.Fatalf("plan_reviewer cohort/join = %q/%q, want plan/all", planReviewer.Cohort, planReviewer.Join)
	}

	// Task-305 T-4: exactly two hub.inline nodes; the code-loop hub must not
	// swallow the plan loop's continue (Task-304 hub routing relies on both).
	hubCount := 0
	hubIDs := map[string]bool{}
	for _, n := range def.Nodes {
		if canonical, ok := NormalizeBehaviorID(n.Behavior); ok && canonical == "hub.inline" {
			hubCount++
			hubIDs[n.ID] = true
		}
	}
	if hubCount != 2 || !hubIDs["plan_synthesis"] || !hubIDs["synthesis"] {
		t.Fatalf("hub.inline nodes = %d %v, want exactly plan_synthesis + synthesis", hubCount, hubIDs)
	}

	// Plan loop wiring: scout -> context (draft) -> plan_writer -> plan_reviewer
	// -> plan_synthesis, then approval flows into the freeze scope lock.
	wantEdges := [][4]string{
		{"preflight_contract_plan", "context", "done", "forward"},
		{"context", "plan_writer", "done", "forward"},
		{"plan_writer", "plan_reviewer", "done", "forward"},
		{"plan_reviewer", "plan_synthesis", "done", "forward"},
		{"plan_synthesis", "preflight_contract_freeze", "done", "forward"},
		{"preflight_contract_freeze", "test_signatures", "done", "forward"},
		{"test_signatures", "implement", "done", "forward"},
		{"synthesis", "audit", "done", "forward"},
		{"audit", "done", "done", "forward"},
	}
	for _, w := range wantEdges {
		if !hasEdge(def.Edges, w[0], w[1], w[2], w[3]) {
			t.Fatalf("task-harness missing edge %s -> %s (when=%s kind=%s)", w[0], w[1], w[2], w[3])
		}
	}

	// test_signatures sits exactly once in the code chain, never inside the
	// plan loop (Task-305 T-6: reviewer sees the plan, not signatures).
	tsCount := 0
	for _, n := range def.Nodes {
		if n.ID == "test_signatures" {
			tsCount++
		}
	}
	if tsCount != 1 {
		t.Fatalf("test_signatures node count = %d, want exactly 1", tsCount)
	}
}

// TestTaskHarnessValidateFlowDefinition (Task-305) proves the dual-loop pack
// passes the CP-55 safety topology with the Task-304 validator widening: two
// continue/back edges load, and the code writers (test_signatures/implement)
// stay under the freeze-dominance + acceptance contract.
func TestTaskHarnessValidateFlowDefinition(t *testing.T) {
	def := taskHarnessDefinition(t)
	if err := ValidateFlowDefinition(def); err != nil {
		t.Fatalf("ValidateFlowDefinition(task-harness) = %v, want nil", err)
	}
	selectable := false
	for _, s := range def.Builtin.SelectableIn {
		if s == "flow" {
			selectable = true
		}
	}
	if !selectable {
		t.Fatalf("task-harness selectableIn = %v, want it to include flow", def.Builtin.SelectableIn)
	}
	if !def.Builtin.Cloneable {
		t.Fatal("task-harness must be cloneable:true (Task-305 T-1)")
	}
}

// --- helpers shared by the harness pack tests ---

func findNodeByID(t *testing.T, def FlowDefinition, id string) FlowNode {
	t.Helper()
	for _, n := range def.Nodes {
		if n.ID == id {
			return n
		}
	}
	t.Fatalf("flow %q has no node %q", def.ID, id)
	return FlowNode{}
}

func hasEdge(edges []FlowEdge, from, to, when, kind string) bool {
	for _, e := range edges {
		if e.From == from && e.To == to && strings.EqualFold(e.When, when) && strings.EqualFold(e.Kind, kind) {
			return true
		}
	}
	return false
}
