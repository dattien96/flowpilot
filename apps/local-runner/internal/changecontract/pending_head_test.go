package changecontract

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func TestPendingCanonicalStoreStagesUpdate(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	rec := PendingCanonicalRecord{RunID: "run-1", FeatureKey: "calc-core", Head: CanonicalHead{FeatureKey: "calc-core", Status: HeadStatusCurrent}}
	if err := store.Stage(rec); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.GetPending("run-1", "calc-core")
	if err != nil || !ok {
		t.Fatalf("expected a staged pending record, ok=%v err=%v", ok, err)
	}
	if got.Head.Status != HeadStatusCurrent {
		t.Fatalf("Head.Status = %q, want %q", got.Head.Status, HeadStatusCurrent)
	}
}

func TestPendingCanonicalStoreUsesLatestContractVersion(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Two successive stages for the same (run, feature) — e.g. the coder
	// passed the gate twice across a validation retry loop. The SECOND
	// staged value must win, not the first.
	if err := store.Stage(PendingCanonicalRecord{RunID: "run-1", FeatureKey: "calc-core", Head: CanonicalHead{FeatureKey: "calc-core", BehaviorStatement: "v1"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Stage(PendingCanonicalRecord{RunID: "run-1", FeatureKey: "calc-core", Head: CanonicalHead{FeatureKey: "calc-core", BehaviorStatement: "v2"}}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.GetPending("run-1", "calc-core")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if got.Head.BehaviorStatement != "v2" {
		t.Fatalf("BehaviorStatement = %q, want v2 (the latest stage)", got.Head.BehaviorStatement)
	}

	// Re-open fresh: the latest-wins result must be durable, not just an
	// in-memory artifact of the instance that staged it.
	reopened, err := NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got2, ok2, err := reopened.GetPending("run-1", "calc-core")
	if err != nil || !ok2 || got2.Head.BehaviorStatement != "v2" {
		t.Fatalf("after reopen: got=%+v ok=%v err=%v, want BehaviorStatement=v2", got2, ok2, err)
	}
}

func TestPendingCanonicalStoreIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	rec := PendingCanonicalRecord{RunID: "run-1", FeatureKey: "calc-core", Head: CanonicalHead{FeatureKey: "calc-core"}, StagedAt: time.Date(2026, 7, 31, 0, 0, 0, 0, time.UTC)}
	if err := store.Stage(rec); err != nil {
		t.Fatal(err)
	}
	if err := store.Stage(rec); err != nil {
		t.Fatal(err)
	}
	// Verify via a fresh reopen that exactly one line was ever written, not
	// two — a byte-count check, not just a GetPending lookup that would pass
	// identically whether the duplicate write happened or not (CP-55 P-5
	// review finding I-6).
	data, err := os.ReadFile(store.recordsPath)
	if err != nil {
		t.Fatal(err)
	}
	if n := bytes.Count(bytes.TrimSpace(data), []byte("\n")) + 1; n != 1 {
		t.Fatalf("expected exactly 1 record line after a duplicate Stage, got %d:\n%s", n, data)
	}
	reopened, err := NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := reopened.GetPending("run-1", "calc-core")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	_ = got
}

// TestPendingCanonicalStoreRestageAfterAbandonIsPendingAgain is the
// regression test for CP-55 P-5 review finding C-1: isActiveLocked used to
// trust only the latest status event's Status, so once a key was ever
// finalized/abandoned, GetPending/ListPendingForRun reported it as inactive
// forever — even after a brand new Stage for that same (run, feature).
func TestPendingCanonicalStoreRestageAfterAbandonIsPendingAgain(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Stage(PendingCanonicalRecord{RunID: "run-1", FeatureKey: "calc-core", Head: CanonicalHead{FeatureKey: "calc-core", BehaviorStatement: "v1"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendStatus("run-1", "calc-core", PendingCanonicalStatusAbandoned, "flow stopped", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := store.GetPending("run-1", "calc-core"); ok {
		t.Fatal("sanity check: should be abandoned before the re-stage")
	}

	// The run is redriven (a supported follow-up path in this codebase) and
	// the coder passes the gate again for the same feature.
	if err := store.Stage(PendingCanonicalRecord{RunID: "run-1", FeatureKey: "calc-core", Head: CanonicalHead{FeatureKey: "calc-core", BehaviorStatement: "v2"}}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.GetPending("run-1", "calc-core")
	if err != nil || !ok {
		t.Fatalf("expected the re-stage to be pending again, ok=%v err=%v", ok, err)
	}
	if got.Head.BehaviorStatement != "v2" {
		t.Fatalf("BehaviorStatement = %q, want v2 (the re-staged value)", got.Head.BehaviorStatement)
	}

	// Durable across a fresh reopen too — not just an in-memory artifact of
	// the instance that did the re-stage.
	reopened, err := NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	got2, ok2, err := reopened.GetPending("run-1", "calc-core")
	if err != nil || !ok2 || got2.Head.BehaviorStatement != "v2" {
		t.Fatalf("after reopen: got=%+v ok=%v err=%v, want pending v2", got2, ok2, err)
	}
}

// TestPendingCanonicalStoreStageRejectsFeatureKeyMismatch is the regression
// test for CP-55 P-5 review finding I-5: SaveHead derives its target file
// from Head.FeatureKey, not from the record's own FeatureKey — a mismatch
// must be rejected at Stage time rather than silently finalizing under one
// key while writing an unexpected file.
func TestPendingCanonicalStoreStageRejectsFeatureKeyMismatch(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	err = store.Stage(PendingCanonicalRecord{RunID: "run-1", FeatureKey: "calc-core", Head: CanonicalHead{FeatureKey: "other-feature"}})
	if err == nil {
		t.Fatal("expected Stage to reject a record whose FeatureKey does not match Head.FeatureKey")
	}
}

func TestPendingCanonicalStoreListPendingForRunAcrossFeatureKeys(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Stage(PendingCanonicalRecord{RunID: "run-1", FeatureKey: "calc-core", Head: CanonicalHead{FeatureKey: "calc-core"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Stage(PendingCanonicalRecord{RunID: "run-1", FeatureKey: "other-feature", Head: CanonicalHead{FeatureKey: "other-feature"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.Stage(PendingCanonicalRecord{RunID: "run-2", FeatureKey: "calc-core", Head: CanonicalHead{FeatureKey: "calc-core"}}); err != nil {
		t.Fatal(err)
	}
	got, err := store.ListPendingForRun("run-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 pending records for run-1 (across 2 feature keys), got %d", len(got))
	}
}

func TestPendingCanonicalStoreGetPendingFalseAfterFinalized(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Stage(PendingCanonicalRecord{RunID: "run-1", FeatureKey: "calc-core", Head: CanonicalHead{FeatureKey: "calc-core"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendStatus("run-1", "calc-core", PendingCanonicalStatusFinalized, "done", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.GetPending("run-1", "calc-core"); err != nil || ok {
		t.Fatalf("a finalized record must no longer be reported as pending, ok=%v err=%v", ok, err)
	}
	if got, err := store.ListPendingForRun("run-1"); err != nil || len(got) != 0 {
		t.Fatalf("ListPendingForRun after finalize: got=%v err=%v, want empty", got, err)
	}
}

func TestPendingCanonicalStoreGetPendingFalseAfterAbandoned(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Stage(PendingCanonicalRecord{RunID: "run-1", FeatureKey: "calc-core", Head: CanonicalHead{FeatureKey: "calc-core"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendStatus("run-1", "calc-core", PendingCanonicalStatusAbandoned, "flow stopped", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.GetPending("run-1", "calc-core"); err != nil || ok {
		t.Fatalf("an abandoned record must no longer be reported as pending, ok=%v err=%v", ok, err)
	}
}

func TestPendingCanonicalStoreAppendStatusRejectsUnknownKey(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendStatus("no-such-run", "no-such-feature", PendingCanonicalStatusFinalized, "", time.Now().UTC()); err == nil {
		t.Fatal("appending a status for a never-staged (run, feature) must fail")
	}
}

func TestPendingCanonicalStoreAppendStatusIdempotentOnSameTerminalStatus(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Stage(PendingCanonicalRecord{RunID: "run-1", FeatureKey: "calc-core", Head: CanonicalHead{FeatureKey: "calc-core"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendStatus("run-1", "calc-core", PendingCanonicalStatusFinalized, "done", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	// Retrying finalize after a crash must not error the second time.
	if err := store.AppendStatus("run-1", "calc-core", PendingCanonicalStatusFinalized, "done (retry)", time.Now().UTC()); err != nil {
		t.Fatalf("a repeated finalize of an already-finalized record must be idempotent, got error: %v", err)
	}
}

func TestPendingCanonicalStoreCorruptRecordsLineFailsClosed(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Stage(PendingCanonicalRecord{RunID: "run-1", FeatureKey: "calc-core", Head: CanonicalHead{FeatureKey: "calc-core"}}); err != nil {
		t.Fatal(err)
	}
	if err := appendRawLine(store.recordsPath, "{not valid json"); err != nil {
		t.Fatal(err)
	}
	if _, err := NewPendingCanonicalStore(dir); err == nil {
		t.Fatal("a corrupt trailing records line must fail NewPendingCanonicalStore, not be silently skipped")
	}
}

func TestPendingCanonicalStoreCorruptEventsLineFailsClosed(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Stage(PendingCanonicalRecord{RunID: "run-1", FeatureKey: "calc-core", Head: CanonicalHead{FeatureKey: "calc-core"}}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendStatus("run-1", "calc-core", PendingCanonicalStatusFinalized, "", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if err := appendRawLine(store.eventsPath, "{also not valid"); err != nil {
		t.Fatal(err)
	}
	if _, err := NewPendingCanonicalStore(dir); err == nil {
		t.Fatal("a corrupt trailing events line must fail NewPendingCanonicalStore, not be silently skipped")
	}
}

func TestPendingCanonicalStoreConcurrentInstancesDoNotInterleave(t *testing.T) {
	dir := t.TempDir()
	const n = 20
	done := make(chan error, n)
	for i := 0; i < n; i++ {
		go func(i int) {
			store, err := NewPendingCanonicalStore(dir)
			if err != nil {
				done <- err
				return
			}
			done <- store.Stage(PendingCanonicalRecord{
				RunID:      "run-1",
				FeatureKey: "feature-" + string(rune('a'+i)),
				Head:       CanonicalHead{FeatureKey: "feature-" + string(rune('a'+i))},
			})
		}(i)
	}
	for i := 0; i < n; i++ {
		if err := <-done; err != nil {
			t.Fatalf("concurrent Stage failed: %v", err)
		}
	}
	final, err := NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatalf("final reload must not see a torn/interleaved write: %v", err)
	}
	got, err := final.ListPendingForRun("run-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != n {
		t.Fatalf("got %d records, want %d — a torn write dropped or merged lines", len(got), n)
	}
}

// TestSecretStoreSignsAndAcceptsItsOwnStagedRecord is the CP-55 P-8
// Claude-agent review Critical Finding 1 regression test (positive case): a
// record staged through NewPendingCanonicalStoreWithSecret must still be
// readable as pending — signing must not itself break the normal path.
func TestSecretStoreSignsAndAcceptsItsOwnStagedRecord(t *testing.T) {
	dir := t.TempDir()
	secret := []byte("test-marker-secret-32-bytes-long")
	store, err := NewPendingCanonicalStoreWithSecret(dir, secret)
	if err != nil {
		t.Fatal(err)
	}
	rec := PendingCanonicalRecord{RunID: "run-1", FeatureKey: "calc-core", Head: CanonicalHead{FeatureKey: "calc-core", Status: HeadStatusCurrent}}
	if err := store.Stage(rec); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.GetPending("run-1", "calc-core")
	if err != nil || !ok {
		t.Fatalf("expected the signed record to be readable as pending, ok=%v err=%v", ok, err)
	}
	if got.Signature == "" {
		t.Fatal("expected Stage to have set a non-empty Signature when the store was opened with a secret")
	}

	// A fresh store instance against the same secret (simulating a restart)
	// must also accept it.
	reopened, err := NewPendingCanonicalStoreWithSecret(dir, secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := reopened.GetPending("run-1", "calc-core"); err != nil || !ok {
		t.Fatalf("expected the signed record to survive a restart with the same secret, ok=%v err=%v", ok, err)
	}
}

// TestSecretStoreRejectsForgedRecord is the CP-55 P-8 Claude-agent review
// Critical Finding 1 regression test (negative case): a record appended to
// the records file WITHOUT going through Stage (simulating a coder's own
// file-write tool appending a raw NDJSON line to exploit the gate's
// bookkeeping-path scope-drift exemption) must NOT be treated as pending by
// a store opened with a secret — closing the "arbitrary Canonical Head
// forgery" gap the review found.
func TestSecretStoreRejectsForgedRecord(t *testing.T) {
	dir := t.TempDir()
	secret := []byte("test-marker-secret-32-bytes-long")

	// Simulate the forgery: append a record with NO signature at all, for a
	// feature key entirely unrelated to anything legitimately frozen/staged.
	forged := PendingCanonicalRecord{
		RunID:      "run-1",
		FeatureKey: "auth",
		Head:       CanonicalHead{FeatureKey: "auth", BehaviorStatement: "attacker-controlled"},
		StagedAt:   time.Now().UTC(),
		Seq:        1,
	}
	appendRawPendingCanonicalRecordForTest(t, dir, forged)

	store, err := NewPendingCanonicalStoreWithSecret(dir, secret)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.GetPending("run-1", "auth"); err != nil || ok {
		t.Fatalf("expected the forged (unsigned) record to be rejected, ok=%v err=%v", ok, err)
	}
	pending, err := store.ListPendingForRunFresh("run-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected zero active pending records for run-1, got %+v", pending)
	}
}

// TestSecretStoreRejectsRecordSignedWithWrongSecret proves the check is a
// real signature verification, not merely "does a Signature field exist" —
// a record signed with a DIFFERENT secret than the store's own must also be
// rejected, matching the case where an attacker discovered/reused a stale or
// unrelated secret value.
func TestSecretStoreRejectsRecordSignedWithWrongSecret(t *testing.T) {
	dir := t.TempDir()
	wrongSecret := []byte("a-completely-different-secret-32")
	realSecret := []byte("test-marker-secret-32-bytes-long")

	rec := PendingCanonicalRecord{RunID: "run-1", FeatureKey: "auth", Head: CanonicalHead{FeatureKey: "auth"}}
	sig, err := signPendingCanonicalRecord(rec, wrongSecret)
	if err != nil {
		t.Fatal(err)
	}
	rec.Signature = sig
	rec.Seq = 1
	rec.StagedAt = time.Now().UTC()
	appendRawPendingCanonicalRecordForTest(t, dir, rec)

	store, err := NewPendingCanonicalStoreWithSecret(dir, realSecret)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.GetPending("run-1", "auth"); err != nil || ok {
		t.Fatalf("expected a record signed with a different secret to be rejected, ok=%v err=%v", ok, err)
	}
}

// TestUnsecretedStoreIgnoresSignatureEntirely proves the plain constructor
// (used throughout this package's own tests and every CP-55 P-5/P-8 runner
// test that reads a staged record back directly) is completely unaffected —
// no signing on write, no verification on read, matching every pre-existing
// test's expectations exactly.
func TestUnsecretedStoreIgnoresSignatureEntirely(t *testing.T) {
	dir := t.TempDir()
	store, err := NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	rec := PendingCanonicalRecord{RunID: "run-1", FeatureKey: "calc-core", Head: CanonicalHead{FeatureKey: "calc-core"}}
	if err := store.Stage(rec); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.GetPending("run-1", "calc-core")
	if err != nil || !ok {
		t.Fatalf("expected the unsigned record to be readable, ok=%v err=%v", ok, err)
	}
	if got.Signature != "" {
		t.Fatalf("Signature = %q, want empty — the plain constructor must never sign", got.Signature)
	}

	// Even a raw, forged record with no signature is still trusted when the
	// reading store itself has no secret configured — this is the documented,
	// intentional "no-op default," not an oversight.
	forged := PendingCanonicalRecord{RunID: "run-1", FeatureKey: "auth", Head: CanonicalHead{FeatureKey: "auth"}, Seq: 1, StagedAt: time.Now().UTC()}
	appendRawPendingCanonicalRecordForTest(t, dir, forged)
	reopened, err := NewPendingCanonicalStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := reopened.GetPending("run-1", "auth"); err != nil || !ok {
		t.Fatalf("expected an unsecreted store to trust a raw record same as always, ok=%v err=%v", ok, err)
	}
}

// appendRawPendingCanonicalRecordForTest appends rec directly to the
// records NDJSON file, bypassing Stage entirely — simulating a coder's own
// file-write tool forging an entry rather than a legitimate Stage call.
func appendRawPendingCanonicalRecordForTest(t *testing.T, workspace string, rec PendingCanonicalRecord) {
	t.Helper()
	dir := workspace + "/.flowpilot/" + pendingCanonicalStoreDir
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := dir + "/" + pendingCanonicalStoreRecordsFile
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		t.Fatal(err)
	}
}
