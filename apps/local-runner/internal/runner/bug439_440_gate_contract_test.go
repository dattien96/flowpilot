package runner

import (
	"context"
	"strings"
	"testing"

	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/flowgate"
)

// BUG-440: the reprompt carrier must hold the UNION of the failing turn's
// observed diff and its confirmed write events — provider EventFileChanged
// streams can be partial (e.g. only a doc file emits an event while the real
// code file only exists in the git diff). The previous "WrittenPaths, else
// diff" fallback dropped the diff side whenever any single event fired.
// r-newtest (diff-based) drives the reprompt here because doc-scope rules
// like r-ca key off WrittenPaths, which this scenario deliberately makes
// partial. Provider-agnostic (Case 1): shared gate path.
func TestBug440RepromptCarryUnionsPartialWriteEvents(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	p4WriteFile(t, dir, ".flowpilot/settings/flow-rules.json", `[
		{"id":"r-ca","scope":"step","trigger":"code_changed","required_output":"change_audit_note","action":"reprompt","enabled":true},
		{"id":"r-contract","scope":"step","trigger":"code_changed_no_contract","required_output":"declared_change_contract","action":"warn","enabled":false},
		{"id":"r-fk","scope":"step","trigger":"commit_feature_key_missing","required_output":"verified_feature_key","action":"warn","enabled":false},
		{"id":"r-bug","scope":"step","trigger":"bug_fixed","required_output":"bugfix_doc","action":"warn","enabled":false},
		{"id":"r-task","scope":"step","trigger":"task_referenced","required_output":"task_doc","action":"warn","enabled":false},
		{"id":"r-scope","scope":"step","trigger":"edit_outside_declared_scope","required_output":"confirm_or_revert_out_of_scope","action":"warn","enabled":false},
		{"id":"r-tests","scope":"step","trigger":"tests_failed","required_output":"tests_green_or_explained","action":"warn","enabled":false},
		{"id":"r-reg","scope":"step","trigger":"regression_test_broke","required_output":"restore_green_without_weakening","action":"warn","enabled":false},
		{"id":"r-newtest","scope":"step","trigger":"production_change_no_new_test","required_output":"new_additive_test_file","action":"reprompt","enabled":true}
	]`)
	p4WriteFile(t, dir, "go.mod", "module bug440bed\n\ngo 1.21\n")
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

	// The real code change is on disk (in the observed diff) but the provider
	// only emitted a file event for a doc — the partial-events shape.
	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	blocked := svc.runFlowGateAtEpoch(context.Background(), rs, "turn-code", finalizeInput{
		FinalMessage: "implemented calc",
		ChangedFiles: []string{"docs/note.md"},
	}, 0)
	if !blocked {
		t.Fatal("coding turn without change-audit note must be reprompted (r-ca)")
	}
	svc.mu.Lock()
	carried := append([]string(nil), rs.pendingGateCodePaths...)
	svc.mu.Unlock()
	foundSrc, foundDoc := false, false
	for _, p := range carried {
		if p == "src/calc.go" {
			foundSrc = true
		}
		if p == "docs/note.md" {
			foundDoc = true
		}
	}
	if !foundSrc {
		t.Fatalf("carry must include the diff-observed code path src/calc.go, got %v", carried)
	}
	if !foundDoc {
		t.Fatalf("carry must keep the confirmed write event docs/note.md, got %v", carried)
	}
}

// BUG-440 (lifetime): the carried paths must survive until the turn is fully
// allowed — an LSP diagnostic discovered on the clean-rules path must not
// strand the reprompt turn without the original code scope.
func TestBug440LSPRepromptPreservesCarriedPaths(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	// r-ca is satisfied by the audit note below → zero rule violations → the
	// gate reaches the LSP diagnostics check on the allow path.
	p4WriteFile(t, dir, ".flowpilot/settings/flow-rules.json", `[
		{"id":"r-ca","scope":"step","trigger":"code_changed","required_output":"change_audit_note","action":"reprompt","enabled":true},
		{"id":"r-contract","scope":"step","trigger":"code_changed_no_contract","required_output":"declared_change_contract","action":"warn","enabled":false},
		{"id":"r-fk","scope":"step","trigger":"commit_feature_key_missing","required_output":"verified_feature_key","action":"warn","enabled":false},
		{"id":"r-bug","scope":"step","trigger":"bug_fixed","required_output":"bugfix_doc","action":"warn","enabled":false},
		{"id":"r-task","scope":"step","trigger":"task_referenced","required_output":"task_doc","action":"warn","enabled":false},
		{"id":"r-scope","scope":"step","trigger":"edit_outside_declared_scope","required_output":"confirm_or_revert_out_of_scope","action":"warn","enabled":false},
		{"id":"r-tests","scope":"step","trigger":"tests_failed","required_output":"tests_green_or_explained","action":"warn","enabled":false},
		{"id":"r-reg","scope":"step","trigger":"regression_test_broke","required_output":"restore_green_without_weakening","action":"warn","enabled":false},
		{"id":"r-newtest","scope":"step","trigger":"production_change_no_new_test","required_output":"new_additive_test_file","action":"warn","enabled":false}
	]`)
	p4WriteFile(t, dir, "go.mod", "module bug440bed\n\ngo 1.21\n")
	svc := newFreezeTestService(t)
	svc.lspChecker = &lspFakeChecker{msg: "src/calc.go:3:1 error: undefined: X"}
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
	// Carry from the original gate-failing coding turn.
	rs.pendingGateCodePaths = []string{"src/calc.go"}
	svc.mu.Unlock()

	// The reprompt turn wrote only the audit note; rules are clean but LSP
	// fires → the diagnostic reprompt must keep the carried code path.
	p4WriteFile(t, dir, "change-audit/CA-999-bug440.md", "# audit\n")
	blocked := svc.runFlowGateAtEpoch(context.Background(), rs, "turn-reprompt", finalizeInput{
		FinalMessage: "wrote the audit note",
		ChangedFiles: []string{"change-audit/CA-999-bug440.md"},
	}, 0)
	if !blocked {
		t.Fatal("LSP diagnostics must block the turn with a reprompt")
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
		t.Fatalf("LSP reprompt must preserve the carried code path src/calc.go, got %v", carried)
	}
}

// BUG-440 (lifetime, first violation): an LSP reprompt on a turn with no
// prior carry must still stash the turn's own code paths — its remediation
// turn's diff will only cover the diagnostic fix.
func TestBug440LSPRepromptStashesCurrentTurnPaths(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	p4WriteFile(t, dir, ".flowpilot/settings/flow-rules.json", `[
		{"id":"r-ca","scope":"step","trigger":"code_changed","required_output":"change_audit_note","action":"reprompt","enabled":true},
		{"id":"r-contract","scope":"step","trigger":"code_changed_no_contract","required_output":"declared_change_contract","action":"warn","enabled":false},
		{"id":"r-fk","scope":"step","trigger":"commit_feature_key_missing","required_output":"verified_feature_key","action":"warn","enabled":false},
		{"id":"r-bug","scope":"step","trigger":"bug_fixed","required_output":"bugfix_doc","action":"warn","enabled":false},
		{"id":"r-task","scope":"step","trigger":"task_referenced","required_output":"task_doc","action":"warn","enabled":false},
		{"id":"r-scope","scope":"step","trigger":"edit_outside_declared_scope","required_output":"confirm_or_revert_out_of_scope","action":"warn","enabled":false},
		{"id":"r-tests","scope":"step","trigger":"tests_failed","required_output":"tests_green_or_explained","action":"warn","enabled":false},
		{"id":"r-reg","scope":"step","trigger":"regression_test_broke","required_output":"restore_green_without_weakening","action":"warn","enabled":false},
		{"id":"r-newtest","scope":"step","trigger":"production_change_no_new_test","required_output":"new_additive_test_file","action":"warn","enabled":false}
	]`)
	p4WriteFile(t, dir, "go.mod", "module bug440bed\n\ngo 1.21\n")
	svc := newFreezeTestService(t)
	svc.lspChecker = &lspFakeChecker{msg: "src/calc.go:3:1 error: undefined: X"}
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

	// Code file + audit note: rules clean, LSP fires on the written code file.
	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	p4WriteFile(t, dir, "change-audit/CA-998-bug440.md", "# audit\n")
	blocked := svc.runFlowGateAtEpoch(context.Background(), rs, "turn-code", finalizeInput{
		FinalMessage: "implemented calc + audit note",
		ChangedFiles: []string{"src/calc.go", "change-audit/CA-998-bug440.md"},
	}, 0)
	if !blocked {
		t.Fatal("LSP diagnostics must block the turn with a reprompt")
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
		t.Fatalf("LSP reprompt must stash this turn's code path src/calc.go, got %v", carried)
	}
}

// BUG-439: an inferred contract must carry a truthful, self-marked intent —
// contracts.ndjson rows with empty intent are content-free telemetry noise.
func TestBug439InferredContractSynthesizesScopeIntent(t *testing.T) {
	dir := t.TempDir()
	p4WriteFile(t, dir, "change-audit/FEATURE-KEYS.md", "- calc-core — calculator core\n")
	diff := []flowgate.ChangedFile{{Path: "src/calc.go", Status: "M"}}
	p := prepareChangeContract(context.Background(), dir, "run-439", "chat-run-439", "", "done", diff, []string{"calc-core"})
	if p.declared {
		t.Fatal("no declaration anywhere — must be inferred")
	}
	if p.contract.Confidence != changecontract.ConfidenceInferred {
		t.Fatalf("confidence = %q, want inferred", p.contract.Confidence)
	}
	if p.contract.FeatureKey != "calc-core" {
		t.Fatalf("feature_key = %q, want registered calc-core", p.contract.FeatureKey)
	}
	if strings.TrimSpace(p.contract.Intent) == "" {
		t.Fatal("inferred contract must not persist an empty intent")
	}
	if !strings.Contains(p.contract.Intent, "src") {
		t.Fatalf("inferred intent %q must reflect the observed scope", p.contract.Intent)
	}
}

// BUG-439: a suggested key that is not registered in FEATURE-KEYS.md is
// auto-catalog noise — the contract must stay explicitly unresolved rather
// than persist a misleading feature identity.
func TestBug439UnregisteredSuggestionIsNotPersisted(t *testing.T) {
	diff := []flowgate.ChangedFile{{Path: "src/calc.go", Status: "M"}}

	// Registry exists but the suggestion is not in it → explicit unresolved.
	dir := t.TempDir()
	p4WriteFile(t, dir, "change-audit/FEATURE-KEYS.md", "- real-key — registered feature\n")
	p := prepareChangeContract(context.Background(), dir, "run-439a", "chat", "", "done", diff, []string{"auto-catalog-noise"})
	if p.contract.FeatureKey != "" {
		t.Fatalf("feature_key = %q, want empty (suggestion not registered)", p.contract.FeatureKey)
	}

	// No registry at all → no verified identity exists; still explicit.
	dir2 := t.TempDir()
	p2 := prepareChangeContract(context.Background(), dir2, "run-439b", "chat", "", "done", diff, []string{"claude"})
	if p2.contract.FeatureKey != "" {
		t.Fatalf("feature_key = %q, want empty (no FEATURE-KEYS.md to verify against)", p2.contract.FeatureKey)
	}
	// Scope inference must still work — only identity is withheld.
	if len(p2.contract.DeclaredPaths) == 0 {
		t.Fatal("declared_paths must still be inferred from the diff")
	}
}

// BUG-439: a fully content-free inferred contract (no verified key, no code
// scope) must not be persisted — it is noise in contracts.ndjson.
func TestBug439ContentFreeInferredContractNotSaved(t *testing.T) {
	// Empty diff: nothing happened → nothing to persist.
	dir := t.TempDir()
	p := prepareChangeContract(context.Background(), dir, "run-439c", "chat", "", "done", nil, nil)
	if err := commitChangeContract(dir, p, canonicalPendingRoute{}, nil); err != nil {
		t.Fatalf("commit: %v", err)
	}
	store, err := changecontract.NewStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store.GetLatestForRun("run-439c"); ok {
		t.Fatal("content-free inferred contract must not be persisted")
	}

	// Docs/audit-only diff: no code scope → still nothing enforceable.
	dir2 := t.TempDir()
	diff := []flowgate.ChangedFile{{Path: "change-audit/CA-1-x.md", Status: "A"}}
	p2 := prepareChangeContract(context.Background(), dir2, "run-439d", "chat", "", "wrote docs", diff, nil)
	if err := commitChangeContract(dir2, p2, canonicalPendingRoute{}, nil); err != nil {
		t.Fatalf("commit: %v", err)
	}
	store2, err := changecontract.NewStore(dir2)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := store2.GetLatestForRun("run-439d"); ok {
		t.Fatal("docs/audit-only inferred contract must not be persisted")
	}
}

// BUG-439 companion guard: the synthesized inferred intent is scope telemetry
// — it must never overwrite a curated Canonical Head behavior statement.
func TestBug439InferredIntentDoesNotRewriteHeadStatement(t *testing.T) {
	dir := t.TempDir()
	p4WriteFile(t, dir, "change-audit/FEATURE-KEYS.md", "- calc-core — calculator core\n")
	if err := changecontract.SaveHead(dir, changecontract.CanonicalHead{
		FeatureKey:        "calc-core",
		BehaviorStatement: "curated behavior statement",
		Status:            changecontract.HeadStatusCurrent,
	}); err != nil {
		t.Fatal(err)
	}
	diff := []flowgate.ChangedFile{{Path: "src/calc.go", Status: "M"}}
	p := prepareChangeContract(context.Background(), dir, "run-439e", "chat", "", "done", diff, []string{"calc-core"})
	if err := commitChangeContract(dir, p, canonicalPendingRoute{}, nil); err != nil {
		t.Fatalf("commit: %v", err)
	}
	head, found, err := changecontract.LoadHead(dir, "calc-core")
	if err != nil || !found {
		t.Fatalf("head missing after commit: found=%v err=%v", found, err)
	}
	if head.BehaviorStatement != "curated behavior statement" {
		t.Fatalf("inferred intent must not overwrite the head statement, got %q", head.BehaviorStatement)
	}
}
