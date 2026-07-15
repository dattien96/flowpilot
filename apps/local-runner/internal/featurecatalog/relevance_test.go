package featurecatalog

import (
	"reflect"
	"testing"

	"flowpilot-runner/internal/changeledger"
)

func TestScoreHistoryEntryCountsExactPathOverlap(t *testing.T) {
	entry := changeledger.Entry{
		CommitHash: "abc123",
		ChangedPaths: []string{
			"src/calc.go",        // exact match — must count
			"src/calc.go.bak",    // locus path is a PREFIX of this — must NOT count
			"src/calc",           // this is a PREFIX of a locus path — must NOT count
			"vendor/src/calc.go", // locus path is a SUBSTRING of this — must NOT count
			"src/unrelated.go",
			"docs/notes.md",
		},
	}
	locus := RetrievalLocus{Paths: []string{"src/calc.go", "src/other.go"}}

	rel := ScoreHistoryEntry(entry, locus)
	if rel.PathOverlap != 1 {
		t.Fatalf("PathOverlap = %d, want 1 (only the exact match src/calc.go overlaps — near-miss prefix/substring paths must not count)", rel.PathOverlap)
	}
	if rel.CommitHash != "abc123" {
		t.Fatalf("CommitHash = %q, want abc123", rel.CommitHash)
	}
}

func TestScoreHistoryEntryDeduplicatesPaths(t *testing.T) {
	entry := changeledger.Entry{
		ChangedPaths: []string{"src/calc.go", "src/calc.go", "src/calc.go"},
	}
	locus := RetrievalLocus{Paths: []string{"src/calc.go"}}

	rel := ScoreHistoryEntry(entry, locus)
	if rel.PathOverlap != 1 {
		t.Fatalf("PathOverlap = %d, want 1 — a path repeated within ChangedPaths must not inflate the score", rel.PathOverlap)
	}

	// Duplicates on the LOCUS side must not inflate either.
	rel = ScoreHistoryEntry(
		changeledger.Entry{ChangedPaths: []string{"src/calc.go"}},
		RetrievalLocus{Paths: []string{"src/calc.go", "src/calc.go"}},
	)
	if rel.PathOverlap != 1 {
		t.Fatalf("PathOverlap = %d, want 1 — a duplicated locus path must not inflate the score", rel.PathOverlap)
	}
}

func TestScoreHistoryEntryReturnsZeroForNilChangedPaths(t *testing.T) {
	entry := changeledger.Entry{CommitHash: "abc123", CommittedAt: "2026-07-01T00:00:00Z"}
	locus := RetrievalLocus{Paths: []string{"src/calc.go"}}

	rel := ScoreHistoryEntry(entry, locus)
	if rel.PathOverlap != 0 {
		t.Fatalf("PathOverlap = %d, want 0 for nil ChangedPaths", rel.PathOverlap)
	}
	// The entry must still be scored on its other fields so it remains
	// eligible for the recency/hash tie-break — nil ChangedPaths is not the
	// same as "drop this entry."
	if rel.CommitHash != "abc123" || rel.CommitUnix == 0 {
		t.Fatalf("expected CommitHash/CommitUnix still populated, got %+v", rel)
	}
}

func TestScoreHistoryEntryReturnsZeroForEmptyLocus(t *testing.T) {
	entry := changeledger.Entry{ChangedPaths: []string{"src/calc.go"}}
	rel := ScoreHistoryEntry(entry, RetrievalLocus{})
	if rel.PathOverlap != 0 {
		t.Fatalf("PathOverlap = %d, want 0 for an empty locus", rel.PathOverlap)
	}
}

func TestScoreHistoryEntryCanonicalizesPathsBeforeComparing(t *testing.T) {
	// A locus path built from a user prompt can retain a leading "./"
	// (flow_context_hint_paths.go deliberately keeps it); a ledger-side path
	// never has one. Both sides must canonicalize to the same form so this
	// still overlaps instead of silently scoring zero.
	entry := changeledger.Entry{ChangedPaths: []string{"src/calc.go"}}
	locus := RetrievalLocus{Paths: []string{"./src/calc.go"}}

	rel := ScoreHistoryEntry(entry, locus)
	if rel.PathOverlap != 1 {
		t.Fatalf("PathOverlap = %d, want 1 — a leading './' on the locus side must not defeat the match", rel.PathOverlap)
	}
}

func TestScoreHistoryEntryHandlesUnparseableCommittedAt(t *testing.T) {
	locus := RetrievalLocus{Paths: []string{"src/calc.go"}}
	for _, bad := range []string{"", "not-a-timestamp", "2026-13-45T99:99:99Z", "2026/01/01 00:00:00"} {
		e := changeledger.Entry{CommitHash: "abc", CommittedAt: bad, ChangedPaths: []string{"src/calc.go"}}
		rel := ScoreHistoryEntry(e, locus)
		if rel.CommitUnix != 0 {
			t.Fatalf("CommittedAt=%q -> CommitUnix=%d, want 0", bad, rel.CommitUnix)
		}
		if rel.PathOverlap != 1 || rel.CommitHash != "abc" {
			t.Fatalf("CommittedAt=%q must not disturb other signals: %+v", bad, rel)
		}
	}
}

func TestScoreHistoryEntryParsesCommittedAtAsUnixSeconds(t *testing.T) {
	// Pins the unit (seconds, not millis/nanos) and same-instant-different-offset handling.
	e := changeledger.Entry{CommittedAt: "2026-01-01T00:00:00Z"}
	if got := ScoreHistoryEntry(e, RetrievalLocus{}).CommitUnix; got != 1767225600 {
		t.Fatalf("CommitUnix = %d, want 1767225600", got)
	}
	off := changeledger.Entry{CommittedAt: "2026-01-01T09:00:00+09:00"}
	if a, b := ScoreHistoryEntry(e, RetrievalLocus{}).CommitUnix,
		ScoreHistoryEntry(off, RetrievalLocus{}).CommitUnix; a != b {
		t.Fatalf("the same instant scored differently under different timezone offsets: %d vs %d", a, b)
	}
}

func TestRankHistoryEntriesOverlapDominatesRecency(t *testing.T) {
	locus := RetrievalLocus{Paths: []string{"src/calc.go"}}
	older := changeledger.Entry{CommitHash: "aaa", CommittedAt: "2026-01-01T00:00:00Z", ChangedPaths: []string{"src/calc.go"}}
	newer := changeledger.Entry{CommitHash: "bbb", CommittedAt: "2026-07-01T00:00:00Z", ChangedPaths: []string{"src/unrelated.go"}}

	ranked := RankHistoryEntries([]changeledger.Entry{newer, older}, locus)
	if ranked[0].Entry.CommitHash != "aaa" {
		t.Fatalf("rank 0 = %q, want the older-but-overlapping entry (aaa) — overlap must dominate recency", ranked[0].Entry.CommitHash)
	}
	if ranked[1].Entry.CommitHash != "bbb" {
		t.Fatalf("rank 1 = %q, want bbb", ranked[1].Entry.CommitHash)
	}
	if ranked[0].Rank != 0 || ranked[1].Rank != 1 {
		t.Fatalf("Rank fields not set correctly: %+v", ranked)
	}
}

func TestRankHistoryEntriesRecencyBreaksEqualOverlap(t *testing.T) {
	locus := RetrievalLocus{Paths: []string{"src/calc.go"}}
	older := changeledger.Entry{CommitHash: "aaa", CommittedAt: "2026-01-01T00:00:00Z", ChangedPaths: []string{"src/calc.go"}}
	newer := changeledger.Entry{CommitHash: "bbb", CommittedAt: "2026-07-01T00:00:00Z", ChangedPaths: []string{"src/calc.go"}}

	ranked := RankHistoryEntries([]changeledger.Entry{older, newer}, locus)
	if ranked[0].Entry.CommitHash != "bbb" {
		t.Fatalf("rank 0 = %q, want the newer entry (bbb) when overlap is equal", ranked[0].Entry.CommitHash)
	}
}

func TestRankHistoryEntriesCommitHashBreaksCompleteTie(t *testing.T) {
	locus := RetrievalLocus{Paths: []string{"src/calc.go"}}
	sameTime := "2026-01-01T00:00:00Z"
	z := changeledger.Entry{CommitHash: "zzz", CommittedAt: sameTime, ChangedPaths: []string{"src/calc.go"}}
	a := changeledger.Entry{CommitHash: "aaa", CommittedAt: sameTime, ChangedPaths: []string{"src/calc.go"}}

	for _, order := range [][2]changeledger.Entry{{z, a}, {a, z}} {
		ranked := RankHistoryEntries([]changeledger.Entry{order[0], order[1]}, locus)
		if ranked[0].Entry.CommitHash != "aaa" {
			t.Fatalf("input order %v: rank 0 = %q, want aaa (lexicographically smaller hash) when overlap and commit time are both tied",
				[2]string{order[0].CommitHash, order[1].CommitHash}, ranked[0].Entry.CommitHash)
		}
	}
}

// TestRankHistoryEntriesIsDeterministicAcrossRuns pins an expected order for
// a fixture that exercises all three ordering levels (distinct overlap,
// tied-overlap-distinct-time, and a complete tie), then verifies every
// permutation of the input slice produces that exact order. This is what
// actually proves the ordering is independent of input order and free of a
// strict-weak-ordering violation in the comparator — merely re-running the
// same input order repeatedly (as an earlier version of this test did)
// cannot distinguish a correct implementation from a deterministic-but-wrong
// one, since re-feeding identical input to a pure function trivially
// reproduces identical output regardless of whether the comparator is
// correct.
func TestRankHistoryEntriesIsDeterministicAcrossRuns(t *testing.T) {
	locus := RetrievalLocus{Paths: []string{"src/calc.go", "src/util.go"}}
	sameTime := "2026-01-01T00:00:00Z"
	entries := []changeledger.Entry{
		{CommitHash: "ccc", CommittedAt: sameTime, ChangedPaths: []string{"src/calc.go"}},
		{CommitHash: "aaa", CommittedAt: sameTime, ChangedPaths: []string{"src/calc.go"}},
		{CommitHash: "bbb", CommittedAt: "2026-02-01T00:00:00Z", ChangedPaths: []string{"src/calc.go", "src/util.go"}},
		{CommitHash: "ddd", CommittedAt: "2026-03-01T00:00:00Z"},
		// "0aa": same PathOverlap (1) as aaa/ccc but an OLDER CommittedAt, and
		// a hash that sorts lexicographically BEFORE aaa/ccc's. This is the
		// pair that actually exercises the recency comparator asymmetrically:
		// a comparator that only special-cases "i newer than j" (e.g. `if
		// i.Unix > j.Unix { return true }` instead of the correct `if i.Unix
		// != j.Unix { return i.Unix > j.Unix }`) falls through to the hash
		// tie-break whenever the FIRST argument is the older one, incorrectly
		// letting a smaller/older hash outrank a tied-but-newer entry.
		{CommitHash: "0aa", CommittedAt: "2025-06-01T00:00:00Z", ChangedPaths: []string{"src/calc.go"}},
	}
	// bbb: overlap 2 (highest) -> rank 0.
	// aaa/ccc: overlap 1, newer CommittedAt than 0aa -> rank above it; tied
	// CommittedAt between themselves -> hash ascending -> aaa then ccc.
	// 0aa: overlap 1 but the oldest CommittedAt among the overlap-1 group.
	// ddd: overlap 0 -> last, despite being the newest commit.
	want := []string{"bbb", "aaa", "ccc", "0aa", "ddd"}

	perms := [][]int{
		{0, 1, 2, 3, 4},
		{4, 3, 2, 1, 0},
		{1, 3, 0, 2, 4},
		{2, 0, 3, 1, 4},
		{0, 3, 2, 1, 4},
		{4, 0, 1, 2, 3},
		{3, 4, 1, 0, 2},
	}
	for _, perm := range perms {
		in := make([]changeledger.Entry, 0, len(entries))
		for _, idx := range perm {
			in = append(in, entries[idx])
		}
		got := RankHistoryEntries(in, locus)
		var order []string
		for _, r := range got {
			order = append(order, r.Entry.CommitHash)
		}
		if !reflect.DeepEqual(order, want) {
			t.Fatalf("input permutation %v produced %v, want %v — ranking must not depend on input order", perm, order, want)
		}
	}
}

func TestRankHistoryEntriesSymbolOverlapPreservesRecencyWhenEqual(t *testing.T) {
	// Symbols-only locus: both entries match one symbol each; recency still decides.
	entries := []changeledger.Entry{
		{CommitHash: "aaa", CommittedAt: "2026-01-01T00:00:00Z", ChangedPaths: []string{"src/calc.go"}},
		{CommitHash: "bbb", CommittedAt: "2026-02-01T00:00:00Z", ChangedPaths: []string{"pkg.Foo"}},
	}
	locusWithSymbolsOnly := RetrievalLocus{Symbols: []string{"pkg.Foo", "src/calc.go"}}

	got := RankHistoryEntries(append([]changeledger.Entry(nil), entries...), locusWithSymbolsOnly)
	if len(got) != 2 || got[0].Entry.CommitHash != "bbb" {
		t.Fatalf("equal symbol overlap must fall through to recency, got first=%q", got[0].Entry.CommitHash)
	}
	for _, r := range got {
		if r.Relevance.PathOverlap != 0 {
			t.Fatalf("entry %q scored PathOverlap=%d against a Symbols-only locus, want 0", r.Entry.CommitHash, r.Relevance.PathOverlap)
		}
		if r.Relevance.SymbolOverlap != 1 {
			t.Fatalf("entry %q scored SymbolOverlap=%d, want 1", r.Entry.CommitHash, r.Relevance.SymbolOverlap)
		}
	}

	// Path overlap still independent of symbol overlap on the same entry.
	entry := changeledger.Entry{CommitHash: "ccc", CommittedAt: "2026-01-01T00:00:00Z", ChangedPaths: []string{"src/calc.go", "src/symbolish.go"}}
	withSymbols := RetrievalLocus{Paths: []string{"src/calc.go"}, Symbols: []string{"src/symbolish.go"}}
	pathsOnly := RetrievalLocus{Paths: []string{"src/calc.go"}}

	gotOverlap := ScoreHistoryEntry(entry, withSymbols)
	wantOverlap := ScoreHistoryEntry(entry, pathsOnly)
	if gotOverlap.PathOverlap != wantOverlap.PathOverlap || gotOverlap.PathOverlap != 1 {
		t.Fatalf("PathOverlap = %d with Symbols vs %d without (want 1 both)", gotOverlap.PathOverlap, wantOverlap.PathOverlap)
	}
	if gotOverlap.SymbolOverlap != 1 || wantOverlap.SymbolOverlap != 0 {
		t.Fatalf("SymbolOverlap with symbols=%d without=%d (want 1/0)", gotOverlap.SymbolOverlap, wantOverlap.SymbolOverlap)
	}
}

func TestRankHistoryEntriesReturnsNilForEmptyInput(t *testing.T) {
	if got := RankHistoryEntries(nil, RetrievalLocus{}); got != nil {
		t.Fatalf("RankHistoryEntries(nil, ...) = %#v, want nil", got)
	}
}
