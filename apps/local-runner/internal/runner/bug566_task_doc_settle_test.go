package runner

// BUG-566 (live run-69320): Task-024's sprint converged — reviewer cohort
// unanimously approved, validation green — but the audit gate parked on
// task_referenced because the Task doc never appeared in the aggregate
// diff: nothing touched requirements/08-Task/todo/Task-024-*.md during the
// sprint (it was committed earlier and stampVibeTaskInProgress no-op'd on an
// unchanged status). The only release was an operator-side `git mv` into
// done/. The flow itself must own the todo→done transition: when a vibe
// sprint reaches audit, the sprint's Task doc settles into
// requirements/08-Task/done/ BEFORE the aggregate diff is observed, so
// task_referenced is satisfied by the move the flow made — never by a human.
//
// New file; no pre-existing test is modified.

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/flowgate"
)

const bug566TaskDocBody = "# Task: Bridge\n\n## Metadata\n\n- Status: `todo`\n\n## Definition of Done\n\n- [ ] engine implemented\n"

func writeBug566TaskDoc(t *testing.T, cwd, rel string) string {
	t.Helper()
	abs := filepath.Join(cwd, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(bug566TaskDocBody), 0o644); err != nil {
		t.Fatal(err)
	}
	return abs
}

// Pure settle: todo/ file moves to done/, contents preserved byte-for-byte,
// returned rel path points at done/.
func TestBug566_SettleMovesTaskDocTodoToDone(t *testing.T) {
	cwd := t.TempDir()
	writeBug566TaskDoc(t, cwd, "requirements/08-Task/todo/Task-024-bridge.md")

	got := settleVibeTaskDocDone(cwd, "requirements/08-Task/todo/Task-024-bridge.md")
	if got != "requirements/08-Task/done/Task-024-bridge.md" {
		t.Fatalf("settleVibeTaskDocDone = %q, want done/ rel path", got)
	}
	if _, err := os.Stat(filepath.Join(cwd, "requirements/08-Task/todo/Task-024-bridge.md")); !os.IsNotExist(err) {
		t.Fatalf("todo/ file must be gone after settle, stat err = %v", err)
	}
	b, err := os.ReadFile(filepath.Join(cwd, "requirements/08-Task/done/Task-024-bridge.md"))
	if err != nil {
		t.Fatalf("done/ file must exist: %v", err)
	}
	if string(b) != bug566TaskDocBody {
		t.Fatalf("settle must preserve contents byte-for-byte, got %q", b)
	}
}

// Idempotent + no-clobber: an already-settled doc returns its done/ path and
// an existing done/ file is never overwritten by a stale todo/ twin.
func TestBug566_SettleIdempotentAndNoClobber(t *testing.T) {
	cwd := t.TempDir()
	doneAbs := writeBug566TaskDoc(t, cwd, "requirements/08-Task/done/Task-024-bridge.md")

	// Already settled (source gone): returns the done/ rel path.
	got := settleVibeTaskDocDone(cwd, "requirements/08-Task/todo/Task-024-bridge.md")
	if got != "requirements/08-Task/done/Task-024-bridge.md" {
		t.Fatalf("already-settled must report done/ path, got %q", got)
	}

	// A stale todo/ twin must not overwrite the settled file.
	writeBug566TaskDoc(t, cwd, "requirements/08-Task/todo/Task-024-bridge.md")
	if err := os.WriteFile(doneAbs, []byte("# settled truth\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = settleVibeTaskDocDone(cwd, "requirements/08-Task/todo/Task-024-bridge.md")
	b, err := os.ReadFile(doneAbs)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "# settled truth\n" {
		t.Fatalf("existing done/ doc must not be clobbered, got %q", b)
	}
}

// Non-todo paths, missing sources and empty input settle nothing.
func TestBug566_SettleRejectsNonTodoAndMissing(t *testing.T) {
	cwd := t.TempDir()
	if got := settleVibeTaskDocDone(cwd, "requirements/08-Task/done/Task-024-bridge.md"); got != "" {
		t.Fatalf("non-todo path must not settle, got %q", got)
	}
	if got := settleVibeTaskDocDone(cwd, "requirements/08-Task/todo/Task-999-missing.md"); got != "" {
		t.Fatalf("missing source must not settle, got %q", got)
	}
	if got := settleVibeTaskDocDone("", "requirements/08-Task/todo/Task-024-bridge.md"); got != "" {
		t.Fatalf("empty cwd must not settle, got %q", got)
	}
}

// The audit seam: when a vibe sprint reaches runAuditNode, its current plan
// task doc settles todo→done BEFORE the aggregate diff is observed — so the
// tier-3 task_referenced rule sees the doc the flow itself moved.
func TestBug566_AuditSettlesCurrentSprintTaskDoc(t *testing.T) {
	svc, runID, nodes, edges, auditNode := ca1096Setup(t, 0, true)

	svc.mu.Lock()
	rs := svc.runs[runID]
	workspace := rs.workspaceCwd
	rs.vibeTaskPlan = []string{"requirements/08-Task/todo/Task-024-bridge.md"}
	rs.vibeSprintIndex = 1
	svc.mu.Unlock()

	writeBug566TaskDoc(t, workspace, "requirements/08-Task/todo/Task-024-bridge.md")

	svc.runAuditNode(context.Background(), runID, edges, nodes, auditNode, "sprint work done")

	doneAbs := filepath.Join(workspace, "requirements/08-Task/done/Task-024-bridge.md")
	if _, err := os.Stat(doneAbs); err != nil {
		t.Fatalf("audit entry must settle the sprint task doc todo→done: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workspace, "requirements/08-Task/todo/Task-024-bridge.md")); !os.IsNotExist(err) {
		t.Fatal("todo/ doc must be moved, not copied")
	}

	// The moved doc must be visible to the same observation the audit gate
	// uses — this is the contract that failed live.
	diff, err := flowgate.ObserveGitDiff(workspace)
	if err != nil {
		t.Fatalf("observe: %v", err)
	}
	if !flowgate.HasTaskDoc(diff) {
		t.Fatal("settled task doc must appear in the observed diff (task_referenced gate)")
	}

	// The plan cursor must track the settled location so downstream status
	// reads (cursor reconcile, boundary DoD stamping) resolve the moved file.
	svc.mu.Lock()
	planEntry := svc.runs[runID].vibeTaskPlan[0]
	svc.mu.Unlock()
	if planEntry != "requirements/08-Task/done/Task-024-bridge.md" {
		t.Fatalf("vibeTaskPlan[0] = %q, want done/ path after settle", planEntry)
	}
}

// No sprint in flight (index 0) → audit settles nothing. The task doc stays
// exactly where the plan put it.
func TestBug566_AuditDoesNotSettleBeforeFirstSprint(t *testing.T) {
	svc, runID, nodes, edges, auditNode := ca1096Setup(t, 0, true)

	svc.mu.Lock()
	rs := svc.runs[runID]
	workspace := rs.workspaceCwd
	rs.vibeTaskPlan = []string{"requirements/08-Task/todo/Task-024-bridge.md"}
	rs.vibeSprintIndex = 0
	svc.mu.Unlock()

	writeBug566TaskDoc(t, workspace, "requirements/08-Task/todo/Task-024-bridge.md")

	svc.runAuditNode(context.Background(), runID, edges, nodes, auditNode, "pre-sprint audit")

	if _, err := os.Stat(filepath.Join(workspace, "requirements/08-Task/todo/Task-024-bridge.md")); err != nil {
		t.Fatalf("doc must stay in todo/ before any sprint: %v", err)
	}
}

// After the settle, the boundary DoD stamp must still find the doc — the
// todo→done move must not strand completion bookkeeping.
func TestBug566_BoundaryDoDStampFollowsSettledDoc(t *testing.T) {
	cwd := t.TempDir()
	writeBug566TaskDoc(t, cwd, "requirements/08-Task/todo/Task-024-bridge.md")

	settled := settleVibeTaskDocDone(cwd, "requirements/08-Task/todo/Task-024-bridge.md")
	if settled == "" {
		t.Fatal("settle failed")
	}
	stampCompletedVibeTask(cwd, []string{"requirements/08-Task/todo/Task-024-bridge.md"}, 1)

	b, err := os.ReadFile(filepath.Join(cwd, "requirements/08-Task/done/Task-024-bridge.md"))
	if err != nil {
		t.Fatalf("settled doc unreadable: %v", err)
	}
	if !strings.Contains(string(b), "- [x] engine implemented") {
		t.Fatalf("DoD box must be ticked on the settled doc, got:\n%s", b)
	}
}
