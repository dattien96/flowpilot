package runner

// BUG-499: localFileSessionStore committed the in-memory map BEFORE the
// durable write. A failed append left memory holding a session the disk
// never received (restart silently loses the "persisted" run), and
// DeleteProviderSession dropped the map entry before the rewrite — a
// failed rewrite resurrected the deleted record on next boot. Contract:
// durable operation first, in-memory commit only on success.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func bug499Store(t *testing.T, ws string) *localFileSessionStore {
	t.Helper()
	s, err := NewLocalFileSessionStore(filepath.Join(ws, "state"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	return s
}

func TestBUG499_UpsertWriteFailureLeavesMemoryClean(t *testing.T) {
	ws := t.TempDir()
	s := bug499Store(t, ws)
	// Poison the path: directory where the file belongs → append fails.
	if err := os.Remove(s.filePath); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.Mkdir(s.filePath, 0o755); err != nil {
		t.Fatal(err)
	}
	err := s.UpsertProviderSession(context.Background(), ProviderSessionState{RunID: "run-x", Status: "running"})
	if err == nil {
		t.Fatal("append into a directory must fail")
	}
	if _, found := s.fakeWorkflowStore.sessions["run-x"]; found {
		t.Fatal("failed durable write must not commit to memory — restart would diverge")
	}
}

func TestBUG499_DeleteRewriteFailureLeavesMemoryIntact(t *testing.T) {
	ws := t.TempDir()
	s := bug499Store(t, ws)
	if err := s.UpsertProviderSession(context.Background(), ProviderSessionState{RunID: "run-del", Status: "running"}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Block the rewrite: make the tmp path an existing directory so the
	// create fails mid-delete.
	tmpPath := s.filePath + ".tmp"
	if err := os.Mkdir(tmpPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteProviderSession(context.Background(), "run-del"); err == nil {
		t.Fatal("blocked rewrite must fail")
	}
	if _, found := s.fakeWorkflowStore.sessions["run-del"]; !found {
		t.Fatal("failed durable rewrite must not delete from memory — the record would resurrect on restart while the live process forgot it")
	}
}

func TestBUG499_DeleteOnMissingFileDropsMemory(t *testing.T) {
	ws := t.TempDir()
	s := bug499Store(t, ws)
	s.fakeWorkflowStore.mu.Lock()
	s.fakeWorkflowStore.sessions["run-mem"] = ProviderSessionState{RunID: "run-mem"}
	s.fakeWorkflowStore.mu.Unlock()
	if err := s.DeleteProviderSession(context.Background(), "run-mem"); err != nil {
		t.Fatalf("delete with no file must succeed: %v", err)
	}
	if _, found := s.fakeWorkflowStore.sessions["run-mem"]; found {
		t.Fatal("memory entry must be dropped when there is nothing on disk to resurrect")
	}
}

func TestBUG499_HealthyRoundTripUnchanged(t *testing.T) {
	ws := t.TempDir()
	s := bug499Store(t, ws)
	ctx := context.Background()
	if err := s.UpsertProviderSession(ctx, ProviderSessionState{RunID: "run-a", Status: "running"}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if _, found, err := s.GetProviderSession(ctx, "run-a"); err != nil || !found {
		t.Fatalf("upserted session not visible: found=%v err=%v", found, err)
	}
	if err := s.DeleteProviderSession(ctx, "run-a"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, found := s.fakeWorkflowStore.sessions["run-a"]; found {
		t.Fatal("healthy delete must still drop the memory entry")
	}
}

// Dispatch-store side: commitLine latches the first persist failure so the
// already-diverged memory cannot accept further mutations.
func TestBUG499_DispatchStoreLatchesPersistFailure(t *testing.T) {
	s := newMemoryDispatchStore()
	var calls int
	s.afterCommit = func(dispatchLogLine) error {
		calls++
		return errors.New("disk write failed")
	}
	rec := DispatchRecord{RunID: "run-d", TurnID: "t1"}
	env := DispatchEnvelope{EnvelopeHash: "h1"}
	if err := s.CreatePrepared(context.Background(), rec, env); err == nil {
		t.Fatal("first mutation must surface the persist failure")
	}
	// The record is in memory but never hit disk — a retry sees
	// ErrAlreadyExists unless the store is latched; either way it must NOT
	// succeed as if durable.
	err := s.CreatePrepared(context.Background(), rec, env)
	if err == nil {
		t.Fatal("second mutation on a degraded store must fail")
	}
	if calls != 1 {
		t.Fatalf("degraded store attempted another persist (calls=%d)", calls)
	}
}
