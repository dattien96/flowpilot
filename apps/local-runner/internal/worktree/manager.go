// Package worktree owns the shared git-worktree lifecycle (CP-71 P-1 /
// Task-407): create isolated worktrees under .flowpilot/worktrees/<prefix><ownerID>,
// diff their changes as patches, apply patches back onto the main workspace,
// validate bindings after restart, and clean up idempotently.
//
// The implementation is generalized from tournament.WorktreeManager
// (Task-369 / CA-877) and internal/tournament delegates to it — one
// implementation, no diverging copies. Git-only, provider-agnostic
// (Case-1: no providerKey anywhere), and it never commits, merges branches,
// or pushes in the user's repository.
package worktree

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// WorktreeRootRel is the scratch root for managed worktrees, relative to the
// repo dir. It is gitignored so managed worktrees never show up as untracked
// dirt in the main repo.
const WorktreeRootRel = ".flowpilot/worktrees"

// GitignoreEntry is appended to the repo .gitignore (additive, best-effort).
const GitignoreEntry = ".flowpilot/worktrees/"

// Info describes a managed worktree binding.
type Info struct {
	OwnerID    string
	Path       string
	Branch     string // empty when the worktree is detached (tournament candidates)
	BaseCommit string
	Slug       string
	CreatedAt  time.Time
}

// MergeConflictError is the typed failure when a patch cannot land on the
// main workspace. It carries the full evidence (patch + conflicting paths)
// for the user-facing merge card. Use errors.As to detect it. Unlike the
// tournament wrapper, the manager itself never deletes the worktree on
// conflict — lifecycle policy (preserve vs cleanup) belongs to the caller.
type MergeConflictError struct {
	OwnerID       string
	Reason        string
	Patch         []byte
	ConflictPaths []string
}

func (e *MergeConflictError) Error() string {
	paths := strings.Join(e.ConflictPaths, ", ")
	if paths == "" {
		paths = "(paths unavailable)"
	}
	return fmt.Sprintf("worktree: cannot merge %s: %s (conflicts in %s)",
		e.OwnerID, e.Reason, paths)
}

var patchFailedLine = regexp.MustCompile(`(?m)^error: patch failed: (.+?):\d+\s*$`)

// conflictPathsFromApplyOutput extracts unique file paths from `git apply`
// stderr lines shaped like "error: patch failed: f.go:12". Best-effort:
// unparseable output yields an empty list, never an error.
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

// applyLocks serializes patch application per repository (SD-27 D-4): two
// applies on one repo never interleave, and the second re-checks drift.
var applyLocks sync.Map // repoDir -> *sync.Mutex

func repoApplyLock(repoDir string) *sync.Mutex {
	mu, _ := applyLocks.LoadOrStore(repoDir, &sync.Mutex{})
	return mu.(*sync.Mutex)
}

// Root returns the on-disk worktree root for a repo.
func Root(repoDir string) string { return filepath.Join(repoDir, WorktreeRootRel) }

// Path returns the on-disk path of a managed worktree (no existence check).
func Path(repoDir, ownerID, prefix string) string {
	return filepath.Join(Root(repoDir), prefix+ownerID)
}

// BaseSidecar returns the path of the sidecar recording the base commit a
// worktree was created from. It lives next to the worktree dir (never inside
// it, so it can never leak into a Diff patch) and is removed by Cleanup.
func BaseSidecar(repoDir, ownerID, prefix string) string {
	return filepath.Join(Root(repoDir), prefix+ownerID+".base")
}

// validateOwnerID rejects ids that could escape the worktree root.
func validateOwnerID(id string) error {
	if id == "" || id == "." || id == ".." ||
		strings.ContainsAny(id, `/\`) || strings.Contains(id, "..") {
		return fmt.Errorf("worktree: invalid owner id %q", id)
	}
	return nil
}

func gitOut(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out)),
			fmt.Errorf("worktree: git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

// Slugify converts a title into a branch-safe slug (Devin-style:
// lowercased, non-alnum collapsed to dashes, bounded length).
func Slugify(title string) string {
	s := strings.ToLower(strings.TrimSpace(title))
	var b strings.Builder
	lastDash := true
	for _, r := range s {
		if b.Len() >= 40 {
			break
		}
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// Manager owns the worktree lifecycle for one naming family. It is
// stateless apart from the shared per-repo apply mutexes.
type Manager struct{}

// NewManager returns the shared manager.
func NewManager() *Manager { return &Manager{} }

// Create adds an isolated worktree for (ownerID, prefix) at baseCommit. When
// slug is non-empty the worktree checks out a new branch fp/<slug>-<short>
// created at baseCommit; when slug is empty the worktree is detached at
// baseCommit (tournament candidate semantics). It fails when repoDir is not
// a git repo, baseCommit does not resolve, or the worktree already exists.
func (Manager) Create(_ context.Context, repoDir, ownerID, prefix, baseCommit, slug string) (Info, error) {
	if err := validateOwnerID(ownerID); err != nil {
		return Info{}, err
	}
	if strings.ContainsAny(prefix, `/\`) || strings.Contains(prefix, "..") {
		return Info{}, fmt.Errorf("worktree: invalid prefix %q", prefix)
	}
	if _, err := gitOut(repoDir, "rev-parse", "--git-dir"); err != nil {
		return Info{}, fmt.Errorf("worktree: %s is not a git repository", repoDir)
	}
	base, err := gitOut(repoDir, "rev-parse", baseCommit+"^{commit}")
	if err != nil {
		return Info{}, fmt.Errorf("worktree: base commit %q does not resolve: %w", baseCommit, err)
	}
	path := Path(repoDir, ownerID, prefix)
	if _, statErr := os.Stat(path); statErr == nil {
		return Info{}, fmt.Errorf("worktree: worktree for owner %q already exists at %s", ownerID, path)
	}
	if err := os.MkdirAll(Root(repoDir), 0o755); err != nil {
		return Info{}, fmt.Errorf("worktree: create worktree root: %w", err)
	}
	branch := ""
	args := []string{"worktree", "add"}
	if slug != "" {
		short := ownerID
		if len(short) > 8 {
			short = short[len(short)-8:]
		}
		branch = "fp/" + slug + "-" + short
		args = append(args, "-b", branch)
	}
	args = append(args, path, base)
	if _, err := gitOut(repoDir, args...); err != nil {
		return Info{}, err
	}
	if err := os.WriteFile(BaseSidecar(repoDir, ownerID, prefix), []byte(base+"\n"), 0o644); err != nil {
		_, _ = gitOut(repoDir, "worktree", "remove", "--force", path)
		return Info{}, fmt.Errorf("worktree: record base commit: %w", err)
	}
	ensureGitignore(repoDir) // best-effort, never fails Create
	return Info{
		OwnerID:    ownerID,
		Path:       path,
		Branch:     branch,
		BaseCommit: base,
		Slug:       slug,
		CreatedAt:  time.Now().UTC(),
	}, nil
}

// ensureGitignore appends the worktree root to the repo .gitignore when not
// already covered. Best-effort: all errors ignored.
func ensureGitignore(repoDir string) {
	ignoreFile := filepath.Join(repoDir, ".gitignore")
	raw, err := os.ReadFile(ignoreFile)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) == GitignoreEntry {
			return
		}
	}
	f, err := os.OpenFile(ignoreFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	prefix := ""
	if len(raw) > 0 && !strings.HasSuffix(string(raw), "\n") {
		prefix = "\n"
	}
	_, _ = f.WriteString(prefix + GitignoreEntry + "\n")
}

// Diff returns the worktree's full change as a patch. Untracked files are
// intent-to-added first so new files enter the diff. When the base sidecar
// exists the diff is anchored at the recorded base commit — work COMMITTED on
// the worktree branch (a provider can `git commit` inside it) is part of the
// delta, not silently dropped (BUG-378). Without a sidecar the legacy bare
// `git diff` (index/working-tree only) is kept. An unchanged worktree yields
// an empty (non-nil-error-free) patch.
func (Manager) Diff(_ context.Context, repoDir, ownerID, prefix string) ([]byte, error) {
	if err := validateOwnerID(ownerID); err != nil {
		return nil, err
	}
	path := Path(repoDir, ownerID, prefix)
	if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("worktree: no worktree for owner %q", ownerID)
	}
	if _, err := gitOut(path, "add", "-N", "."); err != nil {
		return nil, err
	}
	args := []string{"diff", "--no-color"}
	// Anchor at the recorded base commit when available: `git diff <base>`
	// covers committed-branch + index + working-tree + intent-to-add — the
	// complete delta the merge-back contract promises.
	if raw, err := os.ReadFile(BaseSidecar(repoDir, ownerID, prefix)); err == nil {
		if base := strings.TrimSpace(string(raw)); base != "" {
			args = append(args, base)
		}
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = path
	patch, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("worktree: git diff: %w", err)
	}
	return patch, nil
}

// Untracked lists files in the worktree that are untracked vs its HEAD —
// surfaced on the discard confirm so users see what would be lost
// (SD-27 Q-2).
func (Manager) Untracked(_ context.Context, repoDir, ownerID, prefix string) ([]string, error) {
	if err := validateOwnerID(ownerID); err != nil {
		return nil, err
	}
	path := Path(repoDir, ownerID, prefix)
	out, err := gitOut(path, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// ApplyOptions controls the conflict-detection policy for Apply.
type ApplyOptions struct {
	// StrictHead refuses to apply when main HEAD moved since Create
	// (tournament Task-369 semantics: a winner must land on the exact commit
	// it was rolled out from). Run worktrees (CP-71) leave it false —
	// `git apply --check` is the sole oracle, so drift that does not touch
	// the patched regions still merges, and the user can fix conflicting
	// content in the working tree and retry apply_patch (SS-23 retry).
	StrictHead bool
}

// Apply applies the worktree's patch onto the main workspace under strict
// HEAD policy (tournament semantics). See ApplyWithOptions.
func (m Manager) Apply(ctx context.Context, repoDir, ownerID, prefix string) error {
	return m.ApplyWithOptions(ctx, repoDir, ownerID, prefix, ApplyOptions{StrictHead: true})
}

// ApplyWithOptions applies the worktree's patch onto the main workspace. It
// refuses — without touching the workspace — when the patch does not apply
// cleanly (and, under StrictHead, when the main HEAD moved since Create),
// returning *MergeConflictError with the evidence. The worktree is left in
// place (caller decides cleanup). Serialized per repository; on success it
// creates no commit and touches no branch/ref of the main checkout.
func (m Manager) ApplyWithOptions(_ context.Context, repoDir, ownerID, prefix string, opts ApplyOptions) error {
	if err := validateOwnerID(ownerID); err != nil {
		return err
	}
	mu := repoApplyLock(repoDir)
	mu.Lock()
	defer mu.Unlock()

	path := Path(repoDir, ownerID, prefix)
	if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
		return fmt.Errorf("worktree: no worktree for owner %q", ownerID)
	}
	rawBase, err := os.ReadFile(BaseSidecar(repoDir, ownerID, prefix))
	if err != nil {
		return fmt.Errorf("worktree: unknown base commit for owner %q (stale worktree?)", ownerID)
	}
	head, err := gitOut(repoDir, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	headMoved := strings.TrimSpace(string(rawBase)) != head
	if headMoved && opts.StrictHead {
		return &MergeConflictError{
			OwnerID: ownerID,
			Reason:  "main workspace HEAD moved since worktree creation",
		}
	}
	patch, err := m.Diff(context.Background(), repoDir, ownerID, prefix)
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
		reason := "patch does not apply cleanly on the main workspace"
		if headMoved {
			reason += " (main HEAD moved since worktree creation)"
		}
		return &MergeConflictError{
			OwnerID:       ownerID,
			Reason:        reason,
			Patch:         patch,
			ConflictPaths: conflictPathsFromApplyOutput(string(out)),
		}
	}
	apply := exec.Command("git", "apply", "-")
	apply.Dir = repoDir
	apply.Stdin = bytes.NewReader(patch)
	if out, err := apply.CombinedOutput(); err != nil {
		// Raced between --check and apply: same conflict treatment.
		return &MergeConflictError{
			OwnerID:       ownerID,
			Reason:        "patch failed to apply on the main workspace",
			Patch:         patch,
			ConflictPaths: conflictPathsFromApplyOutput(string(out)),
		}
	}
	return nil
}

// Cleanup removes the worktree and its base sidecar, then prunes stale
// worktree metadata. Idempotent: unknown or already-removed owners are
// no-ops. When deleteBranch is true the worktree's branch fp/<slug>-<short>
// is deleted too (apply_patch / discard modes); keepBranch leaves it.
func (Manager) Cleanup(_ context.Context, repoDir, ownerID, prefix string, keepBranch bool) error {
	if err := validateOwnerID(ownerID); err != nil {
		return nil // invalid ids are no-ops (idempotent contract)
	}
	path := Path(repoDir, ownerID, prefix)
	branch := ""
	if _, err := os.Stat(path); err == nil {
		if b, bErr := gitOut(path, "rev-parse", "--abbrev-ref", "HEAD"); bErr == nil && b != "HEAD" {
			branch = b
		}
		if _, err := gitOut(repoDir, "worktree", "remove", "--force", path); err != nil {
			_ = os.RemoveAll(path) // fallback: never strand a dir
		}
	}
	_ = os.Remove(BaseSidecar(repoDir, ownerID, prefix))
	if branch != "" && !keepBranch {
		_, _ = gitOut(repoDir, "branch", "-D", branch) // best-effort
	}
	_, _ = gitOut(repoDir, "worktree", "prune")
	return nil
}

// Validate checks that a persisted binding is still usable after restart
// (SD-27 D-7): the worktree is registered in `git worktree list --porcelain`,
// the directory exists, and the .base sidecar is present.
func (Manager) Validate(_ context.Context, repoDir, ownerID, prefix string) error {
	if err := validateOwnerID(ownerID); err != nil {
		return err
	}
	path := Path(repoDir, ownerID, prefix)
	if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
		return fmt.Errorf("worktree: directory missing for owner %q", ownerID)
	}
	if _, err := os.Stat(BaseSidecar(repoDir, ownerID, prefix)); err != nil {
		return fmt.Errorf("worktree: base sidecar missing for owner %q", ownerID)
	}
	out, err := gitOut(repoDir, "worktree", "list", "--porcelain")
	if err != nil {
		return err
	}
	// Normalize both sides: git reports canonical paths while callers may
	// hold 8.3-short or differently-cased forms (Windows), so compare the
	// symlink-resolved, slash-normalized, case-folded paths.
	clean := normalizePath(path)
	found := false
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, "worktree ") {
			continue
		}
		if normalizePath(strings.TrimPrefix(line, "worktree ")) == clean {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("worktree: not registered in git worktree list for owner %q", ownerID)
	}
	return nil
}

// normalizePath resolves symlinks (short/long path forms), converts to
// forward slashes, cleans, and lowercases — suitable for comparing a stored
// binding path against git-reported paths.
func normalizePath(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		p = resolved
	}
	return strings.ToLower(filepath.ToSlash(filepath.Clean(p)))
}
