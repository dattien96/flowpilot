package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// BUG-501 (live-found): when a worktree's .git file goes missing, `git -C
// <worktree>` silently walks UP to the parent repository instead of erroring.
// The worktrees dir is itself gitignored (.flowpilot/worktrees/), so every
// parent-scope dirty scan returns an EMPTY, error-free answer — Untracked/
// Uncommitted report "clean" on a worktree that may hold uncommitted work,
// and the discard gate proceeded to delete the directory (observed live on
// run-260771: .git file renamed → DELETE ?worktree=discard removed the dir
// with zero guard). The fix pins each worktree scan to the worktree's own
// toplevel before trusting any git output.

func gitCmd(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// bug501Bed builds a repo whose worktree lives under a gitignored dir —
// mirroring the production layout (.flowpilot/worktrees/<owner>).
func bug501Bed(t *testing.T) (repoDir, wtPath string) {
	t.Helper()
	repoDir = t.TempDir()
	gitCmd(t, repoDir, "init", "-q")
	gitCmd(t, repoDir, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "init", "--allow-empty")
	wtParent := filepath.Join(repoDir, ".flowpilot", "worktrees")
	if err := os.MkdirAll(wtParent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoDir, ".gitignore"), []byte(".flowpilot/worktrees/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	wtPath = filepath.Join(wtParent, "wt-owner")
	gitCmd(t, repoDir, "worktree", "add", "-q", wtPath)
	return repoDir, wtPath
}

func TestBUG501_UntrackedBrokenGitLinkErrors(t *testing.T) {
	repoDir, wtPath := bug501Bed(t)
	// Plant uncommitted work, then break the worktree's git linkage.
	if err := os.WriteFile(filepath.Join(wtPath, "secret.txt"), []byte("uncommitted"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(wtPath, ".git"), filepath.Join(wtPath, ".git.bak")); err != nil {
		t.Fatal(err)
	}
	// Owner dir naming: Path() maps ownerID under the worktrees root — lay the
	// same shape via a custom owner id matching the dir name.
	_ = repoDir
	// Call the scan against the real path the manager resolves. Path() takes
	// repoDir/ownerID — emulate by pointing ownerID at wt-owner.
	m := Manager{}
	_, err := m.Untracked(t.Context(), repoDir, "wt-owner", "")
	if err == nil {
		t.Fatal("BUG-501: Untracked returned success on a .git-broken worktree — parent-scope confusion")
	}
}

func TestBUG501_UncommittedBrokenGitLinkErrors(t *testing.T) {
	repoDir, wtPath := bug501Bed(t)
	if err := os.WriteFile(filepath.Join(wtPath, "secret.txt"), []byte("uncommitted"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(wtPath, ".git"), filepath.Join(wtPath, ".git.bak")); err != nil {
		t.Fatal(err)
	}
	m := Manager{}
	_, err := m.Uncommitted(t.Context(), repoDir, "wt-owner", "")
	if err == nil {
		t.Fatal("BUG-501: Uncommitted returned success on a .git-broken worktree")
	}
}

func TestBUG501_DiffBrokenGitLinkErrors(t *testing.T) {
	repoDir, wtPath := bug501Bed(t)
	if err := os.WriteFile(filepath.Join(wtPath, "secret.txt"), []byte("uncommitted"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(wtPath, ".git"), filepath.Join(wtPath, ".git.bak")); err != nil {
		t.Fatal(err)
	}
	m := Manager{}
	_, err := m.Diff(t.Context(), repoDir, "wt-owner", "")
	if err == nil {
		t.Fatal("BUG-501: Diff returned success on a .git-broken worktree")
	}
}

func TestBUG501_ValidateBrokenGitLinkFailsClosed(t *testing.T) {
	repoDir, wtPath := bug501Bed(t)
	m := Manager{}
	// Healthy: registered + dir + .git → Validate must pass (sidecar may be
	// absent — Validate also requires it, so create it first).
	if err := os.WriteFile(BaseSidecar(repoDir, "wt-owner", ""), []byte("deadbeef"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(t.Context(), repoDir, "wt-owner", ""); err != nil {
		t.Fatalf("healthy Validate failed: %v", err)
	}
	// Break the link: dir + registration survive, scope must not.
	if err := os.Rename(filepath.Join(wtPath, ".git"), filepath.Join(wtPath, ".git.bak")); err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(t.Context(), repoDir, "wt-owner", ""); err == nil {
		t.Fatal("BUG-501: Validate passed on a .git-broken worktree — binding must be reported lost")
	}
}

// Healthy control: intact .git link keeps the same functions working.
func TestBUG501_HealthyWorktreeStillScans(t *testing.T) {
	repoDir, wtPath := bug501Bed(t)
	if err := os.WriteFile(filepath.Join(wtPath, "secret.txt"), []byte("uncommitted"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := Manager{}
	untracked, err := m.Untracked(t.Context(), repoDir, "wt-owner", "")
	if err != nil {
		t.Fatalf("healthy Untracked errored: %v", err)
	}
	if len(untracked) == 0 {
		t.Fatal("healthy Untracked missed planted untracked file")
	}
}
