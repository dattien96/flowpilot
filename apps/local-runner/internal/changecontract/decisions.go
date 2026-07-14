// Package changecontract: decisions.go implements Task-187 (CP-43 P-4):
// folding negative knowledge (rejected/reverted approaches) into a
// CanonicalHead's Decisions list, so an A -> B(rejected) -> A churn reads as
// "current-A + a closed dead-end", not raw commit noise (SD-21 D-4).
package changecontract

import (
	"strings"
	"time"

	"flowpilot-runner/internal/changeledger"
)

// rejectionMarkers are the deterministic keyword cues a chat_summary bullet
// (already itself a heuristic classification — see runner/chat_summary.go's
// summarizeSentences) is treated as recording a REJECTED approach rather than
// an adopted one. Only the bullet's own text is AI-generated; whether it
// counts as a Decision is a plain keyword match, not a second AI pass.
var rejectionMarkers = []string{"rejected", "reverted", "abandoned", "decided not to", "dropped in favor of", "instead of"}

// revertMarkers classify a changeledger bugfix Entry as reverting a prior
// feature commit — the ledger has no dedicated "revert" ChangeType, so this
// is a keyword match against the entry's own Summary/CAExcerpt text.
var revertMarkers = []string{"revert", "roll back", "rollback", "undo"}

// FoldDecisions extracts rejected/reverted approaches for featureKey from two
// deterministic sources (SD-21 §5): chat_summary bullets carrying a rejection
// marker, and revert-type changeledger bugfix entries. Ordering is
// chronological (oldest first) and reproducible — no AI pass decides
// inclusion or order, only the source text itself (already written by a
// human turn or an earlier summarizer call) is used verbatim as Reason.
func FoldDecisions(featureKey string, chatLedger *changeledger.ChatSummaryLedger, ledger *changeledger.Ledger) ([]Decision, error) {
	var decisions []Decision

	if chatLedger != nil {
		entries, err := chatLedger.GetFeatureSummaries(featureKey)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			for _, bullet := range splitSummaryBullets(entry.Summary) {
				if !containsAnyFold(bullet, rejectionMarkers) {
					continue
				}
				decisions = append(decisions, Decision{
					Tried:   bullet,
					Outcome: DecisionRejected,
					Reason:  bullet,
					At:      parseDecisionTime(entry.CreatedAt),
				})
			}
		}
	}

	if ledger != nil {
		history, err := ledger.GetFeatureHistory(featureKey)
		if err != nil {
			return nil, err
		}
		for _, entry := range history {
			if entry.ChangeType != "bugfix" {
				continue
			}
			text := entry.CAExcerpt
			if text == "" {
				text = entry.Summary
			}
			if !containsAnyFold(text, revertMarkers) {
				continue
			}
			decisions = append(decisions, Decision{
				Tried:       text,
				Outcome:     DecisionReverted,
				Reason:      text,
				SourceDocID: entry.SourceDocID,
				At:          parseDecisionTime(entry.CommittedAt),
			})
		}
	}

	return decisions, nil
}

// splitSummaryBullets splits a chat_summary Summary (one "- " bullet per
// line, per runner/chat_summary.go's normalizeSummaryBullets/
// heuristicSummarizeTurns) into individual bullet strings, prefix stripped.
func splitSummaryBullets(summary string) []string {
	var bullets []string
	for _, line := range strings.Split(summary, "\n") {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "-")
		line = strings.TrimSpace(line)
		if line != "" {
			bullets = append(bullets, line)
		}
	}
	return bullets
}

func containsAnyFold(text string, markers []string) bool {
	lower := strings.ToLower(text)
	for _, m := range markers {
		if strings.Contains(lower, m) {
			return true
		}
	}
	return false
}

// parseDecisionTime parses an RFC3339 timestamp, returning nil (not a zero
// time) on empty/unparseable input so an unknown "at" is omitted from JSON
// rather than rendered as a misleading epoch/zero date.
func parseDecisionTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t, err = time.Parse(time.RFC3339Nano, s)
		if err != nil {
			return nil
		}
	}
	return &t
}
