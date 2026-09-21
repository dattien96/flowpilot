package agentpack

import "testing"

// CA-794 baseline, SUPERSEDED by CP-67 P-5 (Task-382): the CA-794 failure
// mode (tdd writing full bodies) is now structurally impossible — tdd is the
// CP-67 Contract-First Scaffold node (agent.scaffold + scaffold-architect +
// scaffold-contract-tdd prompt) and the coder gets the fill-body-only
// contract. Expectations updated to the CP-67 contract.

func TestPack_VibeSprintTddUsesScaffoldContractPrompt(t *testing.T) {
	def := loadVibeSprint(t)
	tdd := nodeByID(def, "tdd")
	if tdd.PromptTemplate != "prompts/scaffold-contract-tdd.md" {
		t.Fatalf("tdd promptTemplate=%q, want prompts/scaffold-contract-tdd.md", tdd.PromptTemplate)
	}
	if tdd.Agent != "agents/scaffold-architect.md" {
		t.Fatalf("tdd agent=%q, want agents/scaffold-architect.md", tdd.Agent)
	}
	if tdd.Behavior != "agent.scaffold" {
		t.Fatalf("tdd behavior=%q, want agent.scaffold (CP-67)", tdd.Behavior)
	}
	coder := nodeByID(def, "coder")
	if coder.PromptTemplate != "prompts/implement-scaffold-body.md" {
		t.Fatalf("coder promptTemplate=%q, want prompts/implement-scaffold-body.md", coder.PromptTemplate)
	}
}
