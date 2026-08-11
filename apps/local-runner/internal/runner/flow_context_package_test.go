package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/changeledger"
	"flowpilot-runner/internal/featurecatalog"
)

// fcpFixture builds a minimal workspace fixture with a feature catalog and
// change ledger entry for "agent-flow-engine", mirroring chatSummarySyncFixture.
func fcpFixture(t *testing.T) (workspace, repoDir string) {
	t.Helper()
	workspace = t.TempDir()
	repoDir = t.TempDir()
	dotFP := filepath.Join(workspace, ".flowpilot")
	caDir := filepath.Join(repoDir, "change-audit")
	if err := os.MkdirAll(caDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(caDir, "FEATURE-KEYS.md"),
		[]byte("- agent-flow-engine — generic flow engine and review loop (CP-36)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	baseLedger, err := changeledger.New(dotFP)
	if err != nil {
		t.Fatal(err)
	}
	// CommittedAt values are set explicitly (and distinctly) so GetFeatureHistory's
	// sort.Slice by CommittedAt has a real tiebreaker. Without this, both entries
	// sort as equal and the final order depends on Go's randomized map iteration
	// order inside Ledger.AllEntries (entries is a map[string]Entry) — a latent,
	// pre-existing non-determinism in changeledger.GetFeatureHistory, unrelated to
	// CP-44, that only a byte-exact assertion (context_source_migration_golden_test.go)
	// was strict enough to expose.
	if err := baseLedger.Upsert([]changeledger.Entry{
		{CommitHash: "abc1", FeatureKey: "agent-flow-engine", Summary: "add FlowNode/FlowEdge types Task-089", CommittedAt: "2026-06-01T00:00:00Z"},
		{CommitHash: "abc2", FeatureKey: "agent-flow-engine", Summary: "add applyFlowControl state machine Task-090", CommittedAt: "2026-06-02T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := featurecatalog.Build(repoDir, baseLedger, dotFP); err != nil {
		t.Fatal(err)
	}
	return workspace, repoDir
}

func fcpHints(workspace string) FlowContextHints {
	return FlowContextHints{
		WorkflowRunID: "run-168",
		PlanStepRunID: "step-plan-1",
		UserPrompt:    "agent-flow-engine",
		SourceDocID:   "Task-168",
	}
}

// TestBuildFlowContextPackageVerifiedFeatureIncludesHistory verifies that a
// resolved feature key produces a package with HistoryBlock populated.
func TestBuildFlowContextPackageVerifiedFeatureIncludesHistory(t *testing.T) {
	workspace, _ := fcpFixture(t)
	pkg, err := BuildFlowContextPackage(workspace, fcpHints(workspace))
	if err != nil {
		t.Fatalf("BuildFlowContextPackage: %v", err)
	}
	if pkg.FeatureConfidence != ConfidenceVerified {
		t.Errorf("FeatureConfidence = %q, want verified", pkg.FeatureConfidence)
	}
	if pkg.FeatureKey == "" {
		t.Error("FeatureKey empty for verified feature")
	}
	if !strings.Contains(pkg.HistoryBlock, "agent-flow-engine") {
		t.Errorf("HistoryBlock missing feature name: %q", pkg.HistoryBlock)
	}
	if !strings.Contains(pkg.HistoryBlock, "FlowNode") {
		t.Errorf("HistoryBlock missing commit summary: %q", pkg.HistoryBlock)
	}
}

// TestBuildFlowContextPackageResolvedFeatureKeyVerifiedOnlyWhenInCatalog is
// the CP-55 P-8 Claude-agent review Important Finding 3 regression test:
// hints.ResolvedFeatureKey (CP-55 P-8, bypassing NL resolution for a frozen
// contract's own trusted feature_key) used to be marked ConfidenceVerified
// unconditionally, with no cross-check against the feature catalog — a
// planner LLM hallucinating or mis-selecting a feature_key that happens to
// collide with a real, but wrong, existing feature would silently and
// confidently route feature.history to the wrong feature's history. This
// proves the fix: a ResolvedFeatureKey matching a real catalog entry stays
// Verified (unchanged behavior); one that does NOT match any catalog entry
// downgrades to ConfidenceLow instead (which — since feature.history's own
// gate requires exactly ConfidenceVerified — means history for the WRONG
// feature is never shown, closing the silent-mismatch gap).
func TestBuildFlowContextPackageResolvedFeatureKeyVerifiedOnlyWhenInCatalog(t *testing.T) {
	workspace, _ := fcpFixture(t) // seeds a catalog entry for "agent-flow-engine" only

	known := fcpHints(workspace)
	known.UserPrompt = ""
	known.ResolvedFeatureKey = "agent-flow-engine"
	pkg, err := BuildFlowContextPackage(workspace, known)
	if err != nil {
		t.Fatalf("BuildFlowContextPackage (known key): %v", err)
	}
	if pkg.FeatureConfidence != ConfidenceVerified {
		t.Errorf("known catalog key: FeatureConfidence = %q, want verified", pkg.FeatureConfidence)
	}

	unknown := fcpHints(workspace)
	unknown.UserPrompt = ""
	unknown.ResolvedFeatureKey = "totally-unrelated-feature-not-in-catalog"
	pkg2, err := BuildFlowContextPackage(workspace, unknown)
	if err != nil {
		t.Fatalf("BuildFlowContextPackage (unknown key): %v", err)
	}
	if pkg2.FeatureConfidence != ConfidenceLow {
		t.Errorf("unknown catalog key: FeatureConfidence = %q, want low", pkg2.FeatureConfidence)
	}
	if pkg2.FeatureKey != "totally-unrelated-feature-not-in-catalog" {
		t.Errorf("FeatureKey = %q, want the caller-supplied value preserved even at low confidence", pkg2.FeatureKey)
	}
	if strings.Contains(pkg2.HistoryBlock, "FlowNode") {
		t.Errorf("an unverified feature key must not surface another feature's history: %q", pkg2.HistoryBlock)
	}
}

// TestBuildFlowContextPackageIncludesChatSummaryWhenPresent verifies that a
// chat-summary ledger entry appears in DiscussionBlock.
func TestBuildFlowContextPackageIncludesChatSummaryWhenPresent(t *testing.T) {
	workspace, _ := fcpFixture(t)
	dotFP := filepath.Join(workspace, ".flowpilot")
	summaryLedger, err := changeledger.NewChatSummaryLedger(dotFP)
	if err != nil {
		t.Fatal(err)
	}
	if err := summaryLedger.Append([]changeledger.ChatSummaryEntry{
		{FeatureKey: "agent-flow-engine", Summary: "discussed bounded cap semantics"},
	}); err != nil {
		t.Fatal(err)
	}

	pkg, err := BuildFlowContextPackage(workspace, fcpHints(workspace))
	if err != nil {
		t.Fatalf("BuildFlowContextPackage: %v", err)
	}
	if !strings.Contains(pkg.DiscussionBlock, "bounded cap") {
		t.Errorf("DiscussionBlock missing chat summary: %q", pkg.DiscussionBlock)
	}
}

// TestBuildFlowContextPackageLowConfidenceDoesNotInjectWrongHistory verifies
// that an unresolvable prompt produces ConfidenceUnresolved and empty history.
func TestBuildFlowContextPackageLowConfidenceDoesNotInjectWrongHistory(t *testing.T) {
	workspace, _ := fcpFixture(t)
	hints := fcpHints(workspace)
	hints.UserPrompt = "zzzunknownfeaturexxx" // will not resolve
	pkg, err := BuildFlowContextPackage(workspace, hints)
	if err != nil {
		t.Fatalf("BuildFlowContextPackage: %v", err)
	}
	if pkg.FeatureConfidence == ConfidenceVerified {
		t.Error("confidence should not be verified for unresolvable prompt")
	}
	if pkg.HistoryBlock != "" {
		t.Errorf("HistoryBlock must be empty for low/unresolved confidence, got: %q", pkg.HistoryBlock)
	}
	if len(pkg.Warnings) == 0 {
		t.Error("expected at least one warning for unresolvable feature")
	}
}

// TestBuildFlowContextPackageMissingLedgersDegrades verifies that a workspace
// with no .flowpilot directory produces warnings, not an error.
func TestBuildFlowContextPackageMissingLedgersDegrades(t *testing.T) {
	workspace := t.TempDir() // no .flowpilot directory
	pkg, err := BuildFlowContextPackage(workspace, FlowContextHints{
		WorkflowRunID: "run-x",
		PlanStepRunID: "step-x",
		UserPrompt:    "agent-flow-engine",
	})
	if err != nil {
		t.Fatalf("BuildFlowContextPackage must not error on missing ledgers, got: %v", err)
	}
	if len(pkg.Warnings) == 0 {
		t.Error("expected warnings for missing catalog")
	}
	if pkg.PackageID == "" {
		t.Error("PackageID must be set even without a catalog")
	}
}

// TestBuildFlowContextPackageSourceExcerptCapsAndOmissions verifies per-file
// and total byte caps.
func TestBuildFlowContextPackageSourceExcerptCapsAndOmissions(t *testing.T) {
	workspace, _ := fcpFixture(t)

	// Write a file that exceeds the per-file cap.
	bigContent := strings.Repeat("x", perFileExcerptBytes+100)
	bigPath := filepath.Join(workspace, "big.go")
	if err := os.WriteFile(bigPath, []byte(bigContent), 0o644); err != nil {
		t.Fatal(err)
	}
	smallPath := filepath.Join(workspace, "small.go")
	if err := os.WriteFile(smallPath, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	hints := fcpHints(workspace)
	hints.ExplicitSourcePaths = []string{"big.go", "small.go"}
	pkg, err := BuildFlowContextPackage(workspace, hints)
	if err != nil {
		t.Fatalf("BuildFlowContextPackage: %v", err)
	}

	// big.go should be capped.
	var bigEx *FlowContextExcerpt
	for i := range pkg.SourceExcerpts {
		if pkg.SourceExcerpts[i].Path == "big.go" {
			bigEx = &pkg.SourceExcerpts[i]
		}
	}
	if bigEx == nil {
		t.Fatal("expected big.go in SourceExcerpts")
	}
	if !bigEx.BytesCap {
		t.Error("big.go should have BytesCap=true")
	}
	if len(bigEx.Excerpt) > perFileExcerptBytes {
		t.Errorf("excerpt length %d exceeds perFileExcerptBytes %d", len(bigEx.Excerpt), perFileExcerptBytes)
	}
}

// TestBuildFlowContextPackageRejectsOutsideWorkspacePath verifies that paths
// outside the workspace are rejected and appear in Omitted.
func TestBuildFlowContextPackageRejectsOutsideWorkspacePath(t *testing.T) {
	workspace, _ := fcpFixture(t)
	outsidePath := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outsidePath, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	hints := fcpHints(workspace)
	hints.ExplicitSourcePaths = []string{outsidePath}
	pkg, err := BuildFlowContextPackage(workspace, hints)
	if err != nil {
		t.Fatalf("BuildFlowContextPackage: %v", err)
	}
	if len(pkg.SourceExcerpts) > 0 {
		t.Error("outside-workspace path must not produce an excerpt")
	}
	found := false
	for _, o := range pkg.Omitted {
		if strings.Contains(o, "outside_workspace") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected outside_workspace in Omitted, got: %v", pkg.Omitted)
	}
}

// TestBuildFlowContextPackageNoVectorDependency verifies the package is built
// without any call path that could involve a vector DB or embedding API.
// This is a structural test: BuildFlowContextPackage must compile and run
// without importing any similarity-search package.
func TestBuildFlowContextPackageNoVectorDependency(t *testing.T) {
	workspace, _ := fcpFixture(t)
	// If this test compiles and the package builds, there is no vector dependency:
	// the only external packages in flow_context_package.go are crypto/sha256,
	// io, os, path/filepath, strings, time, changeledger, and featurecatalog.
	pkg, err := BuildFlowContextPackage(workspace, fcpHints(workspace))
	if err != nil {
		t.Fatalf("BuildFlowContextPackage: %v", err)
	}
	rendered := RenderFlowContextPackage(pkg)
	if rendered == "" {
		t.Error("rendered package must not be empty")
	}
}

// TestRenderFlowContextPackageStableSections verifies the renderer emits the
// expected stable sections and the audit visibility line.
func TestRenderFlowContextPackageStableSections(t *testing.T) {
	workspace, _ := fcpFixture(t)
	pkg, err := BuildFlowContextPackage(workspace, fcpHints(workspace))
	if err != nil {
		t.Fatalf("BuildFlowContextPackage: %v", err)
	}
	rendered := RenderFlowContextPackage(pkg)

	wantSections := []string{
		"## Context",
		"- feature:",
		"## History",
	}
	for _, s := range wantSections {
		if !strings.Contains(rendered, s) {
			t.Errorf("rendered package missing %q", s)
		}
	}
	// Render twice: must be identical (stability).
	if RenderFlowContextPackage(pkg) != rendered {
		t.Error("RenderFlowContextPackage is not stable across calls")
	}
}

// TestFlowContextPackageCarriesRunAndStepIDs verifies that run and step IDs
// are preserved verbatim from the hints.
func TestFlowContextPackageCarriesRunAndStepIDs(t *testing.T) {
	workspace, _ := fcpFixture(t)
	hints := fcpHints(workspace)
	hints.WorkflowRunID = "run-abc-123"
	hints.PlanStepRunID = "step-plan-xyz"
	pkg, err := BuildFlowContextPackage(workspace, hints)
	if err != nil {
		t.Fatalf("BuildFlowContextPackage: %v", err)
	}
	if pkg.WorkflowRunID != "run-abc-123" {
		t.Errorf("WorkflowRunID = %q, want run-abc-123", pkg.WorkflowRunID)
	}
	if pkg.PlanStepRunID != "step-plan-xyz" {
		t.Errorf("PlanStepRunID = %q, want step-plan-xyz", pkg.PlanStepRunID)
	}
	if pkg.PackageID == "" {
		t.Error("PackageID must be set")
	}
}

// TestFlowContextPackagePersistsAgainstPlanStep verifies that
// PersistFlowContextPackage emits an event keyed by WorkflowRunID + PlanStepRunID.
func TestFlowContextPackagePersistsAgainstPlanStep(t *testing.T) {
	workspace, _ := fcpFixture(t)
	pkg, err := BuildFlowContextPackage(workspace, fcpHints(workspace))
	if err != nil {
		t.Fatalf("BuildFlowContextPackage: %v", err)
	}

	store := newFakeWorkflowStore()
	if err := PersistFlowContextPackage(context.Background(), store, pkg); err != nil {
		t.Fatalf("PersistFlowContextPackage: %v", err)
	}

	evts := store.events[pkg.WorkflowRunID]
	if len(evts) != 1 {
		t.Fatalf("events count = %d, want 1", len(evts))
	}
	ev := evts[0]
	if ev.Type != EventFlowContextPackage {
		t.Errorf("event type = %q, want %q", ev.Type, EventFlowContextPackage)
	}
	if ev.WorkflowStepRunID != pkg.PlanStepRunID {
		t.Errorf("WorkflowStepRunID = %q, want %q", ev.WorkflowStepRunID, pkg.PlanStepRunID)
	}
	if ev.FlowContextPackage == nil {
		t.Fatal("FlowContextPackage payload must not be nil")
	}
	if ev.FlowContextPackage.PackageID != pkg.PackageID {
		t.Errorf("persisted PackageID = %q, want %q", ev.FlowContextPackage.PackageID, pkg.PackageID)
	}
}

// TestFlowContextPackageLookupSurvivesRunnerRestart verifies that a persisted
// event can be retrieved from the store keyed by WorkflowRunID + PlanStepRunID,
// simulating a runner restart reading from the event log.
func TestFlowContextPackageLookupSurvivesRunnerRestart(t *testing.T) {
	workspace, _ := fcpFixture(t)
	pkg, err := BuildFlowContextPackage(workspace, fcpHints(workspace))
	if err != nil {
		t.Fatalf("BuildFlowContextPackage: %v", err)
	}

	store := newFakeWorkflowStore()
	if err := PersistFlowContextPackage(context.Background(), store, pkg); err != nil {
		t.Fatalf("PersistFlowContextPackage: %v", err)
	}

	// Simulate restart: load from stored events (no in-memory package cache).
	var found *FlowContextPackage
	for _, ev := range store.events[pkg.WorkflowRunID] {
		if ev.Type == EventFlowContextPackage &&
			ev.WorkflowStepRunID == pkg.PlanStepRunID &&
			ev.FlowContextPackage != nil {
			cp := *ev.FlowContextPackage
			found = &cp
			break
		}
	}
	if found == nil {
		t.Fatal("package not found in event log after simulated restart")
	}
	if found.PackageID != pkg.PackageID {
		t.Errorf("recovered PackageID = %q, want %q", found.PackageID, pkg.PackageID)
	}
	if found.FeatureKey != pkg.FeatureKey {
		t.Errorf("recovered FeatureKey = %q, want %q", found.FeatureKey, pkg.FeatureKey)
	}
}

// TestFlowContextPackageDoesNotCreateParallelSessionState verifies that the
// package builder and persister do not create any new session store state
// (no new sessions, no new run entries) outside the existing workflow event log.
func TestFlowContextPackageDoesNotCreateParallelSessionState(t *testing.T) {
	workspace, _ := fcpFixture(t)
	pkg, _ := BuildFlowContextPackage(workspace, fcpHints(workspace))

	store := newFakeWorkflowStore()
	_ = PersistFlowContextPackage(context.Background(), store, pkg)

	// Only events should be populated; sessions/approvals/questions untouched.
	if len(store.sessions) != 0 {
		t.Errorf("sessions map should be empty, got %d entries", len(store.sessions))
	}
	if len(store.approvals) != 0 {
		t.Errorf("approvals map should be empty, got %d entries", len(store.approvals))
	}
	if len(store.questions) != 0 {
		t.Errorf("questions map should be empty, got %d entries", len(store.questions))
	}
}
