package runner

import (
	"context"
	"errors"
	"testing"
)

// CP-51 Task-249 (P1): the live Stop handler must advance the durable RunStopState
// so a concurrent send's send_claimed→send_started CAS is fenced in revision order
// (INV-3). These tests exercise requestRunStopV2 — the exact seam stopAgentLoop now
// calls — against the memory store, proving root and parent fences fire.

func newStopFenceService(t *testing.T) (*InteractiveService, DispatchStore) {
	t.Helper()
	svc := newInteractiveService(newProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	store := NewMemoryDispatchStore()
	svc.SetDispatchStore(store)
	return svc, store
}

// mustClaimed drives a fresh prepared record to send_claimed and returns its revision.
func mustClaimed(t *testing.T, store DispatchStore, runID, turnID string) int64 {
	t.Helper()
	ctx := context.Background()
	rec := testPrepared(runID, turnID)
	env := testEnvelope(runID, turnID)
	if err := store.CreatePrepared(ctx, rec, env); err != nil {
		t.Fatalf("CreatePrepared: %v", err)
	}
	rev, err := store.CASAdvance(ctx, runID, turnID, 1, DispatchPrepared, DispatchSendClaimed, nil)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	return rev
}

func TestLiveStop_AdvancesRunStopState_FencesRootSend(t *testing.T) {
	svc, store := newStopFenceService(t)
	ctx := context.Background()
	rev := mustClaimed(t, store, "r1", "t1")

	// The live Stop seam.
	svc.requestRunStopV2(ctx, "r1")

	st, err := store.GetRunStopState(ctx, "r1")
	if err != nil {
		t.Fatalf("GetRunStopState: %v", err)
	}
	if !st.Stopped {
		t.Fatal("requestRunStopV2 did not mark RunStopState stopped")
	}
	_ = rev // pre-stop revision is intentionally stale after the cancel sweep below.
	// The stop sweep cancel-requests the active record (bumping its revision), so a
	// send using the pre-stop revision is rejected as stale — a send can never
	// linearize on a snapshot older than the Stop (INV-3). linearizeSendStarted
	// reloads on stale, so re-read and prove the fresh send is still blocked.
	got, freshRev, err := store.Get(ctx, "r1", "t1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.CancelRequested {
		t.Fatal("requestRunStopV2 did not set CancelRequested on the active record")
	}
	_, err = store.CASAdvance(ctx, "r1", "t1", freshRev, DispatchSendClaimed, DispatchSendStarted, nil)
	if err == nil {
		t.Fatal("send_claimed→send_started must be blocked after a live Stop, but CAS succeeded")
	}
}

func TestLiveStop_NoOpForNonV2Run(t *testing.T) {
	svc, store := newStopFenceService(t)
	ctx := context.Background()
	// No dispatch record for r-none → not V2-activated → requestRunStopV2 is a no-op
	// and must NOT spuriously create activation/stop state.
	svc.requestRunStopV2(ctx, "r-none")
	st, err := store.GetRunStopState(ctx, "r-none")
	if err != nil {
		t.Fatalf("GetRunStopState: %v", err)
	}
	if st.Stopped {
		t.Fatal("requestRunStopV2 must not stop a run with no V2 dispatch records")
	}
}

func TestLiveStop_ParentStopFencesChildSend(t *testing.T) {
	svc, store := newStopFenceService(t)
	ctx := context.Background()

	// Parent has a V2 record (so parent is V2-activated and requestRunStopV2 acts).
	_ = mustClaimed(t, store, "parent", "pt1")

	// Child record carries a ParentStopFence bound to the parent at generation 0.
	childRec := testPrepared("child", "ct1")
	childRec.ParentStopFence = &ParentStopFence{ParentRunID: "parent", ExpectedStopGeneration: 0}
	childEnv := testEnvelope("child", "ct1")
	if err := store.CreatePrepared(ctx, childRec, childEnv); err != nil {
		t.Fatalf("child CreatePrepared: %v", err)
	}
	childRev, err := store.CASAdvance(ctx, "child", "ct1", 1, DispatchPrepared, DispatchSendClaimed, nil)
	if err != nil {
		t.Fatalf("child claim: %v", err)
	}

	// Stop the parent (as stopAgentLoop now does for the parent run).
	svc.requestRunStopV2(ctx, "parent")

	// The child's send must trip the parent fence (generation moved off 0 / stopped).
	_, err = store.CASAdvance(ctx, "child", "ct1", childRev, DispatchSendClaimed, DispatchSendStarted, nil)
	var pf ErrParentStopFence
	if !errors.As(err, &pf) {
		t.Fatalf("child send should be parent-Stop fenced, got %v", err)
	}
}

// failingRunStopStore wraps a real DispatchStore but forces RequestRunStop to
// fail, simulating a durable-store outage exactly at the fence-commit step.
type failingRunStopStore struct {
	DispatchStore
	failRequestRunStop bool
}

func (f *failingRunStopStore) RequestRunStop(ctx context.Context, runID string, expectedRunStopRev int64, reason StopReason) (RunStopState, error) {
	if f.failRequestRunStop {
		return RunStopState{}, errors.New("simulated durable store outage")
	}
	return f.DispatchStore.RequestRunStop(ctx, runID, expectedRunStopRev, reason)
}

func TestLiveStop_FailClosed_ReturnsErrorWhenDurableFenceCannotBeWritten(t *testing.T) {
	// CP-51 Task-249 fail-closed (Codex review 2026-07-17): requestRunStopV2 must
	// surface a durable-store failure instead of silently swallowing it — a caller
	// treating a void/no-error result as "Stop is guaranteed" would be wrong.
	svc, store := newStopFenceService(t)
	ctx := context.Background()
	_ = mustClaimed(t, store, "r1", "t1")

	failing := &failingRunStopStore{DispatchStore: store, failRequestRunStop: true}
	svc.SetDispatchStore(failing)

	err := svc.requestRunStopV2(ctx, "r1")
	if err == nil {
		t.Fatal("requestRunStopV2 must return an error when the durable RequestRunStop write fails")
	}

	// And the fence was indeed never persisted.
	st, gerr := store.GetRunStopState(ctx, "r1")
	if gerr != nil {
		t.Fatalf("GetRunStopState: %v", gerr)
	}
	if st.Stopped {
		t.Fatal("RunStopState must not be Stopped when RequestRunStop failed")
	}
}

func TestStopAgentLoop_FailClosed_ReportsErrorWhenDurableStopFenceFails(t *testing.T) {
	// End-to-end: the live HTTP-facing stopAgentLoop must NOT report success when
	// the durable Stop fence could not be written (Codex review 2026-07-17).
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(ctx context.Context, _ TurnRequest, _ TurnBridge) error {
				<-ctx.Done()
				return ctx.Err()
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	memStore := NewMemoryDispatchStore()
	svc.SetDispatchStore(memStore)

	handle, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	if _, apiErr := svc.startTurn(handle.RunID, TurnInput{StepID: handle.StepID, Prompt: "keep running"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn: %v", apiErr)
	}

	// Swap in a store whose RequestRunStop always fails — simulates a durable
	// outage exactly when Stop tries to write the fence.
	svc.SetDispatchStore(&failingRunStopStore{DispatchStore: memStore, failRequestRunStop: true})

	_, apiErr := svc.stopAgentLoop(handle.RunID)
	if apiErr == nil {
		t.Fatal("stopAgentLoop must report an error when the durable stop fence could not be persisted, not silent success")
	}
	if apiErr.code != "dispatch_stop_fence_failed" {
		t.Fatalf("apiErr.code = %q, want dispatch_stop_fence_failed", apiErr.code)
	}
}

// The remaining tests close named §4.2 gaps for Task-249 that are pure store
// contract behavior (no adapter/service plumbing needed) — CommitReceiptAndClearIntent
// and CommitPreSendCancellationAndClearIntent already implement the guard; these
// prove it, closing DOD `RC`/`RE`'s store-contract half (the live-adapter half is
// vacuously satisfied: Codex/Grok/Claude have no Accepted call site at all, proven
// by TestCapabilityEvidence_CodexGrokClaudeNoAcceptedSeam).

func TestAcceptedRejectsPreSendAndPayloadConflict(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r1", "t1")
	env := testEnvelope("r1", "t1")
	if err := store.CreatePrepared(ctx, rec, env); err != nil {
		t.Fatal(err)
	}

	// Pre-send: still `prepared`. Accepted must be rejected — no clear, no
	// transition — receipts are only legal at send_started/provider_accepted.
	rcpt := ReceiptEvidence{ProviderKey: "fake", ReceiptID: "rid-1", EvidenceKind: "ack", PayloadSHA256: "p1"}
	if _, err := store.CommitReceiptAndClearIntent(ctx, "r1", "t1", 1, rcpt, "r1", rec.OuterIntentKey, 1); err == nil {
		t.Fatal("receipt commit must be rejected before send_started")
	}
	got, _, err := store.Get(ctx, "r1", "t1")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != DispatchPrepared {
		t.Fatalf("pre-send rejected receipt must not transition state, got %s", got.State)
	}
	if cleared, _ := store.IsIntentCleared(ctx, "r1", rec.OuterIntentKey, 1); cleared {
		t.Fatal("pre-send rejected receipt must not clear intent")
	}

	// Advance to send_started, commit a receipt, then attempt a conflicting
	// payload under the SAME receipt identity — must be ErrReceiptConflict, and
	// the original receipt/state must be untouched.
	rev, err := store.CASAdvance(ctx, "r1", "t1", 1, DispatchPrepared, DispatchSendClaimed, nil)
	if err != nil {
		t.Fatal(err)
	}
	rev, err = store.CASAdvance(ctx, "r1", "t1", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	if err != nil {
		t.Fatal(err)
	}
	rev, err = store.CommitReceiptAndClearIntent(ctx, "r1", "t1", rev, rcpt, "r1", rec.OuterIntentKey, 1)
	if err != nil {
		t.Fatal(err)
	}
	conflicting := ReceiptEvidence{ProviderKey: "fake", ReceiptID: "rid-1", EvidenceKind: "ack", PayloadSHA256: "DIFFERENT"}
	if _, err := store.CommitReceiptAndClearIntent(ctx, "r1", "t1", rev, conflicting, "r1", rec.OuterIntentKey, 1); !errors.Is(err, ErrReceiptConflict) {
		t.Fatalf("divergent payload under same receipt identity must be ErrReceiptConflict, got %v", err)
	}
}

func TestOuterIntentNotClearedWhileSendClaimed(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r1", "t1")
	env := testEnvelope("r1", "t1")
	if err := store.CreatePrepared(ctx, rec, env); err != nil {
		t.Fatal(err)
	}
	rev, err := store.CASAdvance(ctx, "r1", "t1", 1, DispatchPrepared, DispatchSendClaimed, nil)
	if err != nil {
		t.Fatal(err)
	}
	// send_claimed is not send_started — a receipt commit attempt here must be
	// rejected (same guard as TestAcceptedRejectsPreSendAndPayloadConflict, but
	// specifically for the send_claimed state named by this DOD row).
	rcpt := ReceiptEvidence{ProviderKey: "fake", ReceiptID: "rid-1", EvidenceKind: "ack", PayloadSHA256: "p1"}
	if _, err := store.CommitReceiptAndClearIntent(ctx, "r1", "t1", rev, rcpt, "r1", rec.OuterIntentKey, 1); err == nil {
		t.Fatal("receipt commit must be rejected while send_claimed")
	}
	if cleared, _ := store.IsIntentCleared(ctx, "r1", rec.OuterIntentKey, 1); cleared {
		t.Fatal("outer intent must not clear while send_claimed")
	}
	got, _, err := store.Get(ctx, "r1", "t1")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != DispatchSendClaimed {
		t.Fatalf("state must remain send_claimed, got %s", got.State)
	}
}

func TestPostSendCancelRequiresTerminalProof(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r1", "t1")
	env := testEnvelope("r1", "t1")
	if err := store.CreatePrepared(ctx, rec, env); err != nil {
		t.Fatal(err)
	}
	rev, err := store.CASAdvance(ctx, "r1", "t1", 1, DispatchPrepared, DispatchSendClaimed, nil)
	if err != nil {
		t.Fatal(err)
	}
	rev, err = store.CASAdvance(ctx, "r1", "t1", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	if err != nil {
		t.Fatal(err)
	}
	// A cancel request alone (no TerminalEvidence yet) must NOT terminalize —
	// TP/SC ledger rows: only proof commits terminal state/outcome.
	if _, err := store.SetCancelRequested(ctx, "r1", "t1", rev, 1); err != nil {
		t.Fatal(err)
	}
	got, _, err := store.Get(ctx, "r1", "t1")
	if err != nil {
		t.Fatal(err)
	}
	if got.State.IsTerminal() {
		t.Fatalf("cancel request alone must not terminalize, got %s", got.State)
	}
	if !got.CancelRequested {
		t.Fatal("CancelRequested must be set")
	}

	// Now genuine TerminalEvidence proves completed-before-cancel: outcome comes
	// from proof.Outcome, and StopOutcome=cancelled_in_flight records the Stop
	// attribution without changing that proof-derived outcome (TP/SA).
	proof := TerminalEvidence{
		ProviderKey: "fake", EvidenceKind: "turn_completed", Outcome: "completed",
		PayloadSHA256: "done-hash", ObservedAt: "2026-07-17T00:00:00Z",
	}
	got2, curRev, err := store.Get(ctx, "r1", "t1")
	if err != nil {
		t.Fatal(err)
	}
	_ = got2
	if _, err := store.CommitTerminalAndSettleIntent(ctx, "r1", "t1", curRev, proof, "r1", rec.OuterIntentKey, 1); err != nil {
		t.Fatal(err)
	}
	final, _, err := store.Get(ctx, "r1", "t1")
	if err != nil {
		t.Fatal(err)
	}
	if final.State != DispatchTerminalCompleted {
		t.Fatalf("proof.Outcome=completed must drive terminal state, got %s", final.State)
	}
	if final.StopOutcome != StopOutcomeCancelledInFlight {
		t.Fatalf("cancel-requested-then-completed must record StopOutcome=cancelled_in_flight, got %q", final.StopOutcome)
	}
}
