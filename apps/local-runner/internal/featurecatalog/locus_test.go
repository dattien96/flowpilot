package featurecatalog

import "testing"

func TestRetrievalLocusIsEmptyOnZeroValue(t *testing.T) {
	if !(RetrievalLocus{}).IsEmpty() {
		t.Fatal("zero-value locus must be empty so callers fall back to recency")
	}
}

func TestRetrievalLocusPathsMakeItNonEmpty(t *testing.T) {
	if (RetrievalLocus{Paths: []string{"a.go"}}).IsEmpty() {
		t.Fatal("a locus with paths must not report empty")
	}
}

func TestRetrievalLocusSymbolsAloneMakeItNonEmpty(t *testing.T) {
	// Symbols are unpopulated today (BUG-323 Q-2), but a symbol-only locus must
	// still count as an anchor once something does populate them.
	if (RetrievalLocus{Symbols: []string{"ScopeDiff"}}).IsEmpty() {
		t.Fatal("a locus with symbols must not report empty")
	}
}

func TestRetrievalLocusRunIDAloneIsStillEmpty(t *testing.T) {
	// A run id is not a code anchor: it cannot rank commit history, so it must
	// not switch callers out of recency ordering on its own.
	if !(RetrievalLocus{RunID: "run-1"}).IsEmpty() {
		t.Fatal("a run id alone is not a code anchor and must report empty")
	}
}
