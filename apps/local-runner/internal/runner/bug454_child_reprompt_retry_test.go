package runner

import (
	"testing"
	"time"
)

// scheduleChildTurn's transient-busy re-arm must work for CHILD runs too. The
// verdict reprompt (settleFlowChildTurnCompletedLocked) sets reinvokeInFlight
// on the child and races the previous turn's teardown (turnInFlight still
// true) — startTurn rejects turn_in_progress, the error path re-arms
// pendingHubReinvoke, but notifyTurnIdle only drains that flag for parent runs
// (rs.parentRunID == ""). On a child the reprompt is dropped silently: the
// child stays "running" and its cohort never joins (the multi-round
// review-loop E2E hang shape).
func TestChildVerdictRepromptRetriesAfterTransientTurnInProgress(t *testing.T) {
	svc := bug289Service(t)
	parentID := "run-parent-cr"
	childID := "run-child-cr"
	parent := &interactiveRun{
		id:               parentID,
		autoOrchestrate:  true,
		flowEngineDriven: true,
		status:           RunStatusRunning,
		subs:             map[int64]chan ProviderEvent{},
	}
	rs := &interactiveRun{
		id:                childID,
		parentRunID:       parentID,
		reinvokeInFlight:  true,
		turnInFlight:      true, // previous turn's teardown window — first startTurn rejects
		status:            RunStatusRunning,
		providerAccountID: svc.activeAccountID, // skip the provider_account_changed admission gate
		subs:              map[int64]chan ProviderEvent{},
	}
	svc.mu.Lock()
	svc.runs[parentID] = parent
	svc.runs[childID] = rs
	svc.mu.Unlock()

	svc.scheduleChildTurn(childID, "step-vr", "submit_review_outcome NOW")

	// Wait for the first rejection (turn_in_progress while the teardown window
	// is still open), THEN clear turnInFlight — simulating teardown completing
	// a beat after the reprompt was scheduled.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		fails := rs.hubReinvokeStartFailCount
		svc.mu.Unlock()
		if fails >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	svc.mu.Lock()
	rs.turnInFlight = false
	svc.mu.Unlock()

	// The transient reject must produce a retry — the fail counter advancing
	// past the first rejection is the observable signal (each attempt calls
	// startTurn). On the broken path the reprompt is re-armed into a
	// pendingHubReinvoke that can never drain on a child, so failCount stays 1.
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		fails := rs.hubReinvokeStartFailCount
		svc.mu.Unlock()
		if fails >= 2 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	t.Fatalf("child reprompt dropped after transient reject: failCount=%d pendingHubReinvoke=%v (undrainable on a child run)",
		rs.hubReinvokeStartFailCount, rs.pendingHubReinvoke)
}
