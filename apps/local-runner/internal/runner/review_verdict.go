package runner

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Task-338 (CP-62 P-2): deterministic AC extraction and verdict-coverage
// validation for the reviewer's submit_review_outcome verdict rows. Pure Go,
// 0 LLM tokens — the same artifact the reviewer read supplies the expected AC
// set, so coverage is a set diff, not a heuristic.

var acIDPattern = regexp.MustCompile(`\bAC-\d+\b`)

// ExtractACIDs returns the deduplicated, lexicographically sorted AC ids
// (AC-N) present in an SS-13 artifact body. Acceptance checklists ("- [ ]
// AC-1: ..."), headings ("## AC-3 notes"), and inline references all count —
// the reviewer must rule on every criterion the artifact names.
func ExtractACIDs(md string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range acIDPattern.FindAllString(md, -1) {
		if !seen[m] {
			seen[m] = true
			out = append(out, m)
		}
	}
	sort.Strings(out)
	return out
}

// ValidateReviewOutcomeVerdicts checks that every expected AC has exactly one
// verdict row (shape/enum validity was already enforced at the shared parse
// point in parseReviewOutcomeInput — every provider path funnels through it).
//
// The returned error names the missing ACs verbatim: surfacing it as the tool
// result IS the one-shot reprompt — the reviewer retries in its own turn with
// the exact gap named. If the reviewer still finishes without a valid verdict
// payload, the CP-61 hub-done gate (review_done_verdict.go) refuses `done` —
// the fail-closed backstop, so a silent approve is impossible.
func ValidateReviewOutcomeVerdicts(in ReviewOutcomeInput, expectedACs []string) error {
	if len(expectedACs) == 0 {
		return nil
	}
	got := map[string]bool{}
	for _, row := range in.Verdicts {
		got[strings.TrimSpace(row.ACID)] = true
	}
	var missing []string
	for _, ac := range expectedACs {
		if !got[strings.TrimSpace(ac)] {
			missing = append(missing, ac)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return fmt.Errorf("submit_review_outcome: missing verdicts for required ACs: %s — add one verdicts row (pass|fail|blocked) per listed AC and re-call the tool",
		strings.Join(missing, ", "))
}
