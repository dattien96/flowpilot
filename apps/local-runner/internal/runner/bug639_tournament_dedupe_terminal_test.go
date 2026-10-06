package runner

import (
	"testing"
)

// BUG-639 (live run-306526): both tournament dedupe scans — the cheap
// pre-check in maybeEscalateCapToTournament and the authoritative in-lock
// scan in escalateToTournament — matched any child labelled
// "tournament_escalation" without checking terminal status. A stopped/
// cancelled rescue leg (run-306526-tournament) therefore satisfied the
// dedupe forever: every later cap logged tournament_escalation_deduped and
// no rescue could ever re-dispatch.
//
// Correct behaviour: only a LIVE (non-terminal) tournament child counts as
// an in-flight rescue; terminal children are historical residue.
func TestTournamentDedupeIgnoresTerminalChild(t *testing.T) {
	svc, parentID := tournamentEscalationFixture(t, true)

	// Zombie rescue leg: stopped, still resident in s.runs.
	svc.mu.Lock()
	svc.runs[parentID+"-tournament"] = &interactiveRun{
		id:          parentID + "-tournament",
		parentRunID: parentID,
		label:       "tournament_escalation",
		status:      RunStatusCancelled,
	}
	svc.mu.Unlock()

	if !svc.maybeEscalateCapToTournament(parentID, "cap reached") {
		t.Fatal("terminal tournament child must not satisfy the dedupe — a fresh rescue must dispatch")
	}
	// The new child gets a unique id — the zombie stays resident.
	svc.mu.Lock()
	defer svc.mu.Unlock()
	fresh := svc.runs[parentID+"-tournament-2"]
	if fresh == nil || fresh.label != "tournament_escalation" {
		t.Fatal("expected a fresh tournament child run-tournament-cap-tournament-2")
	}
	if svc.runs[parentID+"-tournament"] == nil {
		t.Fatal("terminal zombie must not be deleted — audit residue stays")
	}
}

// Same fix, opposite direction: a LIVE tournament child must still dedupe
// (BUG-446's single-rescue invariant is unchanged).
func TestTournamentDedupeStillBlocksOnLiveChild(t *testing.T) {
	svc, parentID := tournamentEscalationFixture(t, true)

	svc.mu.Lock()
	svc.runs[parentID+"-tournament"] = &interactiveRun{
		id:          parentID + "-tournament",
		parentRunID: parentID,
		label:       "tournament_escalation",
		status:      RunStatusRunning,
	}
	svc.mu.Unlock()

	if svc.maybeEscalateCapToTournament(parentID, "cap reached") {
		t.Fatal("live tournament child must still dedupe the rescue")
	}
	if _, err := svc.escalateToTournament(parentID, "cap reached"); err == nil {
		t.Fatal("in-lock dedupe must still refuse while a live child exists")
	}
}
