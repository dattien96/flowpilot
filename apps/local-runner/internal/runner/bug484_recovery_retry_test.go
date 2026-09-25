package runner

// BUG-484 (CP-51 INV-5): a transient dispatch-store outage during boot
// recovery must not wedge non-terminal records until the next process
// restart. ScanDispatchRecoveryOnBoot retries the pass with bounded backoff
// until each record reaches terminal / safely-retryable / explicit
// attention — while the process keeps running.

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// bug484FlakyStore fails ListRecoverable / Get a controlled number of times,
// then heals — a transient store outage window.
type bug484FlakyStore struct {
	DispatchStore
	failListRecoverable atomic.Int32
	failGet             atomic.Int32
	listCalls           atomic.Int32
}

func (s *bug484FlakyStore) ListRecoverable(ctx context.Context, runID string) ([]DispatchRecord, error) {
	s.listCalls.Add(1)
	if s.failListRecoverable.Load() > 0 {
		s.failListRecoverable.Add(-1)
		return nil, errors.New("injected transient store outage")
	}
	return s.DispatchStore.ListRecoverable(ctx, runID)
}

func (s *bug484FlakyStore) Get(ctx context.Context, runID, turnID string) (DispatchRecord, int64, error) {
	if s.failGet.Load() > 0 {
		s.failGet.Add(-1)
		return DispatchRecord{}, 0, errors.New("injected transient get failure")
	}
	return s.DispatchStore.Get(ctx, runID, turnID)
}

func bug484SendStarted(t *testing.T, store DispatchStore, runID, turnID string) {
	t.Helper()
	ctx := context.Background()
	if err := store.CreatePrepared(ctx, testPrepared(runID, turnID), testEnvelope(runID, turnID)); err != nil {
		t.Fatalf("CreatePrepared: %v", err)
	}
	rev, err := store.CASAdvance(ctx, runID, turnID, 1, DispatchPrepared, DispatchSendClaimed, nil)
	if err != nil {
		t.Fatalf("send_claimed: %v", err)
	}
	if _, err := store.CASAdvance(ctx, runID, turnID, rev, DispatchSendClaimed, DispatchSendStarted, nil); err != nil {
		t.Fatalf("send_started: %v", err)
	}
}

// Boot enumeration failing transiently must retry in-process until the store
// heals — the send_started record must still reach DispatchUncertain without
// a restart.
func TestBUG484_BootListFailureRetriesUntilRecovered(t *testing.T) {
	base := dispatchRecoveryRetryBase
	dispatchRecoveryRetryBase = time.Millisecond
	defer func() { dispatchRecoveryRetryBase = base }()

	store := &bug484FlakyStore{DispatchStore: NewMemoryDispatchStore()}
	bug484SendStarted(t, store, "run-484", "t-484")
	store.failListRecoverable.Store(3) // first 3 passes fail, then heal

	svc := NewInteractiveService()
	svc.dispatchStore = store
	svc.ScanDispatchRecoveryOnBoot(context.Background())

	if got := store.listCalls.Load(); got < 4 {
		t.Fatalf("recovery pass never retried after transient outage (ListRecoverable calls=%d)", got)
	}
	got, _, err := store.Get(context.Background(), "run-484", "t-484")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != DispatchUncertain {
		t.Fatalf("send_started must reconcile to uncertain once the store heals, got %s", got.State)
	}
}

// A per-record transient failure mid-pass marks the pass failed and retries —
// lease ownership is respected (same owner reclaims or skips cleanly).
func TestBUG484_PerRecordFailureRetriesThatRecord(t *testing.T) {
	base := dispatchRecoveryRetryBase
	dispatchRecoveryRetryBase = time.Millisecond
	defer func() { dispatchRecoveryRetryBase = base }()

	store := &bug484FlakyStore{DispatchStore: NewMemoryDispatchStore()}
	bug484SendStarted(t, store, "run-484b", "t-484b")
	store.failGet.Store(1) // first reconcile's re-read fails once

	svc := NewInteractiveService()
	svc.dispatchStore = store
	svc.ScanDispatchRecoveryOnBoot(context.Background())

	got, _, err := store.Get(context.Background(), "run-484b", "t-484b")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.State != DispatchUncertain {
		t.Fatalf("per-record transient failure must be retried to uncertain, got %s", got.State)
	}
}

// Retry is bounded: a permanently failing store does not hang the boot
// path — it gives up after the cap and leaves records for the next trigger.
func TestBUG484_RetryIsBounded(t *testing.T) {
	base := dispatchRecoveryRetryBase
	maxA := dispatchRecoveryMaxAttempts
	dispatchRecoveryRetryBase = time.Millisecond
	dispatchRecoveryMaxAttempts = 4
	defer func() {
		dispatchRecoveryRetryBase = base
		dispatchRecoveryMaxAttempts = maxA
	}()

	store := &bug484FlakyStore{DispatchStore: NewMemoryDispatchStore()}
	bug484SendStarted(t, store, "run-484c", "t-484c")
	store.failListRecoverable.Store(100) // never heals in-window

	svc := NewInteractiveService()
	svc.dispatchStore = store
	done := make(chan struct{})
	go func() { svc.ScanDispatchRecoveryOnBoot(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("recovery coordinator did not respect its bound — boot path hangs")
	}
	if got := store.listCalls.Load(); got <= 1 || got > int32(maxA)+2 {
		t.Fatalf("expected bounded retries (2..%d), got %d calls", maxA+2, got)
	}
}

// Context cancellation aborts the retry loop promptly.
func TestBUG484_CtxCancelStopsRetry(t *testing.T) {
	base := dispatchRecoveryRetryBase
	dispatchRecoveryRetryBase = 50 * time.Millisecond
	defer func() { dispatchRecoveryRetryBase = base }()

	store := &bug484FlakyStore{DispatchStore: NewMemoryDispatchStore()}
	bug484SendStarted(t, store, "run-484d", "t-484d")
	store.failListRecoverable.Store(100)

	svc := NewInteractiveService()
	svc.dispatchStore = store
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { svc.ScanDispatchRecoveryOnBoot(ctx); close(done) }()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("ctx cancel did not stop the recovery retry loop")
	}
}
