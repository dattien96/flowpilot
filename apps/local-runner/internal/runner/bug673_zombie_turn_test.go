package runner

import (
	"context"
	"testing"
	"time"
)

// BUG-673 (live run-752742, turn-816726): a turn was minted, turnInFlight
// armed, and the dispatch driver died before the provider ever accepted the
// send — prompt row only, no transcript. The run then rejected every new
// turn and relay with turn_in_progress until an operator interrupt. The
// wedge sweep must release the flag when the durable dispatch record proves
// the turn is dead — and must NOT release while the record can still be a
// live provider turn.

func bug673Run(svc *InteractiveService, runID, turnID, updatedAt string) {
	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:            runID,
		status:        RunStatusRunning,
		turnInFlight:  true,
		currentTurnID: turnID,
		updatedAt:     updatedAt,
		subs:          map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
}

func bug673Inflight(svc *InteractiveService, runID string) bool {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	return rs != nil && rs.turnInFlight
}

func TestBug673_StalePreparedDispatchReleasesZombieFlag(t *testing.T) {
	svc := bug289Service(t)
	svc.dispatchStore = NewMemoryDispatchStore()
	runID, turnID := "run-673a", "t-673a"

	if err := svc.dispatchStore.CreatePrepared(context.Background(), testPrepared(runID, turnID), testEnvelope(runID, turnID)); err != nil {
		t.Fatalf("CreatePrepared: %v", err)
	}
	bug673Run(svc, runID, turnID, time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano))

	svc.sweepWedgedFlowWork()

	if bug673Inflight(svc, runID) {
		t.Fatal("turnInFlight still pinned on a stale prepared dispatch — zombie turn blocks every new turn")
	}
}

func TestBug673_TerminalDispatchReleasesZombieFlag(t *testing.T) {
	svc := bug289Service(t)
	svc.dispatchStore = NewMemoryDispatchStore()
	runID, turnID := "run-673b", "t-673b"

	ctx := context.Background()
	if err := svc.dispatchStore.CreatePrepared(ctx, testPrepared(runID, turnID), testEnvelope(runID, turnID)); err != nil {
		t.Fatalf("CreatePrepared: %v", err)
	}
	if _, err := svc.dispatchStore.CASAdvance(ctx, runID, turnID, 1, DispatchPrepared, DispatchTerminalCancelled, nil); err != nil {
		t.Fatalf("terminal advance: %v", err)
	}
	bug673Run(svc, runID, turnID, time.Now().UTC().Format(time.RFC3339Nano))

	svc.sweepWedgedFlowWork()

	if bug673Inflight(svc, runID) {
		t.Fatal("turnInFlight still pinned on a terminal dispatch — zombie turn blocks every new turn")
	}
}

// A fresh pre-acceptance record is a dispatch still materializing — the sweep
// must not race it.
func TestBug673_FreshPreparedDispatchKeepsFlag(t *testing.T) {
	svc := bug289Service(t)
	svc.dispatchStore = NewMemoryDispatchStore()
	runID, turnID := "run-673c", "t-673c"

	if err := svc.dispatchStore.CreatePrepared(context.Background(), testPrepared(runID, turnID), testEnvelope(runID, turnID)); err != nil {
		t.Fatalf("CreatePrepared: %v", err)
	}
	bug673Run(svc, runID, turnID, time.Now().UTC().Format(time.RFC3339Nano))

	svc.sweepWedgedFlowWork()

	if !bug673Inflight(svc, runID) {
		t.Fatal("sweep released turnInFlight on a fresh dispatch — legitimate turn_in_progress protection weakened")
	}
}

// provider_accepted means the provider acknowledged the turn — stale or not,
// that is a live lane owned by recovery/settle, never a zombie.
func TestBug673_ProviderAcceptedDispatchKeepsFlag(t *testing.T) {
	svc := bug289Service(t)
	svc.dispatchStore = NewMemoryDispatchStore()
	runID, turnID := "run-673d", "t-673d"

	ctx := context.Background()
	if err := svc.dispatchStore.CreatePrepared(ctx, testPrepared(runID, turnID), testEnvelope(runID, turnID)); err != nil {
		t.Fatalf("CreatePrepared: %v", err)
	}
	rev, err := svc.dispatchStore.CASAdvance(ctx, runID, turnID, 1, DispatchPrepared, DispatchSendClaimed, nil)
	if err != nil {
		t.Fatalf("send_claimed: %v", err)
	}
	rev, err = svc.dispatchStore.CASAdvance(ctx, runID, turnID, rev, DispatchSendClaimed, DispatchSendStarted, nil)
	if err != nil {
		t.Fatalf("send_started: %v", err)
	}
	if _, err := svc.dispatchStore.CASAdvance(ctx, runID, turnID, rev, DispatchSendStarted, DispatchProviderAccepted, nil); err != nil {
		t.Fatalf("provider_accepted: %v", err)
	}
	bug673Run(svc, runID, turnID, time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano))

	svc.sweepWedgedFlowWork()

	if !bug673Inflight(svc, runID) {
		t.Fatal("sweep released turnInFlight on a provider-accepted dispatch — could double-drive a live provider turn")
	}
}
