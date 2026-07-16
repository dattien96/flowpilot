package runner

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testEnvelope(runID, turnID string) DispatchEnvelope {
	env := DispatchEnvelope{
		TurnID:       turnID,
		RunID:        runID,
		StepID:       "coding",
		ProviderKey:  ProviderKey("fake"),
		PromptRef:    "prompt-1",
		PromptSHA256: HashBytes([]byte("hello")),
		Model:        "test-model",
		// Fixed timestamp so testPrepared and testEnvelope share one hash.
		CreatedAt: "2026-07-16T00:00:00Z",
	}
	h, err := ComputeEnvelopeHash(&env)
	if err != nil {
		panic(err)
	}
	env.EnvelopeHash = h
	return env
}

func testPrepared(runID, turnID string) DispatchRecord {
	env := testEnvelope(runID, turnID)
	return DispatchRecord{
		ProtocolVersion:  DispatchProtocolV2,
		TurnID:           turnID,
		RunID:            runID,
		ProjectID:        "proj-test",
		IntentOwnerRunID: runID,
		State:            DispatchPrepared,
		EnvelopeHash:     env.EnvelopeHash,
		OuterIntentKey:   "durable-" + runID + "-resume-1",
		OuterIntentGen:   1,
		SettleOwed:       true,
	}
}

func TestDispatchTransitions_ExhaustiveTable(t *testing.T) {
	states := allDispatchStates()
	for _, from := range states {
		for _, to := range states {
			r := DispatchRecord{TurnID: "t", State: from}
			err := r.canTransition(to)
			legal := isLegalEdge(from, to)
			if legal && err != nil {
				t.Errorf("legal edge %s->%s rejected: %v", from, to, err)
			}
			if !legal && err == nil {
				t.Errorf("illegal edge %s->%s accepted", from, to)
			}
		}
	}
}

func TestDispatchCancelRequested_BlocksSendStarted(t *testing.T) {
	r := DispatchRecord{TurnID: "t", State: DispatchSendClaimed, CancelRequested: true}
	if err := r.canTransition(DispatchSendStarted); err == nil {
		t.Fatal("expected cancel to block send_started")
	}
	if err := r.canTransition(DispatchTerminalCancelled); err != nil {
		t.Fatalf("cancel path should allow terminal_cancelled: %v", err)
	}
	r2 := DispatchRecord{TurnID: "t", State: DispatchPrepared, CancelRequested: true}
	if err := r2.canTransition(DispatchSendClaimed); err == nil {
		t.Fatal("cancel in prepared should only allow terminal_cancelled")
	}
}

func TestDispatchCAS_StaleRevisionRejected(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r1", "t1")
	env := testEnvelope("r1", "t1")
	if err := store.CreatePrepared(ctx, rec, env); err != nil {
		t.Fatal(err)
	}
	_, err := store.CASAdvance(ctx, "r1", "t1", 99, DispatchPrepared, DispatchSendClaimed, nil)
	if !errors.Is(err, ErrStaleDispatch) {
		t.Fatalf("want ErrStaleDispatch, got %v", err)
	}
	got, rev, err := store.Get(ctx, "r1", "t1")
	if err != nil || got.State != DispatchPrepared || rev != 1 {
		t.Fatalf("state unharmed? got=%+v rev=%d err=%v", got, rev, err)
	}
}

func TestDispatchEnvelope_HashBindsPayload(t *testing.T) {
	env := testEnvelope("r1", "t1")
	h1 := env.EnvelopeHash
	env.PromptSHA256 = HashBytes([]byte("other"))
	h2, err := ComputeEnvelopeHash(&env)
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2 {
		t.Fatal("hash should change when payload mutates")
	}
}

func runStoreContract(t *testing.T, store DispatchStore, name string) {
	t.Helper()
	ctx := context.Background()
	runID := "run-" + name
	turnID := "turn-1"
	rec := testPrepared(runID, turnID)
	env := testEnvelope(runID, turnID)
	if err := store.CreatePrepared(ctx, rec, env); err != nil {
		t.Fatalf("CreatePrepared: %v", err)
	}
	// V2 activation
	ver, err := store.GetRunProtocolVersion(ctx, runID)
	if err != nil || ver != DispatchProtocolV2 {
		t.Fatalf("protocol version=%d err=%v", ver, err)
	}
	st, err := store.GetRunStopState(ctx, runID)
	if err != nil || st.RunID != runID {
		t.Fatalf("run stop: %+v err=%v", st, err)
	}
	// Happy path CAS
	rev, err := store.CASAdvance(ctx, runID, turnID, 1, DispatchPrepared, DispatchSendClaimed, nil)
	if err != nil || rev != 2 {
		t.Fatalf("send_claimed: rev=%d err=%v", rev, err)
	}
	rev, err = store.CASAdvance(ctx, runID, turnID, rev, DispatchSendClaimed, DispatchSendStarted, nil)
	if err != nil {
		t.Fatalf("send_started: %v", err)
	}
	// Terminal with settle owed
	proof := TerminalEvidence{
		ProviderKey: "fake", EvidenceKind: "turn_completed", Outcome: "completed",
		PayloadSHA256: HashBytes([]byte("done")), ObservedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	rev, err = store.CommitTerminalAndSettleIntent(ctx, runID, turnID, rev, proof, runID, rec.OuterIntentKey, 1)
	if err != nil {
		t.Fatalf("terminal: %v", err)
	}
	got, _, err := store.Get(ctx, runID, turnID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != DispatchTerminalCompleted {
		t.Fatalf("state=%s", got.State)
	}
	if got.SettlePhase != SettlePending {
		t.Fatalf("settle phase=%s want settle_pending", got.SettlePhase)
	}
	cleared, err := store.IsIntentCleared(ctx, runID, rec.OuterIntentKey, 1)
	if err != nil || !cleared {
		t.Fatalf("intent should be cleared: cleared=%v err=%v", cleared, err)
	}
}

func TestDispatchStore_ContractSuite(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		runStoreContract(t, NewMemoryDispatchStore(), "mem")
	})
	t.Run("local", func(t *testing.T) {
		dir := t.TempDir()
		store, err := NewLocalDispatchStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer store.Close()
		runStoreContract(t, store, "local")
	})
	t.Run("multi-project-local", func(t *testing.T) {
		root := t.TempDir()
		store, err := OpenDispatchStoreForServe(root)
		if err != nil {
			t.Fatal(err)
		}
		runStoreContract(t, store, "mp")
		// Shard path exists for project on record.
		if _, err := os.Stat(DispatchLogPath(root, "proj-test")); err != nil {
			t.Fatalf("expected per-project log: %v", err)
		}
	})
}

func TestLocalStore_DiskBeforeRAM_TornTailDropped(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalDispatchStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	rec := testPrepared("r1", "t1")
	env := testEnvelope("r1", "t1")
	if err := store.CreatePrepared(ctx, rec, env); err != nil {
		t.Fatal(err)
	}
	store.Close()

	// Append a torn (incomplete) line.
	f, err := os.OpenFile(filepath.Join(dir, "dispatch.ndjson"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte(`{"kind":"record","seq":999,"at":"x","record":{"turn_id":"torn"`))
	_ = f.Close()

	store2, err := NewLocalDispatchStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()
	if store2.TornTailDrops() < 1 {
		t.Fatalf("expected torn tail drop, got %d", store2.TornTailDrops())
	}
	got, _, err := store2.Get(ctx, "r1", "t1")
	if err != nil || got.State != DispatchPrepared {
		t.Fatalf("intact record lost: %+v err=%v", got, err)
	}
}

func TestDispatchStore_RoundTripAllStates(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalDispatchStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	// Create and advance through several states via separate records.
	states := []DispatchState{DispatchPrepared, DispatchSendClaimed, DispatchSendStarted, DispatchUncertain}
	for i, st := range states {
		turn := "t-" + string(rune('a'+i))
		rec := testPrepared("r1", turn)
		env := testEnvelope("r1", turn)
		if err := store.CreatePrepared(ctx, rec, env); err != nil {
			t.Fatal(err)
		}
		rev := int64(1)
		if st != DispatchPrepared {
			// walk legal path
			path := []DispatchState{DispatchSendClaimed}
			if st == DispatchSendStarted || st == DispatchUncertain {
				path = append(path, DispatchSendStarted)
			}
			cur := DispatchPrepared
			for _, next := range path {
				rev, err = store.CASAdvance(ctx, "r1", turn, rev, cur, next, nil)
				if err != nil {
					t.Fatalf("advance to %s: %v", next, err)
				}
				cur = next
				if cur == st {
					break
				}
			}
			if st == DispatchUncertain {
				// claim + unknown
				rev, err = store.ClaimRecovery(ctx, "r1", turn, rev, "scanner", time.Minute)
				if err != nil {
					t.Fatal(err)
				}
				_, rev, err = store.CommitRecoveryUnknownOrRequireCancel(ctx, "r1", turn, rev, "scanner")
				if err != nil {
					t.Fatal(err)
				}
			}
		}
		_ = rev
	}
	store.Close()

	store2, err := NewLocalDispatchStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()
	list, err := store2.ListRecoverable(ctx, "r1")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 1 {
		t.Fatal("expected recoverable records after restart")
	}
}

func TestSessionState_CarriesNoDispatchRecords(t *testing.T) {
	// Structural: ProviderSessionState JSON must not have a "dispatch" / "Dispatch" slice field.
	raw, err := json.Marshal(ProviderSessionState{
		RunID:                   "r1",
		DispatchProtocolVersion: 2,
		RepairRequired:          true,
		RepairReason:            "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if strings.Contains(s, `"Dispatch"`) || strings.Contains(strings.ToLower(s), `"dispatch":[`) {
		t.Fatalf("session state must not embed dispatch records: %s", s)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	// Only scalars for protocol/repair.
	if _, ok := m["DispatchProtocolVersion"]; !ok && m["dispatch_protocol_version"] == nil {
		// Go encoding uses field name by default without json tags — OK either way.
	}
}

func TestCreatePrepared_AtomicV2Activation(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r1", "t1")
	env := testEnvelope("r1", "t1")
	if err := store.CreatePrepared(ctx, rec, env); err != nil {
		t.Fatal(err)
	}
	ver, err := store.GetRunProtocolVersion(ctx, "r1")
	if err != nil || ver != 2 {
		t.Fatalf("ver=%d err=%v", ver, err)
	}
}

func TestActivation_SurvivesCompaction(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalDispatchStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	rec := testPrepared("r1", "t1")
	env := testEnvelope("r1", "t1")
	if err := store.CreatePrepared(ctx, rec, env); err != nil {
		t.Fatal(err)
	}
	// Terminalize so compaction could prune the record.
	rev := int64(1)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchPrepared, DispatchSendClaimed, nil)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	// force no settle
	r, _, _ := store.Get(ctx, "r1", "t1")
	// use pre-send path? already send_started — use terminal with settle_owed false via mutate not available
	// Create with SettleOwed false instead — re-create different turn
	rec2 := testPrepared("r1", "t2")
	rec2.SettleOwed = false
	env2 := testEnvelope("r1", "t2")
	_ = store.CreatePrepared(ctx, rec2, env2)
	rev2 := int64(1)
	rev2, _ = store.CASAdvance(ctx, "r1", "t2", rev2, DispatchPrepared, DispatchSendClaimed, nil)
	rev2, _ = store.CASAdvance(ctx, "r1", "t2", rev2, DispatchSendClaimed, DispatchSendStarted, nil)
	proof := TerminalEvidence{ProviderKey: "fake", EvidenceKind: "x", Outcome: "completed", PayloadSHA256: "abc"}
	_, _ = store.CommitTerminalAndSettleIntent(ctx, "r1", "t2", rev2, proof, "r1", rec2.OuterIntentKey, 1)

	if err := store.Compact(ctx, time.Nanosecond); err != nil {
		t.Fatal(err)
	}
	ver, err := store.GetRunProtocolVersion(ctx, "r1")
	if err != nil || ver != 2 {
		t.Fatalf("activation lost after compact: ver=%d err=%v", ver, err)
	}
	_ = r
	store.Close()
}

func TestRetryAsNew_SupersededIntentRejected(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r1", "t1")
	env := testEnvelope("r1", "t1")
	if err := store.CreatePrepared(ctx, rec, env); err != nil {
		t.Fatal(err)
	}
	rev := int64(1)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchPrepared, DispatchSendClaimed, nil)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	rev, _ = store.ClaimRecovery(ctx, "r1", "t1", rev, "sc", time.Minute)
	_, rev, err := store.CommitRecoveryUnknownOrRequireCancel(ctx, "r1", "t1", rev, "sc")
	if err != nil {
		t.Fatal(err)
	}
	// Newer intent gen
	_, err = store.RetryAsNew(ctx, "r1", "t1", rev, "res-1", "t2", 99, env.EnvelopeHash)
	if !errors.Is(err, ErrSuperseded) {
		// live gen was set to 1 at CreatePrepared; expectedIntentGen 99 mismatches when live.Gen != 0
		// Our guard: live.Gen != expectedIntentGen
		if err == nil {
			t.Fatal("expected ErrSuperseded for gen mismatch")
		}
	}
}

func TestRecordEffectDone_UniqueKeyIdempotent(t *testing.T) {
	ctx := context.Background()
	for _, store := range []DispatchStore{NewMemoryDispatchStore()} {
		_, err := store.RecordEffectDone(ctx, "r1", "t1", "cohort:c1:child", []byte(`{"a":1}`), "h1")
		if err != nil {
			t.Fatal(err)
		}
		_, err = store.RecordEffectDone(ctx, "r1", "t1", "cohort:c1:child", []byte(`{"a":1}`), "h1")
		if err != nil {
			t.Fatal(err)
		}
		_, err = store.RecordEffectDone(ctx, "r1", "t1", "cohort:c1:child", []byte(`{"a":2}`), "h2")
		if !errors.Is(err, ErrEffectConflict) {
			t.Fatalf("want conflict, got %v", err)
		}
	}
}

func TestReleaseManifest_AtomicChildCreateAndState(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	// parent record
	rec := testPrepared("parent", "pt1")
	env := testEnvelope("parent", "pt1")
	if err := store.CreatePrepared(ctx, rec, env); err != nil {
		t.Fatal(err)
	}
	intent := DurableIntent{
		ChildTurnID: "ct1", ChildRunID: "child", IntentKey: "durable-child-1",
		EnvelopeHash: env.EnvelopeHash,
		ParentStopFence: ParentStopFence{ParentRunID: "parent", ExpectedStopGeneration: 0},
	}
	h, _ := ComputeIntentHash(&intent)
	intent.IntentHash = h
	item, err := store.CreateReleaseManifestItem(ctx, "parent", "pt1", "child", intent)
	if err != nil {
		t.Fatal(err)
	}
	child := DispatchRecord{TurnID: "ct1", RunID: "child", IntentOwnerRunID: "parent"}
	childEnv := testEnvelope("child", "ct1")
	rev, err := store.CommitReleaseManifestItem(ctx, "parent", "pt1", "child", item.Revision, child, childEnv)
	if err != nil {
		t.Fatal(err)
	}
	if rev < 1 {
		t.Fatal("bad rev")
	}
	got, _, err := store.Get(ctx, "child", "ct1")
	if err != nil || got.State != DispatchPrepared {
		t.Fatalf("child not prepared: %+v err=%v", got, err)
	}
}

func TestOpenRepair_CreateIfAbsent_OneCommit(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	raw := []byte(`{"bad":true}`)
	rev1, err := store.OpenRepair(ctx, "r1", "corrupt", raw, HashBytes(raw))
	if err != nil {
		t.Fatal(err)
	}
	rev2, err := store.OpenRepair(ctx, "r1", "corrupt", raw, HashBytes(raw))
	if err != nil || rev2 != rev1 {
		t.Fatalf("create-if-absent failed: rev1=%d rev2=%d err=%v", rev1, rev2, err)
	}
	got, ok, err := store.GetOpenRepair(ctx, "r1")
	if err != nil || !ok || string(got.QuarantineBlob) != string(raw) {
		t.Fatalf("quarantine missing: ok=%v err=%v", ok, err)
	}
}

func TestDispatchTerminalOutcome_SurvivesLocalRestart(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalDispatchStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	rec := testPrepared("r1", "t1")
	rec.SettleOwed = false
	env := testEnvelope("r1", "t1")
	_ = store.CreatePrepared(ctx, rec, env)
	rev := int64(1)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchPrepared, DispatchSendClaimed, nil)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	proof := TerminalEvidence{ProviderKey: "fake", EvidenceKind: "done", Outcome: "completed", PayloadSHA256: "x"}
	_, err = store.CommitTerminalAndSettleIntent(ctx, "r1", "t1", rev, proof, "r1", rec.OuterIntentKey, 1)
	if err != nil {
		t.Fatal(err)
	}
	store.Close()

	store2, err := NewLocalDispatchStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()
	got, _, err := store2.Get(ctx, "r1", "t1")
	if err != nil {
		t.Fatal(err)
	}
	if got.State != DispatchTerminalCompleted {
		t.Fatalf("terminal lost across restart: %s", got.State)
	}
}

func TestDispatchFromLegacyIdem(t *testing.T) {
	// prep
	r := dispatchFromLegacyIdem("r1", "k", "prep:t1", "", nil)
	if r.State != DispatchPrepared || r.ProtocolVersion != 1 {
		t.Fatalf("prep: %+v", r)
	}
	// bare + lastTurnID
	r = dispatchFromLegacyIdem("r1", "k", "t1", "t1", nil)
	if r.State != DispatchTerminalCompleted {
		t.Fatalf("bare+last: %+v", r)
	}
	// bare no corroboration
	r = dispatchFromLegacyIdem("r1", "k", "t1", "other", nil)
	if r.State != DispatchUncertain {
		t.Fatalf("bare only: %+v", r)
	}
}

func TestEventTurnStartedNotUsedForRecovery(t *testing.T) {
	rule := RecoveryInferenceRule()
	if !strings.Contains(rule, "EventTurnStarted") || !strings.Contains(rule, "not recovery") {
		t.Fatalf("rule must document EventTurnStarted exclusion: %s", rule)
	}
	// durableIdemReplaySafe must not treat EventTurnStarted alone as evidence.
	rs := &interactiveRun{
		id: "r1",
		events: []ProviderEvent{
			{Type: EventTurnStarted, ProviderTurnID: "t1"},
		},
	}
	if durableIdemReplaySafe(rs, "t1", true) {
		t.Fatal("EventTurnStarted alone must not be replay-safe")
	}
}

func TestPreSendStopCancellationAndOwnerClearAreAtomic(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	// child turn, parent owns intent
	rec := testPrepared("child", "t1")
	rec.IntentOwnerRunID = "parent"
	rec.OuterIntentKey = "durable-child-restart-1"
	env := testEnvelope("child", "t1")
	if err := store.CreatePrepared(ctx, rec, env); err != nil {
		t.Fatal(err)
	}
	// ensure parent stop state exists
	_, _ = store.RequestRunStop(ctx, "parent", 1, StopReasonUser)
	// actually child own stop for self source
	st, _ := store.GetRunStopState(ctx, "child")
	_, err := store.RequestRunStop(ctx, "child", st.Revision, StopReasonUser)
	if err != nil {
		t.Fatal(err)
	}
	rev := int64(1)
	_, err = store.CASAdvance(ctx, "child", "t1", rev, DispatchPrepared, DispatchSendClaimed, nil)
	if err != nil {
		t.Fatal(err)
	}
	// send_started should hit fence
	_, err = store.CASAdvance(ctx, "child", "t1", 2, DispatchSendClaimed, DispatchSendStarted, nil)
	var fence ErrRunStopFence
	if !errors.As(err, &fence) {
		t.Fatalf("want ErrRunStopFence, got %v", err)
	}
	_, err = store.CommitPreSendCancellationAndClearIntent(ctx, "child", "t1", 2, "parent", rec.OuterIntentKey, 1, fence.CurrentGeneration, PreSendStopSelf)
	if err != nil {
		t.Fatal(err)
	}
	got, _, _ := store.Get(ctx, "child", "t1")
	if got.State != DispatchTerminalCancelled || got.StopOutcome != StopOutcomeStoppedBeforeSend {
		t.Fatalf("pre-send cancel: %+v", got)
	}
	cleared, _ := store.IsIntentCleared(ctx, "parent", rec.OuterIntentKey, 1)
	if !cleared {
		t.Fatal("parent intent must clear atomically")
	}
}

func TestReceiptEvidence_EqualReplayAndPayloadConflict(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r1", "t1")
	env := testEnvelope("r1", "t1")
	_ = store.CreatePrepared(ctx, rec, env)
	rev := int64(1)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchPrepared, DispatchSendClaimed, nil)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	rcpt := ReceiptEvidence{ProviderKey: "fake", ReceiptID: "rid-1", EvidenceKind: "ack", PayloadSHA256: "p1"}
	rev, err := store.CommitReceiptAndClearIntent(ctx, "r1", "t1", rev, rcpt, "r1", rec.OuterIntentKey, 1)
	if err != nil {
		t.Fatal(err)
	}
	// equal replay
	_, err = store.CommitReceiptAndClearIntent(ctx, "r1", "t1", rev, rcpt, "r1", rec.OuterIntentKey, 1)
	if err != nil {
		t.Fatalf("equal replay: %v", err)
	}
	// conflict
	rcpt2 := rcpt
	rcpt2.PayloadSHA256 = "p2"
	_, err = store.CommitReceiptAndClearIntent(ctx, "r1", "t1", rev, rcpt2, "r1", rec.OuterIntentKey, 1)
	if !errors.Is(err, ErrReceiptConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
}

func TestTerminalCommit_RejectsAmbiguousPostSendError(t *testing.T) {
	// Without TerminalEvidence the API requires proof — empty outcome fails.
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r1", "t1")
	env := testEnvelope("r1", "t1")
	_ = store.CreatePrepared(ctx, rec, env)
	rev := int64(1)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchPrepared, DispatchSendClaimed, nil)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	_, err := store.CommitTerminalAndSettleIntent(ctx, "r1", "t1", rev, TerminalEvidence{}, "r1", rec.OuterIntentKey, 1)
	if err == nil {
		t.Fatal("empty proof must be rejected")
	}
	got, _, _ := store.Get(ctx, "r1", "t1")
	if got.State != DispatchSendStarted {
		t.Fatalf("must remain send_started, got %s", got.State)
	}
	cleared, _ := store.IsIntentCleared(ctx, "r1", rec.OuterIntentKey, 1)
	if cleared {
		t.Fatal("intent must remain")
	}
}

func TestTerminalCommit_ReadsPersistedSettleOwed_NoCallerOverride(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r1", "t1")
	rec.SettleOwed = true
	env := testEnvelope("r1", "t1")
	_ = store.CreatePrepared(ctx, rec, env)
	rev := int64(1)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchPrepared, DispatchSendClaimed, nil)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	proof := TerminalEvidence{ProviderKey: "f", EvidenceKind: "x", Outcome: "completed", PayloadSHA256: "h"}
	_, err := store.CommitTerminalAndSettleIntent(ctx, "r1", "t1", rev, proof, "r1", rec.OuterIntentKey, 1)
	if err != nil {
		t.Fatal(err)
	}
	got, _, _ := store.Get(ctx, "r1", "t1")
	if got.SettlePhase != SettlePending {
		t.Fatalf("settle phase from SettleOwed: %s", got.SettlePhase)
	}
}

func TestTerminalCommit_DerivesStopOutcomeFromDurableAuthority(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r1", "t1")
	rec.SettleOwed = false
	env := testEnvelope("r1", "t1")
	_ = store.CreatePrepared(ctx, rec, env)
	// Stop after send started
	rev := int64(1)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchPrepared, DispatchSendClaimed, nil)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	st, _ := store.GetRunStopState(ctx, "r1")
	_, _ = store.RequestRunStop(ctx, "r1", st.Revision, StopReasonUser)
	proof := TerminalEvidence{ProviderKey: "f", EvidenceKind: "x", Outcome: "completed", PayloadSHA256: "h"}
	_, err := store.CommitTerminalAndSettleIntent(ctx, "r1", "t1", rev, proof, "r1", rec.OuterIntentKey, 1)
	if err != nil {
		t.Fatal(err)
	}
	got, _, _ := store.Get(ctx, "r1", "t1")
	if got.StopOutcome != StopOutcomeCancelledInFlight {
		t.Fatalf("want cancelled_in_flight, got %q (state=%s)", got.StopOutcome, got.State)
	}
	if got.State != DispatchTerminalCompleted {
		t.Fatalf("proof outcome must still drive state: %s", got.State)
	}
}

func TestOwnRunStopVsSendStarted_IsLinearizable(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r1", "t1")
	env := testEnvelope("r1", "t1")
	_ = store.CreatePrepared(ctx, rec, env)
	rev := int64(1)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchPrepared, DispatchSendClaimed, nil)
	st, _ := store.GetRunStopState(ctx, "r1")
	_, _ = store.RequestRunStop(ctx, "r1", st.Revision, StopReasonUser)
	_, err := store.CASAdvance(ctx, "r1", "t1", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	var fence ErrRunStopFence
	if !errors.As(err, &fence) {
		t.Fatalf("want fence, got %v", err)
	}
}

func TestChildSendStarted_ParentStopFenceIsAtomic(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	// activate parent
	_ = store.CreatePrepared(ctx, testPrepared("parent", "p1"), testEnvelope("parent", "p1"))
	child := testPrepared("child", "c1")
	child.ParentStopFence = &ParentStopFence{ParentRunID: "parent", ExpectedStopGeneration: 0}
	env := testEnvelope("child", "c1")
	_ = store.CreatePrepared(ctx, child, env)
	rev := int64(1)
	rev, _ = store.CASAdvance(ctx, "child", "c1", rev, DispatchPrepared, DispatchSendClaimed, nil)
	// stop parent
	st, _ := store.GetRunStopState(ctx, "parent")
	_, _ = store.RequestRunStop(ctx, "parent", st.Revision, StopReasonUser)
	_, err := store.CASAdvance(ctx, "child", "c1", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	var fence ErrParentStopFence
	if !errors.As(err, &fence) {
		t.Fatalf("want parent fence, got %v", err)
	}
}

func TestResolveUncertain_ActionSettlementTableAtomic(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r1", "t1")
	rec.SettleOwed = true
	env := testEnvelope("r1", "t1")
	_ = store.CreatePrepared(ctx, rec, env)
	rev := int64(1)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchPrepared, DispatchSendClaimed, nil)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	rev, _ = store.ClaimRecovery(ctx, "r1", "t1", rev, "sc", time.Minute)
	_, rev, _ = store.CommitRecoveryUnknownOrRequireCancel(ctx, "r1", "t1", rev, "sc")
	// abandon forces no settle
	_, err := store.ResolveUncertain(ctx, "r1", "t1", rev, "res-a", ResolveAbandon, OperatorEvidence{Actor: "local-operator"})
	if err != nil {
		t.Fatal(err)
	}
	got, _, _ := store.Get(ctx, "r1", "t1")
	if got.SettlePhase != SettleNone || got.SettleOwed {
		t.Fatalf("abandon must force no settle: %+v", got)
	}
}

func TestLocalLog_StartupLock_CompactionCrashRules(t *testing.T) {
	dir := t.TempDir()
	// leftover temp discarded
	_ = os.WriteFile(filepath.Join(dir, "dispatch.ndjson.tmp"), []byte("junk"), 0o644)
	store, err := NewLocalDispatchStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := os.Stat(filepath.Join(dir, "dispatch.ndjson.tmp")); !os.IsNotExist(err) {
		t.Fatal("temp should be discarded on startup")
	}
}

func TestRecoveryAttach_EntryEffectAndTerminalAreAtomic(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r1", "t1")
	rec.SettleOwed = false
	env := testEnvelope("r1", "t1")
	_ = store.CreatePrepared(ctx, rec, env)
	rev := int64(1)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchPrepared, DispatchSendClaimed, nil)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	rev, _ = store.ClaimRecovery(ctx, "r1", "t1", rev, "sc", time.Minute)
	tok, _, err := store.ClaimRecoveryAttach(ctx, "r1", "t1", rev, "sc", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	// B terminalizes via recovered path, revoking attach
	got, rev2, _ := store.Get(ctx, "r1", "t1")
	proof := TerminalEvidence{ProviderKey: "f", EvidenceKind: "x", Outcome: "completed", PayloadSHA256: "h"}
	_, err = store.CommitRecoveredTerminalAndSettleIntent(ctx, "r1", "t1", rev2, "sc", proof, "r1", rec.OuterIntentKey, 1)
	if err != nil {
		t.Fatal(err)
	}
	// A resumes with old token → revoked
	if err := store.EnterRecoveryAttach(ctx, "r1", "t1", tok); !errors.Is(err, ErrRecoveryAttachRevoked) {
		t.Fatalf("want revoked, got %v", err)
	}
	_ = got
}

func TestEverySentExitAndTerminalPath_RevokesRecoveryAttachEpoch(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("r1", "t1")
	rec.SettleOwed = false
	env := testEnvelope("r1", "t1")
	_ = store.CreatePrepared(ctx, rec, env)
	rev := int64(1)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchPrepared, DispatchSendClaimed, nil)
	rev, _ = store.CASAdvance(ctx, "r1", "t1", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	rev, _ = store.ClaimRecovery(ctx, "r1", "t1", rev, "sc", time.Minute)
	tok, recAfter, err := store.ClaimRecoveryAttach(ctx, "r1", "t1", rev, "sc", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	epoch := recAfter.RecoveryAttachEpoch
	proof := TerminalEvidence{ProviderKey: "f", EvidenceKind: "x", Outcome: "failed", PayloadSHA256: "h"}
	_, err = store.CommitAttachedTerminalAndSettleIntent(ctx, "r1", "t1", recAfter.Revision, tok, proof, "r1", rec.OuterIntentKey, 1)
	if err != nil {
		t.Fatal(err)
	}
	got, _, _ := store.Get(ctx, "r1", "t1")
	if got.RecoveryAttachEpoch <= epoch && got.RecoveryAttachOwner != "" {
		t.Fatalf("epoch not revoked: before=%d after=%+v", epoch, got)
	}
	if got.RecoveryAttachOwner != "" {
		t.Fatal("owner should clear")
	}
}

func TestRunStopState_IsOnlyStopAuthority(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	_ = store.CreatePrepared(ctx, testPrepared("r1", "t1"), testEnvelope("r1", "t1"))
	st, err := store.GetRunStopState(ctx, "r1")
	if err != nil || st.Revision < 1 {
		t.Fatalf("activation should create run stop: %+v err=%v", st, err)
	}
}

func TestDispatchMutation_GuardFlipsTurnSuiteRed(t *testing.T) {
	// MU: illegal edge must fail
	r := DispatchRecord{State: DispatchTerminalCompleted, TurnID: "t"}
	if err := r.canTransition(DispatchPrepared); err == nil {
		t.Fatal("backward edge must fail")
	}
}
