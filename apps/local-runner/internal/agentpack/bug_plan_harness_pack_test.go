package agentpack

import (
	"reflect"
	"testing"
)

// TestBugPlanHarnessPack (Task-324) pins the Bug+Plan tier picker entry:
// bug-plan-harness loads, validates, is selectable in /flow, cloneable,
// cap:5 (dual-loop per-phase budget, same as task-harness), and mirrors
// task-harness structurally — except the plan loop, which
// writes BUG docs (prompts/plan-bug.md + review-bug-plan.md, pathTemplates
// routed to 09-BugFix) instead of Task docs.
func TestBugPlanHarnessPack(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	bug, ok := findFlowByID(pack.Flows, "bug-plan-harness")
	if !ok {
		t.Fatal("bug-plan-harness flow missing from builtin pack")
	}
	task, ok := findFlowByID(pack.Flows, "task-harness")
	if !ok {
		t.Fatal("task-harness flow missing from builtin pack")
	}

	if err := ValidateFlowDefinition(bug); err != nil {
		t.Fatalf("ValidateFlowDefinition(bug-plan-harness) = %v, want nil", err)
	}

	found := false
	for _, in := range bug.Builtin.SelectableIn {
		if in == "flow" {
			found = true
		}
	}
	if !found {
		t.Fatalf("bug-plan-harness selectableIn = %v, want it to include flow", bug.Builtin.SelectableIn)
	}
	if !bug.Builtin.Cloneable {
		t.Fatal("bug-plan-harness must be cloneable:true (Task-324)")
	}
	if bug.Policy.Cap != 5 {
		t.Fatalf("bug-plan-harness policy.cap = %d, want 5 (same dual-loop per-phase budget as task-harness)", bug.Policy.Cap)
	}
	if bug.Builtin.ChatBaseline {
		t.Fatal("bug-plan-harness must not be chatBaseline")
	}

	if len(bug.Nodes) != 12 || len(task.Nodes) != 12 {
		t.Fatalf("bug-plan node count = %d, task-harness = %d, want 12/12", len(bug.Nodes), len(task.Nodes))
	}
	byID := func(nodes []FlowNode) map[string]FlowNode {
		m := make(map[string]FlowNode, len(nodes))
		for _, n := range nodes {
			m[n.ID] = n
		}
		return m
	}
	taskNodes := byID(task.Nodes)
	for _, b := range bug.Nodes {
		// CP-64 (reproduce-first-gate): the bug flows' TDD step is the
		// reproduce-first node; task-harness keeps the legacy empty-signature
		// node under the old id. Assert the CP-64 shape directly instead of
		// comparing it field-by-field against task-harness's test_signatures.
		if b.ID == "reproduce_test" {
			if b.Behavior != "agent.reproduce" || b.Agent != "agents/reproducer.md" || b.PromptTemplate != "prompts/reproduce-failing-test.md" {
				t.Fatalf("bug-plan-harness reproduce_test shape drifted: %+v", b)
			}
			if _, ok := taskNodes["test_signatures"]; !ok {
				t.Fatal("task-harness lost its legacy test_signatures node — CP-64 must not touch new-feature flows")
			}
			continue
		}
		r, ok := taskNodes[b.ID]
		if !ok {
			t.Fatalf("bug-plan-harness node %q has no task-harness counterpart", b.ID)
		}
		switch b.ID {
		case "plan_writer", "plan_reviewer":
			// Compared field-by-field below (prompts + BUG routing).
		default:
			bb, rr := b, r
			if !reflect.DeepEqual(bb, rr) {
				t.Fatalf("bug-plan-harness node %q differs from task-harness:\nbug: %+v\ntask: %+v", b.ID, b, r)
			}
		}
	}
	if !reflect.DeepEqual(renameReproduceEdgeEndpoints(bug.Edges), task.Edges) {
		t.Fatalf("bug-plan-harness edges differ from task-harness beyond the CP-64 rename:\nbug: %+v\ntask: %+v", bug.Edges, task.Edges)
	}
	if !reflect.DeepEqual(bug.AcceptanceNodes, task.AcceptanceNodes) {
		t.Fatalf("bug-plan-harness acceptance_nodes = %v, task-harness = %v, want identical",
			bug.AcceptanceNodes, task.AcceptanceNodes)
	}

	// Plan loop writes BUG docs, not Task docs.
	bugNodes := byID(bug.Nodes)
	writer := bugNodes["plan_writer"]
	if writer.PromptTemplate != "prompts/plan-bug.md" {
		t.Fatalf("plan_writer promptTemplate = %q, want prompts/plan-bug.md", writer.PromptTemplate)
	}
	reviewer := bugNodes["plan_reviewer"]
	if reviewer.PromptTemplate != "prompts/review-bug-plan.md" {
		t.Fatalf("plan_reviewer promptTemplate = %q, want prompts/review-bug-plan.md", reviewer.PromptTemplate)
	}
	const wantTemplate = "requirements/09-BugFix/todo/BUG-{{idx}}-{{slug}}.md"
	for _, tc := range []struct {
		nodeID    string
		direction string
	}{
		{"plan_writer", "output"},
		{"plan_reviewer", "input"},
	} {
		n := bugNodes[tc.nodeID]
		matched := false
		for _, b := range n.ArtifactBindings {
			if b.Direction != tc.direction || b.SlotName != "plan_md" {
				continue
			}
			tmpl, _ := b.ConfigJSON["pathTemplate"].(string)
			if tmpl != wantTemplate {
				t.Fatalf("%s %s plan_md pathTemplate = %q, want %q", tc.nodeID, tc.direction, tmpl, wantTemplate)
			}
			matched = true
		}
		if !matched {
			t.Fatalf("%s missing %s plan_md binding", tc.nodeID, tc.direction)
		}
	}
}
