package runner

import (
	"context"
	"testing"
	"time"
)

// ---- R16-P0: durable idempotency keys survive LocalFileSessionStore --------

func TestDurableIdempotencyKeysRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	sess := ProviderSessionState{
		RunID:       "child-1",
		ProjectID:   "p",
		ProviderKey: ProviderKeyCodex,
		Status:      RunStatusRunning,
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		IdempotencyKeys: map[string]string{
			"durable-child-1-restart-3": "turn-99",
			"http-ephemeral-key":        "turn-ignore", // should not be required; we store as given
		},
	}
	// Only durable-* are snapshotted from run; direct store write keeps full map.
	if err := store.UpsertProviderSession(context.Background(), sess); err != nil {
		t.Fatal(err)
	}
	store2, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, ok, gerr := store2.GetProviderSession(context.Background(), "child-1")
	if gerr != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, gerr)
	}
	if got.IdempotencyKeys["durable-child-1-restart-3"] != "turn-99" {
		t.Fatalf("idempotency keys = %#v", got.IdempotencyKeys)
	}
}

func TestSessionStateOfSnapshotsOnlyDurableIdempotency(t *testing.T) {
	rs := &interactiveRun{
		id: "r1",
		idempotency: map[string]string{
			"durable-r1-restart-1": "turn-a",
			"client-key":           "turn-b",
		},
	}
	snap := sessionStateOf(rs)
	if snap.IdempotencyKeys["durable-r1-restart-1"] != "turn-a" {
		t.Fatalf("missing durable key: %#v", snap.IdempotencyKeys)
	}
	if _, ok := snap.IdempotencyKeys["client-key"]; ok {
		t.Fatal("non-durable client key must not be snapshotted")
	}
}

func TestReconstructRestoresDurableIdempotency(t *testing.T) {
	svc, _ := newTestServer(t)
	st := ProviderSessionState{
		RunID:       "run-x",
		ProjectID:   "p",
		ProviderKey: ProviderKeyCodex,
		Status:      RunStatusRunning,
		StartedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		IdempotencyKeys: map[string]string{
			"durable-run-x-restart-2": "turn-prev",
		},
	}
	rs, aerr := svc.reconstructRun(st)
	if aerr != nil || rs == nil {
		t.Fatalf("reconstruct err=%v", aerr)
	}
	if rs.idempotency["durable-run-x-restart-2"] != "turn-prev" {
		t.Fatalf("reconstruct idempotency = %#v", rs.idempotency)
	}
	// Register reconstructed run so startTurn can short-circuit on durable key.
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	svc.mu.Unlock()
	tid, apiErr := svc.startTurn(rs.id, TurnInput{StepID: "s1", Prompt: "x"}, "", "durable-run-x-restart-2")
	if apiErr != nil {
		t.Fatalf("startTurn: %v", apiErr)
	}
	if tid != "turn-prev" {
		t.Fatalf("idempotency replay turnID=%q, want turn-prev", tid)
	}
}

// ---- R16-P1: per-dir marker secrets do not clobber each other -------------

func TestMarkerSecretPerDirIndependent(t *testing.T) {
	d1 := t.TempDir()
	d2 := t.TempDir()
	InitRunMarkerSecretFromDir(d1)
	mac1 := runMarkerMAC("fcp", "id-a")
	InitRunMarkerSecretFromDir(d2)
	mac2 := runMarkerMAC("fcp", "id-a")
	// R17: second Init activates d2 for mint — MAC may differ from d1.
	// Both must still verify (multi-secret verify path).
	if !verifyRunMarkerMAC("fcp", "id-a", mac1) {
		t.Fatal("verify must accept d1-minted MAC after d2 init")
	}
	if !verifyRunMarkerMAC("fcp", "id-a", mac2) {
		t.Fatal("verify must accept d2-minted MAC")
	}
	// Re-activate d1 and mint should match mac1 again.
	InitRunMarkerSecretFromDir(d1)
	if runMarkerMAC("fcp", "id-a") != mac1 {
		t.Fatal("re-activating d1 must mint with d1 secret again")
	}
}

// ---- R16-P0: Supabase payload includes pending_restart_gen (shape) ---------
// Covered by store field presence compile + upsert marshal via integration
// tests elsewhere; gen field is on ProviderSessionState.

// ---- R16-P1: settle fail stamps blocked into third snap --------------------

func TestMarkPendingSettleThirdPersistIncludesBlocked(t *testing.T) {
	svc, _ := newTestServer(t)
	run, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	failStore := &countingFailUpsertStore{fakeWorkflowStore: newFakeWorkflowStore()}
	svc.workflowStore = failStore
	svc.mu.Lock()
	rs := svc.runs[run.RunID]
	rs.currentTurnID = "t1"
	ok := svc.markPendingFlowGateSettleLocked(rs, "msg", time.Now().UTC().Format(time.RFC3339Nano))
	kind := rs.intentBlockedKind
	notDurable := rs.gateCheckpointNotDurable
	svc.mu.Unlock()
	if ok {
		t.Fatal("expected markPending to return false when all persists fail")
	}
	if kind != "gate_settle_checkpoint" {
		t.Fatalf("blocked kind=%q", kind)
	}
	if !notDurable {
		t.Fatal("gateCheckpointNotDurable must be set when third persist fails")
	}
	// At least 3 Upsert attempts: first, retry, third with blocked fields.
	if failStore.upserts < 3 {
		t.Fatalf("upserts=%d, want >=3 (two fail + third with blocked)", failStore.upserts)
	}
}

type countingFailUpsertStore struct {
	*fakeWorkflowStore
	upserts int
}

func (c *countingFailUpsertStore) UpsertProviderSession(ctx context.Context, session ProviderSessionState) error {
	c.upserts++
	return errAlwaysFail
}
