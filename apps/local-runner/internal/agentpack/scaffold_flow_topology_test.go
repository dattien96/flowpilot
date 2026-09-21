package agentpack

import (
	"testing"
)

// CP-67 P-5 (Task-382): topology contracts for the Contract-First TDD
// renegotiation loop.

func TestTaskHarnessTopologyScaffoldNegotiationLoop(t *testing.T) {
	def := taskHarnessDefinition(t)

	// Node id UNCHANGED (B-7); scaffold wiring + reinvoke lifecycle.
	ts := findNodeByID(t, def, "test_signatures")
	if ts.Behavior != "agent.scaffold" || ts.Agent != "agents/scaffold-architect.md" ||
		ts.PromptTemplate != "prompts/scaffold-contract-tdd.md" {
		t.Fatalf("test_signatures shape = %s/%s/%s, want the scaffold wiring",
			ts.Behavior, ts.Agent, ts.PromptTemplate)
	}
	if ts.Lifecycle != "reinvoke" {
		t.Fatalf("test_signatures lifecycle = %q, want reinvoke (negotiation re-entry)", ts.Lifecycle)
	}
	// Coder prompt switched to fill-body-only.
	impl := findNodeByID(t, def, "implement")
	if impl.PromptTemplate != "prompts/implement-scaffold-body.md" {
		t.Fatalf("implement prompt = %q, want implement-scaffold-body.md", impl.PromptTemplate)
	}

	// The negotiation hub + its three edges (B-6).
	if hub := findNodeByID(t, def, "synthesis_negotiation"); hub.Behavior != "hub.inline" {
		t.Fatalf("synthesis_negotiation behavior = %q, want hub.inline", hub.Behavior)
	}
	if !hasEdge(def.Edges, "synthesis", "synthesis_negotiation", "continue", "forward") {
		t.Fatal("missing synthesis -> synthesis_negotiation (continue) edge")
	}
	if !hasEdge(def.Edges, "synthesis_negotiation", "test_signatures", "continue", "back") {
		t.Fatal("missing negotiation back-edge to test_signatures")
	}
	if !hasEdge(def.Edges, "synthesis_negotiation", "synthesis", "done", "forward") {
		t.Fatal("missing negotiation forward-close edge to synthesis")
	}

	// Phase-scoped budget (B-10).
	if def.Policy.NegotiationCap != 5 {
		t.Fatalf("policy.negotiationCap = %d, want 5", def.Policy.NegotiationCap)
	}

	// CP-67 faces are declared in the flow tools list.
	for _, tool := range def.Tools {
		_ = tool
	}
	if !hasToolPath(def.Tools, "submit-scaffold-outcome.yaml") || !hasToolPath(def.Tools, "submit-coder-outcome.yaml") {
		t.Fatalf("flow tools %v must declare the CP-67 faces", def.Tools)
	}

	if err := ValidateFlowDefinition(def); err != nil {
		t.Fatalf("ValidateFlowDefinition: %v", err)
	}
}

func TestVibeSprintTopologyScaffoldNegotiationLoop(t *testing.T) {
	def := loadVibeSprint(t)

	tdd := nodeByID(def, "tdd")
	if tdd.Behavior != "agent.scaffold" || tdd.Agent != "agents/scaffold-architect.md" ||
		tdd.PromptTemplate != "prompts/scaffold-contract-tdd.md" || tdd.Model != "claude-sonnet-4-5" {
		t.Fatalf("tdd shape = %s/%s/%s/%s, want scaffold wiring + high-reasoning model",
			tdd.Behavior, tdd.Agent, tdd.PromptTemplate, tdd.Model)
	}
	if nodeByID(def, "coder").PromptTemplate != "prompts/implement-scaffold-body.md" {
		t.Fatal("coder prompt must switch to implement-scaffold-body.md")
	}
	if hub := nodeByID(def, "synthesis_negotiation"); hub.Behavior != "hub.inline" {
		t.Fatalf("synthesis_negotiation behavior = %q", hub.Behavior)
	}
	if !hasEdge(def.Edges, "synthesis_negotiation", "tdd", "continue", "back") {
		t.Fatal("missing negotiation back-edge to tdd")
	}
	if !hasEdge(def.Edges, "synthesis_negotiation", "synthesis", "done", "forward") {
		t.Fatal("missing negotiation forward-close edge")
	}
	// B-6: the OLD review-loop back-edge synthesis → coder stays untouched.
	if !hasEdge(def.Edges, "synthesis", "coder", "continue", "back") {
		t.Fatal("review-loop edge synthesis -> coder must remain")
	}
	if def.Policy.NegotiationCap != 5 {
		t.Fatalf("policy.negotiationCap = %d, want 5", def.Policy.NegotiationCap)
	}
	if !hasToolPath(def.Tools, "submit-scaffold-outcome.yaml") || !hasToolPath(def.Tools, "submit-coder-outcome.yaml") {
		t.Fatalf("flow tools %v must declare the CP-67 faces", def.Tools)
	}
	if err := ValidateFlowDefinition(def); err != nil {
		t.Fatalf("ValidateFlowDefinition: %v", err)
	}
}

func TestScaffoldNegotiationCapParsingBounds(t *testing.T) {
	// 1..20 is the declared range; out-of-range values fail the load.
	bad := taskHarnessDefinition(t)
	bad.Policy.NegotiationCap = 21
	if err := ValidateFlowDefinition(bad); err == nil {
		t.Fatal("negotiationCap=21 must fail validation")
	}
	zero := taskHarnessDefinition(t)
	zero.Policy.NegotiationCap = 0
	if err := ValidateFlowDefinition(zero); err != nil {
		t.Fatalf("negotiationCap=0 (runner default) must validate, got %v", err)
	}
}

func TestLegacyTestSignaturesFlowUnaffected(t *testing.T) {
	// The CP-64 / legacy flows keep their shapes: bug flows stay
	// reproduce-first; rag-harness keeps the legacy empty-signature node.
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	for _, defID := range []string{"bug-harness", "bug-plan-harness", "rag-harness"} {
		var def FlowDefinition
		found := false
		for _, f := range pack.Flows {
			if f.ID == defID {
				def = f
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s missing from pack", defID)
		}
		if err := ValidateFlowDefinition(def); err != nil {
			t.Fatalf("%s must still validate: %v", defID, err)
		}
		for _, n := range def.Nodes {
			if n.Behavior == "agent.scaffold" {
				t.Fatalf("%s must not adopt the scaffold behavior (CP-67 scope is task-harness/vibe-sprint only)", defID)
			}
		}
	}
}

func hasToolPath(tools []string, suffix string) bool {
	for _, t := range tools {
		if len(t) >= len(suffix) && t[len(t)-len(suffix):] == suffix {
			return true
		}
	}
	return false
}
