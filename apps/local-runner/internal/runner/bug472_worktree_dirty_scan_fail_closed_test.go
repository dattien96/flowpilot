package runner

// BUG-472: keep_branch and discard are destructive resolutions guarded by a
// dirty scan (Uncommitted / Untracked). The guards used to discard the Git
// error, so an inspection failure was read as "clean" and Cleanup ran —
// deleting work the scan never proved was safe to lose. Inspection must fail
// closed: typed retryable error, zero cleanup, zero state transition, and the
// same call must succeed once Git is healthy again.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"flowpilot-runner/internal/worktree"
)

// bug472FaultOps wraps the real worktree.Manager: every method delegates
// except the dirty-scan calls (injected errors) and Cleanup (counted so the
// test can prove the destructive path was never reached).
type bug472FaultOps struct {
	worktree.Manager
	uncommittedErr error
	untrackedErr   error
	cleanupCalls   atomic.Int32
}

func (f *bug472FaultOps) Uncommitted(ctx context.Context, repoDir, ownerID, prefix string) ([]string, error) {
	if f.uncommittedErr != nil {
		return nil, f.uncommittedErr
	}
	return f.Manager.Uncommitted(ctx, repoDir, ownerID, prefix)
}

func (f *bug472FaultOps) Untracked(ctx context.Context, repoDir, ownerID, prefix string) ([]string, error) {
	if f.untrackedErr != nil {
		return nil, f.untrackedErr
	}
	return f.Manager.Untracked(ctx, repoDir, ownerID, prefix)
}

func (f *bug472FaultOps) Cleanup(ctx context.Context, repoDir, ownerID, prefix string, keepBranch bool) error {
	f.cleanupCalls.Add(1)
	return f.Manager.Cleanup(ctx, repoDir, ownerID, prefix, keepBranch)
}

// keep_branch drops the worktree but keeps only the branch pointer — an
// Uncommitted failure means the scan cannot prove nothing would be lost, so
// the resolve must 503 worktree_inspection_failed and touch nothing.
func TestE2EWorktree_KeepBranchInspectionFailureFailsClosed(t *testing.T) {
	repo := initWorktreeRepo(t)
	svc, srv := worktreeHTTPServer(t, repo)
	runID := startWorktreeChatRun(t, srv, repo, nil)
	wtPath := runWorktreePath(t, srv, runID)

	// Staged + unstaged + untracked content that must survive the failure.
	if err := os.WriteFile(filepath.Join(wtPath, "untracked.txt"), []byte("keep me\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtPath, "seed.txt"), []byte("modified\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	markRunTerminal(t, svc, runID)

	ops := &bug472FaultOps{uncommittedErr: errors.New("injected: git status unavailable")}
	svc.wtOps = ops

	status, raw := resolveWorktreeConfirmHTTP(t, srv, runID, "keep_branch", true)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("inspection failure must fail closed: status=%d body=%s", status, raw)
	}
	var e struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatalf("decode error body: %v (%s)", err, raw)
	}
	if e.Error.Code != "worktree_inspection_failed" {
		t.Fatalf("error code=%q want worktree_inspection_failed (%s)", e.Error.Code, raw)
	}
	if ops.cleanupCalls.Load() != 0 {
		t.Fatal("Cleanup ran despite inspection failure — destructive path must not execute")
	}
	if _, err := os.Stat(filepath.Join(wtPath, "untracked.txt")); err != nil {
		t.Fatalf("uncommitted work destroyed by failed inspection: %v", err)
	}
	if st := runWorktreeState(t, srv, runID); st == "kept_branch" || st == "discarded" || st == "merged" {
		t.Fatalf("state transitioned to %q after failed inspection", st)
	}

	// The error is retryable: once Git is healthy the same call resolves.
	ops.uncommittedErr = nil
	status, raw = resolveWorktreeConfirmHTTP(t, srv, runID, "keep_branch", true)
	if status != http.StatusOK {
		t.Fatalf("retry after recovery: %d %s", status, raw)
	}
	if st := runWorktreeState(t, srv, runID); st != "kept_branch" {
		t.Fatalf("state=%s want kept_branch", st)
	}
}

// discard removes the worktree entirely; an Untracked failure means the scan
// cannot prove no artifacts would be lost — same fail-closed contract.
func TestE2EWorktree_DiscardInspectionFailureFailsClosed(t *testing.T) {
	repo := initWorktreeRepo(t)
	svc, srv := worktreeHTTPServer(t, repo)
	runID := startWorktreeChatRun(t, srv, repo, nil)
	wtPath := runWorktreePath(t, srv, runID)

	if err := os.WriteFile(filepath.Join(wtPath, "artifact.log"), []byte("precious\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	markRunTerminal(t, svc, runID)

	ops := &bug472FaultOps{untrackedErr: errors.New("injected: git ls-files unavailable")}
	svc.wtOps = ops

	status, raw := resolveWorktreeConfirmHTTP(t, srv, runID, "discard", true)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("inspection failure must fail closed: status=%d body=%s", status, raw)
	}
	var e struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatalf("decode error body: %v (%s)", err, raw)
	}
	if e.Error.Code != "worktree_inspection_failed" {
		t.Fatalf("error code=%q want worktree_inspection_failed (%s)", e.Error.Code, raw)
	}
	if ops.cleanupCalls.Load() != 0 {
		t.Fatal("Cleanup ran despite inspection failure — destructive path must not execute")
	}
	if _, err := os.Stat(filepath.Join(wtPath, "artifact.log")); err != nil {
		t.Fatalf("untracked artifact destroyed by failed inspection: %v", err)
	}
	if st := runWorktreeState(t, srv, runID); st == "discarded" || st == "merged" {
		t.Fatalf("state transitioned to %q after failed inspection", st)
	}

	ops.untrackedErr = nil
	status, raw = resolveWorktreeConfirmHTTP(t, srv, runID, "discard", true)
	if status != http.StatusOK {
		t.Fatalf("retry after recovery: %d %s", status, raw)
	}
	if st := runWorktreeState(t, srv, runID); st != "discarded" {
		t.Fatalf("state=%s want discarded", st)
	}
}
