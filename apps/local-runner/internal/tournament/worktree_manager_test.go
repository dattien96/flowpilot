package tournament

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func tournamentGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := gitOut(dir, args...)
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return out
}

// initTournamentTestRepo builds a real git repo with one committed file.
// core.autocrlf/eol are pinned so byte comparisons stay stable on Windows.
func initTournamentTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	tournamentGit(t, dir, "init")
	tournamentGit(t, dir, "config", "user.email", "tournament@test.local")
	tournamentGit(t, dir, "config", "user.name", "tournament-test")
	tournamentGit(t, dir, "config", "core.autocrlf", "false")
	tournamentGit(t, dir, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(dir, "base.txt"), []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tournamentGit(t, dir, "add", "base.txt")
	tournamentGit(t, dir, "commit", "-m", "base")
	return dir
}

func baseHead(t *testing.T, dir string) string {
	t.Helper()
	return tournamentGit(t, dir, "rev-parse", "HEAD")
}

func worktreeList(t *testing.T, dir string) string {
	t.Helper()
	return tournamentGit(t, dir, "worktree", "list", "--porcelain")
}

func TestWorktreeManagerCreatesIsolatedWorktrees(t *testing.T) {
	var mgr WorktreeManager
	repo := initTournamentTestRepo(t)
	base := baseHead(t, repo)

	pathA, err := mgr.Create(repo, base, "a")
	if err != nil {
		t.Fatalf("Create a: %v", err)
	}
	pathB, err := mgr.Create(repo, base, "b")
	if err != nil {
		t.Fatalf("Create b: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Cleanup(repo, []string{"a", "b"}) })

	for _, p := range []string{pathA, pathB} {
		if fi, err := os.Stat(p); err != nil || !fi.IsDir() {
			t.Fatalf("worktree dir missing: %s", p)
		}
	}
	// Same base commit in both worktrees.
	for _, p := range []string{pathA, pathB} {
		if got := tournamentGit(t, p, "rev-parse", "HEAD"); got != base {
			t.Fatalf("worktree at %s on %s, want base %s", p, got, base)
		}
	}
	// Write into A: invisible in B and in the main workspace.
	if err := os.WriteFile(filepath.Join(pathA, "a-only.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(pathB, "a-only.txt")); !os.IsNotExist(err) {
		t.Fatal("file written in worktree A leaked into worktree B")
	}
	if _, err := os.Stat(filepath.Join(repo, "a-only.txt")); !os.IsNotExist(err) {
		t.Fatal("file written in worktree A leaked into the main workspace")
	}
	// .gitignore covers the scratch root (additive).
	ignoreRaw, err := os.ReadFile(filepath.Join(repo, ".gitignore"))
	if err != nil || !strings.Contains(string(ignoreRaw), ".flowpilot/worktrees/") {
		t.Fatalf(".gitignore must cover the worktree root, got %q (err %v)", string(ignoreRaw), err)
	}
}

func TestWorktreeManagerCleansUpAfterTournament(t *testing.T) {
	var mgr WorktreeManager
	repo := initTournamentTestRepo(t)
	base := baseHead(t, repo)

	if _, err := mgr.Create(repo, base, "a"); err != nil {
		t.Fatalf("Create a: %v", err)
	}
	if _, err := mgr.Create(repo, base, "b"); err != nil {
		t.Fatalf("Create b: %v", err)
	}
	// Simulate candidate output, tracked + untracked.
	if err := os.WriteFile(filepath.Join(worktreePath(repo, "a"), "a-only.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := mgr.Cleanup(repo, []string{"a", "b"}); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	list := worktreeList(t, repo)
	if strings.Contains(list, "candidate-") {
		t.Fatalf("worktree list must be clean after Cleanup, got:\n%s", list)
	}
	for _, id := range []string{"a", "b"} {
		if _, err := os.Stat(worktreePath(repo, id)); !os.IsNotExist(err) {
			t.Fatalf("worktree dir for %s must be gone", id)
		}
		if _, err := os.Stat(baseSidecar(repo, id)); !os.IsNotExist(err) {
			t.Fatalf("base sidecar for %s must be gone", id)
		}
	}
	// Second call is a no-op (idempotent: verdict-time + flow-done).
	if err := mgr.Cleanup(repo, []string{"a", "b", "never-existed"}); err != nil {
		t.Fatalf("second Cleanup must be a no-op, got %v", err)
	}
}

func TestWorktreeManagerMergesWinningCandidate(t *testing.T) {
	var mgr WorktreeManager
	repo := initTournamentTestRepo(t)
	base := baseHead(t, repo)

	pathW, err := mgr.Create(repo, base, "winner")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Cleanup(repo, []string{"winner"}) })

	// Winner edits a tracked file and adds a new one.
	if err := os.WriteFile(filepath.Join(pathW, "base.txt"), []byte("v1-fixed-by-winner\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pathW, "fix.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	patch, err := mgr.Diff(repo, "winner")
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(patch) == 0 {
		t.Fatal("Diff must be non-empty for a changed worktree")
	}

	if err := mgr.MergeWinner(repo, "winner"); err != nil {
		t.Fatalf("MergeWinner: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(repo, "base.txt"))
	if err != nil || string(got) != "v1-fixed-by-winner\n" {
		t.Fatalf("main workspace must carry the winner fix, got %q (err %v)", string(got), err)
	}
	if _, err := os.Stat(filepath.Join(repo, "fix.txt")); err != nil {
		t.Fatalf("winner new file must land in the main workspace: %v", err)
	}
	// No new commit, no branch/ref touched.
	if got := baseHead(t, repo); got != base {
		t.Fatalf("merge must not move HEAD: was %s now %s", base, got)
	}
	if got := tournamentGit(t, repo, "rev-list", "--count", "HEAD"); got != "1" {
		t.Fatalf("merge must not create commits, count=%s", got)
	}
}

func TestWorktreeManagerMergeConflictKeepsEvidenceAndCleans(t *testing.T) {
	var mgr WorktreeManager
	repo := initTournamentTestRepo(t)
	base := baseHead(t, repo)

	pathW, err := mgr.Create(repo, base, "winner")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Winner and main workspace drift on the same region.
	if err := os.WriteFile(filepath.Join(pathW, "base.txt"), []byte("v1-winner\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "base.txt"), []byte("v1-drifted\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err = mgr.MergeWinner(repo, "winner")
	var conflict *MergeConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("drifted workspace must yield *MergeConflictError, got %v", err)
	}
	if len(conflict.Patch) == 0 {
		t.Fatal("conflict error must carry the winner patch for the ask_user card")
	}
	found := false
	for _, p := range conflict.ConflictPaths {
		if p == "base.txt" {
			found = true
		}
	}
	if !found {
		t.Fatalf("conflict paths must name base.txt, got %q", conflict.ConflictPaths)
	}
	// Main workspace untouched by the failed merge.
	if got, _ := os.ReadFile(filepath.Join(repo, "base.txt")); string(got) != "v1-drifted\n" {
		t.Fatalf("failed merge must not touch the workspace, got %q", string(got))
	}
	// No-orphan policy: worktree already cleaned despite the conflict.
	if _, statErr := os.Stat(worktreePath(repo, "winner")); !os.IsNotExist(statErr) {
		t.Fatal("conflict worktree must be cleaned (evidence lives in the error, not the dir)")
	}
	// Merging again is a clean error, not a panic.
	if err := mgr.MergeWinner(repo, "winner"); err == nil {
		t.Fatal("second MergeWinner on a cleaned worktree must fail")
	}
}

func TestWorktreeManagerRejectsBadInput(t *testing.T) {
	var mgr WorktreeManager
	repo := initTournamentTestRepo(t)
	base := baseHead(t, repo)

	for _, id := range []string{"", ".", "..", "a/b", `a\b`, "a..b"} {
		if _, err := mgr.Create(repo, base, id); err == nil {
			t.Fatalf("Create must reject candidate id %q", id)
		}
	}
	if _, err := mgr.Create(filepath.Join(repo, "nope"), base, "a"); err == nil {
		t.Fatal("Create must fail outside a git repository")
	}
	if _, err := mgr.Create(repo, "deadbeef-dead-beef-dead-beefdeadbeef", "a"); err == nil {
		t.Fatal("Create must fail on an unresolvable base commit")
	}
	if _, err := mgr.Create(repo, base, "dup"); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Cleanup(repo, []string{"dup"}) })
	if _, err := mgr.Create(repo, base, "dup"); err == nil {
		t.Fatal("second Create on the same id must fail")
	}
	if _, err := mgr.Diff(repo, "ghost"); err == nil {
		t.Fatal("Diff on a missing worktree must fail")
	}
	if err := mgr.MergeWinner(repo, "ghost"); err == nil {
		t.Fatal("MergeWinner on a missing worktree must fail")
	}
}
