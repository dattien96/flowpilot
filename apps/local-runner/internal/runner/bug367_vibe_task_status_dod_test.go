package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/workingmode"
)

func writeVibeTaskDoc(t *testing.T, cwd, rel, status string) string {
	t.Helper()
	abs := filepath.Join(cwd, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	body := strings.Join([]string{
		"---",
		"id: Task-910",
		"status: " + status,
		"---",
		"",
		"## Metadata",
		"",
		"- Status: `" + status + "`",
		"",
		"## 6. Acceptance Check",
		"",
		"- [ ] go test ./snake green",
		"- [ ] feature key registered",
		"",
		"## 7. Out of Scope",
		"",
		"- [ ] sibling P-2 (must stay unchecked)",
		"",
	}, "\n")
	if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return abs
}

func TestBUG367_StampTaskInProgressNeverDone(t *testing.T) {
	cwd := t.TempDir()
	rel := "requirements/08-Task/todo/Task-910-core.md"
	abs := writeVibeTaskDoc(t, cwd, rel, "draft")
	stampVibeTaskInProgress(cwd, rel)
	got := readFileString(t, abs)
	if !strings.Contains(got, "status: in_progress") {
		t.Fatalf("frontmatter: %s", got)
	}
	if !strings.Contains(got, "- Status: `in_progress`") {
		t.Fatalf("metadata: %s", got)
	}
	if strings.Contains(got, "status: done") || strings.Contains(got, "`done`") {
		t.Fatalf("must not stamp done: %s", got)
	}
}

func TestBUG367_StampDoDTicksAcceptanceNotOutOfScope(t *testing.T) {
	cwd := t.TempDir()
	rel := "requirements/08-Task/todo/Task-910-core.md"
	abs := writeVibeTaskDoc(t, cwd, rel, "done")
	stampVibeTaskDoDChecked(cwd, rel)
	got := readFileString(t, abs)
	if strings.Contains(got, "status: done") {
		t.Fatalf("completed sprint must rewrite done → in_progress: %s", got)
	}
	if !strings.Contains(got, "status: in_progress") {
		t.Fatalf("want in_progress, got %s", got)
	}
	if !strings.Contains(got, "- [x] go test ./snake green") {
		t.Fatalf("DoD not ticked: %s", got)
	}
	if !strings.Contains(got, "- [ ] sibling P-2 (must stay unchecked)") {
		t.Fatalf("out-of-scope checkbox must stay open: %s", got)
	}
}

func TestBUG367_CPLockStampsApprovedNotDone(t *testing.T) {
	cwd := t.TempDir()
	dir := filepath.Join(cwd, "requirements", "07-Coding-Plan", "todo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(dir, "CP-01-snake-mvp.md")
	body := "---\nid: CP-01\nstatus: draft\n---\n\n- Status: `draft`\n\n## 10. Definition of Done\n\n- [ ] CP locked\n- [ ] all P-* implemented\n"
	if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	stampVibeCPApproved(cwd)
	got := readFileString(t, abs)
	if !strings.Contains(got, "status: approved") {
		t.Fatalf("CP lock must stamp approved: %s", got)
	}
	if strings.Contains(got, "status: done") {
		t.Fatalf("CP must not be done: %s", got)
	}
	if !strings.Contains(got, "- [ ] CP locked") {
		t.Fatalf("lock must not tick DoD yet: %s", got)
	}
}

func TestBUG367_LastSprintTicksTaskAndCPDoD(t *testing.T) {
	cwd := t.TempDir()
	rel := "requirements/08-Task/todo/Task-912.md"
	writeVibeTaskDoc(t, cwd, rel, "in_progress")
	cpDir := filepath.Join(cwd, "requirements", "07-Coding-Plan", "todo")
	if err := os.MkdirAll(cpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cpAbs := filepath.Join(cpDir, "CP-01-snake-mvp.md")
	if err := os.WriteFile(cpAbs, []byte("---\nstatus: approved\n---\n\n## 10. Definition of Done\n\n- [ ] all P-* implemented\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := []string{rel}
	stampCompletedVibeTask(cwd, plan, 1)
	task := readFileString(t, filepath.Join(cwd, filepath.FromSlash(rel)))
	if !strings.Contains(task, "- [x] go test ./snake green") {
		t.Fatalf("last sprint task DoD: %s", task)
	}
	cp := readFileString(t, cpAbs)
	if !strings.Contains(cp, "- [x] all P-* implemented") {
		t.Fatalf("last sprint CP DoD: %s", cp)
	}
	if strings.Contains(cp, "status: done") || strings.Contains(task, "status: done") {
		t.Fatal("last sprint must not mark docs done")
	}
}

func TestBUG367_BoundaryParkTicksCompletedTaskDoD(t *testing.T) {
	svc, _ := newTestServer(t)
	cwd := t.TempDir()
	rel := "requirements/08-Task/todo/Task-904-sprint1.md"
	writeVibeTaskDoc(t, cwd, rel, "in_progress")
	plan := []string{rel, "requirements/08-Task/todo/Task-905.md"}
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, plan, 1)
	svc.mu.Lock()
	svc.runs[runID].workspaceCwd = cwd
	svc.mu.Unlock()

	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit", false) {
		t.Fatal("want boundary park with tasks remaining")
	}
	got := readFileString(t, filepath.Join(cwd, filepath.FromSlash(rel)))
	if !strings.Contains(got, "- [x] go test ./snake green") {
		t.Fatalf("completed sprint DoD not ticked: %s", got)
	}
	if strings.Contains(got, "status: done") {
		t.Fatalf("completed sprint status must not be done: %s", got)
	}
}

func TestBUG367_GraphSnapshotCarriesTaskProgress(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.vibeTaskPlan = []string{
		"requirements/08-Task/todo/Task-910-core.md",
		"requirements/08-Task/todo/Task-911-tick.md",
		"requirements/08-Task/todo/Task-912-play.md",
	}
	rs.vibeSprintIndex = 1
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	snap := svc.agentGraphSnapshot(parent.RunID)
	if snap.LoopState.VibeTaskIndex != 1 || snap.LoopState.VibeTaskTotal != 3 {
		t.Fatalf("progress=%d/%d want 1/3", snap.LoopState.VibeTaskIndex, snap.LoopState.VibeTaskTotal)
	}
	if snap.LoopState.VibeTaskName != "Task-910-core.md" {
		t.Fatalf("name=%q", snap.LoopState.VibeTaskName)
	}
}

func TestBUG367_VibeTaskProgressTable(t *testing.T) {
	plan := []string{"a/Task-1.md", "a/Task-2.md", "a/Task-3.md"}
	cur, total, name := vibeTaskProgress(plan, 2)
	if cur != 2 || total != 3 || name != "Task-2.md" {
		t.Fatalf("got %d/%d %q", cur, total, name)
	}
	cur, total, name = vibeTaskProgress(plan, 0)
	if cur != 0 || total != 3 || name != "" {
		t.Fatalf("unstarted got %d/%d %q", cur, total, name)
	}
}

func readFileString(t *testing.T, abs string) string {
	t.Helper()
	b, err := os.ReadFile(abs)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
