package runner

// BUG-473: the boot worktree GC must prune only proven orphans. A persistence
// read error is currently indistinguishable from "no binding" — a
// ListProviderSessionsByChat or GetProviderSession failure marks a live
// binding unbound and `git worktree remove --force` destroys unmerged work.
// The sweep must consult binding authorities as a three-state verdict
// (bound | proven orphan | unknown) and defer every owner it cannot prove.

import (
	"bytes"
	"context"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/worktree"
)

// bug473Store wraps the real local file store and fails the two binding
// lookups the GC consults, independently.
type bug473Store struct {
	*localFileSessionStore
	chatListErr error
	getErr      error
}

func (s *bug473Store) ListProviderSessionsByChat(ctx context.Context, chatID string) ([]ProviderSessionState, error) {
	if s.chatListErr != nil {
		return nil, s.chatListErr
	}
	return s.localFileSessionStore.ListProviderSessionsByChat(ctx, chatID)
}

func (s *bug473Store) GetProviderSession(ctx context.Context, runID string) (ProviderSessionState, bool, error) {
	if s.getErr != nil {
		return ProviderSessionState{}, false, s.getErr
	}
	return s.localFileSessionStore.GetProviderSession(ctx, runID)
}

// captureGCLog swaps the std log output for the duration of fn and returns
// everything the GC logged while it ran.
func captureGCLog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(old)
	fn()
	return buf.String()
}

// swapWorkflowStore replaces the service's store with a failing wrapper and
// returns it so the test can clear errors.
func swapWorkflowStore(t *testing.T, svc *InteractiveService) *bug473Store {
	t.Helper()
	inner, ok := svc.workflowStore.(*localFileSessionStore)
	if !ok {
		t.Fatalf("workflowStore is %T, want *localFileSessionStore", svc.workflowStore)
	}
	wrapped := &bug473Store{localFileSessionStore: inner}
	svc.workflowStore = wrapped
	return wrapped
}

// A non-resident chat-owned binding whose leg list cannot be read must never
// become GC-eligible: "authority unavailable" is not "proven orphan".
func TestBUG473_ChatBindingSurvivesStoreReadFailure(t *testing.T) {
	repo := initWorktreeRepo(t)
	_, srv := worktreeHTTPServer(t, repo)
	runID := startWorktreeChatRun(t, srv, repo, nil)
	wtPath := runWorktreePath(t, srv, runID)
	if err := os.WriteFile(filepath.Join(wtPath, "unmerged.txt"), []byte("do not lose\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Restart: second service over the same store — the run is NOT resident.
	svc2, _ := worktreeHTTPServer(t, repo)
	wrapped := swapWorkflowStore(t, svc2)
	wrapped.chatListErr = errors.New("injected: sessions store unavailable")
	wrapped.getErr = errors.New("injected: sessions store unavailable")

	out := captureGCLog(t, func() { svc2.sweepOrphanedWorktrees(context.Background()) })

	if _, err := os.Stat(filepath.Join(wtPath, "unmerged.txt")); err != nil {
		t.Fatalf("GC pruned a worktree it could not verify: %v\nGC log:\n%s", err, out)
	}
	if _, err := os.Stat(worktree.BaseSidecar(repo, filepath.Base(wtPath), "")); err != nil {
		t.Fatalf("GC removed the base sidecar of an unverifiable worktree: %v", err)
	}
	owner := filepath.Base(wtPath)
	if strings.Contains(out, "pruned orphan worktree owner="+owner) {
		t.Fatalf("GC logged a successful prune for an unverifiable owner:\n%s", out)
	}
	if !strings.Contains(out, owner) {
		t.Fatalf("GC silently skipped an unverifiable owner — expected a deferred diagnostic:\n%s", out)
	}
}

// A flow-owned binding (owner = runId) whose session record cannot be read
// must likewise survive the sweep.
func TestBUG473_FlowBindingSurvivesStoreReadFailure(t *testing.T) {
	repo := initWorktreeRepo(t)
	_, srv := worktreeHTTPServer(t, repo)
	runID := startWorktreeFlowRun(t, srv, repo)
	wtPath := runWorktreePath(t, srv, runID)
	if err := os.WriteFile(filepath.Join(wtPath, "unmerged.txt"), []byte("do not lose\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc2, _ := worktreeHTTPServer(t, repo)
	wrapped := swapWorkflowStore(t, svc2)
	wrapped.getErr = errors.New("injected: session record unreadable")

	out := captureGCLog(t, func() { svc2.sweepOrphanedWorktrees(context.Background()) })

	if _, err := os.Stat(filepath.Join(wtPath, "unmerged.txt")); err != nil {
		t.Fatalf("GC pruned a flow-owned worktree it could not verify: %v\nGC log:\n%s", err, out)
	}
	if strings.Contains(out, "pruned orphan worktree owner="+runID) {
		t.Fatalf("GC logged a successful prune for an unverifiable owner:\n%s", out)
	}
}

// A persisted binding row matching the owner but with a zero/corrupt
// WorktreeState is ambiguous — GC must treat it as unknown, not unbound.
func TestBUG473_CorruptBindingRowDefersGC(t *testing.T) {
	repo := initWorktreeRepo(t)
	svc, srv := worktreeHTTPServer(t, repo)
	runID := startWorktreeChatRun(t, srv, repo, nil)
	wtPath := runWorktreePath(t, srv, runID)

	svc2, _ := worktreeHTTPServer(t, repo)
	inner, _ := svc2.workflowStore.(*localFileSessionStore)
	row, found, err := inner.GetProviderSession(context.Background(), runID)
	if err != nil || !found {
		t.Fatalf("session row: found=%v err=%v", found, err)
	}
	row.WorktreeState = "" // partial write: owner set, lifecycle state lost
	if err := svc2.workflowStore.(InteractiveStateStore).UpsertProviderSession(context.Background(), row); err != nil {
		t.Fatalf("corrupt upsert: %v", err)
	}
	_ = svc // keep the first service alive so the store stays populated

	svc2.sweepOrphanedWorktrees(context.Background())

	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("GC pruned on a corrupt/zero-state binding row: %v", err)
	}
}

// Once the store is healthy again the same sweep must prune true orphans and
// still preserve the bound worktree — fail-closed is a deferral, not a wedge.
func TestBUG473_SweepRecoversAndPrunesTrueOrphan(t *testing.T) {
	repo := initWorktreeRepo(t)
	_, srv := worktreeHTTPServer(t, repo)
	runID := startWorktreeChatRun(t, srv, repo, nil)
	wtPath := runWorktreePath(t, srv, runID)

	svc2, _ := worktreeHTTPServer(t, repo)
	wrapped := swapWorkflowStore(t, svc2)
	wrapped.chatListErr = errors.New("injected outage")
	wrapped.getErr = errors.New("injected outage")
	svc2.sweepOrphanedWorktrees(context.Background())
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("bound worktree pruned during store outage: %v", err)
	}

	// Store healthy: a real orphan (dir with no binding anywhere) is pruned,
	// the bound worktree survives, and the prune is logged as success.
	wrapped.chatListErr = nil
	wrapped.getErr = nil
	orphan := filepath.Join(repo, ".flowpilot", "worktrees", "ghost-473")
	if err := os.MkdirAll(orphan, 0o755); err != nil {
		t.Fatal(err)
	}
	out := captureGCLog(t, func() { svc2.sweepOrphanedWorktrees(context.Background()) })

	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatalf("true orphan was not pruned after recovery: stat err=%v", err)
	}
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("bound worktree pruned after store recovery: %v", err)
	}
	if !strings.Contains(out, "pruned orphan worktree owner=ghost-473") {
		t.Fatalf("successful prune was not logged:\n%s", out)
	}
}
