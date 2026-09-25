package runner

// Cluster Q — dispatch durability ordering (BUG-447, BUG-448, BUG-449).
// Reproduce-first: each test injects an fsync failure at the exact seam where
// the pre-fix code persisted out of order / mutated RAM before disk.

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

var errInjectedCommit = errors.New("injected commit failure")

// armCommitFailure makes a localDispatchStore fail commitLine whenever the
// outgoing line's Kind matches. Disarm by passing nil. Always delegates to
// store.persistLine so re-arming never stacks stale closures.
func armCommitFailure(t *testing.T, store *localDispatchStore, kinds map[string]bool) {
	t.Helper()
	inner := store.memoryDispatchStore
	inner.afterCommit = func(line dispatchLogLine) error {
		if kinds[line.Kind] {
			return errInjectedCommit
		}
		return store.persistLine(line)
	}
}

func readLogSeqs(t *testing.T, dir string) []int64 {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, "dispatch.ndjson"))
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
	return seqs
}

func assertContiguousSeqs(t *testing.T, seqs []int64) {
	t.Helper()
	for i := 1; i < len(seqs); i++ {
		if seqs[i] != seqs[i-1]+1 {
			t.Fatalf("seq gap: line %d has seq %d after seq %d — a failed commit burned a sequence",
				i, seqs[i], seqs[i-1])
		}
	}
}

func driveToSendStarted(t *testing.T, store DispatchStore, runID, turnID string) {
	t.Helper()
	ctx := context.Background()
	if err := store.CreatePrepared(ctx, testPrepared(runID, turnID), testEnvelope(runID, turnID)); err != nil {
		t.Fatalf("CreatePrepared: %v", err)
	}
	if _, err := store.CASAdvance(ctx, runID, turnID, 1, DispatchPrepared, DispatchSendClaimed, nil); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if _, err := store.CASAdvance(ctx, runID, turnID, 2, DispatchSendClaimed, DispatchSendStarted, nil); err != nil {
		t.Fatalf("send_started: %v", err)
	}
}

// BUG-449: a failed terminal fsync must not permanently consume an audit seq —
// the durable log must stay gap-free and the in-memory audit list must not
// retain a phantom entry for an operation that never landed.
func TestBug449_FailedTerminalCommitDoesNotBurnSeq(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := NewLocalDispatchStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	driveToSendStarted(t, store, "r1", "t1")

	// Fail the terminal record commit once.
	armCommitFailure(t, store, map[string]bool{"record": true})
	proof := TerminalEvidence{ProviderKey: "fake", EvidenceKind: "provider_event", Outcome: "completed",
		PayloadSHA256: HashBytes([]byte("done"))}
	if _, err := store.CommitTerminalAndSettleIntent(ctx, "r1", "t1", 3, proof, "r1", "intent-t1", 1); err == nil {
		t.Fatal("terminal commit must surface the injected fsync failure")
	}
	armCommitFailure(t, store, nil)

	// Retry must succeed at the same revision and leave a gap-free log.
	if _, err := store.CommitTerminalAndSettleIntent(ctx, "r1", "t1", 3, proof, "r1", "intent-t1", 1); err != nil {
		t.Fatalf("retry after failed terminal commit: %v", err)
	}
	assertContiguousSeqs(t, readLogSeqs(t, dir))

	// In-memory audit must not contain a phantom entry for the failed attempt.
	entries, err := store.ListAudit(ctx, "r1", "t1")
	if err != nil {
		t.Fatal(err)
	}
	terminalAudits := 0
	for _, e := range entries {
		if e.Kind == "commit_terminal" {
			terminalAudits++
		}
	}
	if terminalAudits != 1 {
		t.Fatalf("commit_terminal audit entries = %d, want 1 — phantom audit survived the failed fsync", terminalAudits)
	}
}

// BUG-448: a receipt commit whose fsync fails must stay safely retryable —
// the in-memory record must not advance to provider_accepted ahead of disk,
// otherwise the retry short-circuits on receiptEqual and the durable receipt
// is never written.
func TestBug448_ReceiptCommitFailureStaysRetryable(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := NewLocalDispatchStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	driveToSendStarted(t, store, "r1", "t1")

	receipt := ReceiptEvidence{ProviderKey: "fake", ReceiptID: "rcpt-1", EvidenceKind: "provider_event",
		PayloadSHA256: HashBytes([]byte("payload"))}

	armCommitFailure(t, store, map[string]bool{"record": true})
	if _, err := store.CommitReceiptAndClearIntent(ctx, "r1", "t1", 3, receipt, "r1", "intent-t1", 1); err == nil {
		t.Fatal("receipt commit must surface the injected fsync failure")
	}
	armCommitFailure(t, store, nil)

	// Retry the way a caller would: re-read the record, retry with the
	// reported revision. It must actually persist the receipt — not no-op on
	// unpersisted RAM state.
	rec0, rev0, err := store.Get(ctx, "r1", "t1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CommitReceiptAndClearIntent(ctx, "r1", "t1", rev0, receipt, "r1", "intent-t1", 1); err != nil {
		t.Fatalf("receipt retry must stay safely retryable, got %v (state=%s)", err, rec0.State)
	}
	assertContiguousSeqs(t, readLogSeqs(t, dir))

	// Restart: the durable record must carry the receipt + accepted state.
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store2, err := NewLocalDispatchStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()
	rec, _, err := store2.Get(ctx, "r1", "t1")
	if err != nil {
		t.Fatal(err)
	}
	if rec.State != DispatchProviderAccepted {
		t.Fatalf("durable state after restart = %s, want provider_accepted (receipt was lost)", rec.State)
	}
	if rec.ReceiptEvidence == nil || rec.ReceiptEvidence.ReceiptID != "rcpt-1" {
		t.Fatalf("durable receipt missing after restart: %+v", rec.ReceiptEvidence)
	}
}

// BUG-447: repair-abandon must not persist "resolved" while affected dispatch
// records remain non-terminal. The resolution is only durable once every
// stranded record has been terminalized — a mid-loop fsync failure leaves the
// repair OPEN and safely retryable, never resolved-over-live-records.
func TestBug447_AbandonNeverResolvedOverLiveRecords(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	store, err := NewLocalDispatchStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Two stranded records — a partial terminalization is the dangerous shape.
	driveToSendStarted(t, store, "run-1", "t1")
	driveToSendStarted(t, store, "run-1", "t2")
	if _, err := store.OpenRepair(ctx, "run-1", "cancel_required: send_started after stop", nil, ""); err != nil {
		t.Fatal(err)
	}
	attemptRev, _, err := store.BeginRepairResolution(ctx, "run-1", 1, "op-1", RepairActionAbandon)
	if err != nil {
		t.Fatal(err)
	}

	// Fail the FIRST record-terminalization commit inside the resolution.
	// Post-fix ordering writes record lines before the repair line, so this
	// aborts before "resolved" is ever persisted.
	armCommitFailure(t, store, map[string]bool{"record": true})
	if _, err := store.CommitRepairResolution(ctx, "run-1", attemptRev, "op-1", RepairResolvedAbandon, "operator abandon"); err == nil {
		t.Fatal("abandon must surface the injected fsync failure")
	}
	armCommitFailure(t, store, nil)

	// Restart: the durable state must be self-consistent — a resolved repair
	// with live records is the forbidden shape.
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store2, err := NewLocalDispatchStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()
	rep, open, err := store2.GetOpenRepair(ctx, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	recs, err := store2.ListRecoverable(ctx, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	live := 0
	for _, r := range recs {
		if !r.State.IsTerminal() {
			live++
		}
	}
	if !open && live > 0 {
		t.Fatalf("repair resolved durably while %d records still live — no retry path can reach them", live)
	}
	if !open {
		t.Fatalf("expected the repair to remain open after a failed abandon (rep=%+v)", rep)
	}
	_ = rep

	// Retry the same resolution — must complete idempotently and terminalize
	// every remaining live record.
	attemptRev2, _, err := store2.BeginRepairResolution(ctx, "run-1", rep.RepairRevision, "op-1", RepairActionAbandon)
	if err != nil {
		t.Fatalf("retry BeginRepairResolution: %v", err)
	}
	if _, err := store2.CommitRepairResolution(ctx, "run-1", attemptRev2, "op-1", RepairResolvedAbandon, "operator abandon"); err != nil {
		t.Fatalf("retry CommitRepairResolution: %v", err)
	}
	for _, turn := range []string{"t1", "t2"} {
		rec, _, err := store2.Get(ctx, "run-1", turn)
		if err != nil {
			t.Fatal(err)
		}
		if rec.State != DispatchTerminalCancelled {
			t.Fatalf("record %s state = %s after retried abandon, want terminal_cancelled", turn, rec.State)
		}
	}
	assertContiguousSeqs(t, readLogSeqs(t, dir))
}
