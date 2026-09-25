package changeledger

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// BUG-416: installing the post-commit hook in a directory that is not a git
// work tree must NOT fabricate a .git/ skeleton — it must report a truthful
// "not a git repository" outcome instead.
func TestBug416_HookInstallSkipsNonGitDir(t *testing.T) {
	dir := t.TempDir()

	err := InstallPostCommitHook(dir)
	if !errors.Is(err, ErrNotGitRepo) {
		t.Fatalf("InstallPostCommitHook on non-git dir: err=%v, want ErrNotGitRepo", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, ".git")); !os.IsNotExist(statErr) {
		t.Fatalf(".git was fabricated in a non-git directory (stat err=%v)", statErr)
	}
}

// BUG-416: a linked worktree exposes .git as a FILE containing "gitdir: <path>"
// — the hook must land in the real git dir's hooks/, not fail on mkdir of a
// .git/ directory that cannot exist.
func TestBug416_HookInstallResolvesWorktreeGitdir(t *testing.T) {
	worktree := t.TempDir()
	realGitDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(realGitDir, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, ".git"),
		[]byte("gitdir: "+realGitDir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := InstallPostCommitHook(worktree); err != nil {
		t.Fatalf("InstallPostCommitHook on worktree: %v", err)
	}
	hook := filepath.Join(realGitDir, "hooks", "post-commit")
	if _, err := os.Stat(hook); err != nil {
		t.Fatalf("hook not installed at real gitdir hooks/: %v", err)
	}
}
