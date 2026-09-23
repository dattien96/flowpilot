package worktree

// Task-407 test signatures — full lifecycle matrix on real temp git repos
// (Task-369 convention: real git, no mocks).

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func repo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	git(t, dir, "init")
	git(t, dir, "config", "user.email", "test@flowpilot")
	git(t, dir, "config", "user.name", "flowpilot-test")
	git(t, dir, "config", "core.autocrlf", "false")
	if err := os.WriteFile(filepath.Join(dir, "seed.txt"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-m", "seed")
	return dir
}

func TestManager_CreateIsolatesOwners(t *testing.T) {
	r := repo(t)
	base := git(t, r, "rev-parse", "HEAD")
	m := NewManager()
	ctx := context.Background()

	a, err := m.Create(ctx, r, "owner-a", "", base, "feat-a")
	if err != nil {
		t.Fatalf("create a: %v", err)
	}
	b, err := m.Create(ctx, r, "owner-b", "", base, "feat-b")
	if err != nil {
		t.Fatalf("create b: %v", err)
	}
	if a.Path == b.Path {
		t.Fatal("owners share a path")
	}
	if a.Branch == "" || !strings.HasPrefix(a.Branch, "fp/") {
		t.Fatalf("branch=%q want fp/…", a.Branch)
	}
	if a.BaseCommit != base {
		t.Fatalf("base=%s want %s", a.BaseCommit, base)
	}
	// Write in A is invisible in B and main.
	if err := os.WriteFile(filepath.Join(a.Path, "a-only.txt"), []byte("a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{b.Path, r} {
		if _, err := os.Stat(filepath.Join(p, "a-only.txt")); !os.IsNotExist(err) {
			t.Fatalf("a-only.txt leaked into %s", p)
		}
	}
}

func TestManager_CreateRejectsInvalidOwnerID(t *testing.T) {
	r := repo(t)
	base := git(t, r, "rev-parse", "HEAD")
	m := NewManager()
	for _, bad := range []string{"", ".", "..", "a/b", `a\b`, "..x", "x..y"} {
		if _, err := m.Create(context.Background(), r, bad, "", base, ""); err == nil {
			t.Fatalf("id %q must be rejected", bad)
		}
	}
	if _, err := m.Create(context.Background(), r, "ok", "", base, ""); err != nil {
		t.Fatalf("valid id rejected: %v", err)
	}
}

func TestManager_CreateDetachedWhenNoSlug(t *testing.T) {
	r := repo(t)
	base := git(t, r, "rev-parse", "HEAD")
	info, err := NewManager().Create(context.Background(), r, "cand-1", "candidate-", base, "")
	if err != nil {
		t.Fatal(err)
	}
	if info.Branch != "" {
		t.Fatalf("detached worktree should have no branch, got %q", info.Branch)
	}
	if !strings.HasSuffix(info.Path, "candidate-cand-1") {
		t.Fatalf("path=%s want candidate- prefix", info.Path)
	}
	if got := git(t, info.Path, "rev-parse", "--abbrev-ref", "HEAD"); got != "HEAD" {
		t.Fatalf("expected detached HEAD, got %s", got)
	}
}

func TestManager_DiffIncludesUntracked(t *testing.T) {
	r := repo(t)
	base := git(t, r, "rev-parse", "HEAD")
	m := NewManager()
	info, _ := m.Create(context.Background(), r, "o1", "", base, "x")
	if err := os.WriteFile(filepath.Join(info.Path, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(info.Path, "seed.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	patch, err := m.Diff(context.Background(), r, "o1", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(patch), "new.txt") || !strings.Contains(string(patch), "seed.txt") {
		t.Fatalf("patch missing files:\n%s", patch)
	}
}

func TestManager_ResolveApplyPatchClean(t *testing.T) {
	r := repo(t)
	base := git(t, r, "rev-parse", "HEAD")
	headBefore := base
	m := NewManager()
	info, _ := m.Create(context.Background(), r, "o1", "", base, "x")
	if err := os.WriteFile(filepath.Join(info.Path, "feature.txt"), []byte("work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := m.Apply(context.Background(), r, "o1", ""); err != nil {
		t.Fatalf("apply: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(r, "feature.txt"))
	if err != nil || string(got) != "work\n" {
		t.Fatalf("patch not applied: %v %q", err, got)
	}
	if git(t, r, "rev-parse", "HEAD") != headBefore {
		t.Fatal("apply must not create a commit")
	}
}

func TestManager_ResolveConflictErrorCarriesEvidence(t *testing.T) {
	r := repo(t)
	base := git(t, r, "rev-parse", "HEAD")
	m := NewManager()
	info, _ := m.Create(context.Background(), r, "o1", "", base, "x")
	if err := os.WriteFile(filepath.Join(info.Path, "seed.txt"), []byte("wt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Drift: main commits a different version.
	if err := os.WriteFile(filepath.Join(r, "seed.txt"), []byte("main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, r, "add", ".")
	git(t, r, "commit", "-m", "drift")

	err := m.Apply(context.Background(), r, "o1", "")
	var conflict *MergeConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("want MergeConflictError, got %v", err)
	}
	if conflict.OwnerID != "o1" || conflict.Reason == "" {
		t.Fatalf("bad evidence: %+v", conflict)
	}
	// Worktree preserved (caller decides lifecycle).
	if _, err := os.Stat(info.Path); err != nil {
		t.Fatal("worktree must survive conflict")
	}
}

func TestManager_CleanupIdempotent(t *testing.T) {
	r := repo(t)
	base := git(t, r, "rev-parse", "HEAD")
	m := NewManager()
	info, _ := m.Create(context.Background(), r, "o1", "", base, "keep-me")
	if err := m.Cleanup(context.Background(), r, "o1", "", false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(info.Path); !os.IsNotExist(err) {
		t.Fatal("worktree dir still present")
	}
	if _, err := os.Stat(BaseSidecar(r, "o1", "")); !os.IsNotExist(err) {
		t.Fatal("sidecar still present")
	}
	branches := git(t, r, "branch")
	if strings.Contains(branches, info.Branch) {
		t.Fatal("branch not deleted on cleanup")
	}
	if err := m.Cleanup(context.Background(), r, "o1", "", false); err != nil {
		t.Fatal("second cleanup must be no-op")
	}
	if !strings.Contains(normalizePath(git(t, r, "worktree", "list", "--porcelain")), normalizePath(r)) {
		t.Fatal("main worktree vanished")
	}
}

func TestManager_CleanupKeepBranch(t *testing.T) {
	r := repo(t)
	base := git(t, r, "rev-parse", "HEAD")
	m := NewManager()
	info, _ := m.Create(context.Background(), r, "o1", "", base, "keep-me")
	if err := m.Cleanup(context.Background(), r, "o1", "", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(info.Path); !os.IsNotExist(err) {
		t.Fatal("worktree dir still present")
	}
	if !strings.Contains(git(t, r, "branch"), info.Branch) {
		t.Fatalf("branch %s must survive keep_branch", info.Branch)
	}
}

func TestManager_ValidateDetectsExternalDelete(t *testing.T) {
	r := repo(t)
	base := git(t, r, "rev-parse", "HEAD")
	m := NewManager()
	info, _ := m.Create(context.Background(), r, "o1", "", base, "x")
	if err := m.Validate(context.Background(), r, "o1", ""); err != nil {
		t.Fatalf("valid binding rejected: %v", err)
	}
	git(t, r, "worktree", "remove", "--force", info.Path)
	if err := m.Validate(context.Background(), r, "o1", ""); err == nil {
		t.Fatal("externally-deleted worktree must fail validation")
	}
	if _, err := os.Stat(info.Path); err == nil {
		t.Fatal("validate must not recreate the worktree")
	}
}

func TestManager_SerializedApplyMutex(t *testing.T) {
	r := repo(t)
	base := git(t, r, "rev-parse", "HEAD")
	m := NewManager()
	for _, id := range []string{"o1", "o2"} {
		info, err := m.Create(context.Background(), r, id, "", base, "s-"+id)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(info.Path, id+".txt"), []byte(id+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	done := make(chan error, 2)
	for _, id := range []string{"o1", "o2"} {
		go func(id string) { done <- m.Apply(context.Background(), r, id, "") }(id)
	}
	for i := 0; i < 2; i++ {
		if err := <-done; err != nil {
			t.Fatalf("serialized apply failed: %v", err)
		}
	}
	for _, f := range []string{"o1.txt", "o2.txt"} {
		if _, err := os.Stat(filepath.Join(r, f)); err != nil {
			t.Fatalf("%s not applied", f)
		}
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Fix Login Bug":     "fix-login-bug",
		"  Hello, World!! ": "hello-world",
		"CP-71: Worktree!":  "cp-71-worktree",
		"---":               "",
		"":                  "",
		"averyveryverylongtitlethatexceedsfortycharactersindeed": "averyveryverylongtitlethatexceedsfortych",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q)=%q want %q", in, got, want)
		}
	}
}

// BUG-378: work committed INSIDE the worktree on its branch (providers can
// `git commit` when instructed) is invisible to a bare `git diff` — the
// merge-back reported applied:true while silently dropping the commit.
// Diff must cover committed-branch + staged + unstaged + untracked vs the
// recorded base commit.
func TestManager_DiffIncludesCommittedBranchWork(t *testing.T) {
	r := repo(t)
	base := git(t, r, "rev-parse", "HEAD")
	m := NewManager()
	info, err := m.Create(context.Background(), r, "o1", "", base, "x")
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a provider committing inside the worktree.
	if err := os.WriteFile(filepath.Join(info.Path, "committed.txt"), []byte("committed work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, info.Path, "add", "committed.txt")
	git(t, info.Path, "commit", "-m", "provider commit inside worktree")
	// Plus an uncommitted change for good measure.
	if err := os.WriteFile(filepath.Join(info.Path, "seed.txt"), []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	patch, err := m.Diff(context.Background(), r, "o1", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(patch), "committed.txt") {
		t.Fatalf("committed branch work dropped from patch:\n%s", patch)
	}
	if !strings.Contains(string(patch), "seed.txt") {
		t.Fatalf("uncommitted work dropped from patch:\n%s", patch)
	}
}

func TestManager_ApplyCarriesCommittedBranchWork(t *testing.T) {
	r := repo(t)
	base := git(t, r, "rev-parse", "HEAD")
	m := NewManager()
	info, err := m.Create(context.Background(), r, "o2", "", base, "x")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(info.Path, "committed.txt"), []byte("committed work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, info.Path, "add", "committed.txt")
	git(t, info.Path, "commit", "-m", "provider commit inside worktree")
	if err := m.ApplyWithOptions(context.Background(), r, "o2", "", ApplyOptions{StrictHead: false}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(r, "committed.txt"))
	if err != nil || string(got) != "committed work\n" {
		t.Fatalf("committed worktree change not applied: %v %q", err, got)
	}
}
