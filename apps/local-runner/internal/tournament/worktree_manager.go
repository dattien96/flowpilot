package tournament

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"flowpilot-runner/internal/worktree"
)

// This file is the tournament-scoped delegation layer over the shared
// internal/worktree manager (CP-71 P-1 / SD-27 D-1). The public API —
// WorktreePath, WorktreeManager.{Create,Diff,MergeWinner,Cleanup}, and
// *MergeConflictError — is byte-compatible with the Task-369 contract
// (candidate-<id> paths, no-orphan cleanup on conflict, identical error
// semantics) so CP-65 behavior is unchanged; the tournament test suite is
// the regression oracle.

// candidatePrefix preserves the historical .flowpilot/worktrees/candidate-<id>
// layout through the shared manager's prefix parameter.
const candidatePrefix = "candidate-"

// WorktreePath returns the on-disk path of a candidate worktree (exported
// for runner glue and tests; no existence check).
func WorktreePath(repoDir, candidateID string) string {
	return worktree.Path(repoDir, candidateID, candidatePrefix)
}

// --- legacy helpers kept for the pre-existing test suite (Task-369 tests
// reference these unexported names; do not remove) ---

func worktreePath(repoDir, candidateID string) string {
	return worktree.Path(repoDir, candidateID, candidatePrefix)
}

func baseSidecar(repoDir, candidateID string) string {
	return worktree.BaseSidecar(repoDir, candidateID, candidatePrefix)
}

func gitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out)),
			fmt.Errorf("tournament: git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// MergeConflictError is the typed failure when a winner patch cannot land
// on the main workspace (Task-369 T-3). Per the 2026-09-16 no-orphan policy
// it carries the full evidence (patch + conflicting paths) for the P-3
// behavior to attach to the ask_user card — the worktree itself is already
// cleaned up when this error returns, so callers must NOT keep it for
// investigation. Use errors.As to detect it.
type MergeConflictError struct {
	CandidateID   string
	Reason        string
	Patch         []byte
	ConflictPaths []string
}

func (e *MergeConflictError) Error() string {
	paths := strings.Join(e.ConflictPaths, ", ")
	if paths == "" {
		paths = "(paths unavailable)"
	}
	return fmt.Sprintf("tournament: cannot merge winner %s: %s (conflicts in %s)",
		e.CandidateID, e.Reason, paths)
}

// WorktreeManager owns the git-worktree lifecycle for tournament candidates
// (Task-369): Create isolated worktrees from one base commit, Diff a
// candidate's change as a patch, MergeWinner it onto the main workspace, and
// Cleanup everything afterwards. It shells out to git only and never takes
// a providerKey (provider parity Case-1 agnostic). It never commits.
type WorktreeManager struct {
	shared worktree.Manager
}

// Create adds an isolated worktree for candidateID at baseCommit
// (Task-369 T-1). baseCommit is typically rs.flowStartGitHead so every
// candidate starts from the identical point. It fails when repoDir is not a
// git repo, baseCommit does not resolve, or the worktree already exists.
func (m WorktreeManager) Create(repoDir, baseCommit, candidateID string) (string, error) {
	info, err := m.shared.Create(context.Background(), repoDir, candidateID, candidatePrefix, baseCommit, "")
	if err != nil {
		return "", err
	}
	return info.Path, nil
}

// Diff returns the candidate's full change as a patch (Task-369 T-2).
// Untracked files are intent-to-added first so new files enter the diff.
// An unchanged worktree yields an empty (non-nil-error) patch.
func (m WorktreeManager) Diff(repoDir, candidateID string) ([]byte, error) {
	return m.shared.Diff(context.Background(), repoDir, candidateID, candidatePrefix)
}

// MergeWinner applies the winner's patch onto the main workspace
// (Task-369 T-3). It refuses — without touching the workspace — when the
// main HEAD moved since Create or the patch no longer applies cleanly, in
// which case it cleans the worktree up and returns *MergeConflictError with
// the evidence for the ask_user card (no-orphan policy). On success it
// creates no commit and touches no branch/ref; the caller deletes the
// winner worktree at flow done via Cleanup.
func (m WorktreeManager) MergeWinner(repoDir, candidateID string) error {
	err := m.shared.Apply(context.Background(), repoDir, candidateID, candidatePrefix)
	if err == nil {
		return nil
	}
	var conflict *worktree.MergeConflictError
	if !errors.As(err, &conflict) {
		// Wrap non-conflict errors in the historical error wording only where
		// it matters for callers; upstream errors pass through as-is.
		return err
	}
	_ = m.Cleanup(repoDir, []string{candidateID}) // no-orphan policy
	return &MergeConflictError{
		CandidateID:   candidateID,
		Reason:        conflict.Reason,
		Patch:         conflict.Patch,
		ConflictPaths: conflict.ConflictPaths,
	}
}

// Cleanup removes the given candidate worktrees and their base sidecars,
// then prunes stale worktree metadata (Task-369 T-4). Idempotent: unknown
// or already-removed ids are no-ops, so calling it twice (verdict-time +
// flow-done, or success + deferred error path) is safe.
func (m WorktreeManager) Cleanup(repoDir string, candidateIDs []string) error {
	for _, id := range candidateIDs {
		// keepBranch=false: candidates never create branches (detached).
		_ = m.shared.Cleanup(context.Background(), repoDir, id, candidatePrefix, false)
	}
	return nil
}
