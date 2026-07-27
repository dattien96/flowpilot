package featurecatalog

// RetrievalLocus is the region of code a turn is about to touch — the anchor
// history gets ranked against (CP-54 P-2 / Task-262).
//
// It exists so relevance is decided at *query* time ("what am I about to
// change?") instead of at *write* time ("what label did this commit get?").
// That distinction is the whole point of CP-54: any static partition of
// history, including a finer feature_key or a sub-key, still guesses relevance
// when the commit is written, which is before anyone knows what the next turn
// will touch.
//
// An empty locus is not an error. It is the signal that a caller should keep
// its existing recency ordering, which is what makes the ranking a safe
// superset of today's behavior rather than a replacement (CP-54 QĐ-4).
type RetrievalLocus struct {
	// Paths are concrete, repo-relative, forward-slash code files — deduped and
	// sorted. Directory buckets, globs and docs are filtered out before they
	// reach here, because they match nearly everything and would reintroduce
	// the dilution this is meant to remove.
	Paths []string

	// Symbols is the symbol-level anchor. It is empty in practice today:
	// nothing populates Contract.DeclaredSymbols — the declaration parser reads
	// only feature/intent/files, and inference records paths only — so this
	// stays nil until BUG-323 Q-2 settles where symbols would come from. Kept
	// so the scorer and Task-259 can grow into it without a type change.
	Symbols []string

	// RunID lets chat-summary ranking prefer entries from the same run, which
	// is the only strong anchor that data has (it carries no changed paths).
	RunID string
}

// IsEmpty reports whether the locus carries no anchor at all, in which case
// callers must fall back to their existing recency behavior.
func (l RetrievalLocus) IsEmpty() bool {
	return len(l.Paths) == 0 && len(l.Symbols) == 0
}
