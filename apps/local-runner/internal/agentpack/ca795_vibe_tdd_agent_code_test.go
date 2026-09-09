package agentpack

import "testing"

// CA-795: vibe-sprint tdd is agent.code like harness test_signatures so
// freeze binds tdd first, then coder. New file — do not edit CA-785 tests.

func TestPack_VibeSprintTddIsAgentCode(t *testing.T) {
	def := loadVibeSprint(t)
	tdd := nodeByID(def, "tdd")
	if tdd.Behavior != "agent.code" {
		t.Fatalf("tdd behavior=%q, want agent.code", tdd.Behavior)
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
