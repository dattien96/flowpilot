package runner

// BUG-481: a destructive worktree resolution is a multi-effect transaction —
// repo effect (apply_patch), filesystem cleanup, and durable state
// finalization. The pre-fix code ignored cleanup errors and persisted state
// best-effort, so the API could report merged/kept_branch/discarded while
// the filesystem and session record still described a live binding. The fix
// phases the resolution durably (resolution_requested →
// repository_effect_committed → cleanup_committed → finalized) so every
// boundary is replayable and success is only reported after durable
// finalization.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/worktree"
)

// bug481FaultOps injects a Cleanup failure while delegating everything else
// to the real manager.
type bug481FaultOps struct {
	worktree.Manager
	cleanupErr error
}

func (f *bug481FaultOps) Cleanup(ctx context.Context, repoDir, ownerID, prefix string, keepBranch bool) error {
	if f.cleanupErr != nil {
		return f.cleanupErr
	}
	return f.Manager.Cleanup(ctx, repoDir, ownerID, prefix, keepBranch)
}

// bug481Store fails UpsertProviderSession exactly when the row being written
// carries the configured marker — failWhenState matches the terminal
// WorktreeState a finalize write would carry, failWhenPhase matches the
// resolution phase a mid-flight durable write would carry.
type bug481Store struct {
	*localFileSessionStore
	failWhenState string
	failWhenPhase string
}

func (s *bug481Store) UpsertProviderSession(ctx context.Context, session ProviderSessionState) error {
	if s.failWhenState != "" && session.WorktreeState == s.failWhenState {
		return errors.New("injected: session write failed at finalize")
	}
	if s.failWhenPhase != "" && session.WorktreeResolutionPhase == s.failWhenPhase {
		return errors.New("injected: session write failed at phase " + s.failWhenPhase)
	}
	return s.localFileSessionStore.UpsertProviderSession(ctx, session)
}

func swapWorkflowStore481(t *testing.T, svc *InteractiveService) *bug481Store {
	t.Helper()
	inner, ok := svc.workflowStore.(*localFileSessionStore)
	if !ok {
		t.Fatalf("workflowStore is %T, want *localFileSessionStore", svc.workflowStore)
	}
	wrapped := &bug481Store{localFileSessionStore: inner}
	svc.workflowStore = wrapped
	return wrapped
}

func resolutionErrPayload(t *testing.T, raw []byte) (code string, out map[string]any) {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode body: %v (%s)", err, raw)
	}
	_ = json.Unmarshal(raw, &out)
	code = body.Error.Code
	if code == "" {
		if c, ok := out["code"].(string); ok {
			code = c
		}
	}
	return code, out
}

// Cleanup errors in every destructive mode must surface as a retryable
// failure — never as a terminal success.
func TestBUG481_CleanupFailureDoesNotReportSuccess(t *testing.T) {
	for _, mode := range []string{"keep_branch", "discard", "apply_patch"} {
		t.Run(mode, func(t *testing.T) {
			repo := initWorktreeRepo(t)
			svc, srv := worktreeHTTPServer(t, repo)
			var runID string
			if mode == "apply_patch" {
				runID = startWorktreeFlowRun(t, srv, repo)
			} else {
				runID = startWorktreeChatRun(t, srv, repo, nil)
			}
			wtPath := runWorktreePath(t, srv, runID)
			if err := os.WriteFile(filepath.Join(wtPath, "out.txt"), []byte("work\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			markRunTerminal(t, svc, runID)

			svc.wtOps = &bug481FaultOps{cleanupErr: errors.New("injected: worktree remove failed")}
			status, raw := resolveWorktreeConfirmHTTP(t, srv, runID, mode, true)
			if status != http.StatusServiceUnavailable {
				t.Fatalf("%s: cleanup failure must be a retryable error, got %d %s", mode, status, raw)
			}
			code, out := resolutionErrPayload(t, raw)
			if code != "worktree_resolution_failed" {
				t.Fatalf("%s: error code=%q want worktree_resolution_failed (%s)", mode, code, raw)
			}
			if out["resolutionId"] == nil || out["phase"] == nil {
				t.Fatalf("%s: failure payload must carry resolutionId+phase, got %v", mode, out)
			}
			if _, err := os.Stat(wtPath); err != nil {
				t.Fatalf("%s: worktree removed despite cleanup failure: %v", mode, err)
			}
			if st := runWorktreeState(t, srv, runID); st == "merged" || st == "kept_branch" || st == "discarded" {
				t.Fatalf("%s: terminal state %q reported after cleanup failure", mode, st)
			}

			svc.wtOps = nil
			status, raw = resolveWorktreeConfirmHTTP(t, srv, runID, mode, true)
			if status != http.StatusOK {
				t.Fatalf("%s: retry after recovery: %d %s", mode, status, raw)
			}
		})
	}
}

// The split-brain at the heart of BUG-481: cleanup committed for real but
// the terminal state write fails. The API must report a retryable failure
// with the durable phase — and a restart must resume at cleanup_committed
// and only run the finalize step.
func TestBUG481_FinalizePersistFailureResumesAfterRestart(t *testing.T) {
	repo := initWorktreeRepo(t)
	svc, srv := worktreeHTTPServer(t, repo)
	runID := startWorktreeChatRun(t, srv, repo, nil)
	wtPath := runWorktreePath(t, srv, runID)
	if err := os.WriteFile(filepath.Join(wtPath, "work.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	markRunTerminal(t, svc, runID)

	wrapped := swapWorkflowStore481(t, svc)
	wrapped.failWhenState = "kept_branch" // fails exactly the finalize write

	status, raw := resolveWorktreeConfirmHTTP(t, srv, runID, "keep_branch", true)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("finalize persist failure must fail: %d %s", status, raw)
	}
	code, out := resolutionErrPayload(t, raw)
	if code != "worktree_resolution_failed" {
		t.Fatalf("error code=%q want worktree_resolution_failed (%s)", code, raw)
	}
	if out["phase"] != "cleanup_committed" {
		t.Fatalf("durable phase=%v want cleanup_committed (%s)", out["phase"], raw)
	}
	// Cleanup really happened — the failure is honest about it.
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Fatalf("worktree dir should be gone after committed cleanup, stat err=%v", err)
	}
	// Durable record must NOT show the terminal state.
	inner := wrapped.localFileSessionStore
	row, found, err := inner.GetProviderSession(context.Background(), runID)
	if err != nil || !found {
		t.Fatalf("session row: found=%v err=%v", found, err)
	}
	if row.WorktreeState == "kept_branch" {
		t.Fatal("durable record shows terminal state after failed finalize — split-brain")
	}
	if row.WorktreeResolutionPhase != "cleanup_committed" {
		t.Fatalf("durable phase=%q want cleanup_committed", row.WorktreeResolutionPhase)
	}
	if logHasEvent(runEventLog(t, srv, runID), "worktree_resolved") {
		t.Fatal("worktree_resolved emitted before durable finalization")
	}

	// Restart: new service over the same store resumes the intent — only the
	// finalize step should run.
	wrapped.failWhenState = ""
	svc2, srv2 := worktreeHTTPServer(t, repo)
	status, raw = resolveWorktreeConfirmHTTP(t, srv2, runID, "keep_branch", true)
	if status != http.StatusOK {
		t.Fatalf("resume after restart: %d %s", status, raw)
	}
	if st := runWorktreeState(t, srv2, runID); st != "kept_branch" {
		t.Fatalf("state=%s want kept_branch", st)
	}
	row2, found2, err := svc2.workflowStore.(SessionHistoryReader).GetProviderSession(context.Background(), runID)
	if err != nil || !found2 {
		t.Fatal("session row missing after resume")
	}
	if row2.WorktreeResolutionPhase != "" || row2.WorktreeResolutionID != "" {
		t.Fatalf("resolution intent not cleared at finalize: %+v", row2)
	}
	_ = svc2
}

// Crash between the repo effect and its durable write: the patch landed on
// main but the phase stayed requested. The retry must detect the landed
// patch (reverse-apply check) instead of re-applying or reporting conflict.
func TestBUG481_ApplyPatchResumeWithLandedRepoEffect(t *testing.T) {
	repo := initWorktreeRepo(t)
	svc, srv := worktreeHTTPServer(t, repo)
	runID := startWorktreeFlowRun(t, srv, repo)
	wtPath := runWorktreePath(t, srv, runID)
	if err := os.WriteFile(filepath.Join(wtPath, "feature.txt"), []byte("isolated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	markRunTerminal(t, svc, runID)

	wrapped := swapWorkflowStore481(t, svc)
	wrapped.failWhenPhase = "repository_effect_committed"

	status, raw := resolveWorktreeHTTP(t, srv, runID, "apply_patch")
	if status != http.StatusServiceUnavailable {
		t.Fatalf("phase write failure must fail: %d %s", status, raw)
	}
	// The patch already landed in the main workspace.
	if got, err := os.ReadFile(filepath.Join(repo, "feature.txt")); err != nil || string(got) != "isolated\n" {
		t.Fatalf("repo effect did not land: %v %q", err, got)
	}
	row, _, _ := wrapped.localFileSessionStore.GetProviderSession(context.Background(), runID)
	if row.WorktreeResolutionPhase != "resolution_requested" {
		t.Fatalf("durable phase=%q want resolution_requested (write failed before commit)", row.WorktreeResolutionPhase)
	}

	wrapped.failWhenPhase = ""
	status, raw = resolveWorktreeHTTP(t, srv, runID, "apply_patch")
	if status != http.StatusOK {
		t.Fatalf("resume must converge, got %d %s (landed patch must not be re-applied or reported as conflict)", status, raw)
	}
	if st := runWorktreeState(t, srv, runID); st != "merged" {
		t.Fatalf("state=%s want merged", st)
	}
	if got, err := os.ReadFile(filepath.Join(repo, "feature.txt")); err != nil || string(got) != "isolated\n" {
		t.Fatalf("feature.txt corrupted by resume: %v %q", err, got)
	}
}

// A different destructive mode while an intent is past the requested phase
// is a conflict — the in-flight resolution owns the binding.
func TestBUG481_ConflictingModeRejected(t *testing.T) {
	repo := initWorktreeRepo(t)
	svc, srv := worktreeHTTPServer(t, repo)
	runID := startWorktreeChatRun(t, srv, repo, nil)
	markRunTerminal(t, svc, runID)

	wrapped := swapWorkflowStore481(t, svc)
	wrapped.failWhenState = "kept_branch"
	status, _ := resolveWorktreeConfirmHTTP(t, srv, runID, "keep_branch", true)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("setup: expected 503 driving to cleanup_committed, got %d", status)
	}

	status, raw := resolveWorktreeConfirmHTTP(t, srv, runID, "discard", true)
	if status != http.StatusConflict {
		t.Fatalf("conflicting mode must 409, got %d %s", status, raw)
	}
	code, out := resolutionErrPayload(t, raw)
	if code != "worktree_resolution_conflict" {
		t.Fatalf("error code=%q want worktree_resolution_conflict (%s)", code, raw)
	}
	if out["inFlightMode"] != "keep_branch" {
		t.Fatalf("conflict payload must name the in-flight mode, got %v", out)
	}

	wrapped.failWhenState = ""
	status, raw = resolveWorktreeConfirmHTTP(t, srv, runID, "keep_branch", true)
	if status != http.StatusOK {
		t.Fatalf("completing the in-flight resolution: %d %s", status, raw)
	}
}

// Duplicate resolutions are idempotent; a different mode on an
// already-resolved binding is rejected.
func TestBUG481_TerminalReplayIdempotent(t *testing.T) {
	repo := initWorktreeRepo(t)
	svc, srv := worktreeHTTPServer(t, repo)
	runID := startWorktreeChatRun(t, srv, repo, nil)
	markRunTerminal(t, svc, runID)

	status, raw := resolveWorktreeConfirmHTTP(t, srv, runID, "keep_branch", true)
	if status != http.StatusOK {
		t.Fatalf("first resolve: %d %s", status, raw)
	}
	status, raw = resolveWorktreeConfirmHTTP(t, srv, runID, "keep_branch", true)
	if status != http.StatusOK {
		t.Fatalf("same-mode replay must be idempotent, got %d %s", status, raw)
	}
	if !strings.Contains(string(raw), "kept_branch") {
		t.Fatalf("replay must report the terminal state: %s", raw)
	}
	status, raw = resolveWorktreeConfirmHTTP(t, srv, runID, "discard", true)
	if status != http.StatusConflict {
		t.Fatalf("discard after kept_branch must 409, got %d %s", status, raw)
	}
	code, _ := resolutionErrPayload(t, raw)
	if code != "worktree_already_resolved" {
		t.Fatalf("error code=%q want worktree_already_resolved (%s)", code, raw)
	}
}

// An intent still at resolution_requested has committed no external effects —
// the operator may switch modes and the new intent replaces it.
func TestBUG481_RequestedIntentAllowsModeChange(t *testing.T) {
	repo := initWorktreeRepo(t)
	svc, srv := worktreeHTTPServer(t, repo)
	runID := startWorktreeChatRun(t, srv, repo, nil)
	markRunTerminal(t, svc, runID)

	svc.wtOps = &bug481FaultOps{cleanupErr: errors.New("injected")}
	status, _ := resolveWorktreeConfirmHTTP(t, srv, runID, "keep_branch", true)
	if status != http.StatusServiceUnavailable {
		t.Fatalf("setup: expected 503, got %d", status)
	}
	svc.wtOps = nil
	status, raw := resolveWorktreeConfirmHTTP(t, srv, runID, "discard", true)
	if status != http.StatusOK {
		t.Fatalf("mode change at requested phase must proceed, got %d %s", status, raw)
	}
	if st := runWorktreeState(t, srv, runID); st != "discarded" {
		t.Fatalf("state=%s want discarded", st)
	}
}
