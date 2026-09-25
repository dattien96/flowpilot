package runner

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Cluster G — dispatch/durability fixes (BUG-405..409). All tests are
// reproduce-first: each failed by assertion on the clean baseline before the
// production change landed.

// BUG-405: a restarted chat leg must restore legState/legClosedReason/
// switchFromRunID from the persisted session — otherwise the timeline renders
// legState:"" for every reconstructed leg and switch-provider returns
// chat_no_active_leg (live cht_40a2a29ee70c).
func TestBug405_ReconstructRestoresLegFields(t *testing.T) {
	store := newFakeWorkflowStore()
	svc := newInteractiveService(newProviderRegistry(), newInteractiveCatalog(), store)
	st := ProviderSessionState{
		RunID:           "run-leg",
		ProjectID:       "proj",
		Status:          RunStatusCompleted,
		RunKind:         "chat",
		ChatID:          "cht_x",
		LegSeq:          2,
		LegState:        LegStateClosed,
		LegClosedReason: LegClosedReasonProviderSwitch,
		SwitchFromRunID: "run-prev-leg",
		ProviderKey:     ProviderKeyCodex,
	}
	rs, apiErr := svc.reconstructRun(st)
	if apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	if rs.legState != LegStateClosed {
		t.Fatalf("legState = %q, want %q (persisted value lost on restart)", rs.legState, LegStateClosed)
	}
	if rs.legClosedReason != LegClosedReasonProviderSwitch {
		t.Fatalf("legClosedReason = %q, want %q", rs.legClosedReason, LegClosedReasonProviderSwitch)
	}
	if rs.switchFromRunID != "run-prev-leg" {
		t.Fatalf("switchFromRunID = %q, want run-prev-leg", rs.switchFromRunID)
	}
}

// BUG-406: every line in dispatch.ndjson must carry a strictly-unique,
// contiguous seq. commitTerminal wrote the record line with the pre-increment
// s.seq BEFORE appendAuditLocked bumped it — every terminal commit duplicated
// the previous seq and burned the next one on an invisible audit (11/11 live).
func TestBug406_TerminalCommitSeqUniqueAndContiguous(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := NewLocalDispatchStore(filepath.Join(dir, "dispatch"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	for i := 0; i < 3; i++ {
		runID := fmt.Sprintf("r%d", i)
		turnID := fmt.Sprintf("t%d", i)
		if err := store.CreatePrepared(ctx, testPrepared(runID, turnID), testEnvelope(runID, turnID)); err != nil {
			t.Fatalf("CreatePrepared: %v", err)
		}
		if _, err := store.CASAdvance(ctx, runID, turnID, 1, DispatchPrepared, DispatchSendClaimed, nil); err != nil {
			t.Fatalf("CASAdvance claimed: %v", err)
		}
		if _, err := store.CASAdvance(ctx, runID, turnID, 2, DispatchSendClaimed, DispatchSendStarted, nil); err != nil {
			t.Fatalf("CASAdvance started: %v", err)
		}
		proof := TerminalEvidence{
			ProviderKey: "fake", EvidenceKind: "provider_event", Outcome: "completed",
			PayloadSHA256: HashBytes([]byte("done")),
		}
		if _, err := store.CommitTerminalAndSettleIntent(ctx, runID, turnID, 3, proof, runID, "intent-"+turnID, 1); err != nil {
			t.Fatalf("CommitTerminal: %v", err)
		}
	}

	f, err := os.Open(filepath.Join(dir, "dispatch", "dispatch.ndjson"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var seqs []int64
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var ll dispatchLogLine
		if err := json.Unmarshal(sc.Bytes(), &ll); err != nil {
			t.Fatalf("parse line: %v", err)
		}
		seqs = append(seqs, ll.Seq)
	}
	seen := map[int64]bool{}
	for i, s := range seqs {
		if seen[s] {
			t.Fatalf("duplicate seq %d at line %d — commitTerminal wrote record with pre-increment seq", s, i)
		}
		seen[s] = true
		if i > 0 && s != seqs[i-1]+1 {
			t.Fatalf("seq gap: line %d has seq %d after seq %d — audit increment consumed invisibly", i, s, seqs[i-1])
		}
	}
}

// BUG-407: replaying POST repair-resolution with the same resolutionId must
// return the recorded outcome, not HTTP 502 "repair is not open" (contract row
// RR — idempotent by resolutionID).
func TestBug407_RepairResolutionReplayReturnsRecordedOutcome(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	if _, err := store.OpenRepair(ctx, "run-1", "cancel_required: test", nil, ""); err != nil {
		t.Fatalf("OpenRepair: %v", err)
	}
	attemptRev, _, err := store.BeginRepairResolution(ctx, "run-1", 1, "op-repair-1", RepairActionAbandon)
	if err != nil {
		t.Fatalf("BeginRepairResolution: %v", err)
	}
	if _, err := store.CommitRepairResolution(ctx, "run-1", attemptRev, "op-repair-1", RepairResolvedAbandon, "operator abandon"); err != nil {
		t.Fatalf("CommitRepairResolution: %v", err)
	}
	// Replay: same resolutionID → recorded outcome, not ErrRepairNotOpen.
	_, _, err = store.BeginRepairResolution(ctx, "run-1", 1, "op-repair-1", RepairActionAbandon)
	var replay *RepairResolutionReplay
	if !errors.As(err, &replay) {
		t.Fatalf("replay must surface the recorded outcome, got err=%v", err)
	}
	if replay.Outcome != string(RepairResolvedAbandon) {
		t.Fatalf("replay outcome = %q, want %q", replay.Outcome, RepairResolvedAbandon)
	}
	// A DIFFERENT resolutionID on a closed repair is a real conflict, not a replay.
	_, _, err = store.BeginRepairResolution(ctx, "run-1", 1, "op-repair-OTHER", RepairActionAbandon)
	if !errors.Is(err, ErrRepairNotOpen) {
		t.Fatalf("different resolutionID on closed repair = %v, want ErrRepairNotOpen", err)
	}
}

// BUG-407b: ErrRepairNotOpen is a state conflict (409), not a transport 502.
func TestBug407_RepairNotOpenMapsToConflict(t *testing.T) {
	apiErr := dispatchOperatorErr(ErrRepairNotOpen)
	if apiErr == nil {
		t.Fatal("dispatchOperatorErr(ErrRepairNotOpen) returned nil")
	}
	if apiErr.status != 409 {
		t.Fatalf("ErrRepairNotOpen mapped to HTTP %d, want 409 conflict", apiErr.status)
	}
}

// BUG-408: repair-resolution abandon must terminalize the run's stranded
// non-terminal dispatch records — otherwise the boot scanner re-claims the
// record and re-opens a fresh repair at rev=1 forever (live run-1302).
func TestBug408_RepairAbandonTerminalizesStrandedRecords(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	if err := store.CreatePrepared(ctx, testPrepared("run-1", "t1"), testEnvelope("run-1", "t1")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CASAdvance(ctx, "run-1", "t1", 1, DispatchPrepared, DispatchSendClaimed, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CASAdvance(ctx, "run-1", "t1", 2, DispatchSendClaimed, DispatchSendStarted, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.OpenRepair(ctx, "run-1", "cancel_required: send_started after stop", nil, ""); err != nil {
		t.Fatal(err)
	}
	attemptRev, _, err := store.BeginRepairResolution(ctx, "run-1", 1, "op-1", RepairActionAbandon)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitRepairResolution(ctx, "run-1", attemptRev, "op-1", RepairResolvedAbandon, "operator abandon"); err != nil {
		t.Fatal(err)
	}
	rec, _, err := store.Get(ctx, "run-1", "t1")
	if err != nil {
		t.Fatal(err)
	}
	if !rec.State.IsTerminal() {
		t.Fatalf("abandoned repair left record in %s — boot scanner will re-open repair forever", rec.State)
	}
	if rec.State != DispatchTerminalCancelled {
		t.Fatalf("abandoned record state = %s, want terminal_cancelled", rec.State)
	}
}

// BUG-408b: after abandon, the boot scanner must not resurrect the repair at
// rev=1 over the resolved record.
func TestBug408_ScannerDoesNotResurrectResolvedRepair(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	if err := store.CreatePrepared(ctx, testPrepared("run-1", "t1"), testEnvelope("run-1", "t1")); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CASAdvance(ctx, "run-1", "t1", 1, DispatchPrepared, DispatchSendClaimed, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CASAdvance(ctx, "run-1", "t1", 2, DispatchSendClaimed, DispatchSendStarted, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := store.OpenRepair(ctx, "run-1", "cancel_required: send_started after stop", nil, ""); err != nil {
		t.Fatal(err)
	}
	attemptRev, _, _ := store.BeginRepairResolution(ctx, "run-1", 1, "op-1", RepairActionAbandon)
	if _, err := store.CommitRepairResolution(ctx, "run-1", attemptRev, "op-1", RepairResolvedAbandon, "operator abandon"); err != nil {
		t.Fatal(err)
	}
	// Arm the run stop so a still-stranded send_started record classifies as
	// cancel_required — the live run-1302 shape that resurrected the repair.
	st, _ := store.GetRunStopState(ctx, "run-1")
	if _, err := store.RequestRunStop(ctx, "run-1", st.Revision, StopReasonUser); err != nil {
		t.Fatal(err)
	}
	// Simulate the boot scanner re-scanning the run.
	sc := &RecoveryScanner{Store: store, Owner: "test", Lease: 0}
	if err := sc.ScanRun(ctx, "run-1"); err != nil {
		t.Fatalf("ScanRun: %v", err)
	}
	rep, ok, err := store.GetOpenRepair(ctx, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatalf("scanner re-opened a resolved repair at rev=%d — attention item resurrected", rep.RepairRevision)
	}
}

// BUG-409: a configured-but-dead mirror store must not disable built-in flows —
// ResolveBuiltin falls back to the embedded pack on store error (live run-4 /
// run-9 / run-13 silent degradation).
type errFlowStore struct{}

func (errFlowStore) GetByPackFlow(context.Context, string, string) (FlowDefinitionRecord, bool, error) {
	return FlowDefinitionRecord{}, false, fmt.Errorf("dial tcp: lookup demo-ref.supabase.co: no such host")
}
func (errFlowStore) GetByRef(context.Context, string) (FlowDefinitionRecord, bool, error) {
	return FlowDefinitionRecord{}, false, fmt.Errorf("dial tcp: lookup demo-ref.supabase.co: no such host")
}
func (errFlowStore) Upsert(context.Context, FlowDefinitionRecord) (FlowDefinitionRecord, error) {
	return FlowDefinitionRecord{}, fmt.Errorf("store unavailable")
}
func (errFlowStore) ListAll(context.Context) ([]FlowDefinitionRecord, error) {
	return nil, fmt.Errorf("store unavailable")
}

func TestBug409_ResolveBuiltinFallsBackOnStoreError(t *testing.T) {
	r := &FlowDefinitionResolver{store: errFlowStore{}}
	rec, err := r.ResolveBuiltin(context.Background(), "flowpilot-core-flow-pack", "bug-harness")
	if err != nil {
		t.Fatalf("built-in flow must resolve from embedded pack despite dead mirror: %v", err)
	}
	if rec.Definition.ID != "bug-harness" {
		t.Fatalf("resolved wrong flow: %q", rec.Definition.ID)
	}
}

func TestBug409_ResolveBareRefFallsBackOnStoreError(t *testing.T) {
	r := &FlowDefinitionResolver{store: errFlowStore{}}
	rec, err := r.ResolveFlowRef(context.Background(), "bug-harness")
	if err != nil {
		t.Fatalf("bare built-in ref must resolve despite dead mirror: %v", err)
	}
	if rec.Definition.ID != "bug-harness" {
		t.Fatalf("resolved wrong flow: %q", rec.Definition.ID)
	}
}

// BUG-409 converse: a NON-builtin ref still fails closed — the store error
// must surface, never silently degrade to a chat turn or the wrong pack.
func TestBug409_NonBuiltinRefStillErrorsOnDeadStore(t *testing.T) {
	r := &FlowDefinitionResolver{store: errFlowStore{}}
	_, err := r.ResolveFlowRef(context.Background(), "user-pack/custom-flow")
	if err == nil {
		t.Fatal("non-builtin ref with dead store must fail closed")
	}
	if !strings.Contains(err.Error(), "store") && !strings.Contains(err.Error(), "unknown pack") {
		t.Fatalf("error must surface the failure, got %v", err)
	}
}
