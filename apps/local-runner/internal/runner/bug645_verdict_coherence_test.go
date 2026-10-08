package runner

import (
	"strings"
	"testing"
)

// BUG-645 (live run-523131, evt-527258): submit_review_outcome accepted
// status:"approved" alongside per-AC verdicts of "blocked" and returned
// done/advancing — the verdict table was the actual gate input, so "not
// reviewable" routed as "passed". Per-AC verdicts are the truth; a fail/
// blocked row under an approved envelope must reject (fail closed), forcing
// an in-turn reprompt instead of an inverted verdict.

func TestBug645_ApprovedWithBlockedACRejected(t *testing.T) {
	for _, bad := range []string{"fail", "blocked"} {
		_, err := parseReviewOutcomeInput(map[string]any{
			"status": "approved",
			"verdicts": []any{
				map[string]any{"ac_id": "ac-1", "verdict": "pass"},
				map[string]any{"ac_id": "ac-2", "verdict": bad},
			},
		})
		if err == nil {
			t.Fatalf("approved + verdict=%q must reject — the inverted envelope read 'passed' for a not-reviewable leg", bad)
		}
		if !strings.Contains(err.Error(), "inconsistent") && !strings.Contains(err.Error(), "per-AC") {
			t.Fatalf("rejection must explain the coherence rule, got %q", err)
		}
	}
}

func TestBug645_CoherentEnvelopesStillAccepted(t *testing.T) {
	// approved + all-pass is coherent.
	if _, err := parseReviewOutcomeInput(map[string]any{
		"status": "approved",
		"verdicts": []any{
			map[string]any{"ac_id": "ac-1", "verdict": "pass"},
			map[string]any{"ac_id": "ac-2", "verdict": "pass"},
		},
	}); err != nil {
		t.Fatalf("approved + all pass must parse: %v", err)
	}
	// changes_requested + a fail row is coherent (stricter envelope wins).
	if _, err := parseReviewOutcomeInput(map[string]any{
		"status":   "changes_requested",
		"feedback": "ac-2 fails the null-input edge",
		"verdicts": []any{
			map[string]any{"ac_id": "ac-1", "verdict": "pass"},
			map[string]any{"ac_id": "ac-2", "verdict": "fail"},
		},
	}); err != nil {
		t.Fatalf("changes_requested + fail row must parse: %v", err)
	}
	// blocked envelope + blocked row is coherent.
	if _, err := parseReviewOutcomeInput(map[string]any{
		"status": "blocked",
		"verdicts": []any{
			map[string]any{"ac_id": "ac-1", "verdict": "blocked"},
		},
	}); err != nil {
		t.Fatalf("blocked + blocked row must parse: %v", err)
	}
	// No verdict rows at all: coverage is enforced downstream
	// (ValidateReviewOutcomeVerdicts), not here — the call parses.
	if _, err := parseReviewOutcomeInput(map[string]any{"status": "approved"}); err != nil {
		t.Fatalf("verdict-less approved must still parse (coverage enforced downstream): %v", err)
	}
}
