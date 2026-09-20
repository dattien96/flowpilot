package agentpack

import "testing"

// CA-795 baseline, SUPERSEDED by CP-67 P-5 (Task-382): tdd is now
// agent.scaffold — still a freeze-bound writer (scaffold joins the writer
// set per B-3), with the scaffold-architect persona instead of agent.code.

func TestPack_VibeSprintTddIsScaffoldWriter(t *testing.T) {
	def := loadVibeSprint(t)
	tdd := nodeByID(def, "tdd")
	if tdd.Behavior != "agent.scaffold" {
		t.Fatalf("tdd behavior=%q, want agent.scaffold", tdd.Behavior)
	}
	coder := nodeByID(def, "coder")
	if coder.Behavior != "agent.code" {
		t.Fatalf("coder behavior=%q, want agent.code", coder.Behavior)
	}
	if err := ValidateFlowSafetyTopology(def); err != nil {
		t.Fatalf("ValidateFlowSafetyTopology: %v", err)
	}
	if !ForwardDominates(def, "preflight_contract_freeze", "tdd") {
		t.Fatal("tdd must be freeze-dominated")
	}
	if !ForwardDominates(def, "preflight_contract_freeze", "coder") {
		t.Fatal("coder must be freeze-dominated")
	}
}
