package agentpack

import "testing"

// CP-55 P-8: coverage that the three built-in coding flows this phase
// migrated (review-loop, rag-harness, context-coding-review-synthesis) each
// declare a real preflight-contract-freeze node that forward-dominates their
// own writer, and that the pack as a whole still passes full safety-topology
// validation. New file — no pre-existing test is modified.

// TestReviewLoopHasPreflightBeforeEveryWriter asserts review-loop.yaml's
// preflight_contract_freeze node forward-dominates its sole writer (coder) —
// no forward path from any real entry can reach coder without first crossing
// the freeze node.
func TestReviewLoopHasPreflightBeforeEveryWriter(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	def, ok := findFlowByID(pack.Flows, "review-loop")
	if !ok {
		t.Fatal("review-loop flow missing")
	}
	assertHasPreflightBeforeEveryWriter(t, def)
}

// TestRAGHarnessHasPreflightBeforeEveryWriter is the same assertion for
// rag-harness.yaml's writer (implement).
func TestRAGHarnessHasPreflightBeforeEveryWriter(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	def, ok := findFlowByID(pack.Flows, "rag-harness")
	if !ok {
		t.Fatal("rag-harness flow missing")
	}
	assertHasPreflightBeforeEveryWriter(t, def)
}

// TestContextCodingReviewSynthesisHasPreflightBeforeEveryWriter is the same
// assertion for context-coding-review-synthesis.yaml's writer (coder).
func TestContextCodingReviewSynthesisHasPreflightBeforeEveryWriter(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	def, ok := findFlowByID(pack.Flows, "context-coding-review-synthesis")
	if !ok {
		t.Fatal("context-coding-review-synthesis flow missing")
	}
	assertHasPreflightBeforeEveryWriter(t, def)
}

// assertHasPreflightBeforeEveryWriter fails t unless def declares at least
// one agent.code writer and every one of them is forward-dominated by a
// contract.freeze node — i.e. no forward path from a real entry can reach
// the writer without a preflight contract having been frozen first.
func assertHasPreflightBeforeEveryWriter(t *testing.T, def FlowDefinition) {
	t.Helper()
	writers, freezes := classifyWriterAndFreezeNodes(def)
	if len(writers) == 0 {
		t.Fatalf("flow %q declares no agent.code writer node", def.ID)
	}
	if len(freezes) == 0 {
		t.Fatalf("flow %q declares no contract.freeze node", def.ID)
	}
	for _, w := range writers {
		dominated := false
		for _, f := range freezes {
			if ForwardDominates(def, f, w) {
				dominated = true
				break
			}
		}
		if !dominated {
			t.Fatalf("flow %q: writer %q is not forward-dominated by any contract.freeze node", def.ID, w)
		}
	}
}

// TestBuiltInCodingFlowsPassSafetyTopologyValidation asserts every built-in
// flow in the pack — not just the three CP-55 P-8 explicitly migrated —
// passes ValidateFlowSafetyTopology's full check (writer-not-entry, freeze
// dominance, declared acceptance boundary, every done-path crosses
// acceptance). A flow with zero agent.code nodes still passes vacuously, so
// this is a safe, permanent regression guard for the whole pack, not just
// the migrated three.
func TestBuiltInCodingFlowsPassSafetyTopologyValidation(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	migrated := map[string]bool{
		"review-loop":                     false,
		"rag-harness":                     false,
		"context-coding-review-synthesis": false,
	}
	for _, def := range pack.Flows {
		if err := ValidateFlowSafetyTopology(def); err != nil {
			t.Errorf("flow %q: ValidateFlowSafetyTopology error = %v, want nil", def.ID, err)
		}
		if _, ok := migrated[def.ID]; ok {
			writers, _ := classifyWriterAndFreezeNodes(def)
			migrated[def.ID] = len(writers) > 0
		}
	}
	for id, sawWriter := range migrated {
		if !sawWriter {
			t.Errorf("expected built-in flow %q to declare an agent.code writer node (CP-55 P-8 migration), found none", id)
		}
	}
}
