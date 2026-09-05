package agentpack

import (
	"reflect"
	"testing"
)

// TestBugHarnessPackClone (CP-58 Task-305 T-5, operator option (a) 2026-09-04)
// pins the Bug tier picker entry: bug-harness loads, is selectable in /flow,
// cloneable, cap:3, and stays a byte-identical clone of rag-harness except
// id/description (rag-harness keeps the chatBaseline role).
func TestBugHarnessPackClone(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	bug, ok := findFlowByID(pack.Flows, "bug-harness")
	if !ok {
		t.Fatal("bug-harness flow missing from builtin pack")
	}
	rag, ok := findFlowByID(pack.Flows, "rag-harness")
	if !ok {
		t.Fatal("rag-harness flow missing from builtin pack")
	}

	if err := ValidateFlowDefinition(bug); err != nil {
		t.Fatalf("ValidateFlowDefinition(bug-harness) = %v, want nil", err)
	}

	found := false
	for _, in := range bug.Builtin.SelectableIn {
		if in == "flow" {
			found = true
		}
	}
	if !found {
		t.Fatalf("bug-harness selectableIn = %v, want it to include flow", bug.Builtin.SelectableIn)
	}
	if !bug.Builtin.Cloneable {
		t.Fatal("bug-harness must be cloneable:true (Task-305 T-5)")
	}
	if bug.Policy.Cap != 3 {
		t.Fatalf("bug-harness policy.cap = %d, want 3 (same cost envelope as rag-harness)", bug.Policy.Cap)
	}
	if bug.Builtin.ChatBaseline {
		t.Fatal("bug-harness must not be chatBaseline (rag-harness keeps that role)")
	}

	if len(bug.Nodes) != len(rag.Nodes) {
		t.Fatalf("bug-harness node count = %d, rag-harness = %d, want identical", len(bug.Nodes), len(rag.Nodes))
	}
	for i := range bug.Nodes {
		b, r := bug.Nodes[i], rag.Nodes[i]
		b.ID, r.ID = "", ""
		if !reflect.DeepEqual(b, r) {
			t.Fatalf("bug-harness node %d differs from rag-harness:\nbug: %+v\nrag: %+v", i, bug.Nodes[i], rag.Nodes[i])
		}
	}
	if !reflect.DeepEqual(bug.Edges, rag.Edges) {
		t.Fatalf("bug-harness edges differ from rag-harness:\nbug: %+v\nrag: %+v", bug.Edges, rag.Edges)
	}
	if !reflect.DeepEqual(bug.AcceptanceNodes, rag.AcceptanceNodes) {
		t.Fatalf("bug-harness acceptance_nodes = %v, rag-harness = %v, want identical", bug.AcceptanceNodes, rag.AcceptanceNodes)
	}
}
