package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/changeledger"
)

// seedRankedLedger writes n ledger entries for featureKey, each touching a
// distinct file, ascending by commit time (entries[n-1] is newest).
func seedRankedLedger(t *testing.T, dotFP, featureKey string, n int) {
	t.Helper()
	ledger, err := changeledger.New(dotFP)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := make([]changeledger.Entry, n)
	for i := 0; i < n; i++ {
		entries[i] = changeledger.Entry{
			CommitHash:   fmt.Sprintf("hash%03d", i),
			FeatureKey:   featureKey,
			Summary:      fmt.Sprintf("entry %d", i),
			CommittedAt:  base.Add(time.Duration(i) * time.Hour).Format(time.RFC3339),
			ChangedPaths: []string{fmt.Sprintf("src/file%03d.go", i)},
		}
	}
	if err := ledger.Upsert(entries); err != nil {
		t.Fatal(err)
	}
}

func TestFeatureHistorySourceBuildsLocusFromFrozenContract(t *testing.T) {
	ws := newLocusRepo(t)
	dotFP := filepath.Join(ws, ".flowpilot")
	// 40 ledger entries (above DefaultHistoryRankingConfig.ActivationThreshold
	// = 30) so ranking actually activates; the contract declares only
	// file005.go, deliberately OUTSIDE the legacy fallback's own "most recent
	// 15 of 40" window (indices 25-39) — a fixture inside that window would
	// pass whether or not ranking genuinely ran, since the fallback would
	// surface it too (CP-55 P-7 review finding M-1).
	seedRankedLedger(t, dotFP, "calc-core", 40)
	saveContract(t, ws, "run-1", []string{"src/file005.go"})

	src := &featureHistorySource{priority: 2}
	section, err := src.Fetch(context.Background(), FlowContextHints{
		Workspace: ws, WorkflowRunID: "run-1", FeatureKey: "calc-core", FeatureConfidence: ConfidenceVerified,
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(section.Body, "ranked by relevance") {
		t.Fatalf("expected ranked output once the contract locus is built, got %q", section.Body)
	}
	if !strings.Contains(section.Body, "entry 5") {
		t.Fatalf("expected the contract-declared file's entry to surface in the ranked output even though it is outside the legacy fallback's recency window, got %q", section.Body)
	}
}

func TestFeatureHistorySourceMergesContractDiffAndPromptPaths(t *testing.T) {
	ws := newLocusRepo(t)
	dotFP := filepath.Join(ws, ".flowpilot")
	seedRankedLedger(t, dotFP, "calc-core", 40)
	// Contract declares file010.go; an uncommitted edit touches seed.go
	// (already tracked by newLocusRepo's seed commit); the prompt names
	// file020.go. All three should merge into one locus.
	saveContract(t, ws, "run-1", []string{"src/file010.go"})
	if err := os.WriteFile(filepath.Join(ws, "seed.go"), []byte("package main // edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	src := &featureHistorySource{priority: 2}
	section, err := src.Fetch(context.Background(), FlowContextHints{
		Workspace: ws, WorkflowRunID: "run-1", UserPrompt: "please also look at src/file020.go",
		FeatureKey: "calc-core", FeatureConfidence: ConfidenceVerified,
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(section.Body, "entry 10") {
		t.Fatalf("expected the contract-declared path's entry to surface, got %q", section.Body)
	}
	if !strings.Contains(section.Body, "entry 20") {
		t.Fatalf("expected the prompt-named path's entry to surface, got %q", section.Body)
	}
}

func TestFeatureHistoryRanksWhenChangeContractRenderingIsDisabled(t *testing.T) {
	ws := newLocusRepo(t)
	dotFP := filepath.Join(ws, ".flowpilot")
	seedRankedLedger(t, dotFP, "calc-core", 40)
	saveContract(t, ws, "run-1", []string{"src/file015.go"})

	registry := NewContextSourceRegistry()
	if err := registry.Register(&featureHistorySource{priority: 2}); err != nil {
		t.Fatal(err)
	}
	hints := FlowContextHints{
		Workspace: ws, WorkflowRunID: "run-1", FeatureKey: "calc-core", FeatureConfidence: ConfidenceVerified,
	}
	// change.contract is deliberately NOT in the enabled set — its own
	// rendered section never runs — but feature.history's own locus-building
	// reads the Contract store directly off disk (buildRetrievalLocus →
	// changecontract.OpenStoreReadOnly), independent of that source's
	// registration/enablement, so ranking must still activate here.
	sections, _ := registry.Collect(context.Background(), []string{"feature.history"}, hints)
	if len(sections) != 1 {
		t.Fatalf("expected exactly one section, got %d", len(sections))
	}
	if !strings.Contains(sections[0].Body, "ranked by relevance") {
		t.Fatalf("expected ranking to still activate with change.contract's own rendering disabled, got %q", sections[0].Body)
	}
	if !strings.Contains(sections[0].Body, "entry 15") {
		t.Fatalf("expected the contract-declared path's entry to surface, got %q", sections[0].Body)
	}
}

// countingFeatureHistorySource wraps featureHistorySource to count actual
// Fetch invocations — used to prove ranking never RUNS when the source is
// disabled, not merely that no section is returned (CP-55 P-7 review finding
// I-8: those are subtly different claims, and a wasteful implementation that
// computed the ranking and then discarded the result would satisfy the
// weaker one).
type countingFeatureHistorySource struct {
	*featureHistorySource
	calls *int
}

func (c countingFeatureHistorySource) Fetch(ctx context.Context, hints FlowContextHints) (FlowContextSection, error) {
	*c.calls++
	return c.featureHistorySource.Fetch(ctx, hints)
}

func TestFeatureHistoryProducesNoOutputWhenSourceIsDisabled(t *testing.T) {
	ws := newLocusRepo(t)
	dotFP := filepath.Join(ws, ".flowpilot")
	seedRankedLedger(t, dotFP, "calc-core", 40)
	saveContract(t, ws, "run-1", []string{"src/file015.go"})

	var calls int
	registry := NewContextSourceRegistry()
	if err := registry.Register(countingFeatureHistorySource{&featureHistorySource{priority: 2}, &calls}); err != nil {
		t.Fatal(err)
	}
	hints := FlowContextHints{
		Workspace: ws, WorkflowRunID: "run-1", FeatureKey: "calc-core", FeatureConfidence: ConfidenceVerified,
	}

	// Positive control: with the source enabled, Fetch runs exactly once —
	// proves the counter is actually wired before trusting its zero reading
	// below.
	if _, _ = registry.Collect(context.Background(), []string{"feature.history"}, hints); calls != 1 {
		t.Fatalf("positive control: calls = %d, want 1 (counter is not wired correctly)", calls)
	}
	calls = 0

	// "feature.history" itself is excluded from the enabled set this time —
	// Collect must never call its Fetch at all, so no ranking work runs and
	// no section is produced.
	sections, _ := registry.Collect(context.Background(), []string{}, hints)
	if calls != 0 {
		t.Fatalf("Fetch was called %d time(s) while feature.history was disabled — ranking work must never run, not merely be hidden from output", calls)
	}
	if len(sections) != 0 {
		t.Fatalf("expected no sections when feature.history is disabled, got %+v", sections)
	}
}

func TestChatSummaryRemainsRecencyBased(t *testing.T) {
	ws := t.TempDir()
	dotFP := filepath.Join(ws, ".flowpilot")
	summaryLedger, err := changeledger.NewChatSummaryLedger(dotFP)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := summaryLedger.UpsertForRun(changeledger.ChatSummaryEntry{
			RunID:      fmt.Sprintf("run-%d", i),
			FeatureKey: "calc-core",
			Summary:    fmt.Sprintf("discussion %d", i),
			CreatedAt:  time.Date(2026, 1, 1+i, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
		}); err != nil {
			t.Fatal(err)
		}
	}
	// A contract declaring paths that could anchor a locus is present too —
	// chat.summary must not pick it up, unlike feature.history.
	saveContract(t, ws, "run-1", []string{"src/only.go"})

	src := &chatSummarySource{priority: 5}
	section, err := src.Fetch(context.Background(), FlowContextHints{
		Workspace: ws, WorkflowRunID: "run-1", FeatureKey: "calc-core", FeatureConfidence: ConfidenceVerified,
	})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if strings.Contains(section.Body, "ranked by relevance") {
		t.Fatalf("chat.summary must remain recency-based and must never rank, got %q", section.Body)
	}
	if !strings.Contains(section.Body, "discussion 0") || !strings.Contains(section.Body, "discussion 2") {
		t.Fatalf("expected all recency-ordered discussion entries present, got %q", section.Body)
	}
}
