package changecontract

import (
	"fmt"
	"strings"
)

// statusChip renders a short human-readable label for h.Status, used inline
// in the packed Head block (Task-188, CP-43 P-5, SD-21 D-3).
func statusChip(h CanonicalHead) string {
	chip := h.Status
	if chip == "" {
		chip = HeadStatusCurrent
	}
	if h.SpecConfidence == SpecConfidenceSpecLess {
		return chip + " (spec-less — low confidence)"
	}
	return chip
}

// shortSignature truncates an intent_signature to a readable prefix for
// inline display — the full hash is still in the Head JSON on disk for exact
// comparison; the packed prompt only needs enough to eyeball "did this change".
func shortSignature(sig string) string {
	if len(sig) <= 12 {
		return sig
	}
	return sig[:12]
}

// RenderHeadBlock renders h as the mandatory, lead-first prompt block (Task-188
// T-1/T-3, SD-21 D-3): "## Canonical state" with behavior_statement,
// intent_signature + status chip, and a "do NOT re-attempt" list built from h's
// rejected/reverted Decisions — so the AI reads current truth and closed
// dead-ends before any raw ordered history. Returns "" for a zero-value Head
// (FeatureKey empty) so callers can safely prepend the result unconditionally.
func RenderHeadBlock(h CanonicalHead) string {
	if strings.TrimSpace(h.FeatureKey) == "" {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## Canonical state of %q (status: %s, signature: %s)\n", h.FeatureKey, statusChip(h), shortSignature(h.IntentSignature)))
	if behavior := strings.TrimSpace(h.BehaviorStatement); behavior != "" {
		sb.WriteString(behavior + "\n")
	}

	rejected := rejectedDecisions(h.Decisions)
	if len(rejected) > 0 {
		sb.WriteString("\nDo NOT re-attempt these — already tried and rejected:\n")
		for _, d := range rejected {
			line := "- " + d.Tried
			if d.Reason != "" && d.Reason != d.Tried {
				line += " (" + d.Reason + ")"
			}
			sb.WriteString(line + "\n")
		}
	}
	return strings.TrimRight(sb.String(), "\n") + "\n"
}

func rejectedDecisions(decisions []Decision) []Decision {
	var out []Decision
	for _, d := range decisions {
		if d.Outcome == DecisionRejected || d.Outcome == DecisionReverted {
			out = append(out, d)
		}
	}
	return out
}
