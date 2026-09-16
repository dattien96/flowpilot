package agentpack

import (
	"reflect"
	"testing"
)

// TestBugHarnessPackClone (CP-58 Task-305 T-5, operator option (a) 2026-09-04)
// pins the Bug tier picker entry: bug-harness loads, is selectable in /flow,
// cloneable, cap:3, and stays a byte-identical clone of rag-harness except
// id/description (rag-harness keeps the chatBaseline role).
// CP-64 update (reproduce-first-gate): the TDD node is the ONE sanctioned
// divergence — bug flows reproduce the bug with a real red test instead of
// writing empty signatures; see the comparison block below.
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

	// CP-64 (reproduce-first-gate): bug-harness now intentionally diverges from
	// rag-harness at exactly ONE node — the empty-signature TDD step became the
	// reproduce-first gate node (reproduce_test / agent.reproduce /
	// reproducer). Every other node, the edge shape, and acceptance_nodes must
	// stay identical, so the tier still shares rag-harness's cost envelope.
	const (
		reproduceNodeIndex = 3
		reproduceNodeID    = "reproduce_test"
		legacyTDDNodeID    = "test_signatures"
	)
	if len(bug.Nodes) != len(rag.Nodes) {
		t.Fatalf("bug-harness node count = %d, rag-harness = %d, want identical", len(bug.Nodes), len(rag.Nodes))
	}
	for i := range bug.Nodes {
		b, r := bug.Nodes[i], rag.Nodes[i]
		if i == reproduceNodeIndex {
			if b.ID != reproduceNodeID || r.ID != legacyTDDNodeID {
				t.Fatalf("bug-harness node %d = %q/%q, want %q vs rag %q", i, b.ID, b.Behavior, reproduceNodeID, legacyTDDNodeID)
			}
			if b.Behavior != "agent.reproduce" || b.Agent != "agents/reproducer.md" || b.PromptTemplate != "prompts/reproduce-failing-test.md" {
				t.Fatalf("reproduce node shape drifted: %+v", b)
			}
			// Non-TDD fields (run/lifecycle) must still match the harness shape.
			b.ID, r.ID = "", ""
			b.Behavior, b.Agent, b.PromptTemplate = r.Behavior, r.Agent, r.PromptTemplate
			if !reflect.DeepEqual(b, r) {
				t.Fatalf("reproduce node diverges from rag-harness beyond the CP-64 fields:\nbug: %+v\nrag: %+v", b, r)
			}
			continue
		}
		b.ID, r.ID = "", ""
		if !reflect.DeepEqual(b, r) {
			t.Fatalf("bug-harness node %d differs from rag-harness:\nbug: %+v\nrag: %+v", i, bug.Nodes[i], rag.Nodes[i])
		}
	}
	// Edges are identical once the TDD hop's node id is translated back to the
	// legacy name (same topology, one node renamed by CP-64).
	if !reflect.DeepEqual(renameReproduceEdgeEndpoints(bug.Edges), rag.Edges) {
		t.Fatalf("bug-harness edges differ from rag-harness beyond the CP-64 rename:\nbug: %+v\nrag: %+v", bug.Edges, rag.Edges)
	}
	if !reflect.DeepEqual(bug.AcceptanceNodes, rag.AcceptanceNodes) {
		t.Fatalf("bug-harness acceptance_nodes = %v, rag-harness = %v, want identical", bug.AcceptanceNodes, rag.AcceptanceNodes)
	}
}
