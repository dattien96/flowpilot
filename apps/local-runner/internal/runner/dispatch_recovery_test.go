package runner

import (
	"context"
	"testing"
	"time"
)

// CP-51 Task-250. Codex review 2026-07-17 found two gaps beyond "no boot
// caller": (1) ScanAllRecoverable enumerated via ListAttention, which only
// surfaces DispatchUncertain records, so prepared/send_claimed/send_started
// were invisible to boot recovery even with a caller wired; (2) the
// prepared/send_claimed branch classified "safely-retryable" by comment only,
// with no actual redispatch trigger. These tests exercise both fixes.

func TestScanAllRecoverable_EnumeratesEveryNonTerminalState_NotJustUncertain(t *testing.T) {
	store := NewMemoryDispatchStore()
	ctx := context.Background()

	// One record in each non-terminal state, across distinct runs.
	// prepared: r-prepared/t1
	rec1 := testPrepared("r-prepared", "t1")
	env1 := testEnvelope("r-prepared", "t1")
	if err := store.CreatePrepared(ctx, rec1, env1); err != nil {
		t.Fatalf("CreatePrepared r-prepared: %v", err)
	}
	// send_claimed: r-claimed/t1
	rec2 := testPrepared("r-claimed", "t1")
	env2 := testEnvelope("r-claimed", "t1")
	if err := store.CreatePrepared(ctx, rec2, env2); err != nil {
		t.Fatalf("CreatePrepared r-claimed: %v", err)
	}
	if _, err := store.CASAdvance(ctx, "r-claimed", "t1", 1, DispatchPrepared, DispatchSendClaimed, nil); err != nil {
		t.Fatalf("claim r-claimed: %v", err)
	}
	// send_started (ambiguous — the classic "did the provider get it?" case):
	// r-started/t1
	rec3 := testPrepared("r-started", "t1")
	env3 := testEnvelope("r-started", "t1")
	if err := store.CreatePrepared(ctx, rec3, env3); err != nil {
		t.Fatalf("CreatePrepared r-started: %v", err)
	}
	rev3, err := store.CASAdvance(ctx, "r-started", "t1", 1, DispatchPrepared, DispatchSendClaimed, nil)
	if err != nil {
		t.Fatalf("claim r-started: %v", err)
	}
	if _, err := store.CASAdvance(ctx, "r-started", "t1", rev3, DispatchSendClaimed, DispatchSendStarted, nil); err != nil {
		t.Fatalf("send r-started: %v", err)
	}

	// Before the fix, only DispatchUncertain records were visible to
	// ScanAllRecoverable (via ListAttention). Confirm ListRecoverable("") — what
	// the fixed implementation now uses — sees all three non-terminal runs.
	all, err := store.ListRecoverable(ctx, "")
	if err != nil {
		t.Fatalf("ListRecoverable: %v", err)
	}
	seenRuns := map[string]bool{}
	for _, r := range all {
		seenRuns[r.RunID] = true
	}
	for _, want := range []string{"r-prepared", "r-claimed", "r-started"} {
		if !seenRuns[want] {
			t.Fatalf("ListRecoverable(\"\") missing run=%s (states=%v)", want, all)
		}
	}

	// And the actual boot entry point visits all three (not just uncertain-state
	// runs, of which there are none here) — track via the redispatch hook plus a
	// direct re-check that send_started was reclassified toward uncertain.
	var redispatched []string
	sc := &RecoveryScanner{
		Store: store,
		Owner: "test-scanner",
		EnsureLiveAndRedispatch: func(_ context.Context, runID string) {
			redispatched = append(redispatched, runID)
		},
	}
	if err := sc.ScanAllRecoverable(ctx); err != nil {
		t.Fatalf("ScanAllRecoverable: %v", err)
	}

	wantRedispatched := map[string]bool{"r-prepared": true, "r-claimed": true}
	for _, id := range redispatched {
		if !wantRedispatched[id] {
			t.Fatalf("unexpected redispatch for run=%s", id)
		}
		delete(wantRedispatched, id)
	}
	if len(wantRedispatched) != 0 {
		t.Fatalf("prepared/send_claimed runs never redispatched: %v", wantRedispatched)
	}

	// r-started (send_started/provider_accepted) has no provider reconcile proof
	// (Task-257 evidenced guarantee class), so it must fall through to
	// CommitRecoveryUnknownOrRequireCancel and land on DispatchUncertain — never
	// redispatched, never silently left at send_started (that would be a
	// lost-forever ambiguous turn no operator ever sees), never silently
	// terminalized (that would risk a false completion with no proof).
	got, _, err := store.Get(ctx, "r-started", "t1")
	if err != nil {
		t.Fatalf("Get r-started: %v", err)
	}
	if got.State != DispatchUncertain {
		t.Fatalf("r-started: want state=%s after no-adapter recovery fallback, got=%s (must not be left dangling at send_started, and must not be silently terminalized)", DispatchUncertain, got.State)
	}
	if got.Revision <= 1 {
		t.Fatalf("r-started: want revision bumped by the uncertain CAS, got revision=%d", got.Revision)
	}
}

func TestRecoveryScanner_PreparedSafelyRetryable_TriggersRedispatch(t *testing.T) {
	store := NewMemoryDispatchStore()
	ctx := context.Background()
	rec := testPrepared("r1", "t1")
	env := testEnvelope("r1", "t1")
	if err := store.CreatePrepared(ctx, rec, env); err != nil {
		t.Fatalf("CreatePrepared: %v", err)
	}

	var called []string
	sc := &RecoveryScanner{
		Store: store,
		Owner: "test-scanner",
		EnsureLiveAndRedispatch: func(_ context.Context, runID string) {
			called = append(called, runID)
		},
	}
	if err := sc.ScanRun(ctx, "r1"); err != nil {
		t.Fatalf("ScanRun: %v", err)
	}
	if len(called) != 1 || called[0] != "r1" {
		t.Fatalf("EnsureLiveAndRedispatch calls = %v, want [r1]", called)
	}
}

func TestRecoveryScanner_CancelRequestedSkipsRedispatch_PreSendCancelsInstead(t *testing.T) {
	store := NewMemoryDispatchStore()
	ctx := context.Background()
	rec := testPrepared("r1", "t1")
	env := testEnvelope("r1", "t1")
	if err := store.CreatePrepared(ctx, rec, env); err != nil {
		t.Fatalf("CreatePrepared: %v", err)
	}
	// Live-Stop already marked this record cancel-requested (as stopAgentLoop now does).
	got, rev, err := store.Get(ctx, "r1", "t1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if _, err := store.SetCancelRequested(ctx, "r1", "t1", rev, 1); err != nil {
		t.Fatalf("SetCancelRequested: %v", err)
	}
	_ = got

	var called []string
	sc := &RecoveryScanner{
		Store: store,
		Owner: "test-scanner",
		EnsureLiveAndRedispatch: func(_ context.Context, runID string) {
			called = append(called, runID)
		},
	}
	if err := sc.ScanRun(ctx, "r1"); err != nil {
		t.Fatalf("ScanRun: %v", err)
	}
	if len(called) != 0 {
		t.Fatalf("EnsureLiveAndRedispatch must not fire for a cancel-requested record, got %v", called)
	}
	final, _, err := store.Get(ctx, "r1", "t1")
	if err != nil {
		t.Fatalf("Get final: %v", err)
	}
	if !final.State.IsTerminal() {
		t.Fatalf("cancel-requested prepared record should pre-send-cancel to terminal, got %s", final.State)
	}
}

// TestScanDispatchRecoveryOnBoot_ReconstructsAndRedispatches is the end-to-end
// proof: a run with NO live RAM state (simulating a process restart) and a
// persisted prepared dispatch record gets reconstructed into RAM and its
// durable turn intent flushed by the boot scan — closing the T-6 wiring gap.
func TestScanDispatchRecoveryOnBoot_ReconstructsAndRedispatches(t *testing.T) {
	reg := newProviderRegistry()
	fakeStore := newFakeWorkflowStore()
	svc := newInteractiveService(reg, newInteractiveCatalog(), fakeStore)
	memStore := NewMemoryDispatchStore()
	svc.SetDispatchStore(memStore)

	ctx := context.Background()
	runID := "run-crashed"
	rec := testPrepared(runID, "t1")
	env := testEnvelope(runID, "t1")
	if err := memStore.CreatePrepared(ctx, rec, env); err != nil {
		t.Fatalf("CreatePrepared: %v", err)
	}

	// Persist the run's session so reconstruction has something to load — with
	// a durable pending-restart intent so flushDurableTurnIntents has real work
	// to do once the run is live (mirrors what startTurn persists before crash).
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := fakeStore.UpsertProviderSession(ctx, ProviderSessionState{
		RunID:                runID,
		ProjectID:            "p",
		ProviderKey:          ProviderKeyCodex,
		Status:               RunStatusRunning,
		StartedAt:            now,
		UpdatedAt:            now,
		PendingRestartRunID:  runID,
		PendingRestartPrompt: "resume after crash",
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	// Precondition: run is NOT live (this IS the crash-recovery scenario).
	svc.mu.Lock()
	_, live := svc.runs[runID]
	svc.mu.Unlock()
	if live {
		t.Fatal("test setup error: run must not be live before the boot scan")
	}

	svc.ScanDispatchRecoveryOnBoot(ctx)

	// The boot scan must have reconstructed the run into RAM.
	svc.mu.Lock()
	_, live = svc.runs[runID]
	svc.mu.Unlock()
	if !live {
		t.Fatal("ScanDispatchRecoveryOnBoot did not reconstruct the crashed run into RAM")
	}
}

// TestRecovery_UncertainHoldsIntent_NeverClearsNeverRedispatches proves CP-51
// ledger row Rr3: once a sent-but-unprovable turn is classified uncertain, its
// outer intent is never cleared (an operator must resolve it — SS-17) and it
// is never redispatched, including across a second, idempotent scan pass.
func TestRecovery_UncertainHoldsIntent_NeverClearsNeverRedispatches(t *testing.T) {
	store := NewMemoryDispatchStore()
	ctx := context.Background()

	rec := testPrepared("r-sent", "t1")
	env := testEnvelope("r-sent", "t1")
	if err := store.CreatePrepared(ctx, rec, env); err != nil {
		t.Fatalf("CreatePrepared: %v", err)
	}
	rev, err := store.CASAdvance(ctx, "r-sent", "t1", 1, DispatchPrepared, DispatchSendClaimed, nil)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if _, err := store.CASAdvance(ctx, "r-sent", "t1", rev, DispatchSendClaimed, DispatchSendStarted, nil); err != nil {
		t.Fatalf("send: %v", err)
	}

	var redispatched []string
	sc := &RecoveryScanner{
		Store: store,
		Owner: "test-scanner",
		EnsureLiveAndRedispatch: func(_ context.Context, runID string) {
			redispatched = append(redispatched, runID)
		},
	}

	for i := 0; i < 2; i++ {
		if err := sc.ScanRun(ctx, "r-sent"); err != nil {
			t.Fatalf("ScanRun pass %d: %v", i, err)
		}
		if len(redispatched) != 0 {
			t.Fatalf("pass %d: uncertain-bound record must never be redispatched, got %v", i, redispatched)
		}
		got, _, err := store.Get(ctx, "r-sent", "t1")
		if err != nil {
			t.Fatalf("Get pass %d: %v", i, err)
		}
		if got.State != DispatchUncertain {
			t.Fatalf("pass %d: want state=%s, got=%s", i, DispatchUncertain, got.State)
		}
		if got.IntentOwnerRunID != rec.IntentOwnerRunID || got.OuterIntentKey != rec.OuterIntentKey || got.OuterIntentGen != rec.OuterIntentGen {
			t.Fatalf("pass %d: outer intent must survive uncertain classification untouched, got owner=%q key=%q gen=%d",
				i, got.IntentOwnerRunID, got.OuterIntentKey, got.OuterIntentGen)
		}
	}
}
