package runner

// BUG-486: multiProjectDispatchStore swallows per-shard read errors —
// ListRecoverable/ListAttention return partial data with a nil error while a
// shard is unreadable, so ScanAllRecoverable's pass appears successful
// (BUG-484's retry never fires) and the SSE attention path can't distinguish
// "no attention" from "authority unreadable" (BUG-475's resync branch is
// unreachable on the file store).

import (
	"context"
	"errors"
	"os"
	"testing"
)

// bug486SeedShard writes one dispatch record into a project shard so the
// shard demonstrably contains recoverable data.
func bug486SeedShard(t *testing.T, chatsRoot, projectID, runID string) {
	t.Helper()
	dir := DispatchProjectDir(chatsRoot, projectID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	line := `{"kind":"record","seq":1,"record":{"protocol_version":2,"turn_id":"turn-486","run_id":"` + runID + `","project_id":"` + projectID + `","state":"send_started","revision":1,"envelope_hash":"abc","settle_owed":true}}` + "\n"
	if err := os.WriteFile(DispatchLogPath(chatsRoot, projectID), []byte(line), 0o644); err != nil {
		t.Fatal(err)
	}
}

// bug486CorruptShard makes a shard unreadable without permission bits:
// dispatch.ndjson as a directory opens fine but ReadBytes fails on first
// read — deterministic on every platform.
func bug486CorruptShard(t *testing.T, chatsRoot, projectID string) {
	t.Helper()
	p := DispatchLogPath(chatsRoot, projectID)
	_ = os.Remove(p)
	if err := os.Mkdir(p, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestBUG486_ListRecoverableSurfacesShardErrors(t *testing.T) {
	root := t.TempDir()
	bug486SeedShard(t, root, "proj-good", "run-good486")
	bug486SeedShard(t, root, "proj-bad", "run-bad486")
	bug486CorruptShard(t, root, "proj-bad")

	m := newMultiProjectDispatchStore(root)
	recs, err := m.ListRecoverable(context.Background(), "")
	if err == nil {
		t.Fatalf("ListRecoverable returned nil error with an unreadable shard; recoverable records in proj-bad were silently dropped (got %d records)", len(recs))
	}
	// Partial results may accompany the error — the good shard's record must
	// still be present.
	found := false
	for _, r := range recs {
		if r.RunID == "run-good486" {
			found = true
		}
	}
	if len(recs) > 0 && !found {
		t.Fatalf("readable shard's record missing from partial results: %#v", recs)
	}
}

func TestBUG486_ListAttentionSurfacesShardErrors(t *testing.T) {
	root := t.TempDir()
	bug486SeedShard(t, root, "proj-bad2", "run-bad486b")
	bug486CorruptShard(t, root, "proj-bad2")

	m := newMultiProjectDispatchStore(root)
	_, err := m.ListAttention(context.Background())
	if err == nil {
		t.Fatal("ListAttention returned nil error with an unreadable shard — the SSE attention path cannot distinguish 'empty' from 'authority failed'")
	}
}

func TestBUG486_ListRecoverableRunIDSurfacesShardErrors(t *testing.T) {
	root := t.TempDir()
	bug486SeedShard(t, root, "proj-bad3", "run-target486")
	bug486CorruptShard(t, root, "proj-bad3")

	m := newMultiProjectDispatchStore(root)
	recs, err := m.ListRecoverable(context.Background(), "run-target486")
	if err == nil {
		t.Fatalf("ListRecoverable(runID) on an unreadable shard returned nil error — absence is unprovable (got %d records)", len(recs))
	}
}

func TestBUG486_NotFoundStillClean(t *testing.T) {
	root := t.TempDir()
	bug486SeedShard(t, root, "proj-ok", "run-present486")

	m := newMultiProjectDispatchStore(root)
	recs, err := m.ListRecoverable(context.Background(), "run-absent486")
	if err != nil {
		t.Fatalf("absent run on healthy shards must stay a clean not-found, got err=%v", err)
	}
	if len(recs) != 0 {
		t.Fatalf("absent run returned records: %#v", recs)
	}
}

func TestBUG486_ScanAllRecoverableSeesPassFailure(t *testing.T) {
	root := t.TempDir()
	bug486SeedShard(t, root, "proj-bad4", "run-bad486d")
	bug486CorruptShard(t, root, "proj-bad4")

	m := newMultiProjectDispatchStore(root)
	sc := &RecoveryScanner{Store: m}
	if err := sc.ScanAllRecoverable(context.Background()); err == nil {
		t.Fatal("ScanAllRecoverable reported success over an unreadable shard — the BUG-484 retry coordinator has no failure to retry on")
	}
}

func TestBUG486_ForRunNotFoundDistinctFromShardFailure(t *testing.T) {
	root := t.TempDir()
	bug486SeedShard(t, root, "proj-ok5", "run-here486")
	bug486SeedShard(t, root, "proj-bad5", "run-there486")
	bug486CorruptShard(t, root, "proj-bad5")

	m := newMultiProjectDispatchStore(root)
	// Absent run with a corrupt shard: absence is unprovable → error, not ErrNotFound.
	_, err := m.forRun("run-nowhere486")
	if err == nil || errors.Is(err, ErrNotFound) {
		t.Fatalf("forRun over an unreadable shard returned clean not-found (err=%v) — must surface the shard failure", err)
	}
}
