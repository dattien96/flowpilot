package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/flowgate"
)

// BUG-425: a gate-reprompt turn starts from a fresh turnStartGitHead, so its
// own observed diff only covers the remediation delta — usually just the
// change-audit note the reprompt asked for. prepareChangeContract then ran
// InferFromDiff over that near-empty diff and committed contracts with no
// declared_paths, no intent, and noise/empty feature keys, poisoning the
// feature catalog. The failing coding turn's paths must be carried on the
// run (pendingGateCodePaths, already durable) and folded back into contract
// inference + feature-key suggestion on the reprompt turn — WITHOUT widening
// tr.GitDiff, which would wrongly re-fire r-tests/r-reg on the old change.
// Provider-agnostic (Case 1): the gate path is shared by claude/codex/grok.

func TestBug425MergeCarriedPathsIntoDiff(t *testing.T) {
	diff := []flowgate.ChangedFile{
		{Path: "change-audit/CA-999-test.md", Status: "A"},
	}
	merged := mergeCarriedPathsIntoDiff(diff, []string{"src/calc.go", "src/calc.go", " change-audit/CA-999-test.md ", ""})
	if len(merged) != 2 {
		t.Fatalf("merged diff len = %d, want 2 (dedupe + blank skip): %+v", len(merged), merged)
	}
	paths := map[string]bool{}
	for _, f := range merged {
		paths[f.Path] = true
	}
	if !paths["src/calc.go"] || !paths["change-audit/CA-999-test.md"] {
		t.Fatalf("merged diff paths = %v, want carried src/calc.go + existing audit note", paths)
	}
	// Empty carry is a no-op (same slice content, no synthetic entries).
	if got := mergeCarriedPathsIntoDiff(diff, nil); len(got) != 1 {
		t.Fatalf("empty carry must not grow the diff, got %+v", got)
	}
}

func TestBug425GateRepromptCarriesFailingTurnCodePaths(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	// No gate-config.json → enforce mode (warn would downgrade reprompt).
	// r-ca is the only enabled rule so the coding turn resolves to reprompt.
	p4WriteFile(t, dir, ".flowpilot/settings/flow-rules.json", `[
		{"id":"r-ca","scope":"step","trigger":"code_changed","required_output":"change_audit_note","action":"reprompt","enabled":true},
		{"id":"r-contract","scope":"step","trigger":"code_changed_no_contract","required_output":"declared_change_contract","action":"warn","enabled":false},
		{"id":"r-fk","scope":"step","trigger":"commit_feature_key_missing","required_output":"verified_feature_key","action":"warn","enabled":false},
		{"id":"r-bug","scope":"step","trigger":"bug_fixed","required_output":"bugfix_doc","action":"warn","enabled":false},
		{"id":"r-task","scope":"step","trigger":"task_referenced","required_output":"task_doc","action":"warn","enabled":false},
		{"id":"r-scope","scope":"step","trigger":"edit_outside_declared_scope","required_output":"confirm_or_revert_out_of_scope","action":"warn","enabled":false},
		{"id":"r-tests","scope":"step","trigger":"tests_failed","required_output":"tests_green_or_explained","action":"warn","enabled":false},
		{"id":"r-reg","scope":"step","trigger":"regression_test_broke","required_output":"restore_green_without_weakening","action":"warn","enabled":false}
	]`)
	// go.mod keeps baseline capture out of gate_blind in enforce mode.
	p4WriteFile(t, dir, "go.mod", "module bug425bed\n\ngo 1.21\n")
	svc := newFreezeTestService(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	rs := newP4ChildRun(svc, "root-turn", "", dir, head)
	rs.id = parent.RunID
	svc.mu.Lock()
	svc.runs[parent.RunID] = rs
	rs.workspaceCwd = dir
	svc.mu.Unlock()

	// The original coding turn writes src/calc.go but no change-audit note →
	// r-ca reprompt. Its code paths must be carried onto the run.
	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	blocked := svc.runFlowGateAtEpoch(context.Background(), rs, "turn-code", finalizeInput{
		FinalMessage: "implemented calc",
		ChangedFiles: []string{"src/calc.go"},
	}, 0)
	if !blocked {
		t.Fatal("coding turn without change-audit note must be reprompted (r-ca)")
	}
	svc.mu.Lock()
	carried := append([]string(nil), rs.pendingGateCodePaths...)
	svc.mu.Unlock()
	found := false
	for _, p := range carried {
		if p == "src/calc.go" {
			found = true
		}
	}
	if !found {
		t.Fatalf("reprompt must carry the failing turn's code paths, got %v", carried)
	}
}

func TestBug425RepromptGateInfersFromCarriedCodePaths(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	relaxGateRulesForPendingCanonicalTests(t, dir)
	svc := newFreezeTestService(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	rs := newP4ChildRun(svc, "root-turn", "", dir, head)
	rs.id = parent.RunID
	svc.mu.Lock()
	svc.runs[parent.RunID] = rs
	rs.workspaceCwd = dir
	// The failing coding turn's paths, as the reprompt branch stashes them.
	rs.pendingGateCodePaths = []string{"src/calc.go"}
	svc.mu.Unlock()

	// Registered feature key + catalog glob so suggestion has a real key to
	// resolve from the carried src/ path — proves feature key comes from the
	// original change, not provider/model noise.
	p4WriteFile(t, dir, "change-audit/FEATURE-KEYS.md", "- calc-core — calculator core\n")
	p4WriteFile(t, dir, ".flowpilot/catalog/features.ndjson", `{"feature_key":"calc-core","title":"Calc","file_globs":["src/**"]}`+"\n")

	// The reprompt turn's own delta: only the change-audit note (r-ca
	// remediation). Its gate must still commit a contract describing
	// src/calc.go, not an empty inferred record.
	p4WriteFile(t, dir, "change-audit/CA-999-bug425.md", "# audit\n")
	blocked := svc.runFlowGateAtEpoch(context.Background(), rs, "turn-reprompt", finalizeInput{
		FinalMessage: "wrote the audit note",
		ChangedFiles: []string{"change-audit/CA-999-bug425.md"},
	}, 0)
	if blocked {
		t.Fatal("reprompt turn carrying the audit note must pass the gate")
	}

	store, storeErr := changecontract.NewStore(dir)
	if storeErr != nil {
		t.Fatal(storeErr)
	}
	got, ok := store.GetLatestForRun(rs.id)
	if !ok {
		t.Fatal("reprompt turn must still commit an inferred change contract")
	}
	if len(got.DeclaredPaths) == 0 {
		t.Fatalf("inferred contract lost the original code change: declared_paths empty (bug425)")
	}
	found := false
	for _, p := range got.DeclaredPaths {
		if p == "src" || p == "src/calc.go" {
			found = true
		}
	}
	if !found {
		t.Fatalf("declared_paths %v must cover the carried src/calc.go change", got.DeclaredPaths)
	}
	if got.FeatureKey != "calc-core" {
		t.Fatalf("feature_key = %q, want calc-core resolved from carried src/ paths", got.FeatureKey)
	}

	svc.mu.Lock()
	leftover := append([]string(nil), rs.pendingGateCodePaths...)
	svc.mu.Unlock()
	if len(leftover) != 0 {
		t.Fatalf("a passed reprompt turn must clear the carried code paths, still holding %v", leftover)
	}
}
