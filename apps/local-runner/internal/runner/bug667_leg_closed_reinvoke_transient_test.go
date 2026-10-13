package runner

import (
	"testing"
	"time"
)

// BUG-667 (live run-752742): a hub reinvoke that races a leg rotation gets a
// leg_closed admission reject — the chat's successor leg either hasn't been
// minted yet (rotation in flight) or the chat is momentarily legless
// (BUG-666). Both shapes are transient like hub_parked/flow_awaiting_user,
// NOT real failures: counting leg_closed against hubReinvokeStartFailCount
// poisons the channel — once the count passes 3 the armed pendingHubReinvoke
// has no drain path left (shouldDrain=false AND the hub_stall tick gate
// requires count<=3), so the loop cycles running↔blocked hub_stalled forever
// with no turn ever starting. Live evidence: 3+ consecutive
// hub_reinvoke_start_failed entries on run-752742, all carrying "this run's
// leg was superseded; resubmit on the chat's active leg"; the sprint sat in
// hub_stalled for hours until manual /turns injection reached a live leg.
func TestBug667_LegClosedReinvokeDoesNotBurnFailBudget(t *testing.T) {
	svc := bug289Service(t)
	runID := "run-667-transient"
	rs := &interactiveRun{
		id:               runID,
		chatID:           "chat-667",
		legState:         LegStateClosed,
		legClosedReason:  LegClosedReasonProviderSwitch,
		autoOrchestrate:  true,
		flowEngineDriven: true,
		reinvokeInFlight: true,
		status:           RunStatusRunning,
		subs:             map[int64]chan ProviderEvent{},
	}
	svc.mu.Lock()
	svc.runs[runID] = rs
	svc.mu.Unlock()

	svc.scheduleChildTurn(runID, "step-hub", "synthesize the cohort note")

	// Wait for the reject path to settle (flag cleared).
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		done := !rs.reinvokeInFlight
		svc.mu.Unlock()
		if done {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if rs.reinvokeInFlight {
		t.Fatal("reinvokeInFlight still true after startTurn reject — flag must clear on the error path")
	}
	if rs.hubReinvokeStartFailCount != 0 {
		t.Fatalf("leg_closed burned the consecutive-fail budget: count=%d (want 0 — transient like hub_parked/flow_awaiting_user, drained by the next stall tick or a new leg mint)", rs.hubReinvokeStartFailCount)
	}
	if !rs.pendingHubReinvoke {
		t.Fatal("pending reinvoke dropped after transient reject — must stay armed for the post-rotation drain")
	}
}
