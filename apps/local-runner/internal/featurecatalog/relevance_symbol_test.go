package featurecatalog

import (
	"testing"

	"flowpilot-runner/internal/changeledger"
)

func TestScoreHistoryEntrySymbolOverlapFromDerivedBasename(t *testing.T) {
	entry := changeledger.Entry{
		CommitHash:   "aaa",
		CommittedAt:  "2026-01-01T00:00:00Z",
		ChangedPaths: []string{"internal/runner/gate_hook.go"},
	}
	locus := RetrievalLocus{Symbols: []string{"GateHook"}}

	rel := ScoreHistoryEntry(entry, locus)
	if rel.SymbolOverlap != 1 {
		t.Fatalf("SymbolOverlap = %d, want 1 (gate_hook_test.go → GateHook)", rel.SymbolOverlap)
	}
	if rel.PathOverlap != 0 {
		t.Fatalf("PathOverlap = %d, want 0 when only symbols anchor the locus", rel.PathOverlap)
	}
}

func TestRankHistoryEntriesSymbolOverlapDominatesRecency(t *testing.T) {
	locus := RetrievalLocus{Symbols: []string{"CalcCore"}}
	entries := []changeledger.Entry{
		{CommitHash: "old", CommittedAt: "2026-01-01T00:00:00Z", ChangedPaths: []string{"pkg/other.go"}},
		{CommitHash: "new", CommittedAt: "2026-06-01T00:00:00Z", ChangedPaths: []string{"src/calc_core.go"}},
	}
	ranked := RankHistoryEntries(entries, locus)
	if len(ranked) != 2 || ranked[0].Entry.CommitHash != "new" {
		t.Fatalf("symbol overlap should beat recency: got first=%q", ranked[0].Entry.CommitHash)
	}
	if ranked[0].Relevance.SymbolOverlap != 1 {
		t.Fatalf("new entry SymbolOverlap = %d, want 1", ranked[0].Relevance.SymbolOverlap)
	}
}

func TestRankHistoryEntriesPathOverlapStillBeatsSymbolOverlap(t *testing.T) {
	locus := RetrievalLocus{
		Paths:   []string{"src/calc.go"},
		Symbols: []string{"CalcCore"},
	}
	entries := []changeledger.Entry{
		{CommitHash: "symOnly", CommittedAt: "2026-06-01T00:00:00Z", ChangedPaths: []string{"src/calc_core.go"}},
		{CommitHash: "pathHit", CommittedAt: "2026-01-01T00:00:00Z", ChangedPaths: []string{"src/calc.go"}},
	}
	ranked := RankHistoryEntries(entries, locus)
	if ranked[0].Entry.CommitHash != "pathHit" {
		t.Fatalf("path overlap must rank above symbol-only overlap, got %q first", ranked[0].Entry.CommitHash)
	}
}
