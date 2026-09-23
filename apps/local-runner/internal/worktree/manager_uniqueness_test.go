package worktree

// Task-424 (CP-82 P-3): executable proof that parallel same-repo runs can
// never share a worktree — owner-keyed paths, owner-suffixed branches, and
// fail-closed collision behavior. Additive only; uses the repo()/git()
// helpers from manager_test.go (real temp git repos, no mocks).

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestManager_CreateDistinctOwnersGetDistinctPaths(t *testing.T) {
	r := repo(t)
	base := git(t, r, "rev-parse", "HEAD")
	m := NewManager()
	ctx := context.Background()

	// Same slug on purpose — two parallel runs may share a title; the owner
	// key is what must separate them.
	a, err := m.Create(ctx, r, "chat-aaa", "", base, "same-slug")
	if err != nil {
		t.Fatalf("create a: %v", err)
	}
	b, err := m.Create(ctx, r, "run-bbb", "", base, "same-slug")
	if err != nil {
		t.Fatalf("create b: %v", err)
	}
	if a.Path == b.Path {
		t.Fatalf("distinct owners share path %q", a.Path)
	}
	if a.Branch == b.Branch {
		t.Fatalf("distinct owners share branch %q", a.Branch)
	}
}

func TestManager_CreateFailsClosedOnExistingPath(t *testing.T) {
	r := repo(t)
	base := git(t, r, "rev-parse", "HEAD")
	m := NewManager()
	ctx := context.Background()

	first, err := m.Create(ctx, r, "owner-x", "", base, "feat")
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := m.Create(ctx, r, "owner-x", "", base, "feat"); err == nil {
		t.Fatal("second create for the same owner must fail closed")
	} else if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("expected 'already exists' collision error, got %v", err)
	}
	// The original worktree is untouched and still registered.
	if _, err := os.Stat(first.Path); err != nil {
		t.Fatalf("original worktree missing after failed second create: %v", err)
	}
	out := git(t, r, "worktree", "list", "--porcelain")
	if strings.Count(out, "worktree ") != 2 { // repo root + the one worktree
		t.Fatalf("expected exactly one managed worktree, porcelain shows: %s", out)
	}
}

func TestManager_BranchNamesIncludeOwnerSuffix(t *testing.T) {
	r := repo(t)
	base := git(t, r, "rev-parse", "HEAD")
	m := NewManager()

	// Callers pass a pre-slugified value (Slugify is the runner's job).
	info, err := m.Create(context.Background(), r, "ownerid-1234567890", "", base, "my-feature")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	// Branch is fp/<slug>-<last8 of ownerID> — the owner suffix is what keeps
	// same-slug parallel runs on distinct branches.
	wantSuffix := "ownerid-1234567890"[len("ownerid-1234567890")-8:]
	if !strings.HasPrefix(info.Branch, "fp/my-feature-") {
		t.Fatalf("branch %q missing fp/<slug>- prefix", info.Branch)
	}
	if !strings.HasSuffix(info.Branch, wantSuffix) {
		t.Fatalf("branch %q missing owner suffix %q", info.Branch, wantSuffix)
	}
}
