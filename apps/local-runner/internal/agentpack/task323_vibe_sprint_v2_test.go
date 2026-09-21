package agentpack

import (
	"testing"
)

func loadVibeSprint(t *testing.T) FlowDefinition {
	t.Helper()
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	for _, def := range pack.Flows {
		if def.ID == "vibe-sprint" {
			return def
		}
	}
	t.Fatal("missing vibe-sprint")
	return FlowDefinition{}
}

func TestPack_VibeSprintV2Topology(t *testing.T) {
	def := loadVibeSprint(t)
	got := map[string]string{}
	order := make([]string, 0, len(def.Nodes))
	for _, n := range def.Nodes {
		got[n.ID] = n.Behavior
		order = append(order, n.ID)
	}
	// CP-67 P-5: synthesis_negotiation joins the topology (B-6).
	want := []string{
		"preflight_contract_plan", "preflight_contract_freeze", "context",
		"tdd", "coder", "validate", "synthesis", "synthesis_negotiation", "audit",
	}
	if len(order) != len(want) {
		t.Fatalf("nodes=%v, want %v", order, want)
	}
	for i, id := range want {
		if order[i] != id {
			t.Fatalf("node[%d]=%s, want %s (order=%v)", i, order[i], id, order)
		}
	}
	if got["context"] != "context.produce" {
		t.Fatalf("context behavior=%q", got["context"])
	}
	if got["validate"] != "command.validate" {
		t.Fatalf("validate behavior=%q", got["validate"])
	}
	if got["audit"] != "artifact.audit_draft" {
		t.Fatalf("audit behavior=%q", got["audit"])
	}
	if got["coder"] != "agent.code" {
		t.Fatalf("coder behavior=%q", got["coder"])
	}
	// CP-67 supersession: tdd carries the scaffold architect.
	tdd := nodeByID(def, "tdd")
	if tdd.Agent != "agents/scaffold-architect.md" {
		t.Fatalf("tdd agent=%q, want agents/scaffold-architect.md", tdd.Agent)
	}
	// CP-67 supersession: two continue back-edges now — the review-loop
	// synthesis→coder edge (unchanged) plus the negotiation
	// synthesis_negotiation→tdd edge (B-6).
	back := 0
	tddToCoder := false
	negotiationBack := false
	for _, e := range def.Edges {
		if e.Kind == "back" && e.When == "continue" {
			back++
			if e.From == "synthesis_negotiation" && e.To == "tdd" {
				negotiationBack = true
				continue
			}
			if e.From != "synthesis" || e.To != "coder" {
				t.Fatalf("continue back-edge %s → %s", e.From, e.To)
			}
		}
		if e.From == "tdd" && e.To == "coder" && e.When == "done" {
			tddToCoder = true
		}
	}
	if back != 2 {
		t.Fatalf("continue back-edges=%d, want 2", back)
	}
	if !negotiationBack {
		t.Fatal("missing synthesis_negotiation → tdd continue back-edge (CP-67 renegotiation loop)")
	}
	if !tddToCoder {
		t.Fatal("missing tdd → coder done edge (bypass)")
	}
	acc := map[string]struct{}{}
	for _, id := range def.AcceptanceNodes {
		acc[id] = struct{}{}
	}
	for _, id := range []string{"validate", "synthesis", "audit"} {
		if _, ok := acc[id]; !ok {
			t.Fatalf("acceptance missing %s: %v", id, def.AcceptanceNodes)
		}
	}
	if err := ValidateFlowSafetyTopology(def); err != nil {
		t.Fatalf("ValidateFlowSafetyTopology: %v", err)
	}
}

func nodeByID(def FlowDefinition, id string) FlowNode {
	for _, n := range def.Nodes {
		if n.ID == id {
			return n
		}
	}
	return FlowNode{}
}

func TestPack_VibeSprintV2NoReviewerCohort(t *testing.T) {
	def := loadVibeSprint(t)
	for _, n := range def.Nodes {
		switch n.ID {
		case "reviewer", "plan_reviewer", "plan_writer", "plan_synthesis":
			t.Fatalf("harness-only node %s leaked into vibe-sprint", n.ID)
		}
	}
}
