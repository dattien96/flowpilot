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

// HistorySlot returns the packed change history for a feature key formatted for
// prompt injection. Follows CP-35 §4.2.1 packing format.
func HistorySlot(featureKey string, ledger interface {
	GetFeatureHistory(string) ([]changeledger.Entry, error)
}) string {
	entries, err := ledger.GetFeatureHistory(featureKey)
	if err != nil || len(entries) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Prior work on %q (oldest → newest — build on the NEWEST, do not undo it)\n", featureKey))
	// Cap to the most recent N entries so a long-lived feature can't flood the
	// prompt; note how many older entries were dropped (newest is kept as truth).
	start := 0
	if len(entries) > recentHistoryEntryCount {
		start = len(entries) - recentHistoryEntryCount
		sb.WriteString(fmt.Sprintf("- (… %d older entries omitted; showing the most recent %d …)\n", start, recentHistoryEntryCount))
	}
	for i := start; i < len(entries); i++ {
		e := entries[i]
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
		if i == len(entries)-1 {
			marker = "   ← current truth"
		}
		sb.WriteString(fmt.Sprintf("- [%s %s] %s%s\n", id, date, e.Summary, marker))
		if e.CAExcerpt != "" && i >= len(entries)-recentHistoryExcerptCount {
			for _, line := range strings.Split(e.CAExcerpt, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				sb.WriteString("  - " + line + "\n")
			}
		}
	}
	return sb.String()
}
