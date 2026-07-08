package changeledger

import "testing"

// TestGetFeatureHistoryDeterministicForEqualCommittedAt is the regression
// test for BUG-266: Ledger.entries is a map[string]Entry, and
// Ledger.AllEntries ranges over it — Go intentionally randomizes map
// iteration order on every range call. Before this fix, GetFeatureHistory's
// sort.Slice comparator only ordered by CommittedAt, so two entries with
// equal (including both-empty) CommittedAt were "equal" to the sort and their
// final relative order depended on whatever order AllEntries handed them —
// meaning "newest = current truth" (SD-17 D-3) could silently flip between
// two different commits across repeated calls against the exact same data.
// The fix adds CommitHash as a deterministic tiebreaker.
func TestGetFeatureHistoryDeterministicForEqualCommittedAt(t *testing.T) {
	dir := t.TempDir()
	l, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Every entry shares the same (empty) CommittedAt, and there are enough
	// distinct commit hashes that map-iteration randomization would very
	// likely surface a different order across repeated calls if the
	// tiebreaker were missing.
	entries := []Entry{
		{CommitHash: "c1", FeatureKey: "widget", Summary: "one"},
		{CommitHash: "c2", FeatureKey: "widget", Summary: "two"},
		{CommitHash: "c3", FeatureKey: "widget", Summary: "three"},
		{CommitHash: "c4", FeatureKey: "widget", Summary: "four"},
		{CommitHash: "c5", FeatureKey: "widget", Summary: "five"},
	}
	if err := l.Upsert(entries); err != nil {
		t.Fatal(err)
	}

	first, err := l.GetFeatureHistory("widget")
	if err != nil {
		t.Fatal(err)
	}
	firstOrder := hashesInOrder(first)

	for i := 0; i < 50; i++ {
		next, err := l.GetFeatureHistory("widget")
		if err != nil {
			t.Fatal(err)
		}
		nextOrder := hashesInOrder(next)
		if nextOrder != firstOrder {
			t.Fatalf("GetFeatureHistory order changed across calls (iteration %d): got %v, want %v", i, nextOrder, firstOrder)
		}
	}

	// The tiebreaker is CommitHash ascending, so with equal CommittedAt the
	// order must be exactly c1..c5.
	want := "c1,c2,c3,c4,c5"
	if firstOrder != want {
		t.Errorf("order = %q, want %q (ascending CommitHash tiebreak)", firstOrder, want)
	}
}

func hashesInOrder(entries []Entry) string {
	out := ""
	for i, e := range entries {
		if i > 0 {
			out += ","
		}
		out += e.CommitHash
	}
	return out
}
