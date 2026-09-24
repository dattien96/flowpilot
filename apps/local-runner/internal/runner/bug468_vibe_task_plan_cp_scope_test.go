package runner

import (
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// BUG-468: live fp-beds/full run-91517 + run-91606 — collectVibeTaskPlan globs
// every requirements/08-Task/todo/Task-*.md regardless of which CP produced
// them. With stale tasks from earlier features on disk (Task-1/2/8 calc +
// strutil, Parent Documents: none) plus this run's own tasks (Task-12/13/14,
// Parent Documents: CP-02), the sprint plan became all 9 files sorted — and
// both vibe runs sprinted Task-1-calc-gcd-lcm (index 1/9) under a snake CP:
// wrong-feature work, twice, on one bed. The fix scopes the sprint plan (and
// the "this run's tasks" presence checks) to tasks whose `Parent Documents`
// metadata names the CP the run ingested (rs.vibeCpDocID). Empty cpID keeps
// the legacy unscoped glob so older fixtures/runs behave identically.

const bug468TaskCP01 = "# Task\n\n## Metadata\n\n- Document ID: `Task-9`\n- Phase: `task`\n- Status: `draft`\n- Parent Documents: `CP-01` (work item `P-1`), `SS-01`\n"
const bug468TaskCP02 = "# Task\n\n## Metadata\n\n- Document ID: `Task-12`\n- Phase: `task`\n- Status: `draft`\n- Parent Documents: `CP-02` (work item `P-1`), `SS-01`\n- Related Documents: `CP-01` (prior approved plan for the same SS set)\n"
const bug468TaskNoParent = "# Task\n\n## Metadata\n\n- Document ID: `Task-1`\n- Phase: `task`\n- Status: `in_progress`\n- Parent Documents: `none (feature intent supplied via flow context)`\n"

func bug468SeedTasks(t *testing.T, dir string) {
	t.Helper()
	taskDir := filepath.Join(dir, "requirements", "08-Task", "todo")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"Task-1-calc-gcd-lcm.md":      bug468TaskNoParent,
		"Task-9-snake-step.md":        bug468TaskCP01,
		"Task-12-snake-core-model.md": bug468TaskCP02,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(taskDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// The scoped collector returns only tasks parented to the requested CP; a
// Related-Documents mention of another CP must not pull the task in.
func TestBUG468_CollectVibeTaskPlanForCPScopesByParentDocuments(t *testing.T) {
	dir := t.TempDir()
	bug468SeedTasks(t, dir)

	got := collectVibeTaskPlanForCP(dir, "CP-02")
	if len(got) != 1 || filepath.Base(got[0]) != "Task-12-snake-core-model.md" {
		t.Fatalf("CP-02 scope = %v, want only Task-12", got)
	}
	got = collectVibeTaskPlanForCP(dir, "CP-01")
	if len(got) != 1 || filepath.Base(got[0]) != "Task-9-snake-step.md" {
		t.Fatalf("CP-01 scope = %v, want only Task-9 (Task-12's Related mention must not match)", got)
	}
	// Unknown CP → empty (callers gate/park; never silently widen to all).
	if got := collectVibeTaskPlanForCP(dir, "CP-99"); len(got) != 0 {
		t.Fatalf("CP-99 scope = %v, want empty", got)
	}
	// Empty cpID keeps the legacy unscoped glob (fixture/resume compat).
	if got := collectVibeTaskPlanForCP(dir, ""); len(got) != 3 {
		t.Fatalf("empty scope = %v, want all 3 (legacy)", got)
	}
}

// End-to-end through the real done-path: after task_slicer completes on a bed
// holding stale tasks, the sprint plan must contain only this run's CP tasks.
func TestBUG468_SlicerDoneBuildsPlanScopedToRunCP(t *testing.T) {
	svc, _ := newTestServer(t)
	dir := t.TempDir()
	bug468SeedTasks(t, dir)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-cp-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.workingMode = workingmode.Vibe
	rs.workspaceCwd = dir
	rs.vibeCpDocID = "CP-02"
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "task_slicer", Behavior: "agent.delegate", Agent: "agents/doc-writer.md"},
	}
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "task_slicer", To: "done", When: "done", Kind: "forward"},
	}
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parent.RunID, rs.activeFlowNodes)

	svc.onVibeCpNodeDone(parent.RunID, vibeTaskSlicerNodeID)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs = svc.runs[parent.RunID]
	if len(rs.vibeTaskPlan) != 1 || filepath.Base(rs.vibeTaskPlan[0]) != "Task-12-snake-core-model.md" {
		t.Fatalf("vibeTaskPlan = %v, want [Task-12-snake-core-model.md] (CP-02 scope)", rs.vibeTaskPlan)
	}
	if rs.vibeSprintIndex != 0 {
		t.Fatalf("vibeSprintIndex = %d, want 0 (fresh plan)", rs.vibeSprintIndex)
	}
}

// Same bed, no cpID recorded (pre-fix runs / fixtures): the plan must stay
// the legacy full glob so resume/reconstruct fixtures are untouched.
func TestBUG468_SlicerDoneEmptyCPIDKeepsLegacyUnscopedPlan(t *testing.T) {
	svc, _ := newTestServer(t)
	dir := t.TempDir()
	bug468SeedTasks(t, dir)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-cp-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.workingMode = workingmode.Vibe
	rs.workspaceCwd = dir
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "task_slicer", Behavior: "agent.delegate", Agent: "agents/doc-writer.md"},
	}
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "task_slicer", To: "done", When: "done", Kind: "forward"},
	}
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parent.RunID, rs.activeFlowNodes)

	svc.onVibeCpNodeDone(parent.RunID, vibeTaskSlicerNodeID)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs = svc.runs[parent.RunID]
	if len(rs.vibeTaskPlan) != 3 {
		t.Fatalf("vibeTaskPlan = %v, want all 3 (legacy unscoped, empty cpID)", rs.vibeTaskPlan)
	}
}

// The "did this run's slicer already produce tasks" presence checks must also
// scope: a bed holding only OTHER CPs' tasks counts as missing for this run.
func TestBUG468_ScopedPresenceTreatsForeignTasksAsMissing(t *testing.T) {
	dir := t.TempDir()
	bug468SeedTasks(t, dir)
	if got := collectVibeTaskPlanForCP(dir, "CP-77"); len(got) != 0 {
		t.Fatalf("foreign-CP presence = %v, want empty → restart/park path", got)
	}
}
