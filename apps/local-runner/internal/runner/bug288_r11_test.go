package runner

// BUG-288 Round 11: durable-persist-before-fan-out regression tests.
//
// Findings #3/#4 both moved a mutation (child completion / resumed gate pass)
// to happen only after the durable session write succeeds, and to revalidate
// gateEpoch/claim state once that write returns (a Stop can land while the
// write is in flight, since it happens with s.mu unlocked). These tests drive
// that exact race with a wrapped fakeWorkflowStore that hooks
// UpsertProviderSession to either fail or mutate state mid-call.

import (
	"context"
	"errors"
	"testing"
	"time"
)

// hookProviderSessionStore wraps fakeWorkflowStore so a test can intercept
// UpsertProviderSession — either to fail it (simulating a durable-write
// error) or to mutate service state as a side effect (simulating a concurrent
// Stop landing while the write is in flight, since persistProviderSession is
// always called with s.mu unlocked).
type hookProviderSessionStore struct {
	*fakeWorkflowStore
	onUpsert func(ProviderSessionState) error
}

func (h *hookProviderSessionStore) UpsertProviderSession(ctx context.Context, session ProviderSessionState) error {
	if h.onUpsert != nil {
		if err := h.onUpsert(session); err != nil {
			return err
		}
	}
	return h.fakeWorkflowStore.UpsertProviderSession(ctx, session)
}

// TestRunTurnGatePassPersistFailureDoesNotFanOut (BUG-288 R11 #3): runTurn's
// gate-pass branch must persist the post-gate Completed snapshot BEFORE
// signalChild/broadcast/settleFlowChildTurnCompletedLocked — a persistence
// failure must leave the completion retryable (pendingFlowGateSettle still
// set, status not Completed) instead of fanning out with no durable record.
func TestRunTurnGatePassPersistFailureDoesNotFanOut(t *testing.T) {
	var svc *InteractiveService
	var childID string
	store := &hookProviderSessionStore{fakeWorkflowStore: newFakeWorkflowStore()}
	store.onUpsert = func(session ProviderSessionState) error {
		if session.RunID == childID && session.Status == RunStatusCompleted {
			return errors.New("simulated persist failure")
		}
		return nil
	}
	svc, _ = newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("child: %v", err)
	}
	childID = child.RunID

	// Empty workspace: child artifact-output gate degrades to pass (no block),
	// so the turn drives straight into the gate-pass persist branch.
	dir := t.TempDir()
	svc.mu.Lock()
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[parent.RunID].activeFlowNodes = reviewLoopTestNodes()
	crs := svc.runs[childID]
	crs.parentRunID = parent.RunID
	crs.label = "coder"
	crs.workspaceCwd = dir
	svc.mu.Unlock()

	if _, apiErr := svc.startTurn(childID, TurnInput{StepID: "coder", Prompt: "implement the change"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn: %s", apiErr.msg)
	}

	waitLoop(t, "turn settles after gate-pass persist failure", 2*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		rs := svc.runs[childID]
		return rs != nil && !rs.turnInFlight
	})

	svc.mu.Lock()
	rs := svc.runs[childID]
	status := rs.status
	pendingSettle := rs.pendingFlowGateSettle
	svc.mu.Unlock()

	if status == RunStatusCompleted {
		t.Fatal("a failed durable persist must not leave the run published as Completed")
	}
	if !pendingSettle {
		t.Fatal("pendingFlowGateSettle must remain set so a later resume can retry the completion")
	}
	// emitLocked always appends an in-memory placeholder EventTurnCompleted
	// (deferGateCompleted skips only its own persistEvent call) — the real
	// invariant is that the gate-pass branch's OWN completedEv never reached
	// the durable store, i.e. persistEvent(completedEv) was never called.
	store.mu.Lock()
	persisted := append([]ProviderEvent(nil), store.events[childID]...)
	store.mu.Unlock()
	for _, ev := range persisted {
		if ev.Type == EventTurnCompleted {
			t.Fatal("a failed durable persist must not durably persist the terminal TurnCompleted event")
		}
	}
}

// TestResumePendingFlowGateRevalidatesEpochAfterPersist (BUG-288 R11 #4):
// resumePendingFlowGate releases gateClaimID before its post-gate persist
// call, so a concurrent Stop landing during that unlocked persist can only be
// detected by revalidating gateEpoch afterward. This simulates that race by
// bumping gateEpoch as a side effect of the persist call itself and asserts
// the resumed completion does not still fan out (signal/broadcast/settle).
func TestResumePendingFlowGateRevalidatesEpochAfterPersist(t *testing.T) {
	var svc *InteractiveService
	var childID string
	store := &hookProviderSessionStore{fakeWorkflowStore: newFakeWorkflowStore()}
	store.onUpsert = func(session ProviderSessionState) error {
		if session.RunID == childID {
			svc.mu.Lock()
			if rs := svc.runs[childID]; rs != nil {
				rs.gateEpoch++
			}
			svc.mu.Unlock()
		}
		return nil
	}
	svc, _ = newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("child: %v", err)
	}
	childID = child.RunID

	dir := t.TempDir()
	svc.mu.Lock()
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[parent.RunID].activeFlowNodes = reviewLoopTestNodes()
	crs := svc.runs[childID]
	crs.parentRunID = parent.RunID
	crs.label = "coder"
	crs.workspaceCwd = dir
	crs.pendingFlowGateSettle = true
	crs.pendingFlowGateFinalMsg = "after restart"
	crs.pendingFlowGateOccurredAt = time.Now().UTC().Format(time.RFC3339Nano)
	crs.lastTurnID = "turn-race-1"
	crs.status = RunStatusRunning
	crs.events = nil
	epochBefore := crs.gateEpoch
	svc.mu.Unlock()

	svc.resumePendingFlowGate(childID)

	svc.mu.Lock()
	rs := svc.runs[childID]
	gotEpoch := rs.gateEpoch
	events := append([]ProviderEvent(nil), rs.events...)
	svc.mu.Unlock()

	if gotEpoch == epochBefore {
		t.Fatal("test setup: onUpsert epoch bump did not fire")
	}
	for _, ev := range events {
		if ev.Type == EventTurnCompleted {
			t.Fatal("a gateEpoch bump landing during the unlocked persist call must suppress the resumed fan-out (no materialized TurnCompleted)")
		}
	}
}
