package runner

import (
	"context"
	"testing"
	"time"
)

// BUG-540 (live run-1663/turn-6304, run-2830/turn-4521): a turn that reached
// terminal_completed in the dispatch ledger owed its settle for 37–90 minutes
// in-session — nothing re-drove it until process restart (boot's
// drivePendingSettlesOnBoot). turnInFlight stayed pinned → every reinvoke
// refused turn_in_progress → silent wedge.
//
// Fix shape: an in-session sweep walks terminal+settle_owed records and
// re-drives them (resumePendingFlowGate for gate-pending, scheduleSettleDrive
// otherwise), and releases wedged gate-eval/turnInFlight residue.

// owedSettleRecord drives one dispatch record to terminal_completed with
// settle_owed — the exact wedge seed from the live ledger.
func owedSettleRecord(t *testing.T, svc *InteractiveService, runID, turnID string) {
	t.Helper()
	ctx := context.Background()
	store := svc.dispatchStore
	rec := testPrepared(runID, turnID)
	if err := store.CreatePrepared(ctx, rec, testEnvelope(runID, turnID)); err != nil {
		t.Fatal(err)
	}
	rev := int64(1)
	var err error
	rev, err = store.CASAdvance(ctx, runID, turnID, rev, DispatchPrepared, DispatchSendClaimed, nil)
	if err != nil {
		t.Fatal(err)
	}
	rev, err = store.CASAdvance(ctx, runID, turnID, rev, DispatchSendClaimed, DispatchSendStarted, nil)
	if err != nil {
		t.Fatal(err)
	}
	proof := TerminalEvidence{ProviderKey: "f", EvidenceKind: "x", Outcome: "completed", PayloadSHA256: "h"}
	if _, err := store.CommitTerminalAndSettleIntent(ctx, runID, turnID, rev, proof, runID, rec.OuterIntentKey, 1); err != nil {
		t.Fatal(err)
	}
	got, _, err := store.Get(ctx, runID, turnID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.State.IsTerminal() || !got.SettleOwed || got.SettlePhase.IsSettleFinal() {
		t.Fatalf("setup: record must be terminal+settle_owed+unfinalized, got state=%s settleOwed=%v phase=%s",
			got.State, got.SettleOwed, got.SettlePhase)
	}
}

func settlePhaseOf(t *testing.T, svc *InteractiveService, runID, turnID string) SettlePhase {
	t.Helper()
	rec, _, err := svc.dispatchStore.Get(context.Background(), runID, turnID)
	if err != nil {
		t.Fatal(err)
	}
	return rec.SettlePhase
}

// The in-session sweep must consume an owed settle without any restart —
// a completed run with no pending gate finalizes to settle_finalized.
func TestBug540_SweepConsumesOwedSettleInSession(t *testing.T) {
	svc := bug289Service(t)
	svc.dispatchStore = NewMemoryDispatchStore()
	runID, turnID := "run-540a", "turn-540a"
	owedSettleRecord(t, svc, runID, turnID)

	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:     runID,
		status: RunStatusCompleted,
		subs:   map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()

	svc.driveOwedSettles(context.Background())

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if settlePhaseOf(t, svc, runID, turnID).IsSettleFinal() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("owed settle still pending after sweep: phase=%s", settlePhaseOf(t, svc, runID, turnID))
}

// turnInFlight pinned on a turn that is terminal in the ledger is residue,
// not live work — the sweep must release it so reinvokes stop refusing.
func TestBug540_SweepReleasesResidueTurnInFlight(t *testing.T) {
	svc := bug289Service(t)
	svc.dispatchStore = NewMemoryDispatchStore()
	runID, turnID := "run-540b", "turn-540b"
	owedSettleRecord(t, svc, runID, turnID)

	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:           runID,
		status:       RunStatusRunning,
		turnInFlight: true,
		lastTurnID:   turnID, // currentTurnID cleared post-turn; lastTurnID pins it
		subs:         map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()

	svc.driveOwedSettles(context.Background())

	svc.mu.Lock()
	inFlight := svc.runs[runID].turnInFlight
	svc.mu.Unlock()
	if inFlight {
		t.Fatal("sweep left turnInFlight pinned on a terminal turn — hub stays wedged")
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if settlePhaseOf(t, svc, runID, turnID).IsSettleFinal() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("owed settle still pending after sweep: phase=%s", settlePhaseOf(t, svc, runID, turnID))
}

// A post-turn gate eval wedged past postTurnGateBusyBound holds the settle
// hostage forever (live turn-6304: pendingFlowGateSettle armed, gate_eval never
// written). The sweep must epoch-bump the wedged eval, release its claims, and
// re-drive resumePendingFlowGate — blocked loop keeps intents armed (deferred).
func TestBug540_SweepUnsticksWedgedGateEval(t *testing.T) {
	svc := bug289Service(t)
	svc.dispatchStore = NewMemoryDispatchStore()
	runID, turnID := "run-540c", "turn-540c"
	owedSettleRecord(t, svc, runID, turnID)

	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:                    runID,
		status:                RunStatusRunning,
		turnInFlight:          true,
		lastTurnID:            turnID,
		pendingFlowGateSettle: true,
		pendingFlowGateTurnID: turnID,
		postTurnGateCancel:    func() {},
		postTurnGateStartedAt: time.Now().UTC().Add(-(postTurnGateBusyBound + time.Minute)),
		gateClaimID:           "claim-stale",
		gateEpoch:             1,
		subs:                  map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	// Loop blocked → the re-driven resumePendingFlowGate applies the run63960
	// contract (blocked loop owns the disposition; stale settle fields clear,
	// the re-check obligation survives via pendingGateCodePaths/orphan-cure)
	// so the post-unstick assertions are stable.
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "blocked", BlockReason: "escalate"})

	svc.driveOwedSettles(context.Background())

	// Give the async resumePendingFlowGate a beat to run its blocked defer.
	time.Sleep(150 * time.Millisecond)
	svc.mu.Lock()
	rs := svc.runs[runID]
	gateEpoch := rs.gateEpoch
	cancel := rs.postTurnGateCancel
	claim := rs.gateClaimID
	inFlight := rs.turnInFlight
	svc.mu.Unlock()

	if gateEpoch != 2 {
		t.Fatalf("wedged gate eval not epoch-bumped: gateEpoch=%d want 2", gateEpoch)
	}
	if cancel != nil {
		t.Fatal("stale postTurnGateCancel not released")
	}
	if claim != "" {
		t.Fatalf("wedged gateClaimID not released: %q", claim)
	}
	if inFlight {
		t.Fatal("turnInFlight still pinned on terminal turn after sweep")
	}
}

// A gate eval still inside postTurnGateBusyBound is live work — the sweep must
// not touch its claims.
func TestBug540_SweepLeavesLiveGateEvalAlone(t *testing.T) {
	svc := bug289Service(t)
	svc.dispatchStore = NewMemoryDispatchStore()
	runID, turnID := "run-540d", "turn-540d"
	owedSettleRecord(t, svc, runID, turnID)

	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:                    runID,
		status:                RunStatusRunning,
		turnInFlight:          true,
		lastTurnID:            turnID,
		pendingFlowGateSettle: true,
		pendingFlowGateTurnID: turnID,
		postTurnGateCancel:    func() {},
		postTurnGateStartedAt: time.Now().UTC(),
		gateClaimID:           "claim-live",
		gateEpoch:             1,
		subs:                  map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()

	svc.driveOwedSettles(context.Background())
	time.Sleep(100 * time.Millisecond)

	svc.mu.Lock()
	rs := svc.runs[runID]
	gateEpoch := rs.gateEpoch
	cancel := rs.postTurnGateCancel
	claim := rs.gateClaimID
	inFlight := rs.turnInFlight
	svc.mu.Unlock()

	if gateEpoch != 1 || cancel == nil || claim != "claim-live" {
		t.Fatalf("sweep disturbed a live gate eval: epoch=%d cancel=%v claim=%q", gateEpoch, cancel == nil, claim)
	}
	if !inFlight {
		t.Fatal("turnInFlight cleared while gate eval still live")
	}
}
