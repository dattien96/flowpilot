package agentpack

import "testing"

// Task-293: rag-harness TDD + review-until-clean topology. New file — no
// pre-existing test is modified.

func TestRAGHarnessTDDReviewTopology(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	def, ok := findFlowByID(pack.Flows, "rag-harness")
	if !ok {
		t.Fatal("rag-harness flow missing")
	}
	nodes := map[string]FlowNode{}
	for _, n := range def.Nodes {
		nodes[n.ID] = n
	}

	// test_signatures: agent.code writer using the tester agent, with the
	// static test-signatures promptTemplate.
	ts, ok := nodes["test_signatures"]
	if !ok {
		t.Fatal("expected a test_signatures node")
	}
	if canon, _ := NormalizeBehaviorID(ts.Behavior); canon != "agent.code" {
		t.Fatalf("test_signatures behavior = %q, want agent.code", ts.Behavior)
	}
	if ts.Agent != "agents/tester.md" {
		t.Fatalf("test_signatures agent = %q, want agents/tester.md", ts.Agent)
	}
	if ts.Lifecycle != "once" {
		t.Fatalf("test_signatures lifecycle = %q, want once", ts.Lifecycle)
	}
	if ts.PromptTemplate != "prompts/test-signatures.md" {
		t.Fatalf("test_signatures promptTemplate = %q, want prompts/test-signatures.md", ts.PromptTemplate)
	}

	// implement: agent.code writer with the implement-complete-tests template.
	impl, ok := nodes["implement"]
	if !ok {
		t.Fatal("expected an implement node")
	}
	if canon, _ := NormalizeBehaviorID(impl.Behavior); canon != "agent.code" {
		t.Fatalf("implement behavior = %q, want agent.code", impl.Behavior)
	}
	if impl.PromptTemplate != "prompts/implement-complete-tests.md" {
		t.Fatalf("implement promptTemplate = %q, want prompts/implement-complete-tests.md", impl.PromptTemplate)
	}

	// reviewer: single review-cohort delegate with the safe-fix review template.
	rev, ok := nodes["reviewer"]
	if !ok {
		t.Fatal("expected a reviewer node")
	}
	if canon, _ := NormalizeBehaviorID(rev.Behavior); canon != "agent.delegate" {
		t.Fatalf("reviewer behavior = %q, want agent.delegate", rev.Behavior)
	}
	if rev.Cohort != "review" || rev.Join != "all" {
		t.Fatalf("reviewer cohort=%q join=%q, want review/all", rev.Cohort, rev.Join)
	}
	if rev.PromptTemplate != "prompts/review-safe-fix-contract.md" {
		t.Fatalf("reviewer promptTemplate = %q, want prompts/review-safe-fix-contract.md", rev.PromptTemplate)
	}

	// synthesis: hub.inline gate with join-all.
	syn, ok := nodes["synthesis"]
	if !ok {
		t.Fatal("expected a synthesis node")
	}
	if canon, _ := NormalizeBehaviorID(syn.Behavior); canon != "hub.inline" {
		t.Fatalf("synthesis behavior = %q, want hub.inline", syn.Behavior)
	}
	if syn.Join != "all" {
		t.Fatalf("synthesis join = %q, want all", syn.Join)
	}

	// preflight_contract_plan carries the safe-fix plan template.
	plan, ok := nodes["preflight_contract_plan"]
	if !ok {
		t.Fatal("expected a preflight_contract_plan node")
	}
	if plan.PromptTemplate != "prompts/plan-safe-fix-contract.md" {
		t.Fatalf("preflight_contract_plan promptTemplate = %q, want prompts/plan-safe-fix-contract.md", plan.PromptTemplate)
	}

	// Acceptance + review tool.
	hasSynthesis := false
	for _, id := range def.AcceptanceNodes {
		if id == "synthesis" {
			hasSynthesis = true
		}
	}
	if !hasSynthesis {
		t.Fatalf("acceptance_nodes = %v, want synthesis included (machine-verdict gate)", def.AcceptanceNodes)
	}
	hasReviewTool := false
	for _, tool := range def.Tools {
		if tool == "tools/submit-review-outcome.yaml" {
			hasReviewTool = true
		}
	}
	if !hasReviewTool {
		t.Fatalf("tools = %v, want submit-review-outcome", def.Tools)
	}
}

func TestRAGHarnessTDDReviewEdges(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	def, ok := findFlowByID(pack.Flows, "rag-harness")
	if !ok {
		t.Fatal("rag-harness flow missing")
	}
	edgeSet := map[string]string{}
	for _, e := range def.Edges {
		key := e.From + "|" + e.When + "|" + e.Kind
		edgeSet[key] = e.To
	}
	want := map[string]string{
		"context|done|forward":                 "test_signatures",
		"test_signatures|done|forward":         "implement",
		"implement|done|forward":               "validate",
		"validate|continue|back":               "implement",
		"validate|done|forward":                "reviewer",
		"reviewer|done|forward":                "synthesis",
		"synthesis|done|forward":               "audit",
		"synthesis|escalate|forward":           "ask_user",
		"validate|escalate|forward":            "ask_user",
		"preflight_contract_plan|done|forward": "preflight_contract_freeze",
	}
	for key, wantTo := range want {
		if got, ok := edgeSet[key]; !ok || got != wantTo {
			t.Fatalf("edge %q = %q (present=%v), want %q", key, got, ok, wantTo)
		}
	}
}

func TestRAGHarnessTDDReviewSafeTopology(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	def, ok := findFlowByID(pack.Flows, "rag-harness")
	if !ok {
		t.Fatal("rag-harness flow missing")
	}
	if err := ValidateFlowSafetyTopology(def); err != nil {
		t.Fatalf("ValidateFlowSafetyTopology: %v", err)
	}
	// Both writers must be forward-dominated by the freeze node.
	for _, writer := range []string{"test_signatures", "implement"} {
		if !ForwardDominates(def, "preflight_contract_freeze", writer) {
			t.Fatalf("writer %q is not dominated by preflight_contract_freeze", writer)
		}
		if !EveryDonePathIncludesAcceptance(def, writer) {
			t.Fatalf("writer %q has a done path that skips acceptance", writer)
		}
	}
}

func TestRAGHarnessDeclaresNewPromptsInManifest(t *testing.T) {
	pack, err := LoadBuiltinPack()
	if err != nil {
		t.Fatalf("LoadBuiltinPack: %v", err)
	}
	declared := map[string]bool{}
	for _, p := range pack.Prompts {
		declared[p.Path] = true
	}
	for _, want := range []string{
		"prompts/plan-safe-fix-contract.md",
		"prompts/test-signatures.md",
		"prompts/implement-complete-tests.md",
		"prompts/review-safe-fix-contract.md",
	} {
		if !declared[want] {
			t.Fatalf("manifest does not declare %q", want)
		}
		if _, ok, err := LoadBuiltinPrompt(want); err != nil || !ok {
			t.Fatalf("prompt %q not loadable: ok=%v err=%v", want, ok, err)
		}
	}
}
