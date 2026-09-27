package runner

// Review finding: audit completions inside a runner-managed worktree still
// mutate knowledge state — ensureKnowledgeBaseForWorkspace guards the
// bootstrap path, but updateKnowledgeForAudit had no equivalent check, so
// an audit node finishing inside .flowpilot/worktrees/* wrote the
// pending-update ledger and fired Distill into scratch space (BUG-460 class:
// those writes land in the candidate's captured diff).

import (
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/knowledge"
)

func TestAuditKnowledgeUpdateSkipsRunnerWorktree(t *testing.T) {
	wt := filepath.Join(t.TempDir(), ".flowpilot", "worktrees", "cand-a")
	// Bootstrap a KB inside the worktree so knowledge.Missing does not
	// short-circuit the hook — the guard must fire on the path, not on
	// incidental state.
	kbDir := knowledge.KnowledgeDir(wt)
	if err := os.MkdirAll(kbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(kbDir, "index.json"), []byte(`{"schemaVersion":1,"flows":[],"models":[]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if knowledge.Missing(wt) {
		t.Fatal("fixture KB must exist — Missing() must be false")
	}

	svc := NewInteractiveService()
	// Real entry: onAuditNodeCompleted is the single choke point every audit
	// completion path calls (auto-finalize, successor advance, flow done).
	svc.onAuditNodeCompleted(wt, []string{"main.go"})

	if _, err := os.Stat(knowledgePendingPath(wt)); !os.IsNotExist(err) {
		t.Fatal("audit hook must not write the pending-update ledger inside a runner worktree")
	}
}

func TestEnsureKnowledgeBaseSkipsRunnerWorktree(t *testing.T) {
	wt := filepath.Join(t.TempDir(), ".flowpilot", "worktrees", "cand-a")
	svc := NewInteractiveService()

	svc.ensureKnowledgeBaseForWorkspace(wt)

	// The guard must return before registering the bootstrap — a deferred
	// Distill that later lands inside the worktree is the pollution vector.
	if _, loaded := knowledgeBootstrapOnce.Load(wt); loaded {
		t.Fatal("runner-managed worktree must never enqueue a knowledge bootstrap")
	}
	knowledgeBootstrapOnce.Delete(wt) // global map — don't leak into other tests
}
