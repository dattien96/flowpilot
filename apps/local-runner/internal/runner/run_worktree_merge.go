package runner

// Worktree merge-back (CP-71 P-4 / Task-410, SS-23 AC-4/AC-5, SD-27 D-2/D-4c).
//
// Two triggers, one contract: flow runs emit worktree_merge_requested at
// terminal status; chat runs resolve via the user-initiated merge control or
// the delete-time decision gate. All resolutions go through
// POST /client/workflow-runs/{runId}/worktree/resolve — patch-based only,
// never git merge/commit/push in the user's repo (BR-2).

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"flowpilot-runner/internal/worktree"
)

// EventWorktreeMergeRequested carries the merge card payload in Input
// (map[string]any): ownerId, path, branch, slug, diffStats, conflictPaths?,
// patchArtifactRef?. Additive — unknown event types are ignored by old clients.
const EventWorktreeMergeRequested ProviderEventType = "worktree_merge_requested"

// EventWorktreeLost is emitted when a persisted binding fails D-7 validation
// on resume/attach (SS-23 AC-1b, E-2b). The worktree is never recreated silently.
const EventWorktreeLost ProviderEventType = "worktree_lost"

// EventWorktreeResolved records the merge decision outcome for replay.
const EventWorktreeResolved ProviderEventType = "worktree_resolved"

func worktreeTerminal(status RunStatus) bool {
	return status == RunStatusCompleted || status == RunStatusFailed || status == RunStatusCancelled
}

// maybeEmitWorktreeMergeRequest transitions an active worktree binding on a
// terminal non-chat run to merge_pending and emits the card event once
// (SD-27 D-4c flow trigger). Caller holds s.mu.
func (s *InteractiveService) maybeEmitWorktreeMergeRequest(rs *interactiveRun) {
	if rs == nil || rs.worktree == nil || rs.worktree.State != "active" {
		return
	}
	if rs.runKind == "chat" || !worktreeTerminal(rs.status) {
		return
	}
	rs.worktree.State = "merge_pending"
	s.persistRunWorktreeLocked(rs)
	payload := map[string]any{
		"ownerId": rs.worktree.OwnerID,
		"path":    rs.worktree.Path,
		"branch":  rs.worktree.Branch,
		"slug":    rs.worktree.Slug,
	}
	if patch, err := worktree.NewManager().Diff(context.Background(), s.worktreeRepoDirOfRun(rs), rs.worktree.OwnerID, ""); err == nil {
		payload["diffStats"] = worktreeDiffStats(patch)
		if ref, werr := s.writeWorktreePatchArtifact(rs, patch); werr == nil {
			payload["patchArtifactRef"] = ref
		}
	}
	s.emitLocked(rs, ProviderEvent{Type: EventWorktreeMergeRequested, Input: payload})
}

// worktreeRepoDirOfRun resolves the repo dir that owns a run's worktree: the
// worktree's parent repo — its path is <repo>/.flowpilot/worktrees/<owner>, so
// the repo is three levels up. Falls back to the runner workspace.
func (s *InteractiveService) worktreeRepoDirOfRun(rs *interactiveRun) string {
	if rs.worktree != nil && rs.worktree.Path != "" {
		return filepath.Dir(filepath.Dir(filepath.Dir(rs.worktree.Path)))
	}
	if s.runner != nil {
		return strings.TrimSpace(s.runner.workspace)
	}
	return ""
}

func worktreeDiffStats(patch []byte) map[string]int {
	stats := map[string]int{"files": 0, "added": 0, "removed": 0}
	files := map[string]bool{}
	for _, line := range strings.Split(string(patch), "\n") {
		switch {
		case strings.HasPrefix(line, "+++ b/"), strings.HasPrefix(line, "+++ "):
			if !strings.HasPrefix(line, "+++ /dev/null") {
				files[strings.TrimPrefix(strings.TrimPrefix(line, "+++ b/"), "+++ ")] = true
			}
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			stats["added"]++
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			stats["removed"]++
		}
	}
	stats["files"] = len(files)
	return stats
}

// writeWorktreePatchArtifact stores the merge patch next to the base sidecar
// so conflict evidence survives worktree cleanup (SS-23 E-6).
func (s *InteractiveService) writeWorktreePatchArtifact(rs *interactiveRun, patch []byte) (string, error) {
	ref := filepath.Join(worktree.Root(s.worktreeRepoDirOfRun(rs)), rs.worktree.OwnerID+".patch")
	if err := os.WriteFile(ref, patch, 0o644); err != nil {
		return "", err
	}
	return ref, nil
}

// persistRunWorktreeLocked writes the binding state back to the session
// record. Caller holds s.mu. Returns the store error so callers that must
// not report success on persistence failure (BUG-481 resolution phases) can
// fail instead — best-effort callers ignore the return.
func (s *InteractiveService) persistRunWorktreeLocked(rs *interactiveRun) error {
	reader, ok := s.workflowStore.(SessionHistoryReader)
	if !ok {
		return errors.New("session store cannot read bindings")
	}
	st, found, err := reader.GetProviderSession(context.Background(), rs.id)
	if err != nil {
		return err
	}
	if !found {
		return errors.New("session record missing for run")
	}
	worktreeFieldsToSession(&st, rs.worktree)
	return s.persistProviderSession(st)
}

// worktreeOps is the narrow slice of worktree.Manager resolveWorktree uses.
// The production implementation is the real manager; tests substitute a
// fault injector (BUG-472: dirty-scan errors must fail closed, never proceed
// to cleanup).
type worktreeOps interface {
	Create(ctx context.Context, repoDir, ownerID, prefix, baseCommit, slug string) (worktree.Info, error)
	Diff(ctx context.Context, repoDir, ownerID, prefix string) ([]byte, error)
	Uncommitted(ctx context.Context, repoDir, ownerID, prefix string) ([]string, error)
	Untracked(ctx context.Context, repoDir, ownerID, prefix string) ([]string, error)
	ApplyWithOptions(ctx context.Context, repoDir, ownerID, prefix string, opts worktree.ApplyOptions) error
	Cleanup(ctx context.Context, repoDir, ownerID, prefix string, keepBranch bool) error
}

// resolveWorktree handles the merge decision (Task-410 T-2). Modes:
//   - apply_patch: serialized drift-checked apply; conflict → 409 with
//     evidence, state stays merge_pending (retry = same call after fix).
//   - keep_branch: remove worktree, keep fp/<slug> branch.
//   - discard: remove worktree + branch; response lists untracked artifacts
//     that would have been lost (SD-27 Q-2) — pass ?confirm=1 to proceed.
//   - archive / recreate_empty: lost-state resolutions (Task-411 T-2).
func (s *InteractiveService) resolveWorktree(ctx context.Context, runID, mode string, confirm bool) (map[string]any, *apiErr) {
	s.mu.Lock()
	rs := s.runs[runID]
	s.mu.Unlock()
	if rs == nil {
		// Rebuild without holding s.mu: loadPersistedRun → reconstructRun
		// acquires s.mu internally, so calling it under the lock self-deadlocks
		// on resolve-after-restart (live-found bug).
		if rebuilt, err := s.loadPersistedRun(runID); err == nil {
			rs = rebuilt
		}
	}
	s.mu.Lock()
	if rs == nil || rs.worktree == nil {
		s.mu.Unlock()
		return nil, newAPIErr(http.StatusNotFound, "worktree_not_found", "run has no worktree binding")
	}
	b := rs.worktree
	repoDir := s.worktreeRepoDirOfRun(rs)
	s.mu.Unlock()

	mgr := s.wtOps
	if mgr == nil {
		mgr = worktree.NewManager()
	}
	respond := func(state string, extra map[string]any) (map[string]any, *apiErr) {
		out := map[string]any{"runId": runID, "worktreeState": state, "mode": mode}
		for k, v := range extra {
			out[k] = v
		}
		return out, nil
	}

	switch mode {
	case "apply_patch", "keep_branch", "discard":
		return s.resolveDestructiveWorktree(ctx, rs, b, repoDir, mgr, runID, mode, confirm, respond)

	case "archive":
		// Lost-state resolution: keep the run, mark the binding archived —
		// no worktree ops (SS-23 E-2b).
		s.setWorktreeState(rs, "discarded")
		return respond("discarded", map[string]any{"archived": true})

	case "recreate_empty":
		if b.State != "lost" {
			return nil, newAPIErr(http.StatusConflict, "worktree_not_lost", "worktree is not lost")
		}
		// External deletion (`git worktree remove` / rm -rf) leaves the
		// fp/<slug>-<id> branch behind — `worktree add -b` then collides and
		// the lost binding can never recover. priorChangesLost makes the
		// stale tip disposable: drop it before recreating at the base commit.
		if b.Branch != "" {
			_ = exec.Command("git", "-C", repoDir, "branch", "-D", b.Branch).Run()
		}
		info, err := mgr.Create(ctx, repoDir, b.OwnerID, "", b.BaseCommit, b.Slug)
		if err != nil {
			return nil, newAPIErr(http.StatusConflict, "worktree_recreate_failed", err.Error())
		}
		s.mu.Lock()
		rs.worktree.Path = info.Path
		rs.worktree.Branch = info.Branch
		rs.worktree.State = "active"
		rs.workspaceCwd = info.Path
		s.persistRunWorktreeLocked(rs)
		s.mu.Unlock()
		return respond("active", map[string]any{"path": info.Path, "recreated": true, "priorChangesLost": true})

	default:
		return nil, newAPIErr(http.StatusBadRequest, "invalid_mode",
			"mode must be apply_patch|keep_branch|discard|archive|recreate_empty")
	}
}

// Durable resolution phases (BUG-481). Each transition is persisted before
// the next external effect runs, so a kill/restart resumes only the missing
// idempotent effects; the intent row is cleared at finalize.
const (
	wtResRequested        = "resolution_requested"
	wtResRepoCommitted    = "repository_effect_committed"
	wtResCleanupCommitted = "cleanup_committed"
)

// resolveDestructiveWorktree runs a destructive merge decision
// (apply_patch|keep_branch|discard) as a durable phased transaction
// (BUG-481): evidence gate → mint intent → repo effect → cleanup → finalize.
// Success is reported only after the terminal state is persisted; every
// earlier failure returns a typed retryable error carrying resolutionId +
// the durable phase. Replays resume at the recorded phase — same-mode calls
// converge, conflicting modes past resolution_requested are rejected, and a
// binding already terminal for this mode answers idempotently.
func (s *InteractiveService) resolveDestructiveWorktree(
	ctx context.Context, rs *interactiveRun, b *worktreeBinding, repoDir string,
	mgr worktreeOps, runID, mode string, confirm bool,
	respond func(string, map[string]any) (map[string]any, *apiErr),
) (map[string]any, *apiErr) {
	terminal := map[string]string{
		"apply_patch": "merged", "keep_branch": "kept_branch", "discard": "discarded",
	}[mode]

	if b.State == terminal {
		return respond(terminal, map[string]any{"replayed": true})
	}
	if b.State == "merged" || b.State == "kept_branch" || b.State == "discarded" {
		return map[string]any{
				"runId": runID, "mode": mode, "worktreeState": b.State,
				"code": "worktree_already_resolved",
			}, newAPIErr(http.StatusConflict, "worktree_already_resolved",
				"worktree already resolved as "+b.State)
	}
	if b.State == "lost" && mode != "discard" {
		msg := "worktree is missing; nothing to apply"
		if mode == "keep_branch" {
			msg = "worktree is missing; nothing to keep"
		}
		return nil, newAPIErr(http.StatusConflict, "worktree_lost", msg)
	}

	fail := func(res *worktreeResolution, msg string) (map[string]any, *apiErr) {
		out := map[string]any{"runId": runID, "mode": mode, "code": "worktree_resolution_failed"}
		if res != nil {
			out["resolutionId"] = res.ID
			out["phase"] = res.Phase
		}
		return out, newAPIErr(http.StatusServiceUnavailable, "worktree_resolution_failed", msg)
	}

	// Snapshot any in-flight intent under the lock.
	s.mu.Lock()
	var res *worktreeResolution
	if rs.worktree != nil && rs.worktree.Resolution != nil {
		c := *rs.worktree.Resolution
		res = &c
	}
	s.mu.Unlock()

	if res != nil && res.Mode != mode {
		if res.Phase != wtResRequested {
			out := map[string]any{
				"runId": runID, "mode": mode, "resolutionId": res.ID,
				"inFlightMode": res.Mode, "phase": res.Phase,
				"code": "worktree_resolution_conflict",
			}
			return out, newAPIErr(http.StatusConflict, "worktree_resolution_conflict",
				fmt.Sprintf("worktree resolution %q is in flight at phase %s; complete or retry it before choosing another mode", res.Mode, res.Phase))
		}
		res = nil // no external effects committed — adopt the new mode below
	}

	// Evidence gate (BUG-472 fail-closed): runs while no effect is committed.
	// It is a precondition check, not an effect — failures leave no intent.
	if res == nil || res.Phase == wtResRequested {
		switch mode {
		case "keep_branch":
			uncommitted, err := mgr.Uncommitted(ctx, repoDir, b.OwnerID, "")
			if err != nil {
				return nil, newAPIErr(http.StatusServiceUnavailable, "worktree_inspection_failed",
					"cannot verify the worktree has no uncommitted work; nothing was removed: "+err.Error())
			}
			if len(uncommitted) > 0 && !confirm {
				return map[string]any{
						"runId": runID, "worktreeState": b.State, "mode": mode,
						"requiresConfirm": true, "uncommitted": uncommitted,
					}, newAPIErr(http.StatusConflict, "worktree_keep_branch_confirm",
						"worktree has uncommitted changes that are not on the branch; resend with confirm")
			}
		case "discard":
			if b.State != "lost" {
				untracked, err := mgr.Untracked(ctx, repoDir, b.OwnerID, "")
				if err != nil {
					return nil, newAPIErr(http.StatusServiceUnavailable, "worktree_inspection_failed",
						"cannot verify the worktree has no untracked artifacts; nothing was removed: "+err.Error())
				}
				if len(untracked) > 0 && !confirm {
					return map[string]any{
							"runId": runID, "worktreeState": b.State, "mode": mode,
							"requiresConfirm": true, "untracked": untracked,
						}, newAPIErr(http.StatusConflict, "worktree_discard_confirm",
							"worktree has untracked artifacts; resend with confirm to discard")
				}
			}
		}
	}

	if res == nil {
		res = &worktreeResolution{
			ID:    fmt.Sprintf("wres-%d", s.idCounter.Add(1)),
			Mode:  mode,
			Phase: wtResRequested,
		}
		if err := s.persistWorktreeResolution(rs, res); err != nil {
			return fail(res, "cannot persist resolution intent: "+err.Error())
		}
	}

	// Phase: repository effect (apply_patch only).
	applied := true
	if mode == "apply_patch" && res.Phase == wtResRequested {
		if patch, derr := mgr.Diff(ctx, repoDir, b.OwnerID, ""); derr == nil {
			applied = len(bytes.TrimSpace(patch)) > 0
		}
		repoCommitted := false
		if err := mgr.ApplyWithOptions(ctx, repoDir, b.OwnerID, "",
			worktree.ApplyOptions{StrictHead: false}); err != nil {
			var conflict *worktree.MergeConflictError
			if errors.As(err, &conflict) {
				if gitApplyCheckReverse(ctx, repoDir, conflict.Patch) {
					// Resume after a crash between the apply and its durable
					// write: the patch already landed on main.
					repoCommitted = true
				} else {
					ref := ""
					if p := conflict.Patch; len(p) > 0 {
						if r, werr := s.writeWorktreePatchArtifact(rs, p); werr == nil {
							ref = r
						}
					}
					s.mu.Lock()
					rs.worktree.State = "merge_pending"
					_ = s.persistRunWorktreeLocked(rs)
					s.emitLocked(rs, ProviderEvent{Type: EventWorktreeMergeRequested, Input: map[string]any{
						"ownerId": b.OwnerID, "path": b.Path, "branch": b.Branch, "slug": b.Slug,
						"conflictPaths": conflict.ConflictPaths, "patchArtifactRef": ref,
						"conflict": true,
					}})
					s.mu.Unlock()
					return map[string]any{
						"runId": runID, "worktreeState": "merge_pending", "mode": mode,
						"conflict": true, "conflictPaths": conflict.ConflictPaths,
						"patchArtifactRef": ref, "reason": conflict.Reason,
						"resolutionId": res.ID,
					}, newAPIErr(http.StatusConflict, "worktree_merge_conflict", conflict.Error())
				}
			} else {
				// Merge-engine failure (non-conflict): keep the established
				// 500 worktree_merge_failed contract. The intent is durably
				// recorded at resolution_requested so a retry still resumes
				// the transaction instead of starting a divergent one.
				return nil, newAPIErr(http.StatusInternalServerError, "worktree_merge_failed", err.Error())
			}
		} else {
			repoCommitted = true
		}
		if repoCommitted {
			prev := res.Phase
			res.Phase = wtResRepoCommitted
			if err := s.persistWorktreeResolution(rs, res); err != nil {
				res.Phase = prev
				return fail(res, "cannot persist resolution phase: "+err.Error())
			}
		}
	}

	// Phase: cleanup (dir + sidecar + branch per mode), verified.
	if res.Phase == wtResRequested || res.Phase == wtResRepoCommitted {
		if err := worktreeCleanupEffect(ctx, mgr, repoDir, b, mode); err != nil {
			return fail(res, "cleanup effect failed: "+err.Error())
		}
		prev := res.Phase
		res.Phase = wtResCleanupCommitted
		if err := s.persistWorktreeResolution(rs, res); err != nil {
			res.Phase = prev
			return fail(res, "cannot persist resolution phase: "+err.Error())
		}
	}

	// Phase: finalize — the terminal state is persisted BEFORE success is
	// reported and before worktree_resolved is emitted.
	if e := s.finalizeWorktreeResolution(rs, terminal); e != nil {
		return fail(res, "cannot finalize resolution durably: "+e.msg)
	}
	extra := map[string]any{"resolutionId": res.ID}
	switch mode {
	case "apply_patch":
		extra["applied"] = applied
	case "keep_branch":
		extra["branch"] = b.Branch
	}
	return respond(terminal, extra)
}

// persistWorktreeResolution writes the resolution intent onto the binding
// durably. On store failure the in-memory binding is rolled back so RAM
// never diverges from the session record.
func (s *InteractiveService) persistWorktreeResolution(rs *interactiveRun, res *worktreeResolution) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev := rs.worktree.Resolution
	cp := *res
	rs.worktree.Resolution = &cp
	if err := s.persistRunWorktreeLocked(rs); err != nil {
		rs.worktree.Resolution = prev
		return err
	}
	return nil
}

// finalizeWorktreeResolution commits the terminal binding state durably
// (state + cleared intent), then emits worktree_resolved. On persist failure
// the in-memory binding is rolled back and no event is emitted — the caller
// reports a retryable failure, not success (BUG-481).
func (s *InteractiveService) finalizeWorktreeResolution(rs *interactiveRun, terminal string) *apiErr {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rs.worktree == nil {
		return newAPIErr(http.StatusConflict, "worktree_not_found", "run has no worktree binding")
	}
	prev := *rs.worktree
	rs.worktree.State = terminal
	rs.worktree.Resolution = nil
	if err := s.persistRunWorktreeLocked(rs); err != nil {
		*rs.worktree = prev
		return newAPIErr(http.StatusServiceUnavailable, "worktree_resolution_failed",
			"cannot persist terminal worktree state: "+err.Error())
	}
	s.emitLocked(rs, ProviderEvent{Type: EventWorktreeResolved, Input: map[string]any{
		"ownerId": rs.worktree.OwnerID, "state": terminal, "branch": rs.worktree.Branch,
	}})
	return nil
}

// worktreeCleanupEffect performs the filesystem/Git effects of a destructive
// resolution and verifies the post-conditions the terminal state will claim:
// worktree dir and base sidecar gone, branch present iff keep_branch. Every
// step is idempotent so a retry after a partial failure converges.
func worktreeCleanupEffect(ctx context.Context, mgr worktreeOps, repoDir string, b *worktreeBinding, mode string) error {
	keepBranch := mode == "keep_branch"
	if err := mgr.Cleanup(ctx, repoDir, b.OwnerID, "", keepBranch); err != nil {
		return fmt.Errorf("worktree cleanup: %w", err)
	}
	if mode == "apply_patch" {
		if err := os.Remove(patchArtifactPath(repoDir, b.OwnerID)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove patch artifact: %w", err)
		}
	}
	if !keepBranch && b.Branch != "" {
		// Cleanup reads the branch from the worktree HEAD before removal —
		// on a resume the worktree may already be gone, so delete the branch
		// explicitly (idempotent: "not found" is success).
		if out, err := exec.CommandContext(ctx, "git", "-C", repoDir, "branch", "-D", b.Branch).CombinedOutput(); err != nil &&
			!strings.Contains(string(out), "not found") {
			return fmt.Errorf("delete branch %s: %s", b.Branch, strings.TrimSpace(string(out)))
		}
	}
	wtDir := b.Path
	if wtDir == "" {
		wtDir = worktree.Path(repoDir, b.OwnerID, "")
	}
	if _, err := os.Stat(wtDir); err == nil {
		return fmt.Errorf("cleanup incomplete: worktree dir still present: %s", wtDir)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("verify worktree dir removal: %w", err)
	}
	if _, err := os.Stat(worktree.BaseSidecar(repoDir, b.OwnerID, "")); err == nil {
		return errors.New("cleanup incomplete: base sidecar still present")
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("verify sidecar removal: %w", err)
	}
	if b.Branch != "" {
		out, err := exec.CommandContext(ctx, "git", "-C", repoDir, "rev-parse", "--verify", b.Branch).CombinedOutput()
		if keepBranch && err != nil {
			return fmt.Errorf("cleanup incomplete: kept branch %s missing: %s", b.Branch, strings.TrimSpace(string(out)))
		}
		if !keepBranch && err == nil {
			return fmt.Errorf("cleanup incomplete: branch %s still present", b.Branch)
		}
	}
	return nil
}

// gitApplyCheckReverse reports whether patch is already applied to repoDir:
// `git apply --check --reverse` succeeds only when the forward changes are
// present. It distinguishes "patch landed before the crash" from a real
// merge conflict on resolution resume (BUG-481).
func gitApplyCheckReverse(ctx context.Context, repoDir string, patch []byte) bool {
	if len(bytes.TrimSpace(patch)) == 0 {
		return false
	}
	cmd := exec.CommandContext(ctx, "git", "-C", repoDir, "apply", "--check", "--reverse", "-")
	cmd.Stdin = bytes.NewReader(patch)
	return cmd.Run() == nil
}

func patchArtifactPath(repoDir, ownerID string) string {
	return filepath.Join(worktree.Root(repoDir), ownerID+".patch")
}

// setWorktreeState updates the binding state in memory + session record and
// emits a resolution event.
func (s *InteractiveService) setWorktreeState(rs *interactiveRun, state string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rs.worktree.State = state
	s.persistRunWorktreeLocked(rs)
	s.emitLocked(rs, ProviderEvent{Type: EventWorktreeResolved, Input: map[string]any{
		"ownerId": rs.worktree.OwnerID, "state": state, "branch": rs.worktree.Branch,
	}})
}

// validateWorktreeBindingOnResume runs SD-27 D-7 on a reconstructed/resumed
// run: registered in `git worktree list`, dir exists, .base sidecar present.
// On failure the binding is marked lost + a worktree_lost event is emitted —
// never silently recreated or duplicated (SS-23 AC-1b).
func (s *InteractiveService) validateWorktreeBindingOnResume(rs *interactiveRun) {
	if rs == nil || rs.worktree == nil || rs.worktree.State == "lost" {
		return
	}
	repoDir := s.worktreeRepoDirOfRun(rs)
	if err := worktree.NewManager().Validate(context.Background(), repoDir, rs.worktree.OwnerID, ""); err != nil {
		s.mu.Lock()
		rs.worktree.State = "lost"
		s.persistRunWorktreeLocked(rs)
		s.emitLocked(rs, ProviderEvent{Type: EventWorktreeLost, Input: map[string]any{
			"ownerId": rs.worktree.OwnerID, "path": rs.worktree.Path,
			"reason":  err.Error(),
			"options": []string{"archive", "recreate_empty"},
		}})
		s.mu.Unlock()
	}
}

// worktreeDeleteGate blocks chat/run deletion while a live binding exists
// (SS-23 E-3): the client must pass ?worktree=<mode> so the merge decision is
// resolved first, then the delete proceeds — GC-eligible only after that.
func (s *InteractiveService) worktreeDeleteGate(runID, mode string) *apiErr {
	s.mu.Lock()
	rs := s.runs[runID]
	var b *worktreeBinding
	if rs != nil && rs.worktree != nil {
		b = rs.worktree
	}
	s.mu.Unlock()
	if b == nil {
		// Reconstructed-from-disk runs may not be resident; check the session.
		if reader, ok := s.workflowStore.(SessionHistoryReader); ok {
			if st, found, _ := reader.GetProviderSession(context.Background(), runID); found && st.WorktreePath != "" {
				b = worktreeBindingFromSession(st)
			}
		}
	}
	if b == nil || b.State == "" || b.State == "lost" ||
		b.State == "merged" || b.State == "kept_branch" || b.State == "discarded" {
		return nil
	}
	if strings.TrimSpace(mode) == "" {
		return newAPIErr(http.StatusConflict, "worktree_merge_pending",
			"run has a pending worktree merge decision; resolve it first via ?worktree=apply_patch|keep_branch|discard")
	}
	if _, e := s.resolveWorktree(context.Background(), runID, mode, true); e != nil {
		return e
	}
	return nil
}

// worktreeGCVerdict is the three-state outcome of consulting persisted
// binding authorities for one worktree owner (BUG-473): the sweep may delete
// only on gcOrphan. "Could not check" is never "safe to delete".
type worktreeGCVerdict int

const (
	gcOrphan  worktreeGCVerdict = iota // every consulted authority proved no live binding
	gcBound                            // an active/merge_pending/resumable/lost binding exists
	gcUnknown                          // an authority could not be read or was corrupt — fail closed
)

// worktreeGCVerdictFor resolves one owner's binding status against the
// persisted stores. Chat-owned bindings are proved via the chat-leg list;
// flow-owned bindings via the run session record — a failed chat lookup is
// never re-read as "not found" through the run lookup. Any read error or a
// matching row with a zero/corrupt state yields gcUnknown. A store that
// cannot answer a binding class at all also yields gcUnknown: GC requires a
// positive proof of orphan-hood.
func (s *InteractiveService) worktreeGCVerdictFor(ctx context.Context, ownerID string) worktreeGCVerdict {
	consulted := false
	if reader, ok := s.workflowStore.(ChatSessionReader); ok {
		consulted = true
		rows, err := reader.ListProviderSessionsByChat(ctx, ownerID)
		if err != nil {
			return gcUnknown
		}
		for _, row := range rows {
			if row.WorktreeOwnerID != ownerID {
				continue
			}
			switch row.WorktreeState {
			case "discarded", "merged":
				// Resolved binding — does not protect the dir; keep looking.
			case "":
				return gcUnknown // partially-written/corrupt record
			default:
				return gcBound
			}
		}
	}
	if reader, ok := s.workflowStore.(SessionHistoryReader); ok {
		consulted = true
		row, found, err := reader.GetProviderSession(ctx, ownerID)
		if err != nil {
			return gcUnknown
		}
		if found && row.WorktreeOwnerID == ownerID {
			switch row.WorktreeState {
			case "discarded", "merged":
			case "":
				return gcUnknown
			default:
				return gcBound
			}
		}
	}
	if !consulted {
		return gcUnknown // no authority can prove orphan-hood — fail closed
	}
	return gcOrphan
}

// removeWorktreeDir deletes a proven-orphan worktree dir: `git worktree
// remove` when it is registered, os.RemoveAll for the unregistered dirs this
// GC usually targets. An error means data may remain — the caller must not
// report a prune.
func removeWorktreeDir(repoDir, path string) error {
	if _, err := exec.Command("git", "-C", repoDir, "worktree", "remove", "--force", path).CombinedOutput(); err == nil {
		return nil
	}
	return os.RemoveAll(path)
}

// sweepOrphanedWorktrees is the boot-time GC (SD-27 §8, Task-411 T-3): prune
// worktree dirs whose owner has no live or persisted binding — never touching
// tournament candidates (candidate-* prefix, owned by Task-369 lifecycle) or
// owners whose binding is active/merge_pending/resumable.
//
// BUG-473: deletion requires a proven orphan verdict. Store read errors,
// corrupt rows, or a store that cannot answer a binding class defer the
// owner (gc_deferred diagnostic) — never prune on "authority unavailable".
// A prune is logged only after the removal actually succeeded.
func (s *InteractiveService) sweepOrphanedWorktrees(ctx context.Context) {
	repoDir := ""
	if s.runner != nil {
		repoDir = strings.TrimSpace(s.runner.workspace)
	}
	if repoDir == "" {
		return
	}
	root := worktree.Root(repoDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	live := map[string]bool{}
	s.mu.Lock()
	for _, rs := range s.runs {
		if rs.worktree != nil && rs.worktree.OwnerID != "" {
			live[rs.worktree.OwnerID] = true
		}
	}
	s.mu.Unlock()
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), "candidate-") {
			continue
		}
		ownerID := e.Name()
		if live[ownerID] {
			continue
		}
		// Persisted sessions may bind this owner even when the run is not
		// resident — merge_pending and resumable owners are never GC'd.
		switch s.worktreeGCVerdictFor(ctx, ownerID) {
		case gcBound:
			continue
		case gcUnknown:
			log.Printf("[worktree-gc] gc_deferred owner=%s (binding authority unreadable or corrupt)", ownerID)
			continue
		}
		// Proven orphan: remove dir + sidecars, log pruned only on success.
		path := filepath.Join(root, ownerID)
		if err := removeWorktreeDir(repoDir, path); err != nil {
			log.Printf("[worktree-gc] gc_failed owner=%s path=%s err=%v", ownerID, path, err)
			continue
		}
		_ = os.Remove(worktree.BaseSidecar(repoDir, ownerID, ""))
		_ = os.Remove(patchArtifactPath(repoDir, ownerID))
		log.Printf("[worktree-gc] pruned orphan worktree owner=%s path=%s", ownerID, path)
	}
	_ = exec.Command("git", "-C", repoDir, "worktree", "prune").Run()
}

// handleWorktreeResolve wires POST /client/workflow-runs/{runId}/worktree/resolve.
func (s *InteractiveService) handleWorktreeResolve(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Mode    string `json:"mode"`
		Confirm bool   `json:"confirm"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	out, e := s.resolveWorktree(r.Context(), r.PathValue("runId"), body.Mode, body.Confirm)
	if e != nil {
		// Conflict/resolution responses still carry the evidence payload for
		// the card (BUG-481: resolutionId + durable phase for the client).
		if e.code == "worktree_merge_conflict" || e.code == "worktree_discard_confirm" ||
			e.code == "worktree_keep_branch_confirm" || e.code == "worktree_resolution_failed" ||
			e.code == "worktree_resolution_conflict" || e.code == "worktree_already_resolved" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(e.status)
			_ = json.NewEncoder(w).Encode(out)
			return
		}
		writeInteractiveError(w, e)
		return
	}
	writeInteractiveJSON(w, http.StatusOK, out)
}
