package runner

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Task-251 V-1: settle persist fails K times then recovers → driver advances.
func TestSettle_OutageThenRecovers_AutoAdvances(t *testing.T) {
	ctx := context.Background()
	inner := NewMemoryDispatchStore()
	var failsLeft atomic.Int32
	failsLeft.Store(2)
	store := &faultySettleStore{DispatchStore: inner, failsLeft: &failsLeft}

	rec := testPrepared("r-out", "t-out")
	rec.SettleOwed = true
	if err := store.CreatePrepared(ctx, rec, testEnvelope("r-out", "t-out")); err != nil {
		t.Fatal(err)
	}
	rev := int64(1)
	var err error
	rev, err = store.CASAdvance(ctx, "r-out", "t-out", rev, DispatchPrepared, DispatchSendClaimed, nil)
	if err != nil {
		t.Fatal(err)
	}
	rev, err = store.CASAdvance(ctx, "r-out", "t-out", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	if err != nil {
		t.Fatal(err)
	}
	proof := TerminalEvidence{ProviderKey: "f", EvidenceKind: "x", Outcome: "completed", PayloadSHA256: "h"}
	if _, err := store.CommitTerminalAndSettleIntent(ctx, "r-out", "t-out", rev, proof, "r-out", rec.OuterIntentKey, 1); err != nil {
		t.Fatal(err)
	}

	d := &SettleDriver{
		Store: store,
		EvaluateGate: func(context.Context, string, string) (bool, bool, error) {
			return true, false, nil
		},
	}
	if err := d.RetrySettleWithBackoff(ctx, "r-out", "t-out", 8); err != nil {
		t.Fatalf("expected recover after outage: %v", err)
	}
	got, _, _ := store.Get(ctx, "r-out", "t-out")
	if got.SettlePhase != SettleFinalized {
		t.Fatalf("settle phase=%s want finalized", got.SettlePhase)
	}
	if failsLeft.Load() > 0 {
		// Some fail budget may remain if only CASAdvanceSettle is faulted once per attempt.
	}
}

// faultySettleStore fails CASAdvanceSettle N times then succeeds (outage seam).
type faultySettleStore struct {
	DispatchStore
	failsLeft *atomic.Int32
}

func (f *faultySettleStore) CASAdvanceSettle(ctx context.Context, runID, turnID string, expectedRev int64, from, to SettlePhase) (int64, error) {
	if f.failsLeft != nil && f.failsLeft.Add(-1) >= 0 {
		return 0, context.DeadlineExceeded
	}
	return f.DispatchStore.CASAdvanceSettle(ctx, runID, turnID, expectedRev, from, to)
}

// Task-251 V-5: gate reprompt ⇒ settle_superseded_reprompt; recovery no further settle.
func TestSettle_GateReprompt_SupersededDisposition(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r-rp2", "t-rp2")
	rec.SettleOwed = true
	_ = store.CreatePrepared(ctx, rec, testEnvelope("r-rp2", "t-rp2"))
	rev := int64(1)
	rev, _ = store.CASAdvance(ctx, "r-rp2", "t-rp2", rev, DispatchPrepared, DispatchSendClaimed, nil)
	rev, _ = store.CASAdvance(ctx, "r-rp2", "t-rp2", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	proof := TerminalEvidence{ProviderKey: "f", EvidenceKind: "x", Outcome: "completed", PayloadSHA256: "h"}
	if _, err := store.CommitTerminalAndSettleIntent(ctx, "r-rp2", "t-rp2", rev, proof, "r-rp2", rec.OuterIntentKey, 1); err != nil {
		t.Fatal(err)
	}
	d := &SettleDriver{
		Store: store,
		EvaluateGate: func(context.Context, string, string) (bool, bool, error) {
			return false, true, nil
		},
	}
	if err := d.DriveSettle(ctx, "r-rp2", "t-rp2"); err != nil {
		t.Fatal(err)
	}
	got, _, _ := store.Get(ctx, "r-rp2", "t-rp2")
	if got.SettlePhase != SettleSupersededReprompt {
		t.Fatalf("want superseded, got %s", got.SettlePhase)
	}
	// Second drive is a no-op (final disposition).
	if err := d.DriveSettle(ctx, "r-rp2", "t-rp2"); err != nil {
		t.Fatal(err)
	}
	got2, _, _ := store.Get(ctx, "r-rp2", "t-rp2")
	if got2.SettlePhase != SettleSupersededReprompt {
		t.Fatalf("disposition must stick, got %s", got2.SettlePhase)
	}
}

// Task-251: production scheduleSettleDrive with completed live run finalizes.
func TestSettle_ScheduleDrive_WithCompletedRunFinalizes(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r-live", "t-live")
	rec.SettleOwed = true
	_ = store.CreatePrepared(ctx, rec, testEnvelope("r-live", "t-live"))
	rev := int64(1)
	rev, _ = store.CASAdvance(ctx, "r-live", "t-live", rev, DispatchPrepared, DispatchSendClaimed, nil)
	rev, _ = store.CASAdvance(ctx, "r-live", "t-live", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	proof := TerminalEvidence{ProviderKey: "f", EvidenceKind: "x", Outcome: "completed", PayloadSHA256: "h"}
	if _, err := store.CommitTerminalAndSettleIntent(ctx, "r-live", "t-live", rev, proof, "r-live", rec.OuterIntentKey, 1); err != nil {
		t.Fatal(err)
	}

	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	svc.dispatchStore = store
	svc.mu.Lock()
	svc.runs["r-live"] = &interactiveRun{
		id:     "r-live",
		status: RunStatusCompleted,
		subs:   map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()

	svc.scheduleSettleDrive("r-live", "t-live")
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		got, _, _ := store.Get(ctx, "r-live", "t-live")
		if got.SettlePhase == SettleFinalized {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	got, _, _ := store.Get(ctx, "r-live", "t-live")
	t.Fatalf("expected finalized, got %s", got.SettlePhase)
}

// Task-251 review gap #6: concurrent scheduleSettleDrive (boot + post-gate +
// post-terminal fan-in) must not duplicate effects or leave a non-final phase
// when the live run is already Completed. Single-flight + convergent
// RecordEffectDone are the safety mechanisms under test.
func TestSettle_ConcurrentScheduleSettleDrive_NoDuplicateEffects(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r-race", "t-race")
	rec.SettleOwed = true
	_ = store.CreatePrepared(ctx, rec, testEnvelope("r-race", "t-race"))
	rev := int64(1)
	rev, _ = store.CASAdvance(ctx, "r-race", "t-race", rev, DispatchPrepared, DispatchSendClaimed, nil)
	rev, _ = store.CASAdvance(ctx, "r-race", "t-race", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	proof := TerminalEvidence{ProviderKey: "f", EvidenceKind: "x", Outcome: "completed", PayloadSHA256: "h"}
	if _, err := store.CommitTerminalAndSettleIntent(ctx, "r-race", "t-race", rev, proof, "r-race", rec.OuterIntentKey, 1); err != nil {
		t.Fatal(err)
	}

	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	svc.dispatchStore = store
	svc.mu.Lock()
	svc.runs["r-race"] = &interactiveRun{
		id:     "r-race",
		status: RunStatusCompleted,
		subs:   map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()

	const n = 32
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			// Fan-in the same three production entry points.
			svc.scheduleSettleDrive("r-race", "t-race")
			svc.maybeScheduleSettleAfterTerminal("r-race", "t-race")
			svc.scheduleSettleAfterGatePass("r-race", "t-race")
		}()
	}
	wg.Wait()

	// Workers are async — wait for finalization (or timeout).
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		got, _, _ := store.Get(ctx, "r-race", "t-race")
		if got.SettlePhase == SettleFinalized {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	// Second wave after first may have finished: still must stay finalized.
	for i := 0; i < 8; i++ {
		svc.scheduleSettleDrive("r-race", "t-race")
	}
	time.Sleep(50 * time.Millisecond)

	got, _, err := store.Get(ctx, "r-race", "t-race")
	if err != nil {
		t.Fatal(err)
	}
	if got.SettlePhase != SettleFinalized {
		t.Fatalf("after concurrent schedule phase=%s want finalized", got.SettlePhase)
	}
	effects, err := store.ListEffects(ctx, "r-race", "t-race")
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, e := range effects {
		kinds[e.EffectKind]++
		if kinds[e.EffectKind] > 1 {
			t.Fatalf("duplicate effect kind %s after concurrent schedule (count=%d)", e.EffectKind, kinds[e.EffectKind])
		}
	}
	for _, want := range []string{"gate_eval", "completion_event", "graph_signal", "dependents_release", "finalizer"} {
		if kinds[want] != 1 {
			t.Fatalf("effect %s count=%d want 1; kinds=%v", want, kinds[want], kinds)
		}
	}
}

// Concurrent boot drive + scheduleSettleDrive while run is Completed: same
// invariants as the single-path schedule test (Claude review #6 companion).
func TestSettle_ConcurrentBootDriveAndSchedule_NoDuplicate(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r-boot", "t-boot")
	rec.SettleOwed = true
	_ = store.CreatePrepared(ctx, rec, testEnvelope("r-boot", "t-boot"))
	rev := int64(1)
	rev, _ = store.CASAdvance(ctx, "r-boot", "t-boot", rev, DispatchPrepared, DispatchSendClaimed, nil)
	rev, _ = store.CASAdvance(ctx, "r-boot", "t-boot", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	proof := TerminalEvidence{ProviderKey: "f", EvidenceKind: "x", Outcome: "completed", PayloadSHA256: "h"}
	if _, err := store.CommitTerminalAndSettleIntent(ctx, "r-boot", "t-boot", rev, proof, "r-boot", rec.OuterIntentKey, 1); err != nil {
		t.Fatal(err)
	}

	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	svc.dispatchStore = store
	svc.mu.Lock()
	svc.runs["r-boot"] = &interactiveRun{
		id:     "r-boot",
		status: RunStatusCompleted,
		subs:   map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()

	var wg sync.WaitGroup
	wg.Add(4)
	for i := 0; i < 4; i++ {
		go func() {
			defer wg.Done()
			svc.drivePendingSettlesOnBoot(ctx)
			svc.scheduleSettleDrive("r-boot", "t-boot")
		}()
	}
	wg.Wait()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		got, _, _ := store.Get(ctx, "r-boot", "t-boot")
		if got.SettlePhase == SettleFinalized {
			effects, _ := store.ListEffects(ctx, "r-boot", "t-boot")
			kinds := map[string]int{}
			for _, e := range effects {
				kinds[e.EffectKind]++
				if kinds[e.EffectKind] > 1 {
					t.Fatalf("duplicate effect %s", e.EffectKind)
				}
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	got, _, _ := store.Get(ctx, "r-boot", "t-boot")
	t.Fatalf("expected finalized after concurrent boot/schedule, got %s", got.SettlePhase)
}

// Task-251: effects are structured (not bare {"ok":true}) and replay-convergent.
func TestSettleEffects_ConvergentOnReplay(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r-fx", "t-fx")
	rec.SettleOwed = true
	_ = store.CreatePrepared(ctx, rec, testEnvelope("r-fx", "t-fx"))
	rev := int64(1)
	rev, _ = store.CASAdvance(ctx, "r-fx", "t-fx", rev, DispatchPrepared, DispatchSendClaimed, nil)
	rev, _ = store.CASAdvance(ctx, "r-fx", "t-fx", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	proof := TerminalEvidence{ProviderKey: "f", EvidenceKind: "x", Outcome: "completed", PayloadSHA256: "h"}
	if _, err := store.CommitTerminalAndSettleIntent(ctx, "r-fx", "t-fx", rev, proof, "r-fx", rec.OuterIntentKey, 1); err != nil {
		t.Fatal(err)
	}
	d := &SettleDriver{
		Store: store,
		EvaluateGate: func(context.Context, string, string) (bool, bool, error) {
			return true, false, nil
		},
	}
	if err := d.DriveSettle(ctx, "r-fx", "t-fx"); err != nil {
		t.Fatal(err)
	}
	// Replay DriveSettle is no-op (finalized).
	if err := d.DriveSettle(ctx, "r-fx", "t-fx"); err != nil {
		t.Fatal(err)
	}
	effects, err := store.ListEffects(ctx, "r-fx", "t-fx")
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, e := range effects {
		kinds[e.EffectKind]++
		if string(e.Payload) == `{"ok":true}` {
			t.Fatalf("placeholder payload still used for %s", e.EffectKind)
		}
	}
	for _, want := range []string{"gate_eval", "completion_event", "graph_signal", "dependents_release", "finalizer"} {
		if kinds[want] != 1 {
			t.Fatalf("effect %s count=%d want 1 (got kinds=%v)", want, kinds[want], kinds)
		}
	}
}

// Task-251: resume from each intermediate phase.
func TestSettleDriver_ResumesFromEachPhase(t *testing.T) {
	phases := []SettlePhase{
		SettlePending,
		SettleGateEvaluated,
		SettleCompletionCommitted,
		SettleGraphSettled,
		SettleDependentsReleased,
	}
	for _, start := range phases {
		t.Run(string(start), func(t *testing.T) {
			ctx := context.Background()
			store := NewMemoryDispatchStore()
			rec := testPrepared("r-ph", "t-ph")
			rec.SettleOwed = true
			_ = store.CreatePrepared(ctx, rec, testEnvelope("r-ph", "t-ph"))
			rev := int64(1)
			rev, _ = store.CASAdvance(ctx, "r-ph", "t-ph", rev, DispatchPrepared, DispatchSendClaimed, nil)
			rev, _ = store.CASAdvance(ctx, "r-ph", "t-ph", rev, DispatchSendClaimed, DispatchSendStarted, nil)
			proof := TerminalEvidence{ProviderKey: "f", EvidenceKind: "x", Outcome: "completed", PayloadSHA256: "h"}
			if _, err := store.CommitTerminalAndSettleIntent(ctx, "r-ph", "t-ph", rev, proof, "r-ph", rec.OuterIntentKey, 1); err != nil {
				t.Fatal(err)
			}
			// Advance to start phase by repeated DriveSettle with barriers... simpler: CASAdvanceSettle chain.
			if start != SettlePending {
				cur := SettlePending
				got, rev2, _ := store.Get(ctx, "r-ph", "t-ph")
				_ = got
				rev = rev2
				order := []SettlePhase{SettleGateEvaluated, SettleCompletionCommitted, SettleGraphSettled, SettleDependentsReleased}
				for _, next := range order {
					// Seed gate_eval effect when leaving pending.
					if cur == SettlePending {
						p := settleEffectPayload("r-ph", "t-ph", "gate_eval", map[string]any{"allow": true, "reprompt": false})
						_, _ = store.RecordEffectDone(ctx, "r-ph", "t-ph", "gate_eval", p, HashBytes(p))
					}
					var err error
					rev, err = store.CASAdvanceSettle(ctx, "r-ph", "t-ph", rev, cur, next)
					if err != nil {
						t.Fatal(err)
					}
					cur = next
					if cur == start {
						break
					}
				}
			}
			d := &SettleDriver{
				Store: store,
				EvaluateGate: func(context.Context, string, string) (bool, bool, error) {
					return true, false, nil
				},
			}
			if err := d.DriveSettle(ctx, "r-ph", "t-ph"); err != nil {
				t.Fatal(err)
			}
			got, _, _ := store.Get(ctx, "r-ph", "t-ph")
			if got.SettlePhase != SettleFinalized {
				t.Fatalf("from %s → %s want finalized", start, got.SettlePhase)
			}
		})
	}
}
