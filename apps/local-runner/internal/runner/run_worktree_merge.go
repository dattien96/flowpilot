package runner

// Worktree merge-back (CP-71 P-4 / Task-410, SS-23 AC-4/AC-5, SD-27 D-2/D-4c).
//
// Two triggers, one contract: flow runs emit worktree_merge_requested at
// terminal status; chat runs resolve via the user-initiated merge control or
// the delete-time decision gate. All resolutions go through
// POST /client/workflow-runs/{runId}/worktree/resolve — patch-based only,
// never git merge/commit/push in the user's repo (BR-2).

import (
	"context"
	"encoding/json"
	"errors"
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
// record. Caller holds s.mu.
func (s *InteractiveService) persistRunWorktreeLocked(rs *interactiveRun) {
	reader, ok := s.workflowStore.(SessionHistoryReader)
	if !ok {
		return
	}
	st, found, err := reader.GetProviderSession(context.Background(), rs.id)
	if err != nil || !found {
		return
	}
	worktreeFieldsToSession(&st, rs.worktree)
	_ = s.persistProviderSession(st)
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
	if rs == nil {
		if rebuilt, err := s.loadPersistedRun(runID); err == nil {
			rs = rebuilt
		}
	}
	if rs == nil || rs.worktree == nil {
		s.mu.Unlock()
		return nil, newAPIErr(http.StatusNotFound, "worktree_not_found", "run has no worktree binding")
	}
	b := rs.worktree
	repoDir := s.worktreeRepoDirOfRun(rs)
	s.mu.Unlock()

	mgr := worktree.NewManager()
	respond := func(state string, extra map[string]any) (map[string]any, *apiErr) {
		out := map[string]any{"runId": runID, "worktreeState": state, "mode": mode}
		for k, v := range extra {
			out[k] = v
		}
		return out, nil
	}

	switch mode {
	case "apply_patch":
		if b.State == "lost" {
			return nil, newAPIErr(http.StatusConflict, "worktree_lost", "worktree is missing; nothing to apply")
		}
		patch, _ := mgr.Diff(ctx, repoDir, b.OwnerID, "")
		// Run worktrees use git apply --check as the conflict oracle (not the
		// strict HEAD check): non-conflicting drift still merges, and the user
		// can fix conflicting content then retry apply_patch (SS-23 retry).
		if err := mgr.ApplyWithOptions(ctx, repoDir, b.OwnerID, "",
			worktree.ApplyOptions{StrictHead: false}); err != nil {
			var conflict *worktree.MergeConflictError
			if errors.As(err, &conflict) {
				ref := ""
				if p := conflict.Patch; len(p) > 0 {
					if r, werr := s.writeWorktreePatchArtifact(rs, p); werr == nil {
						ref = r
					}
				}
				s.mu.Lock()
				rs.worktree.State = "merge_pending"
				s.persistRunWorktreeLocked(rs)
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
				}, newAPIErr(http.StatusConflict, "worktree_merge_conflict", conflict.Error())
			}
			return nil, newAPIErr(http.StatusInternalServerError, "worktree_merge_failed", err.Error())
		}
		_ = mgr.Cleanup(ctx, repoDir, b.OwnerID, "", false)
		_ = os.Remove(patchArtifactPath(repoDir, b.OwnerID))
		s.setWorktreeState(rs, "merged")
		return respond("merged", map[string]any{"applied": len(patch) > 0})

	case "keep_branch":
		if b.State == "lost" {
			return nil, newAPIErr(http.StatusConflict, "worktree_lost", "worktree is missing; nothing to keep")
		}
		_ = mgr.Cleanup(ctx, repoDir, b.OwnerID, "", true)
		s.setWorktreeState(rs, "kept_branch")
		return respond("kept_branch", map[string]any{"branch": b.Branch})

	case "discard":
		if b.State != "lost" {
			untracked, _ := mgr.Untracked(ctx, repoDir, b.OwnerID, "")
			if len(untracked) > 0 && !confirm {
				return map[string]any{
					"runId": runID, "worktreeState": b.State, "mode": mode,
					"requiresConfirm": true, "untracked": untracked,
				}, newAPIErr(http.StatusConflict, "worktree_discard_confirm",
					"worktree has untracked artifacts; resend with confirm to discard")
			}
		}
		_ = mgr.Cleanup(ctx, repoDir, b.OwnerID, "", false)
		s.setWorktreeState(rs, "discarded")
		return respond("discarded", nil)

	case "archive":
		// Lost-state resolution: keep the run, mark the binding archived —
		// no worktree ops (SS-23 E-2b).
		s.setWorktreeState(rs, "discarded")
		return respond("discarded", map[string]any{"archived": true})

	case "recreate_empty":
		if b.State != "lost" {
			return nil, newAPIErr(http.StatusConflict, "worktree_not_lost", "worktree is not lost")
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
			"reason": err.Error(),
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

// sweepOrphanedWorktrees is the boot-time GC (SD-27 §8, Task-411 T-3): prune
// worktree dirs whose owner has no live or persisted binding — never touching
// tournament candidates (candidate-* prefix, owned by Task-369 lifecycle) or
// owners whose binding is active/merge_pending/resumable.
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
		bound := false
		if reader, ok := s.workflowStore.(ChatSessionReader); ok {
			if rows, rerr := reader.ListProviderSessionsByChat(ctx, ownerID); rerr == nil {
				for _, row := range rows {
					if row.WorktreeOwnerID == ownerID && row.WorktreeState != "" &&
						row.WorktreeState != "discarded" && row.WorktreeState != "merged" {
						bound = true
						break
					}
				}
			}
		}
		// Flow-run owners are keyed by runId (not chatId) — check the session
		// record directly when the chat lookup found nothing.
		if !bound {
			if reader, ok := s.workflowStore.(SessionHistoryReader); ok {
				if row, found, rerr := reader.GetProviderSession(ctx, ownerID); rerr == nil && found &&
					row.WorktreeOwnerID == ownerID && row.WorktreeState != "" &&
					row.WorktreeState != "discarded" && row.WorktreeState != "merged" {
					bound = true
				}
			}
		}
		if bound {
			continue
		}
		// Orphan: remove dir + sidecars, keep an audit line.
		path := filepath.Join(root, ownerID)
		_ = exec.Command("git", "-C", repoDir, "worktree", "remove", "--force", path).Run()
		_ = os.RemoveAll(path)
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
		// Conflict responses still carry the evidence payload for the card.
		if e.code == "worktree_merge_conflict" || e.code == "worktree_discard_confirm" {
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
