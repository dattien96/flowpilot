package runner

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
)

// newP4CodeWriterFixture builds a parent run whose Flow topology declares a
// single agent.code writer node named "coder".
func newP4CodeWriterFixture(t *testing.T, workspace string) (svc *InteractiveService, parentID string) {
	t.Helper()
	svc = newFreezeTestService(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.activeFlowNodes = []agentpack.FlowNode{{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"}}
	prs.workspaceCwd = workspace
	svc.mu.Unlock()
	return svc, parent.RunID
}

// newP4ChildRun directly constructs a coder child run — the established
// pattern this package already uses for gate-path tests (see bug083_test.go,
// bug288_round11_test.go et al.) rather than driving a full spawn+adapter
// turn, since the gate function under test only reads a handful of plain
// fields off *interactiveRun.
func newP4ChildRun(svc *InteractiveService, childID, parentID, workspace, turnStartHead string) *interactiveRun {
	rs := &interactiveRun{
		id:               childID,
		parentRunID:      parentID,
		label:            "coder",
		workspaceCwd:     workspace,
		turnStartGitHead: turnStartHead,
		status:           RunStatusRunning,
		providerKey:      ProviderKeyCodex,
		subs:             map[int64]chan ProviderEvent{},
		idempotency:      map[string]string{},
	}
	svc.mu.Lock()
	svc.runs[childID] = rs
	svc.mu.Unlock()
	return rs
}

func freezeP4Contract(t *testing.T, workspace, runID, coderStepID, baseSHA string, declaredPaths []string) changecontract.FrozenContractRecord {
	t.Helper()
	store, err := changecontract.NewFrozenStore(workspace)
	if err != nil {
		t.Fatal(err)
	}
	draft := changecontract.PreflightContractDraft{FeatureKey: "calc-core", Intent: "fix rounding", DeclaredPaths: declaredPaths}
	rec, err := changecontract.FreezeContract(workspace, runID, "planner", coderStepID, draft, baseSHA, nil, "", 1, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveFrozen(rec); err != nil {
		t.Fatal(err)
	}
	return rec
}

func p4WriteFile(t *testing.T, dir, relPath, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// --- requires a frozen contract ----------------------------------------------

func TestFlowCoderRequiresFrozenContract(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)

	if !svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: "done", ChangedFiles: []string{"src/calc.go"}}, 0) {
		t.Fatal("expected block: no frozen contract exists for this coding step")
	}
}

func TestFlowCoderRejectsPostTurnDeclarationWithoutFrozenContract(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)

	finalMsg := "[Change Contract]\nfeature: calc-core\nintent: fix\nfiles: src/calc.go\n\ndone"
	if !svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: finalMsg, ChangedFiles: []string{"src/calc.go"}}, 0) {
		t.Fatal("a post-turn declaration must not satisfy the missing-frozen-contract requirement")
	}
	if store, err := changecontract.OpenStoreReadOnly(dir); err == nil {
		if _, ok := store.GetLatestForRun(parentID); ok {
			t.Fatal("agent.code must never fall through to the legacy declared-contract path")
		}
	}
}

// --- uses frozen scope, not the final message --------------------------------

func TestFlowCoderUsesFrozenScopeInsteadOfFinalMessage(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)

	p4WriteFile(t, dir, "src/calc.go", "package calc\n")

	// Final message claims a DIFFERENT file — must be ignored entirely.
	finalMsg := "[Change Contract]\nfeature: calc-core\nintent: fix\nfiles: src/unrelated.go\n\ndone"
	if svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: finalMsg, ChangedFiles: []string{"src/calc.go"}}, 0) {
		t.Fatal("actual write matches the frozen scope; the final message's own (different) claim must not matter")
	}
}

func TestFlowCoderComputesWrittenPathsAgainstFrozenBaseline(t *testing.T) {
	dir, head0 := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head0, []string{"src/calc.go"})

	// An out-of-scope file is committed — this must be attributed to the
	// coder's turn when diffed against the frozen contract's own BaseSHA
	// (head0), even though turnStartGitHead below is deliberately set to a
	// LATER commit (head1) that would hide this diff if that field, rather
	// than rec.BaseSHA, were used to compute written paths.
	p4WriteFile(t, dir, "src/other.go", "package other\n")
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@e", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@e")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("add", "-A")
	runGit("commit", "-m", "out of scope commit")
	head1, err := captureGitHead(dir)
	if err != nil {
		t.Fatal(err)
	}

	rs := newP4ChildRun(svc, "child-1", parentID, dir, head1) // deliberately wrong reference point

	if !svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: "done"}, 0) {
		t.Fatal("expected block: the frozen contract's own BaseSHA (not turnStartGitHead) must be used to compute written paths")
	}
}

// --- scope drift blocks --------------------------------------------------------

func TestFlowScopeDriftBlocksAcceptance(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)

	p4WriteFile(t, dir, "src/unexpected.go", "package x\n")

	if !svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: "done", ChangedFiles: []string{"src/unexpected.go"}}, 0) {
		t.Fatal("writing outside the frozen declared scope must block")
	}
}

func TestFlowScopeDriftReportsExactUnexpectedPaths(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-2", parentID, dir, head)

	p4WriteFile(t, dir, "src/surprise.go", "package x\n")

	svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: "done", ChangedFiles: []string{"src/surprise.go"}}, 0)

	svc.mu.Lock()
	var msg string
	for _, ev := range rs.events {
		if ev.Type == EventFlowGateViolation {
			msg = ev.Error
		}
	}
	svc.mu.Unlock()
	if !strings.Contains(msg, "src/surprise.go") {
		t.Fatalf("gate message must name the exact unexpected path, got %q", msg)
	}
}

func TestFlowScopeDriftCannotBeSatisfiedByInference(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-3", parentID, dir, head)

	p4WriteFile(t, dir, "src/sneaky.go", "package x\n")

	// Final message explicitly declares the out-of-scope path — must not help.
	finalMsg := "[Change Contract]\nfeature: calc-core\nintent: fix\nfiles: src/sneaky.go\n\ndone"
	if !svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: finalMsg, ChangedFiles: []string{"src/sneaky.go"}}, 0) {
		t.Fatal("a coder cannot satisfy Flow preflight by declaring the drifted path after the fact")
	}
	if store, err := changecontract.OpenStoreReadOnly(dir); err == nil {
		if _, ok := store.GetLatestForRun(parentID); ok {
			t.Fatal("agent.code must never persist a legacy declared contract as a side effect")
		}
	}
}

// --- amendment ------------------------------------------------------------------

func TestFlowContractAmendmentCreatesHigherVersionBeforeRetry(t *testing.T) {
	dir := t.TempDir()
	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	draft := changecontract.PreflightContractDraft{FeatureKey: "calc-core", Intent: "fix", DeclaredPaths: []string{"src/calc.go"}}
	rec, err := changecontract.FreezeContract(dir, "run-1", "planner", "coder", draft, "sha1", nil, "", 1, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveFrozen(rec); err != nil {
		t.Fatal(err)
	}

	amended, err := changecontract.AmendFrozenContract(store, dir, rec, []string{"src/extra.go"}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if amended.Version != 2 {
		t.Fatalf("Version = %d, want 2", amended.Version)
	}
	found := false
	for _, p := range amended.DeclaredPaths {
		if p == "src/extra.go" {
			found = true
		}
	}
	if !found {
		t.Fatalf("amended DeclaredPaths must include the new path: %v", amended.DeclaredPaths)
	}

	// Re-open fresh, not `store` (already holds the write in memory): proves
	// the amendment actually reached disk and round-trips through the real
	// NDJSON load path, not just that the in-memory maps were updated.
	reopened, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := reopened.ListVersionsForStep("run-1", "coder")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 persisted versions after reopen, got %d", len(versions))
	}
}

func TestFlowContractAmendmentSupersedesPriorVersion(t *testing.T) {
	dir := t.TempDir()
	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	draft := changecontract.PreflightContractDraft{FeatureKey: "calc-core", Intent: "fix", DeclaredPaths: []string{"src/calc.go"}}
	rec, err := changecontract.FreezeContract(dir, "run-1", "planner", "coder", draft, "sha1", nil, "", 1, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveFrozen(rec); err != nil {
		t.Fatal(err)
	}

	amended, err := changecontract.AmendFrozenContract(store, dir, rec, []string{"src/extra.go"}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if amended.Supersedes != rec.ContractID {
		t.Fatalf("Supersedes = %q, want %q", amended.Supersedes, rec.ContractID)
	}

	// Re-open fresh, not `store`: proves the supersede status event and the
	// new active version both round-trip through the real NDJSON files, not
	// just the in-memory maps the same instance that wrote them already has.
	reopened, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	active, ok, err := reopened.GetFrozenForStep("run-1", "coder")
	if err != nil || !ok {
		t.Fatalf("expected an active version, ok=%v err=%v", ok, err)
	}
	if active.ContractID != amended.ContractID {
		t.Fatalf("active version = %q, want the amended one %q", active.ContractID, amended.ContractID)
	}
	versions, err := reopened.ListVersionsForStep("run-1", "coder")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 {
		t.Fatalf("expected 2 persisted versions, got %d", len(versions))
	}
}

func TestRetryWithoutNewPathsReusesFrozenVersion(t *testing.T) {
	dir := t.TempDir()
	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	draft := changecontract.PreflightContractDraft{FeatureKey: "calc-core", Intent: "fix", DeclaredPaths: []string{"src/calc.go"}}
	rec, err := changecontract.FreezeContract(dir, "run-1", "planner", "coder", draft, "sha1", nil, "", 1, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveFrozen(rec); err != nil {
		t.Fatal(err)
	}

	same, err := changecontract.AmendFrozenContract(store, dir, rec, nil, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if same.ContractID != rec.ContractID || same.Version != rec.Version {
		t.Fatalf("a retry with no new paths must reuse the existing version unchanged, got %+v", same)
	}

	// Re-open fresh: proves no second version was ever written to disk, not
	// just that the in-memory view of the instance that (didn't) write it
	// still shows 1.
	reopened, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	versions, err := reopened.ListVersionsForStep("run-1", "coder")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 {
		t.Fatalf("expected exactly 1 version (no amendment minted), got %d", len(versions))
	}
}

func TestFlowContractAmendmentRejectsNonConcreteAdditionalPath(t *testing.T) {
	dir := t.TempDir()
	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	draft := changecontract.PreflightContractDraft{FeatureKey: "calc-core", Intent: "fix", DeclaredPaths: []string{"src/calc.go"}}
	rec, err := changecontract.FreezeContract(dir, "run-1", "planner", "coder", draft, "sha1", nil, "", 1, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveFrozen(rec); err != nil {
		t.Fatal(err)
	}

	// "Makefile" has no extension, so IsConcreteCodeTarget rejects it — the
	// amendment must fail loudly (CA-427 Finding 5), not silently return
	// `existing` unchanged as if the caller had asked for nothing new.
	if _, err := changecontract.AmendFrozenContract(store, dir, rec, []string{"Makefile"}, time.Now().UTC()); err == nil {
		t.Fatal("expected an explicit error for a non-concrete amendment path, not a silent no-op")
	}
}

// --- Normal chat / non-flow non-regression -------------------------------------

func TestNormalChatKeepsLegacyDeclaredContractBehavior(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	parent, startErr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if startErr != nil {
		t.Fatal(startErr)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.activeFlowNodes = []agentpack.FlowNode{{ID: "coder", Behavior: "agent.delegate", Agent: "agents/coder.md"}}
	prs.workspaceCwd = dir
	svc.mu.Unlock()

	rs := newP4ChildRun(svc, "child-legacy", parent.RunID, dir, head)
	p4WriteFile(t, dir, "legacy.go", "package main\n")

	finalMsg := "[Change Contract]\nfeature: calc-core\nintent: fix\nfiles: legacy.go\n\ndone"
	svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: finalMsg, ChangedFiles: []string{"legacy.go"}}, 0)

	store, err := changecontract.OpenStoreReadOnly(dir)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := store.GetLatestForRun(parent.RunID)
	if !ok {
		t.Fatal("expected the legacy declared Contract to still be persisted for an agent.delegate child, unaffected by CP-55 P-4")
	}
	if c.Confidence != changecontract.ConfidenceDeclared {
		t.Fatalf("Confidence = %q, want declared", c.Confidence)
	}
}

func TestNormalChatKeepsLegacyInferredContractBehavior(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	parent, startErr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if startErr != nil {
		t.Fatal(startErr)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.activeFlowNodes = []agentpack.FlowNode{{ID: "coder", Behavior: "agent.delegate", Agent: "agents/coder.md"}}
	prs.workspaceCwd = dir
	svc.mu.Unlock()

	rs := newP4ChildRun(svc, "child-inferred", parent.RunID, dir, head)
	p4WriteFile(t, dir, "inferred.go", "package main\n")
	// Every doc-scope rule other than r-contract itself defaults to
	// "reprompt"/"block" on an undocumented code change (missing change-audit
	// note, missing feature key, etc.) — none of that is what this test is
	// about. Disable them and downgrade r-contract to "warn" so this test can
	// isolate and prove the one thing under test: the inferred Contract
	// itself still gets computed and committed for an agent.delegate child,
	// unaffected by CP-55 P-4 (which only ever branches on agent.code).
	p4WriteFile(t, dir, ".flowpilot/settings/flow-rules.json", `[
		{"id":"r-contract","scope":"step","trigger":"code_changed_no_contract","required_output":"declared_change_contract","action":"warn","enabled":true},
		{"id":"r-ca","scope":"step","trigger":"code_changed","required_output":"change_audit_note","action":"warn","enabled":false},
		{"id":"r-fk","scope":"step","trigger":"commit_feature_key_missing","required_output":"verified_feature_key","action":"warn","enabled":false},
		{"id":"r-bug","scope":"step","trigger":"bug_fixed","required_output":"bugfix_doc","action":"warn","enabled":false},
		{"id":"r-task","scope":"step","trigger":"task_referenced","required_output":"task_doc","action":"warn","enabled":false},
		{"id":"r-scope","scope":"step","trigger":"edit_outside_declared_scope","required_output":"confirm_or_revert_out_of_scope","action":"warn","enabled":false},
		{"id":"r-tests","scope":"step","trigger":"tests_failed","required_output":"tests_green_or_explained","action":"warn","enabled":false},
		{"id":"r-reg","scope":"step","trigger":"regression_test_broke","required_output":"restore_green_without_weakening","action":"warn","enabled":false}
	]`)

	svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: "did the work", ChangedFiles: []string{"inferred.go"}}, 0)

	store, err := changecontract.OpenStoreReadOnly(dir)
	if err != nil {
		t.Fatal(err)
	}
	c, ok := store.GetLatestForRun(parent.RunID)
	if !ok {
		t.Fatal("expected an inferred Contract to still be persisted for an agent.delegate child, unaffected by CP-55 P-4")
	}
	if c.Confidence != changecontract.ConfidenceInferred {
		t.Fatalf("Confidence = %q, want inferred", c.Confidence)
	}
}

func TestNormalChatCodeRequestIsNotBlockedForMissingFlow(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	parent, startErr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if startErr != nil {
		t.Fatal(startErr)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.workspaceCwd = dir // no activeFlowNodes at all — no flow topology
	svc.mu.Unlock()

	rs := newP4ChildRun(svc, "child-noflow", parent.RunID, dir, head)
	rs.label = "" // no flow node to resolve either

	if svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: "done"}, 0) {
		t.Fatal("a code request with no active flow topology must not be blocked by the CP-55 P-4 frozen-scope check")
	}
}

// --- change.contract source disablement is orthogonal to gate enforcement -----

func TestChangeContractSourceDisabledDoesNotDisableFlowGate(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})

	// Simulate "change.contract" context source not selected for this node —
	// the frozen-scope gate check never reads ContextSources at all; it
	// queries FrozenStore directly, independent of what gets rendered.
	svc.mu.Lock()
	prs := svc.runs[parentID]
	prs.activeFlowNodes = []agentpack.FlowNode{{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md", ContextSources: []string{"source.excerpt"}}}
	svc.mu.Unlock()

	rs := newP4ChildRun(svc, "child-src-disabled", parentID, dir, head)
	p4WriteFile(t, dir, "src/off_scope.go", "package x\n")

	if !svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: "done", ChangedFiles: []string{"src/off_scope.go"}}, 0) {
		t.Fatal("frozen-scope enforcement must not depend on change.contract being an enabled rendered context source")
	}
}

// --- security: only the frozen store's own two files are exempt -------------

// TestFlowScopeDriftDetectsRewrittenGateRulesFile is the direct regression
// test for CA-427 Finding 2: the original .flowpilot/**-wide exemption would
// have let a writer silently rewrite its own gate configuration (disabling
// every rule that would otherwise judge it) with zero drift ever detected.
func TestFlowScopeDriftDetectsRewrittenGateRulesFile(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-self-disable", parentID, dir, head)

	// Not declared, and NOT the frozen store's own bookkeeping file — must
	// count as drift, unlike .flowpilot/contracts/frozen_contracts.ndjson.
	p4WriteFile(t, dir, ".flowpilot/settings/flow-rules.json", `[{"id":"r-contract","enabled":false}]`)

	if !svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: "done", ChangedFiles: []string{".flowpilot/settings/flow-rules.json"}}, 0) {
		t.Fatal("a writer rewriting its own gate rules file must be treated as scope drift, not silently exempted")
	}
}

// TestFlowScopeDriftDetectsForgedFrozenContractFile is the companion
// regression test: a writer directly forging frozen_contracts.ndjson (rather
// than going through FreezeContract/SaveFrozen) must also count as drift —
// only the ONE exact file the real freeze mechanism itself writes is exempt,
// not the whole containing directory.
func TestFlowScopeDriftDetectsForgedFrozenContractFile(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-forge", parentID, dir, head)

	// A DIFFERENT file under .flowpilot/contracts/ than the two exact
	// bookkeeping files — must still count as drift.
	p4WriteFile(t, dir, ".flowpilot/contracts/forged.ndjson", `{"contract_id":"evil"}`)

	if !svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: "done", ChangedFiles: []string{".flowpilot/contracts/forged.ndjson"}}, 0) {
		t.Fatal("a file under .flowpilot/contracts/ other than the store's own two exact files must still count as drift")
	}
}

// --- robust to live-topology resolution failure ------------------------------

// TestFlowCoderEnforcedEvenWhenLiveTopologyUnresolved is the direct
// regression test for CA-427 Finding 3: gating enforcement on flowNodeForRun
// resolving successfully meant a runner restart, topology reload, or any
// other reason the live in-memory lookup fails would silently skip
// enforcement entirely for a step a frozen contract really is bound to. The
// frozen record's own existence for (parentID, coderStepID) must be
// sufficient on its own, independent of whether the node happens to resolve.
func TestFlowCoderEnforcedEvenWhenLiveTopologyUnresolved(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})

	// Simulate topology resolution failing: the parent's activeFlowNodes no
	// longer contains a node matching this child's label (e.g. a runner
	// restart lost the in-memory topology, or the flow was reloaded).
	svc.mu.Lock()
	prs := svc.runs[parentID]
	prs.activeFlowNodes = nil
	svc.mu.Unlock()

	rs := newP4ChildRun(svc, "child-unresolved", parentID, dir, head)
	p4WriteFile(t, dir, "src/unexpected.go", "package x\n")

	if !svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: "done", ChangedFiles: []string{"src/unexpected.go"}}, 0) {
		t.Fatal("a step with a real bound frozen contract must still be enforced even when live topology fails to resolve the node")
	}
}

// --- fails closed, not open, on an unobservable baseline ---------------------

// TestFlowCoderBlocksOnUnobservableFrozenBaseline is the direct regression
// test for CA-427 Finding 1: the original implementation degraded to
// fin.ChangedFiles (AI-tool-call-reported, frequently empty) when the diff
// against the frozen contract's own BaseSHA could not be observed — an empty
// ChangedFiles list then meant zero drift detected, i.e. the turn PASSED
// with nothing actually verified. This must fail closed instead.
func TestFlowCoderBlocksOnUnobservableFrozenBaseline(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	// Freeze against a BaseSHA that does not exist in this repo (simulating a
	// rebase/reset/re-clone since the freeze, or a corrupt ref) — `git diff
	// <badsha>..HEAD` fails (exit 128), so ObserveGitDiffSince errors.
	freezeP4Contract(t, dir, parentID, "coder", "0000000000000000000000000000000000000000", []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-unobservable", parentID, dir, head)

	// No ChangedFiles reported at all — the dangerous case: if the
	// implementation fell back to fin.ChangedFiles on the observation
	// error, an empty list here would read as "nothing written" and pass.
	if !svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: "done"}, 0) {
		t.Fatal("an unobservable frozen baseline must block (fail closed), not silently pass with zero verified drift")
	}
}
