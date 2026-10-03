package runner

// BUG-621 (live run-150388): the step-transition sidecar is keyed by node id
// only, so sprint-1 terminal statuses replay onto sprint-2's reused node ids
// (vibe-sprint remounts the same flow nodes per task). The session-evidence
// path already filters legs by vibe sprint (BUG-616), but the transition-log
// merge did not — sprint-1's coder/reviewer/audit DONE and a cancelled tdd's
// FAILED survived every restart, poisoning pendingVibeResumeFromNode and the
// reconstructed board.
//
// Fix contract: each transition line carries the run's vibeSprintIndex at
// append time, and replay on a run at sprint>1 only applies lines stamped for
// the current sprint. Unstamped legacy lines (pre-fix data) are dropped under
// sprint>1 — they cannot be attributed, and the sprint-filtered session
// evidence walk rebuilds the board correctly. Sprint<=1 and non-vibe runs
// keep the byte-for-byte legacy behavior (all lines apply).

import (
	"context"
	"testing"
)

// Part A — every appended transition line carries the run's sprint index.
func TestBUG621_TransitionLineStampedWithSprintIndex(t *testing.T) {
	store := &r5TransitionStore{fakeWorkflowStore: newFakeWorkflowStore()}
	svc := task451ServiceWithStore(t, store)
	runID := armVibeSprintRun(t, svc)

	svc.mu.Lock()
	svc.runs[runID].vibeSprintIndex = 2
	svc.mu.Unlock()

	svc.setFlowStepStatus(context.Background(), runID, "tdd", StepStatusDone)

	lines := store.lines[runID]
	if len(lines) == 0 {
		t.Fatal("no transition lines appended")
	}
	var stamped *stepTransitionLine
	for i := range lines {
		if lines[i].NodeID == "tdd" && lines[i].Status == string(StepStatusDone) {
			stamped = &lines[i]
		}
	}
	if stamped == nil {
		t.Fatalf("no DONE line for tdd captured: %+v", lines)
	}
	if stamped.VibeSprintIndex != 2 {
		t.Fatalf("transition line sprint index=%d want 2", stamped.VibeSprintIndex)
	}
}

// Part B — replay on a sprint-2 run drops foreign-sprint and unattributable
// lines, so a sprint-1 DONE cannot resurrect a sprint-2 node.
func TestBUG621_ReplayDropsForeignSprintTransitions(t *testing.T) {
	rows := []RuntimeWorkflowStep{
		{ID: "tdd", Status: StepStatusPending},
		{ID: "coder", Status: StepStatusPending},
		{ID: "reviewer", Status: StepStatusPending},
	}
	lines := []stepTransitionLine{
		// Foreign sprint-1 terminals — the contamination class seen live.
		{NodeID: "tdd", Status: string(StepStatusDone), VibeSprintIndex: 1, TS: "2026-01-01T00:00:01Z"},
		{NodeID: "coder", Status: string(StepStatusDone), VibeSprintIndex: 1, TS: "2026-01-01T00:00:02Z"},
		// Current sprint's own outcome must still apply.
		{NodeID: "coder", Status: string(StepStatusCanceled), VibeSprintIndex: 2, TS: "2026-01-01T00:10:00Z"},
		// Unstamped legacy line — cannot be attributed to a sprint, dropped
		// under sprint>1 so session evidence owns the row.
		{NodeID: "reviewer", Status: string(StepStatusDone), TS: "2026-01-01T00:00:03Z"},
	}

	out := applyStepTransitionReplaySprint(rows, lines, nil, 2)
	byID := map[string]RuntimeWorkflowStepStatus{}
	for _, r := range out {
		byID[r.ID] = r.Status
	}
	if byID["tdd"] != StepStatusPending {
		t.Fatalf("tdd: sprint-1 DONE must not replay on sprint 2, got %s", byID["tdd"])
	}
	if byID["coder"] != StepStatusCanceled {
		t.Fatalf("coder: current-sprint CANCELED must apply, got %s", byID["coder"])
	}
	if byID["reviewer"] != StepStatusPending {
		t.Fatalf("reviewer: unstamped line must not replay under sprint>1, got %s", byID["reviewer"])
	}
}

// Part C — legacy byte-compat: a run at sprint<=1 (or non-vibe, sprint 0)
// still applies every line exactly like before.
func TestBUG621_ReplayLegacySprintKeepsAllLines(t *testing.T) {
	mk := func() []RuntimeWorkflowStep {
		return []RuntimeWorkflowStep{
			{ID: "tdd", Status: StepStatusPending},
			{ID: "coder", Status: StepStatusPending},
		}
	}
	lines := []stepTransitionLine{
		{NodeID: "tdd", Status: string(StepStatusDone), TS: "2026-01-01T00:00:01Z"},
		{NodeID: "coder", Status: string(StepStatusDone), VibeSprintIndex: 1, TS: "2026-01-01T00:00:02Z"},
	}
	for _, sprint := range []int{0, 1} {
		out := applyStepTransitionReplaySprint(mk(), lines, nil, sprint)
		byID := map[string]RuntimeWorkflowStepStatus{}
		for _, r := range out {
			byID[r.ID] = r.Status
		}
		if byID["tdd"] != StepStatusDone || byID["coder"] != StepStatusDone {
			t.Fatalf("sprint=%d must keep legacy all-lines replay, got %+v", sprint, byID)
		}
	}
}
