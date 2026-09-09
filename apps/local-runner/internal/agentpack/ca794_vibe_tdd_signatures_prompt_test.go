package agentpack

import "testing"

// CA-794: vibe-sprint tdd must reuse the harness signature-only prompt.
// Live run-219176 wrote snake.go + full test bodies because tdd had tester.md
// and no promptTemplate. New file — do not edit TestPack_VibeSprintV2Topology.

func TestPack_VibeSprintTddUsesHarnessSignaturePrompt(t *testing.T) {
	def := loadVibeSprint(t)
	tdd := nodeByID(def, "tdd")
	if tdd.PromptTemplate != "prompts/test-signatures.md" {
		t.Fatalf("tdd promptTemplate=%q, want prompts/test-signatures.md", tdd.PromptTemplate)
	}
	if tdd.Agent != "agents/tester.md" {
		t.Fatalf("tdd agent=%q", tdd.Agent)
	}
	if tdd.Behavior != "agent.code" {
		t.Fatalf("tdd behavior=%q, want agent.code (CA-795 harness parity)", tdd.Behavior)
	}
	coder := nodeByID(def, "coder")
	if coder.PromptTemplate != "prompts/implement-complete-tests.md" {
		t.Fatalf("coder promptTemplate=%q, want prompts/implement-complete-tests.md", coder.PromptTemplate)
	}
}
