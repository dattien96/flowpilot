package runner

// BUG-624 (live run-174243): maybeSettleVibeOwnerDebate raced the mount's own
// dispatch window. stashVibeFlowForDebate had already swapped the debate graph
// in and set vibeDebateMountInFlight, but the mount goroutine had only stamped
// debate_trigger RUNNING and issued the first owner spawn request — zero owner
// children yet. A concurrent settle check (child-settle path's
// `go s.maybeSettleVibeOwnerDebate`) read that mid-mount state as
// "ownersStarved" and fired vibe_owner_fail_retry → a second
// startResolvedFlow mounted the same debate flow on top of itself and spawned
// a duplicate owner_1+owner_2 pair into one cohort (4 owner legs total).
//
// Fix contract: the settle ladder must not evaluate while
// rs.vibeDebateMountInFlight — the mount goroutine owns that window and its
// post-check (maybeReleaseVibeDebateClaimIfMountDied) is the only legitimate
// starved-mount verdict.

import "testing"

// A starved-shape board (mounted debate graph, zero owner children, all steps
// pending) while the mount goroutine is still in flight must NOT retry — the
// CA-1088 ownersStarved branch only applies once the mount window closes.
func TestBUG624_SettleDoesNotRetryWhileDebateMountInFlight(t *testing.T) {
	svc, runID := seedVibeDebateStarvedRun(t)

	svc.mu.Lock()
	svc.runs[runID].vibeDebateMountInFlight = true
	svc.mu.Unlock()

	if svc.maybeSettleVibeOwnerDebate(runID) {
		t.Fatal("settle retried the debate while its own mount was still dispatching")
	}
	svc.mu.Lock()
	retries := svc.runs[runID].vibeOwnerFailRetries
	svc.mu.Unlock()
	if retries != 0 {
		t.Fatalf("vibeOwnerFailRetries=%d want 0 (mount window is not a fail)", retries)
	}
}

// Once the mount window closes the starved board still retries — the flag
// gates the race, not the CA-1088 remediation itself.
func TestBUG624_SettleRetriesAfterMountWindowCloses(t *testing.T) {
	svc, runID := seedVibeDebateStarvedRun(t)

	// Flag left false — mount finished (or died) already; the same starved
	// board shape must still take the ownersStarved retry ladder.
	if !svc.maybeSettleVibeOwnerDebate(runID) {
		t.Fatal("starved board after mount window closed must still retry")
	}
}
