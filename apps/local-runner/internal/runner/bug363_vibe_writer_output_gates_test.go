package runner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/workingmode"
)

// BUG-363: live run-635006 ran cp_writer (wrote SP-snake-mvp.md, no CP-*.md)
// then task_slicer (wrote zero Task files), yet the flow silently sprinted
// once from the SS-glob fallback and finished done. The slicer-done boundary
// must fail closed when the workspace shows no Task output.
//
// Carve-outs (pinned, not changed):
// - collectVibeSprintPlan SS fallback itself (CA-783).
// - empty-cwd unit shapes (CA-791, CA-783, Task-321/326).
// - present-Task pass-through (CA-791 SlicerAfterIngestJoinStartsSprint).

// Repro: SS present, zero Task files → park, no sprint.
func TestBUG363_SlicerDoneWithoutTasksParks(t *testing.T) {
	svc, _ := newTestServer(t)
	dir := t.TempDir()
	spec := filepath.Join(dir, "requirements", "05-System-Specs")
	if err := os.MkdirAll(spec, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(spec, "SS-snake-mvp.md"), []byte("# SS\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.workingMode = workingmode.Vibe
	rs.vibeAwaitingLock = false
	rs.workspaceCwd = dir
	svc.mu.Unlock()

	svc.onVibeCpNodeDone(parent.RunID, vibeTaskSlicerNodeID)

	st := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if st.Status != "blocked" || st.BlockReason != "requirement" {
		t.Fatalf("loop=%+v want blocked/requirement", st)
	}
	if !strings.Contains(st.GateReason, "Task") {
		t.Fatalf("GateReason=%q must name the missing Task output", st.GateReason)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs = svc.runs[parent.RunID]
	if rs.vibeSprintIndex != 0 {
		t.Fatalf("sprint index=%d want 0 (no sprint from fallback)", rs.vibeSprintIndex)
	}
	for _, n := range rs.activeFlowNodes {
		if n.ID == "tdd" || n.ID == "coder" || n.ID == "preflight_contract_plan" {
			t.Fatalf("sprint node %q started without Task files", n.ID)
		}
	}
}

// Near-miss: legacy sprint_slicer alias parks the same way.
func TestBUG363_LegacySlicerAliasWithoutTasksParks(t *testing.T) {
	svc, _ := newTestServer(t)
	dir := t.TempDir()
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.workingMode = workingmode.Vibe
	rs.vibeAwaitingLock = false
	rs.workspaceCwd = dir
	svc.mu.Unlock()

	svc.onVibeCpNodeDone(parent.RunID, vibeSprintSlicerNodeID)

	st := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if st.Status != "blocked" || st.BlockReason != "requirement" {
		t.Fatalf("loop=%+v want blocked/requirement", st)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.runs[parent.RunID].vibeSprintIndex != 0 {
		t.Fatal("alias slicer must not start a sprint without Task files")
	}
}

// Recovery: park leaves no stale plan — adding Tasks then re-running the
// slicer sprints from the Task files, not the refused SS fallback.
func TestBUG363_ParkThenTasksRecoverFromTasks(t *testing.T) {
	svc, _ := newTestServer(t)
	dir := t.TempDir()
	spec := filepath.Join(dir, "requirements", "05-System-Specs")
	if err := os.MkdirAll(spec, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(spec, "SS-snake-mvp.md"), []byte("# SS\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.workingMode = workingmode.Vibe
	rs.vibeAwaitingLock = false
	rs.workspaceCwd = dir
	svc.mu.Unlock()

	svc.onVibeCpNodeDone(parent.RunID, vibeTaskSlicerNodeID)
	if got := svc.agentOrchestrator.loopStateFor(parent.RunID).Status; got != "blocked" {
		t.Fatalf("precondition: want parked, loop=%q", got)
	}

	todo := filepath.Join(dir, "requirements", "08-Task", "todo")
	if err := os.MkdirAll(todo, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(todo, "Task-1-x.md"), []byte("# T\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	svc.onVibeCpNodeDone(parent.RunID, vibeTaskSlicerNodeID)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs = svc.runs[parent.RunID]
	if rs.vibeSprintIndex < 1 {
		t.Fatal("recovery must start a sprint once Task files exist")
	}
	if len(rs.vibeTaskPlan) == 0 || !strings.Contains(rs.vibeTaskPlan[0], "08-Task") {
		t.Fatalf("plan=%v must come from Task files, not the refused SS fallback", rs.vibeTaskPlan)
	}
}

// Legacy carve-out: empty cwd keeps old behavior (no park).
func TestBUG363_EmptyCwdKeepsLegacyBehavior(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.workingMode = workingmode.Vibe
	rs.vibeAwaitingLock = false
	svc.mu.Unlock()

	svc.onVibeCpNodeDone(parent.RunID, vibeTaskSlicerNodeID)

	st := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if st.Status == "blocked" && st.BlockReason == "requirement" {
		t.Fatalf("empty cwd must not park: %+v", st)
	}
}
