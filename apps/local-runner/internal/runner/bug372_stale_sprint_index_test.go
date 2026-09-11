package runner

import (
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/workingmode"
)

// BUG-372 / live run-225468: after R-TK-D2 CP rewrite + re-slice, stale
// vibeSprintIndex left chip at task 2/3 while Task-904 stayed draft.
func TestBUG372_CpRewriteClearsSprintCursor(t *testing.T) {
	svc, _ := newTestServer(t)
	cwd := t.TempDir()
	task328Write(t, cwd, "requirements/05-System-Specs/SS-01-snake.md", "# SS\n")
	runID := "run-bug372-cp"
	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:                 runID,
		providerKey:        ProviderKeyCodex,
		workingMode:        workingmode.Vibe,
		workspaceCwd:       cwd,
		vibeSSSealed:       true,
		vibeLockedSS:       "requirements/05-System-Specs/SS-01-snake.md",
		chatFlowRef:        workingmode.PackPrefix + vibeCpIngestFlowID,
		vibeCheckpointNode: vibeCpWriterNodeID,
		vibeTaskPlan: []string{
			"requirements/08-Task/todo/Task-904-a.md",
			"requirements/08-Task/todo/Task-905-b.md",
			"requirements/08-Task/todo/Task-906-c.md",
		},
		vibeSprintIndex: 2,
		lastPrompt:      "snake",
		subs:            map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()

	if !svc.restartVibeCpWriterForMissingCP(runID) {
		t.Fatal("expected CP rewrite")
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	if rs.vibeSprintIndex != 0 {
		t.Fatalf("index=%d want 0 after CP rewrite", rs.vibeSprintIndex)
	}
	if len(rs.vibeTaskPlan) != 0 {
		t.Fatalf("plan must clear, got %v", rs.vibeTaskPlan)
	}
}

func TestBUG372_SlicerDoneReloadsPlanAndResetsIndex(t *testing.T) {
	svc, _ := newTestServer(t)
	cwd := t.TempDir()
	task328Write(t, cwd, "requirements/08-Task/todo/Task-904-snake-core.md", "# T1\n")
	task328Write(t, cwd, "requirements/08-Task/todo/Task-905-snake-tick-wasd.md", "# T2\n")
	task328Write(t, cwd, "requirements/08-Task/todo/Task-906-snake-score-run.md", "# T3\n")
	runID := "run-bug372-slicer"
	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:           runID,
		providerKey:  ProviderKeyCodex,
		workingMode:  workingmode.Vibe,
		workspaceCwd: cwd,
		chatFlowRef:  workingmode.PackPrefix + vibeCpIngestFlowID,
		// Stale plan paths + cursor as if mid-sprint before re-slice.
		vibeTaskPlan:    []string{"requirements/08-Task/todo/Task-OLD.md"},
		vibeSprintIndex: 2,
		subs:            map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()

	svc.onVibeCpNodeDone(runID, vibeTaskSlicerNodeID)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	if rs.vibeSprintIndex != 1 {
		// takeNextVibeSprint starts plan[0] then increments to 1.
		t.Fatalf("index=%d want 1 after fresh plan + first sprint start", rs.vibeSprintIndex)
	}
	if len(rs.vibeTaskPlan) != 3 {
		t.Fatalf("plan len=%d want 3 from disk", len(rs.vibeTaskPlan))
	}
	if filepath.Base(rs.vibeTaskPlan[0]) != "Task-904-snake-core.md" {
		t.Fatalf("first task=%q want Task-904", rs.vibeTaskPlan[0])
	}
}

func TestBUG372_ReconcileClampsWhenEarlierTaskStillDraft(t *testing.T) {
	cwd := t.TempDir()
	task328Write(t, cwd, "requirements/08-Task/todo/Task-904-snake-core.md", "status: draft\n- Status: `draft`\n")
	task328Write(t, cwd, "requirements/08-Task/todo/Task-905-snake-tick-wasd.md", "status: in_progress\n- Status: `in_progress`\n")
	rs := &interactiveRun{
		workspaceCwd: cwd,
		vibeTaskPlan: []string{
			"requirements/08-Task/todo/Task-904-snake-core.md",
			"requirements/08-Task/todo/Task-905-snake-tick-wasd.md",
			"requirements/08-Task/todo/Task-906-snake-score-run.md",
		},
		vibeSprintIndex: 2,
	}
	if !reconcileVibeSprintCursor(rs) {
		t.Fatal("expected clamp")
	}
	if rs.vibeSprintIndex != 1 {
		t.Fatalf("index=%d want 1 (chip task 1/3 for draft Task-904)", rs.vibeSprintIndex)
	}
	if vibeSprintCurrentPlanIndex(rs.vibeSprintIndex, len(rs.vibeTaskPlan)) != 0 {
		t.Fatal("current plan slot must be Task-904")
	}
}

func TestBUG372_ForceStartUsesStartedCountNotPlanIndex(t *testing.T) {
	// index=1 means first sprint started → plan[0], not plan[1].
	if got := vibeSprintCurrentPlanIndex(1, 3); got != 0 {
		t.Fatalf("index 1 → plan slot %d want 0", got)
	}
	if got := vibeSprintCurrentPlanIndex(2, 3); got != 1 {
		t.Fatalf("index 2 → plan slot %d want 1", got)
	}
	if got := vibeSprintCurrentPlanIndex(0, 3); got != 0 {
		t.Fatalf("index 0 → plan slot %d want 0", got)
	}
}

func TestBUG372_ProvidersAgnostic(t *testing.T) {
	for _, pk := range []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok} {
		t.Run(string(pk), func(t *testing.T) {
			rs := &interactiveRun{providerKey: pk, vibeSprintIndex: 2, vibeTaskPlan: []string{"a", "b"}}
			clearVibeSprintCursor(rs)
			if rs.vibeSprintIndex != 0 || len(rs.vibeTaskPlan) != 0 {
				t.Fatalf("%s: cursor not cleared", pk)
			}
		})
	}
}
