package featurecatalog

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/changeledger"
)

// makeSelectionEntries builds n ascending-by-time entries (entries[n-1] is
// always the newest), each touching a distinct path so overlap can be
// controlled precisely per test via the caller's locus.
func makeSelectionEntries(n int) []changeledger.Entry {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := make([]changeledger.Entry, n)
	for i := 0; i < n; i++ {
		entries[i] = changeledger.Entry{
			CommitHash:   fmt.Sprintf("hash%03d", i),
			FeatureKey:   "calc-core",
			Summary:      fmt.Sprintf("entry %d", i),
			CommittedAt:  base.Add(time.Duration(i) * time.Hour).Format(time.RFC3339),
			ChangedPaths: []string{fmt.Sprintf("src/file%03d.go", i)},
		}
	}
	return entries
}

func TestSelectHistoryEntriesFallsBackWhenLocusEmpty(t *testing.T) {
	entries := makeSelectionEntries(40)
	config := HistoryRankingConfig{ActivationThreshold: 5, Limit: 3}

	sel := SelectHistoryEntries(entries, RetrievalLocus{}, config)
	if sel.Ranked {
		t.Fatal("an empty locus must always fall back, regardless of candidate count")
	}
	if len(sel.Entries) != len(entries) || &sel.Entries[0] != &entries[0] {
		t.Fatalf("fallback must return entries unchanged, got len=%d", len(sel.Entries))
	}
	if sel.CandidateCount != 40 {
		t.Fatalf("CandidateCount = %d, want 40", sel.CandidateCount)
	}
}

func TestSelectHistoryEntriesFallsBackAtThreshold(t *testing.T) {
	config := HistoryRankingConfig{ActivationThreshold: 10, Limit: 5}
	entries := makeSelectionEntries(10) // exactly at the threshold
	locus := RetrievalLocus{Paths: []string{"src/file000.go"}}

	sel := SelectHistoryEntries(entries, locus, config)
	if sel.Ranked {
		t.Fatal("candidate count exactly at ActivationThreshold must still fall back, not rank")
	}
}

func TestSelectHistoryEntriesRanksAboveThreshold(t *testing.T) {
	config := HistoryRankingConfig{ActivationThreshold: 10, Limit: 5}
	entries := makeSelectionEntries(11) // one past the threshold
	locus := RetrievalLocus{Paths: []string{"src/file000.go"}}

	sel := SelectHistoryEntries(entries, locus, config)
	if !sel.Ranked {
		t.Fatal("candidate count above ActivationThreshold with a non-empty locus must rank")
	}
}

func TestSelectHistoryEntriesKeepsNewestAbsoluteTruth(t *testing.T) {
	config := HistoryRankingConfig{ActivationThreshold: 5, Limit: 3}
	entries := makeSelectionEntries(20)
	// Locus overlaps everything EXCEPT the newest entry (index 19), and the
	// Limit (3) is small enough that the newest, scoring 0 overlap, would
	// otherwise be pushed out entirely.
	var paths []string
	for i := 0; i < 19; i++ {
		paths = append(paths, fmt.Sprintf("src/file%03d.go", i))
	}
	locus := RetrievalLocus{Paths: paths}

	sel := SelectHistoryEntries(entries, locus, config)
	if !sel.Ranked {
		t.Fatal("expected ranking to activate")
	}
	// Strengthened per CP-55 P-7 review finding I-7: the original version of
	// this test only asserted presence, which a broken backfill (appending
	// the newest as an EXTRA entry past Limit, or duplicating it while still
	// leaving the original in place) would also satisfy. Pin the exact
	// shape: length still equals Limit, the newest appears EXACTLY once, and
	// it is at the front (this implementation's own documented placement for
	// a backfilled entry — see SelectHistoryEntries' doc comment).
	if len(sel.Entries) != config.Limit {
		t.Fatalf("len(Entries) = %d, want %d — the backfill must not change the cap", len(sel.Entries), config.Limit)
	}
	newest := entries[len(entries)-1]
	count := 0
	for _, e := range sel.Entries {
		if e.CommitHash == newest.CommitHash {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("the newest entry (%s) appears %d times in %+v, want exactly 1", newest.CommitHash, count, sel.Entries)
	}
	if sel.Entries[0].CommitHash != newest.CommitHash {
		t.Fatalf("a backfilled newest entry must be placed first, got %+v", sel.Entries)
	}
}

func TestSelectHistoryEntriesCapsAtFifteen(t *testing.T) {
	// Uses the real default Limit (15), per the spec invariant's own name.
	config := DefaultHistoryRankingConfig()
	entries := makeSelectionEntries(50)
	var paths []string
	for i := 0; i < 50; i++ {
		paths = append(paths, fmt.Sprintf("src/file%03d.go", i))
	}
	locus := RetrievalLocus{Paths: paths}

	sel := SelectHistoryEntries(entries, locus, config)
	if !sel.Ranked {
		t.Fatal("expected ranking to activate (50 candidates > ActivationThreshold 30)")
	}
	if len(sel.Entries) != 15 {
		t.Fatalf("len(Entries) = %d, want capped at 15", len(sel.Entries))
	}
	if sel.CandidateCount != 50 {
		t.Fatalf("CandidateCount = %d, want 50 (the full candidate count, not the capped count)", sel.CandidateCount)
	}
}

func TestSelectHistoryEntriesHandlesFewerThanLimit(t *testing.T) {
	config := HistoryRankingConfig{ActivationThreshold: 5, Limit: 15}
	entries := makeSelectionEntries(10) // above threshold, but below Limit
	var paths []string
	for i := 0; i < 10; i++ {
		paths = append(paths, fmt.Sprintf("src/file%03d.go", i))
	}
	locus := RetrievalLocus{Paths: paths}

	sel := SelectHistoryEntries(entries, locus, config)
	if !sel.Ranked {
		t.Fatal("expected ranking to activate")
	}
	if len(sel.Entries) != 10 {
		t.Fatalf("len(Entries) = %d, want all 10 present (nothing to cap)", len(sel.Entries))
	}
}

func TestHistorySlotRankedPreservesLegacyBytesOnFallback(t *testing.T) {
	entries := makeSelectionEntries(3)
	stub := &historyStub{entries: entries}
	config := HistoryRankingConfig{ActivationThreshold: 30, Limit: 15}
	locus := RetrievalLocus{Paths: []string{"src/file000.go"}}

	ranked := HistorySlotRanked("calc-core", stub, locus, config)
	legacy := HistorySlot("calc-core", stub)
	if ranked != legacy {
		t.Fatalf("fallback output must be byte-identical to HistorySlot:\nranked=%q\nlegacy=%q", ranked, legacy)
	}

	// CP-55 P-7 review finding M-3: also cover the fallback sub-case where
	// the legacy renderer emits its own "older entries omitted" line
	// (requires > recentHistoryEntryCount=15 entries), not just a small
	// fixture with nothing to cap.
	manyEntries := makeSelectionEntries(25)
	manyStub := &historyStub{entries: manyEntries}
	rankedMany := HistorySlotRanked("calc-core", manyStub, locus, config)
	legacyMany := HistorySlot("calc-core", manyStub)
	if rankedMany != legacyMany {
		t.Fatalf("fallback output must be byte-identical to HistorySlot when the legacy cap-omission line applies:\nranked=%q\nlegacy=%q", rankedMany, legacyMany)
	}
	if !strings.Contains(legacyMany, "older entries omitted") {
		t.Fatal("test fixture sanity check: expected the legacy cap-omission line to actually appear with 25 entries")
	}
}

// TestHistorySlotRankedShowsCurrentTruthMarkerInRankedMode is the regression
// test for CP-55 P-7 review finding M-7(1): the "newest remains current
// truth" invariant was previously verified only at the SelectHistoryEntries
// level, never at the rendered-output level a real prompt actually sees.
func TestHistorySlotRankedShowsCurrentTruthMarkerInRankedMode(t *testing.T) {
	entries := makeSelectionEntries(20)
	stub := &historyStub{entries: entries}
	config := HistoryRankingConfig{ActivationThreshold: 5, Limit: 3}
	// Locus overlaps only the oldest entries, so without the newest-entry
	// backfill the newest (index 19) would not make the top-3 cut at all.
	locus := RetrievalLocus{Paths: []string{"src/file000.go", "src/file001.go", "src/file002.go"}}

	out := HistorySlotRanked("calc-core", stub, locus, config)
	if !strings.Contains(out, "← current truth") {
		t.Fatalf("expected the ranked output to render the current-truth marker for the backfilled newest entry, got %q", out)
	}
}

// TestHistorySlotRankedAlwaysShowsCurrentTruthExcerpt is the regression test
// for CP-55 P-7 review finding I-3: the newest ("current truth") entry must
// keep its CAExcerpt even when its rank position falls outside the
// rank-based excerpt budget (recentHistoryExcerptCount), and NOT only because
// the backfill-to-front logic happens to put it within that budget anyway —
// the fixture below gives the newest entry a small but NONZERO overlap that
// is naturally enough to be selected (no backfill triggers at all,
// newestIncluded is true from ranking alone) yet still ranks it at position 4
// (0-based), one past the 3-entry excerpt budget: four other, older entries
// each score a higher overlap (2) and rank ahead of it.
func TestHistorySlotRankedAlwaysShowsCurrentTruthExcerpt(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	entries := make([]changeledger.Entry, 20)
	for i := 0; i < 20; i++ {
		entries[i] = changeledger.Entry{
			CommitHash:  fmt.Sprintf("hash%03d", i),
			FeatureKey:  "calc-core",
			Summary:     fmt.Sprintf("entry %d", i),
			CommittedAt: base.Add(time.Duration(i) * time.Hour).Format(time.RFC3339),
		}
	}
	// Four older entries each overlap 2 locus paths — they rank ahead of the
	// newest on overlap alone, occupying ranks 0-3.
	for _, i := range []int{10, 11, 12, 13} {
		entries[i].ChangedPaths = []string{fmt.Sprintf("src/high%03d-a.go", i), fmt.Sprintf("src/high%03d-b.go", i)}
	}
	// The newest entry (19) overlaps exactly 1 locus path — enough to be
	// naturally selected within Limit=5 (rank 4), but not enough to outrank
	// the four overlap-2 entries above it, and its own CAExcerpt must still
	// survive rendering.
	entries[19].ChangedPaths = []string{"src/newest.go"}
	entries[19].CAExcerpt = "this is why the current state looks the way it does"

	var locusPaths []string
	for _, i := range []int{10, 11, 12, 13} {
		locusPaths = append(locusPaths, fmt.Sprintf("src/high%03d-a.go", i), fmt.Sprintf("src/high%03d-b.go", i))
	}
	locusPaths = append(locusPaths, "src/newest.go")

	stub := &historyStub{entries: entries}
	config := HistoryRankingConfig{ActivationThreshold: 5, Limit: 5}
	locus := RetrievalLocus{Paths: locusPaths}

	sel := SelectHistoryEntries(entries, locus, config)
	if !sel.Ranked || len(sel.Entries) < 5 || sel.Entries[4].CommitHash != "hash019" {
		t.Fatalf("test fixture sanity check failed: expected the newest entry naturally ranked at position 4 (no backfill), got %+v", sel.Entries)
	}

	out := HistorySlotRanked("calc-core", stub, locus, config)
	if !strings.Contains(out, "this is why the current state looks the way it does") {
		t.Fatalf("expected the current-truth entry's CAExcerpt to always render, got %q", out)
	}
}

// TestHistorySlotRankedReturnsEmptyOnNoEntries pins HistorySlotRanked's
// parity with HistorySlot's own empty-input contract (CP-55 P-7 review
// finding M-7(3)) — this is also the precondition featureHistorySource.Fetch
// relies on for its "no change history found" warning.
func TestHistorySlotRankedReturnsEmptyOnNoEntries(t *testing.T) {
	stub := &historyStub{}
	out := HistorySlotRanked("never-seen-before", stub, RetrievalLocus{Paths: []string{"src/x.go"}}, DefaultHistoryRankingConfig())
	if out != "" {
		t.Fatalf("expected empty output for a feature key with no ledger entries, got %q", out)
	}
}

// TestSelectHistoryEntriesNonPositiveLimitUsesDefaultCap is the regression
// test for CP-55 P-7 review finding I-4: a caller-supplied Limit of 0 (e.g.
// the Go zero value from an incompletely-constructed config) must not be
// read as "no cap" — it silently defeated the one guarantee the legacy
// renderer always enforced unconditionally: a long-lived feature injects a
// bounded block, never the full chronology.
func TestSelectHistoryEntriesNonPositiveLimitUsesDefaultCap(t *testing.T) {
	config := HistoryRankingConfig{ActivationThreshold: 5, Limit: 0}
	entries := makeSelectionEntries(50)
	var paths []string
	for i := 0; i < 50; i++ {
		paths = append(paths, fmt.Sprintf("src/file%03d.go", i))
	}
	locus := RetrievalLocus{Paths: paths}

	sel := SelectHistoryEntries(entries, locus, config)
	if !sel.Ranked {
		t.Fatal("expected ranking to activate")
	}
	if len(sel.Entries) != defaultHistoryRankingConfig.Limit {
		t.Fatalf("len(Entries) = %d with Limit=0, want the default cap (%d), not every candidate uncapped", len(sel.Entries), defaultHistoryRankingConfig.Limit)
	}
}

func TestHistorySlotRankedReportsRankedCandidateCount(t *testing.T) {
	entries := makeSelectionEntries(20)
	stub := &historyStub{entries: entries}
	config := HistoryRankingConfig{ActivationThreshold: 5, Limit: 3}
	var paths []string
	for i := 0; i < 20; i++ {
		paths = append(paths, fmt.Sprintf("src/file%03d.go", i))
	}
	locus := RetrievalLocus{Paths: paths}

	out := HistorySlotRanked("calc-core", stub, locus, config)
	if !strings.Contains(out, "3 of 20 candidates shown") {
		t.Fatalf("expected the ranked output to report the selected/candidate counts, got %q", out)
	}
}
