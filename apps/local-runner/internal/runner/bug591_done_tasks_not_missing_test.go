package runner

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// BUG-591 (live run-139670): resume-confirm `ok` after the sprint finished
// re-entered the ingest chain — restartVibeTaskSlicerForMissingTasks read the
// CP-scoped todo/ glob as "tasks deleted" because Task-026 had moved to
// requirements/08-Task/done/, then force-started task_slicer, which rebound
// the run to a foreign CP (CP-10) and began slicing Task-101.
//
// A task in done/ is completed, not deleted: the missing-tasks check must
// span both directories before it may declare the plan's files gone.
func TestBUG591_DoneTasksAreNotMissing(t *testing.T) {
	svc, _ := newTestServer(t)
	cwd := t.TempDir()
	task328Write(t, cwd, "requirements/05-System-Specs/SS-02-vault.md", "# SS\n")
	task328Write(t, cwd, "requirements/07-Coding-Plan/todo/CP-02-audit.md",
		"# CP\nDocument ID: CP-02\n")
	// The sprint's task — completed and moved to done/, as the live run did.
	task328Write(t, cwd, "requirements/08-Task/done/Task-026-audit-trail.md",
		"# T\nParent Documents: CP-02\n")
	// A foreign, uncompleted task from another CP still sits in todo/ —
	// it must not be claimed by this run and must not matter either way.
	task328Write(t, cwd, "requirements/08-Task/todo/Task-101-asan.md",
		"# T\nParent Documents: CP-10\n")

	runID := "run-bug591"
	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:                 runID,
		providerKey:        ProviderKeyCodex,
		workingMode:        workingmode.Vibe,
		workspaceCwd:       cwd,
		chatFlowRef:        workingmode.PackPrefix + vibeSprintFlowID,
		vibeCpDocID:        "CP-02",
		vibeCheckpointNode: "reviewer",
		vibeTaskPlan: []string{
			"requirements/08-Task/todo/Task-026-audit-trail.md",
		},
		vibeResumeConfirm:  true,
		vibeResumeFromNode: "reviewer",
		activeFlowNodes: []agentpack.FlowNode{
			{ID: "reviewer", Behavior: "agent.review"},
			{ID: "synthesis", Behavior: "agent.code"},
		},
		activeFlowEdges: []agentpack.FlowEdge{
			{From: "reviewer", To: "synthesis", When: "done", Kind: "forward"},
		},
		subs: map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()

	if svc.restartVibeTaskSlicerForMissingTasks(runID) {
		t.Fatal("task moved to done/ is not missing — resume-confirm ok must not re-arm task_slicer")
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	if workingmode.BareFlowID(rs.chatFlowRef) == vibeCpIngestFlowID {
		t.Fatal("run must keep its sprint flow, not morph into cp-ingest")
	}
	if len(rs.vibeTaskPlan) == 0 {
		t.Fatal("plan must be preserved when its files live in done/")
	}
}

// Companion shape: the same run with the task file genuinely deleted must
// still re-slice (R-TK-D1 semantics preserved for real deletions).
func TestBUG591_DeletedTasksStillRestartSlicer(t *testing.T) {
	svc, _ := newTestServer(t)
	cwd := t.TempDir()
	task328Write(t, cwd, "requirements/07-Coding-Plan/todo/CP-02-audit.md",
		"# CP\nDocument ID: CP-02\n")

	runID := "run-bug591-deleted"
	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:                 runID,
		providerKey:        ProviderKeyCodex,
		workingMode:        workingmode.Vibe,
		workspaceCwd:       cwd,
		chatFlowRef:        workingmode.PackPrefix + vibeSprintFlowID,
		vibeCpDocID:        "CP-02",
		vibeCheckpointNode: "tdd",
		vibeTaskPlan: []string{
			"requirements/08-Task/todo/Task-026-audit-trail.md",
		},
		subs: map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()

	if !svc.restartVibeTaskSlicerForMissingTasks(runID) {
		t.Fatal("genuinely deleted tasks must still restart the slicer")
	}
}
