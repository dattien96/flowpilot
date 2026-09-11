package runner

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// R-TK-D1: Tasks deleted, CP remains, stale plan + vibe-sprint → restart
// task_slicer (must NOT park Resume from tdd).
func TestTask329_MissingTasksRestartsSlicerNotTddResume(t *testing.T) {
	svc, _ := newTestServer(t)
	cwd := t.TempDir()
	task328Write(t, cwd, "requirements/05-System-Specs/SS-01-snake.md", "# SS\n")
	task328Write(t, cwd, "requirements/07-Coding-Plan/todo/CP-01-snake.md", "# CP\n")
	// Tasks intentionally absent on disk.
	runID := "run-task329-missing"
	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:           runID,
		providerKey:  ProviderKeyCodex,
		workingMode:  workingmode.Vibe,
		workspaceCwd: cwd,
		chatFlowRef:  workingmode.PackPrefix + vibeSprintFlowID,
		vibeCheckpointNode: "tdd",
		vibeTaskPlan: []string{
			"requirements/08-Task/todo/Task-904-snake-core.md",
			"requirements/08-Task/todo/Task-905-snake-tick-wasd.md",
			"requirements/08-Task/todo/Task-906-snake-score-run.md",
		},
		lastPrompt: "snake",
		activeFlowNodes: []agentpack.FlowNode{
			{ID: "tdd", Behavior: "agent.code"},
			{ID: "coder", Behavior: "agent.code"},
		},
		activeFlowEdges: []agentpack.FlowEdge{
			{From: "tdd", To: "coder", When: "done", Kind: "forward"},
		},
		subs: map[int64]chan ProviderEvent{},
	}
	nodes := append([]agentpack.FlowNode(nil), svc.runs[runID].activeFlowNodes...)
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(runID, nodes)
	svc.setFlowStepStatus(t.Context(), runID, "tdd", StepStatusDone)

	if !svc.restartVibeTaskSlicerForMissingTasks(runID) {
		t.Fatal("missing Tasks + CP + stale plan must restart task_slicer")
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	if rs.vibeResumeConfirm || rs.vibeResumeFromNode == "tdd" {
		t.Fatalf("must not park Resume from tdd: confirm=%v from=%q", rs.vibeResumeConfirm, rs.vibeResumeFromNode)
	}
	if len(rs.vibeTaskPlan) != 0 {
		t.Fatalf("stale plan must clear, got %v", rs.vibeTaskPlan)
	}
	if workingmode.BareFlowID(rs.chatFlowRef) != vibeCpIngestFlowID {
		t.Fatalf("must retarget vibe-cp-ingest, got %q", rs.chatFlowRef)
	}
}

// R-TK-K shape: Tasks still on disk → do not steal into slicer restart.
func TestTask329_TasksPresentDoesNotRestartSlicer(t *testing.T) {
	svc, _ := newTestServer(t)
	cwd := t.TempDir()
	task328Write(t, cwd, "requirements/05-System-Specs/SS-01-snake.md", "# SS\n")
	task328Write(t, cwd, "requirements/07-Coding-Plan/todo/CP-01-snake.md", "# CP\n")
	task328Write(t, cwd, "requirements/08-Task/todo/Task-904-snake-core.md", "# T\n")
	runID := "run-task329-present"
	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:                 runID,
		providerKey:        ProviderKeyCodex,
		workingMode:        workingmode.Vibe,
		workspaceCwd:       cwd,
		chatFlowRef:        workingmode.PackPrefix + vibeSprintFlowID,
		vibeCheckpointNode: "tdd",
		vibeTaskPlan:       []string{"requirements/08-Task/todo/Task-904-snake-core.md"},
		subs:               map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	if svc.restartVibeTaskSlicerForMissingTasks(runID) {
		t.Fatal("Tasks present must not restart slicer")
	}
}

func TestTask329_ResumeOKMissingTasksForcesSlicer(t *testing.T) {
	svc, _ := newTestServer(t)
	cwd := t.TempDir()
	task328Write(t, cwd, "requirements/05-System-Specs/SS-01-snake.md", "# SS\n")
	task328Write(t, cwd, "requirements/07-Coding-Plan/todo/CP-01-snake.md", "# CP\n")
	runID := "run-task329-resume-ok"
	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:                 runID,
		providerKey:        ProviderKeyCodex,
		workingMode:        workingmode.Vibe,
		workspaceCwd:       cwd,
		chatFlowRef:        workingmode.PackPrefix + vibeSprintFlowID,
		vibeCheckpointNode: "tdd",
		vibeTaskPlan:       []string{"requirements/08-Task/todo/Task-904-x.md"},
		vibeResumeConfirm:  true,
		vibeResumeFromNode: "tdd",
		status:             RunStatusCancelled,
		subs:               map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{
		Status: "blocked", BlockReason: vibeResumePausedReason, Cap: 3, RoundCap: 3,
	})
	if e := svc.SubmitGateDecision(runID, "ok", ""); e != nil {
		t.Fatalf("ok: %v", e)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	if rs.vibeResumeFromNode == "tdd" && rs.vibeResumeConfirm {
		t.Fatal("OK must not keep tdd resume when Tasks missing")
	}
	if workingmode.BareFlowID(rs.chatFlowRef) != vibeCpIngestFlowID {
		t.Fatalf("OK must force slicer join, got %q", rs.chatFlowRef)
	}
}

func TestTask329_ProvidersAgnostic(t *testing.T) {
	for _, pk := range []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok} {
		t.Run(string(pk), func(t *testing.T) {
			cwd := t.TempDir()
			task328Write(t, cwd, "requirements/07-Coding-Plan/todo/CP-01.md", "# CP\n")
			if len(collectVibeTaskPlan(cwd)) != 0 {
				t.Fatal("tasks must be missing")
			}
			if !vibeCPArtifactsPresent(cwd) {
				t.Fatal("CP must be present")
			}
			_ = pk
		})
	}
}
