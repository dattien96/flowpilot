package runner

import (
	"os"
	"path/filepath"
	"testing"
)

// BUG-460 (live run-21364): createRun fires ensureGitNexusIndexAsync on the
// child's workspaceCwd — for tournament candidates that is the candidate
// worktree under .flowpilot/worktrees/. `gitnexus analyze` then stamps the
// <!-- gitnexus --> headers into the worktree's tracked AGENTS.md/CLAUDE.md
// (and .gitnexus knowledge writes add untracked files), polluting the
// candidate's captured patch. In run-21364 the codex candidate "won" on a
// diff that was nothing but this index stamp, and the merge wrote
// "indexed as candidate-candidate-b" into the MAIN workspace's AGENTS.md.
// Managed worktrees are runner scratch — never an index target.
func TestBug460AutoIndexSkipsManagedWorktree(t *testing.T) {
	repo := t.TempDir()
	wt := filepath.Join(repo, ".flowpilot", "worktrees", "candidate-x")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	// A git worktree carries a .git file (gitdir pointer), not a directory —
	// os.Stat passes either way, which is exactly how the worktree reached
	// the indexer in the first place.
	if err := os.WriteFile(filepath.Join(wt, ".git"), []byte("gitdir: /tmp/x"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	svc.ensureGitNexusIndexAsync(wt)
	svc.mu.Lock()
	marked := svc.gitnexusAnalyzeOnce[wt]
	svc.mu.Unlock()
	if marked {
		t.Fatal("auto-index must never fire on a runner-managed worktree — its stamp lands in the candidate's patch")
	}
}

func TestIsRunnerManagedWorktreePath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/repo/.flowpilot/worktrees/candidate-a", true},
		{"/repo/.flowpilot/worktrees/candidate-a/sub", true},
		{"/repo/.flowpilot/worktrees/", true},
		{"/repo", false},
		{"/repo/.flowpilot", false},
		{"/repo/.flowpilotXworktrees/x", false},
		{"/repo/.flowpilot/worktrees", true},
		{"", false},
	}
	for _, tc := range cases {
		if got := isRunnerManagedWorktreePath(tc.path); got != tc.want {
			t.Fatalf("isRunnerManagedWorktreePath(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
