package changecontract

import "testing"

// BUG-636 — the runner's adjudication ledger (.flowpilot/adjudications.ndjson,
// vibe_adjudication.go vibeAdjudicationsRel) is appended mid-turn inside the
// diff window the frozen-scope gate observes. Without the bookkeeping
// exemption the engine flags its own adjudication ledger as scope drift and
// parks the writer on its own bookkeeping.
func TestBug636AdjudicationsLedgerIsBookkeeping(t *testing.T) {
	for _, p := range []string{
		".flowpilot/adjudications.ndjson",
		`.flowpilot\adjudications.ndjson`,
	} {
		if !IsRunnerLedgerBookkeepingPath(p) {
			t.Errorf("%q must be recognized as runner-ledger bookkeeping", p)
		}
	}
}

func TestBug636AdjudicationsSiblingsStillDrift(t *testing.T) {
	for _, p := range []string{
		".flowpilot/adjudications2.ndjson",
		".flowpilot/adjudications/other.ndjson",
		"requirements/09-BugFix/todo/BUG-1.md",
	} {
		if IsRunnerLedgerBookkeepingPath(p) {
			t.Errorf("%q must NOT be recognized as runner-ledger bookkeeping", p)
		}
	}
}
