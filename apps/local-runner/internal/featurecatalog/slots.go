package featurecatalog

import (
	"fmt"
	"strings"

	"flowpilot-runner/internal/changeledger"
)

// recentHistoryEntryCount bounds how many commit entries the history block
// renders (newest kept — "newest = truth"); recentHistoryExcerptCount bounds how
// many of those also carry the verbose CA "why" excerpt. A feature with hundreds
// of commits therefore injects a bounded block, not the full chronology.
const recentHistoryEntryCount = 15
const recentHistoryExcerptCount = 3

// ResolveSlot resolves a natural-language feature reference to ranked candidates
// and formats the result for prompt injection.
func ResolveSlot(nl string, catalog *Catalog) string {
	candidates, err := ResolveFeature(nl, catalog)
	if err != nil || len(candidates) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Feature candidates for %q:\n", nl))
	for _, c := range candidates {
		sb.WriteString(fmt.Sprintf("  - %s (score:%.1f)\n", c.Key, c.Score))
	}
	return sb.String()
}

// historyLedger is the minimal ledger surface HistorySlot/HistorySlotRanked
// need — kept as an interface (not *changeledger.Ledger) so a test double
// can stand in without a real on-disk ledger.
type historyLedger interface {
	GetFeatureHistory(string) ([]changeledger.Entry, error)
}

// renderHistoryEntryLine formats one entry as HistorySlot/HistorySlotRanked's
// shared per-entry line, optionally with its CA excerpt. Factored out so the
// two renderers cannot drift from each other (CP-55 P-7 review finding M-5).
func renderHistoryEntryLine(e changeledger.Entry, isCurrentTruth, withExcerpt bool) string {
	var sb strings.Builder
	date := e.CommittedAt
	if len(date) >= 7 {
		date = date[:7]
	}
	id := e.SourceDocID
	if id == "" {
		hash := e.CommitHash
		if len(hash) >= 8 {
			hash = hash[:8]
		}
		id = hash
	}
	marker := ""
	if isCurrentTruth {
		marker = "   ← truth"
	}
	sb.WriteString(fmt.Sprintf("- [%s %s] %s%s\n", id, date, e.Summary, marker))
	if withExcerpt && e.CAExcerpt != "" {
		for _, line := range strings.Split(e.CAExcerpt, "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			sb.WriteString("  - " + line + "\n")
		}
	}
	return sb.String()
}

// HistorySlot returns the packed change history for a feature key formatted for
// prompt injection. Follows CP-35 §4.2.1 packing format.
func HistorySlot(featureKey string, ledger historyLedger) string {
	entries, err := ledger.GetFeatureHistory(featureKey)
	if err != nil || len(entries) == 0 {
		return ""
	}
	return renderHistorySlotChronological(featureKey, entries)
}

// renderHistorySlotChronological is HistorySlot's actual renderer, taking
// entries directly rather than a ledger so HistorySlotRanked's fallback path
// can call it without a second GetFeatureHistory read (CP-55 P-7 review
// finding I-1 — a real ledger read is a full map copy + sort under a mutex;
// reading it twice per Fetch, once to decide fallback-vs-ranked and again
// inside HistorySlot, doubled that cost on every fallback call, which is the
// overwhelmingly common case since fallback triggers below the activation
// threshold. It also meant "fallback preserves current output bytes" held
// only if two consecutive reads of a possibly-concurrently-written ledger
// happened to agree — true for every production caller today, but not
// guaranteed by the historyLedger interface itself).
func renderHistorySlotChronological(featureKey string, entries []changeledger.Entry) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## History %q (newest = truth)\n", featureKey))
	// Cap to the most recent N entries so a long-lived feature can't flood the
	// prompt; note how many older entries were dropped (newest is kept as truth).
	start := 0
	if len(entries) > recentHistoryEntryCount {
		start = len(entries) - recentHistoryEntryCount
		sb.WriteString(fmt.Sprintf("- (%d older omitted)\n", start))
	}
	for i := start; i < len(entries); i++ {
		e := entries[i]
		isCurrentTruth := i == len(entries)-1
		withExcerpt := i >= len(entries)-recentHistoryExcerptCount
		sb.WriteString(renderHistoryEntryLine(e, isCurrentTruth, withExcerpt))
	}
	return sb.String()
}

// HistorySlotRanked returns the packed change history for a feature key,
// ranked by locus relevance when CP-55 P-7's activation conditions are met,
// falling back to HistorySlot's exact existing recency-based output
// otherwise. The fallback branch renders from the SAME entries slice already
// read for the ranking decision (renderHistorySlotChronological), rather
// than re-reading the ledger via HistorySlot — this makes "preserve exact
// fallback output" hold by construction from one read, not by trusting two
// separate reads to agree (CP-55 P-7 review finding I-1).
func HistorySlotRanked(featureKey string, ledger historyLedger, locus RetrievalLocus, config HistoryRankingConfig) string {
	entries, err := ledger.GetFeatureHistory(featureKey)
	if err != nil || len(entries) == 0 {
		return ""
	}

	sel := SelectHistoryEntries(entries, locus, config)
	if !sel.Ranked {
		return renderHistorySlotChronological(featureKey, entries)
	}

	ranked := RankHistoryEntries(entries, locus)
	limit := config.Limit
	if limit <= 0 {
		limit = defaultHistoryRankingConfig.Limit
	}
	LogHistoryRankingSelection(featureKey, locus, ranked, sel.Entries, limit)

	newestHash := entries[len(entries)-1].CommitHash
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(
		"## History %q (ranked, %d/%d; ← truth)\n",
		featureKey, len(sel.Entries), sel.CandidateCount,
	))
	for i, e := range sel.Entries {
		isCurrentTruth := e.CommitHash == newestHash
		// The entry marked current truth always keeps its excerpt regardless
		// of rank position — it is the one entry this invariant exists to
		// protect, so it must never lose its "why" to the rank-based budget
		// (CP-55 P-7 review finding I-3). The remaining excerpt budget still
		// goes to the top-ranked entries, since rank is a better proxy for
		// "likely to matter for this change" than recency once ranking has
		// activated at all.
		withExcerpt := isCurrentTruth || i < recentHistoryExcerptCount
		sb.WriteString(renderHistoryEntryLine(e, isCurrentTruth, withExcerpt))
	}
	return sb.String()
}
