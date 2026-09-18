package tournament

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// worktreeRootRel is the scratch root for candidate worktrees, relative to
// the repo dir (Task-369 T-1). It is gitignored so child worktrees never
// show up as untracked dirt in the main repo.
const worktreeRootRel = ".flowpilot/worktrees"

// gitignoreEntry is appended to the repo .gitignore (additive, best-effort).
const gitignoreEntry = ".flowpilot/worktrees/"

func worktreeRoot(repoDir string) string { return filepath.Join(repoDir, worktreeRootRel) }

func worktreePath(repoDir, candidateID string) string {
	return filepath.Join(worktreeRoot(repoDir), "candidate-"+candidateID)
}

// WorktreePath returns the on-disk path of a candidate worktree (exported
// for runner glue and tests; no existence check).
func WorktreePath(repoDir, candidateID string) string {
	return worktreePath(repoDir, candidateID)
}

// baseSidecar records the base commit a worktree was created from, so
// MergeWinner can refuse when the main workspace drifted since tournament
// start (Task-369 T-2/T-3). It lives next to the worktree dir (never inside
// it, so it can never leak into a Diff patch) and is removed by Cleanup.
func baseSidecar(repoDir, candidateID string) string {
	return filepath.Join(worktreeRoot(repoDir), "candidate-"+candidateID+".base")
}

// validateCandidateID rejects ids that could escape the worktree root.
func validateCandidateID(id string) error {
	if id == "" || id == "." || id == ".." ||
		strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") {
		return fmt.Errorf("tournament: invalid candidate id %q", id)
	}
	return nil
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

var patchFailedLine = regexp.MustCompile(`(?m)^error: patch failed: (.+?):\d+\s*$`)

// conflictPathsFromApplyOutput extracts unique file paths from
// `git apply` stderr lines shaped like "error: patch failed: f.go:12".
// Best-effort: unparseable output yields an empty list, never an error.
func conflictPathsFromApplyOutput(output string) []string {
	var paths []string
	seen := map[string]bool{}
	for _, m := range patchFailedLine.FindAllStringSubmatch(output, -1) {
		p := strings.TrimSpace(m[1])
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		paths = append(paths, p)
	}
	return paths
}

// WorktreeManager owns the git-worktree lifecycle for tournament candidates
// (Task-369): Create isolated worktrees from one base commit, Diff a
// candidate's change as a patch, MergeWinner it onto the main workspace, and
// Cleanup everything afterwards. It shells out to git only and never takes
// a providerKey (provider parity Case-1 agnostic). It never commits.
type WorktreeManager struct{}

// Create adds an isolated worktree for candidateID at baseCommit
// (Task-369 T-1). baseCommit is typically rs.flowStartGitHead so every
// candidate starts from the identical point. It fails when repoDir is not a
// git repo, baseCommit does not resolve, or the worktree already exists.
func (WorktreeManager) Create(repoDir, baseCommit, candidateID string) (string, error) {
	if err := validateCandidateID(candidateID); err != nil {
		return "", err
	}
	if _, err := gitOut(repoDir, "rev-parse", "--git-dir"); err != nil {
		return "", fmt.Errorf("tournament: %s is not a git repository", repoDir)
	}
	base, err := gitOut(repoDir, "rev-parse", baseCommit+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("tournament: base commit %q does not resolve: %w", baseCommit, err)
	}
	path := worktreePath(repoDir, candidateID)
	if _, statErr := os.Stat(path); statErr == nil {
		return "", fmt.Errorf("tournament: worktree for candidate %q already exists at %s", candidateID, path)
	}
	if err := os.MkdirAll(worktreeRoot(repoDir), 0o755); err != nil {
		return "", fmt.Errorf("tournament: create worktree root: %w", err)
	}
	if _, err := gitOut(repoDir, "worktree", "add", path, base); err != nil {
		return "", err
	}
	if err := os.WriteFile(baseSidecar(repoDir, candidateID), []byte(base+"\n"), 0o644); err != nil {
		_, _ = gitOut(repoDir, "worktree", "remove", "--force", path)
		return "", fmt.Errorf("tournament: record base commit: %w", err)
	}
	ensureWorktreeGitignore(repoDir) // best-effort, never fails Create
	return path, nil
}

// ensureWorktreeGitignore appends the worktree root to the repo .gitignore
// when not already covered (Task-369 T-1). Best-effort: all errors ignored.
func ensureWorktreeGitignore(repoDir string) {
	ignoreFile := filepath.Join(repoDir, ".gitignore")
	raw, err := os.ReadFile(ignoreFile)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == gitignoreEntry {
			return
		}
	}
	f, err := os.OpenFile(ignoreFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	trimmed := strings.TrimRight(string(raw), "\n")
	prefix := ""
	if len(raw) > 0 && !strings.HasSuffix(string(raw), "\n") {
		prefix = "\n"
	}
	_ = trimmed // keep anchored forms out of scope: exact-line match only
	_, _ = f.WriteString(prefix + gitignoreEntry + "\n")
}

// Diff returns the candidate's full change as a patch (Task-369 T-2).
// Untracked files are intent-to-added first so new files enter the diff.
// An unchanged worktree yields an empty (non-nil-error) patch.
func (WorktreeManager) Diff(repoDir, candidateID string) ([]byte, error) {
	if err := validateCandidateID(candidateID); err != nil {
		return nil, err
	}
	path := worktreePath(repoDir, candidateID)
	if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("tournament: no worktree for candidate %q", candidateID)
	}
	if _, err := gitOut(path, "add", "-N", "."); err != nil {
		return nil, err
	}
	cmd := exec.Command("git", "diff", "--no-color")
	cmd.Dir = path
	patch, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("tournament: git diff: %w", err)
	}
	return patch, nil
}

// MergeWinner applies the winner's patch onto the main workspace
// (Task-369 T-3). It refuses — without touching the workspace — when the
// main HEAD moved since Create or the patch no longer applies cleanly, in
// which case it cleans the worktree up and returns *MergeConflictError with
// the evidence for the ask_user card (no-orphan policy). On success it
// creates no commit and touches no branch/ref; the caller deletes the
// winner worktree at flow done via Cleanup.
func (WorktreeManager) MergeWinner(repoDir, candidateID string) error {
	var mgr WorktreeManager
	if err := validateCandidateID(candidateID); err != nil {
		return err
	}
	path := worktreePath(repoDir, candidateID)
	if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
		return fmt.Errorf("tournament: no worktree for candidate %q", candidateID)
	}
	rawBase, err := os.ReadFile(baseSidecar(repoDir, candidateID))
	if err != nil {
		return fmt.Errorf("tournament: unknown base commit for candidate %q (stale worktree?)", candidateID)
	}
	head, err := gitOut(repoDir, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(rawBase)) != head {
		_ = mgr.Cleanup(repoDir, []string{candidateID})
		return &MergeConflictError{
			CandidateID: candidateID,
			Reason:      "main workspace HEAD moved since tournament start",
		}
	}
	patch, err := mgr.Diff(repoDir, candidateID)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(patch)) == 0 {
		return nil // nothing to merge — still a successful (empty) win
	}
	check := exec.Command("git", "apply", "--check", "-")
	check.Dir = repoDir
	check.Stdin = bytes.NewReader(patch)
	if out, err := check.CombinedOutput(); err != nil {
		_ = mgr.Cleanup(repoDir, []string{candidateID})
		return &MergeConflictError{
			CandidateID:   candidateID,
			Reason:        "winner patch does not apply cleanly on the main workspace",
			Patch:         patch,
			ConflictPaths: conflictPathsFromApplyOutput(string(out)),
		}
	}
	apply := exec.Command("git", "apply", "-")
	apply.Dir = repoDir
	apply.Stdin = bytes.NewReader(patch)
	if out, err := apply.CombinedOutput(); err != nil {
		// Raced between --check and apply: same conflict treatment.
		_ = mgr.Cleanup(repoDir, []string{candidateID})
		return &MergeConflictError{
			CandidateID:   candidateID,
			Reason:        "winner patch failed to apply on the main workspace",
			Patch:         patch,
			ConflictPaths: conflictPathsFromApplyOutput(string(out)),
		}
	}
	return nil
}

// Cleanup removes the given candidate worktrees and their base sidecars,
// then prunes stale worktree metadata (Task-369 T-4). Idempotent: unknown
// or already-removed ids are no-ops, so calling it twice (verdict-time +
// flow-done, or success + deferred error path) is safe.
func (WorktreeManager) Cleanup(repoDir string, candidateIDs []string) error {
	for _, id := range candidateIDs {
		if err := validateCandidateID(id); err != nil {
			continue
		}
		path := worktreePath(repoDir, id)
		if _, err := os.Stat(path); err == nil {
			if _, err := gitOut(repoDir, "worktree", "remove", "--force", path); err != nil {
				_ = os.RemoveAll(path) // fallback: never strand a dir
			}
		}
		_ = os.Remove(baseSidecar(repoDir, id))
	}
	_, _ = gitOut(repoDir, "worktree", "prune")
	return nil
}
