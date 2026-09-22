package runner

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/changeledger"
)

// --- CP-55 P-8: first coder context reflects the frozen contract ------------

// TestFrozenContractExistsBeforeFirstContextPackage proves the ordering
// invariant the whole freeze-chain mechanism exists to guarantee: by the
// time the writer's own context package is built, a FrozenContractRecord
// already durably exists for (runID, writer step) — the package is never
// built from a still-in-flight or not-yet-persisted contract.
func TestFrozenContractExistsBeforeFirstContextPackage(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainWithContextFixture()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	freezeNode, _ := findFlowNode(nodes, "freeze")

	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
		t.Fatal("expected runContractFreezeNode to return true (handled)")
	}

	store, err := changecontract.NewFrozenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.GetFrozenForStep(runID, "coder"); err != nil || !ok {
		t.Fatalf("expected a frozen record to already exist, ok=%v err=%v", ok, err)
	}

	svc.mu.Lock()
	pkg := svc.runs[runID].planContextPackage
	svc.mu.Unlock()
	if pkg == nil {
		t.Fatal("expected the writer's own context package to have been built during the freeze chain")
	}
}

// TestFirstCoderContextUsesCurrentFlowDeclaredPaths proves the frozen
// contract's DeclaredPaths flow through to source.excerpt for the writer's
// very first context package — via ExplicitSourcePaths, already wired before
// CP-55 P-8 (confirmed by research), pinned here as a regression guard.
func TestFirstCoderContextUsesCurrentFlowDeclaredPaths(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	p4WriteFile(t, dir, "src/calc.go", "package calc\n\nfunc Div(a, b int) int { return a / b }\n")
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainWithContextFixture()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	freezeNode, _ := findFlowNode(nodes, "freeze")

	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
		t.Fatal("expected true (handled)")
	}

	svc.mu.Lock()
	pkg := svc.runs[runID].planContextPackage
	svc.mu.Unlock()
	if pkg == nil {
		t.Fatal("expected a context package to have been built")
	}
	found := false
	for _, ex := range pkg.SourceExcerpts {
		if strings.Contains(ex.Path, "calc.go") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected the frozen contract's declared path (src/calc.go) to surface as a source excerpt, got %+v", pkg.SourceExcerpts)
	}
}

// TestFirstCoderContextRanksFeatureHistoryByCurrentLocus is the regression
// test for the CP-55 P-8 research finding: the first coder context build for
// a freshly frozen contract has no legacy Store record, no diff yet, and a
// UserPrompt that is only the frozen Intent sentence — every source
// buildRetrievalLocus previously had would be empty, silently degrading
// feature-history ranking to recency alone. With the explicitPaths fix
// (retrieval_locus.go) and the ResolvedFeatureKey fix
// (flow_context_package.go, so feature.history's ConfidenceVerified gate
// isn't blocked by a low-confidence NL guess against the one-sentence
// intent), ranking must actually activate here.
func TestFirstCoderContextRanksFeatureHistoryByCurrentLocus(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainWithContextFixture()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	freezeNode, _ := findFlowNode(nodes, "freeze")

	// Seed 40 ledger entries (above DefaultHistoryRankingConfig's activation
	// threshold of 30) for the SAME feature key the frozen contract declares
	// (calc-core, from validPlannerDraft), one of which touches the exact
	// declared path (src/calc.go) but is NOT the newest entry — only
	// locus-based ranking, not recency, would surface it prominently.
	dotFP := dir + "/.flowpilot"
	ledger, err := changeledger.New(dotFP)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := make([]changeledger.Entry, 40)
	for i := 0; i < 40; i++ {
		entries[i] = changeledger.Entry{
			CommitHash:   fmt.Sprintf("hash%03d", i),
			FeatureKey:   "calc-core",
			Summary:      fmt.Sprintf("unrelated entry %d", i),
			CommittedAt:  base.Add(time.Duration(i) * time.Hour).Format(time.RFC3339),
			ChangedPaths: []string{fmt.Sprintf("src/other%03d.go", i)},
		}
	}
	entries[5] = changeledger.Entry{
		CommitHash:   "hash-calc",
		FeatureKey:   "calc-core",
		Summary:      "the calc.go entry ranking must surface",
		CommittedAt:  base.Add(5 * time.Hour).Format(time.RFC3339),
		ChangedPaths: []string{"src/calc.go"},
	}
	if err := ledger.Upsert(entries); err != nil {
		t.Fatal(err)
	}

	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, validPlannerDraft) {
		t.Fatal("expected true (handled)")
	}

	svc.mu.Lock()
	pkg := svc.runs[runID].planContextPackage
	svc.mu.Unlock()
	if pkg == nil {
		t.Fatal("expected a context package to have been built")
	}
	if !strings.Contains(pkg.HistoryBlock, "ranked,") {
		t.Fatalf("expected feature-history ranking to activate for the writer's first context package, got HistoryBlock=%q", pkg.HistoryBlock)
	}
	if !strings.Contains(pkg.HistoryBlock, "the calc.go entry ranking must surface") {
		t.Fatalf("expected the entry touching the frozen contract's declared path to surface in the ranked output, got %q", pkg.HistoryBlock)
	}
}

// --- CP-55 P-8: Canonical Head lifecycle across a frozen writer's flow -----

// TestValidateFailLeavesCanonicalHeadUnchanged: a validation-failure retry
// (applyFlowControl "continue") after a frozen writer's gate pass must never
// finalize or otherwise touch the real Canonical Head.
func TestValidateFailLeavesCanonicalHeadUnchanged(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainFixture()
	parentID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)
	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	if svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: "done", ChangedFiles: []string{"src/calc.go"}}, 0) {
		t.Fatal("gate unexpectedly blocked")
	}

	if _, err := svc.applyFlowControl(parentID, FlowControlInput{Status: "continue", Summary: "validation failed, retry"}); err != nil {
		t.Fatalf("applyFlowControl(continue): %v", err)
	}

	if _, found, err := changecontract.LoadHead(dir, "calc-core"); err != nil || found {
		t.Fatalf("Canonical Head must remain unchanged after a validation-failure retry, found=%v err=%v", found, err)
	}
}

// TestReviewContinueLeavesCanonicalHeadUnchanged: same invariant, for a
// review-loop "continue" (reviewer requested changes) rather than a
// validation failure — this codebase routes both through the identical
// applyFlowControl("continue") path, so this is a traceability alias, not a
// distinct mechanism, matching CP-55 P-5's own established precedent
// (TestValidationFailureDoesNotFinalizeCanonicalHead /
// TestReviewRetryDoesNotFinalizeCanonicalHead).
func TestReviewContinueLeavesCanonicalHeadUnchanged(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainFixture()
	parentID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})
	rs := newP4ChildRun(svc, "child-1", parentID, dir, head)
	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	if svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs, "turn-1", finalizeInput{FinalMessage: "done", ChangedFiles: []string{"src/calc.go"}}, 0) {
		t.Fatal("gate unexpectedly blocked")
	}

	if _, err := svc.applyFlowControl(parentID, FlowControlInput{Status: "continue", Summary: "reviewer requested changes"}); err != nil {
		t.Fatalf("applyFlowControl(continue): %v", err)
	}

	if _, found, err := changecontract.LoadHead(dir, "calc-core"); err != nil || found {
		t.Fatalf("Canonical Head must remain unchanged after a review-requested-changes retry, found=%v err=%v", found, err)
	}
}

// TestTerminalDoneUpdatesCanonicalHeadExactlyOnce: across a full
// stage -> (retry) -> re-stage -> terminal-done sequence, the real Canonical
// Head file is written exactly once (at "done"), and a second "done" call on
// the same already-terminal loop does not write it again.
func TestTerminalDoneUpdatesCanonicalHeadExactlyOnce(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := freezeChainFixture()
	parentID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	freezeP4Contract(t, dir, parentID, "coder", head, []string{"src/calc.go"})

	// First pass stages a value that is then superseded by a second pass —
	// mirroring CP-55 P-5's own established TestFlowDoneFinalizesLatestAcceptedVersionOnly
	// pattern (two staging gate passes back to back; a review-loop's actual
	// "continue" transition is exercised separately by
	// TestReviewContinueLeavesCanonicalHeadUnchanged and is deliberately not
	// combined here, since freezeChainFixture's minimal 2-node topology has
	// no continue back-edge and driving applyFlowControl("continue") against
	// it triggers maybeReinvokeCoderForContinue's own background reinvoke
	// machinery, an unrelated concern this test does not need).
	rs1 := newP4ChildRun(svc, "child-1", parentID, dir, head)
	p4WriteFile(t, dir, "src/calc.go", "package calc\n")
	if svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs1, "turn-1", finalizeInput{FinalMessage: "done", ChangedFiles: []string{"src/calc.go"}}, 0) {
		t.Fatal("first gate pass unexpectedly blocked")
	}

	// Second pass, re-staging over the first: this is the value that must
	// finalize.
	rs2 := newP4ChildRun(svc, "child-2", parentID, dir, head)
	p4WriteFile(t, dir, "src/calc.go", "package calc // v2\n")
	if svc.runChildArtifactOutputGateAtEpoch(context.Background(), rs2, "turn-2", finalizeInput{FinalMessage: "done", ChangedFiles: []string{"src/calc.go"}}, 0) {
		t.Fatal("second gate pass unexpectedly blocked")
	}

	if _, err := svc.applyFlowControl(parentID, FlowControlInput{Status: "done"}); err != nil {
		t.Fatalf("applyFlowControl(done): %v", err)
	}
	first, found, err := changecontract.LoadHead(dir, "calc-core")
	if err != nil || !found {
		t.Fatalf("expected Canonical Head to be written exactly once at terminal done, found=%v err=%v", found, err)
	}

	// A duplicate/stale "done" on the now-terminal loop must not write again.
	if _, err := svc.applyFlowControl(parentID, FlowControlInput{Status: "done"}); err != nil {
		t.Fatalf("second (stale) applyFlowControl(done) call itself should not error: %v", err)
	}
	second, found2, err := changecontract.LoadHead(dir, "calc-core")
	if err != nil || !found2 {
		t.Fatalf("Canonical Head must still exist after the stale done call, found=%v err=%v", found2, err)
	}
	if first.IntentSignature != second.IntentSignature || first.UpdatedAt != second.UpdatedAt {
		t.Fatalf("Canonical Head must not be rewritten by a stale duplicate done call: first=%+v second=%+v", first, second)
	}
}
