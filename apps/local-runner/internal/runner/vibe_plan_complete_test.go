package runner

import (
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/workingmode"
)

// Item R.2-2: "plan-exhausted park is indistinguishable from slicer failure".
// When the vibe sprint plan drains — every task in vibeTaskPlan has been
// started AND finished (vibeSprintIndex >= len(plan)) — the run must land on
// a typed terminal state (loop done + CompletionKind "plan_complete"), not a
// parked "awaiting user" that reads like a wedged slicer. A genuine slicer
// failure (zero tasks produced before any sprint ran) keeps the retryable
// blocked path.

// vibePlanCompleteService builds a vibe-mode parent run with a fully drained
// two-task plan. index==len(plan) means both sprints were started and
// finished — the definitive "plan drained" signal.
func vibePlanCompleteService(t *testing.T, index int) (*InteractiveService, string) {
	t.Helper()
	svc, runID := clusterFService(t)
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.workingMode = workingmode.Vibe
	rs.flowEngineDriven = true
	rs.vibeTaskPlan = []string{"requirements/08-Task/todo/Task-1.md", "requirements/08-Task/todo/Task-2.md"}
	rs.vibeSprintIndex = index
	svc.mu.Unlock()
	return svc, runID
}

// The audit's terminal done-settle must stamp the completion kind when the
// plan is drained, so "all tasks delivered" is distinguishable from a generic
// done (and from a wedged park).
func TestPlanComplete_DoneSettleStampsCompletionKind(t *testing.T) {
	svc, runID := vibePlanCompleteService(t, 2)

	if _, err := svc.applyFlowControl(runID, FlowControlInput{Status: "done", Summary: "audit complete"}); err != nil {
		t.Fatalf("applyFlowControl(done): %v", err)
	}

	st := svc.agentOrchestrator.loopStateFor(runID)
	if st.Status != "done" {
		t.Fatalf("loop status = %q, want done", st.Status)
	}
	if st.CompletionKind != "plan_complete" {
		t.Fatalf("drained plan must stamp completionKind=plan_complete, got %q", st.CompletionKind)
	}
}

// A flow that still has undelivered sprint tasks is NOT plan_complete — an
// operator/machine settle leaves the generic done state (sprint-remaining
// guard only applies to agent-initiated verdicts).
func TestPlanComplete_UndrainedPlanLeavesKindEmpty(t *testing.T) {
	svc, runID := vibePlanCompleteService(t, 1) // index < len(plan): one task left

	if _, err := svc.applyFlowControl(runID, FlowControlInput{Status: "done", Summary: "audit complete"}); err != nil {
		t.Fatalf("applyFlowControl(done): %v", err)
	}

	st := svc.agentOrchestrator.loopStateFor(runID)
	if st.Status != "done" {
		t.Fatalf("loop status = %q, want done", st.Status)
	}
	if st.CompletionKind != "" {
		t.Fatalf("undrained plan must not stamp completionKind, got %q", st.CompletionKind)
	}
}

// The chain seam: takeNextVibeSprint says Done (nothing left to start) —
// that used to be a silent return leaving the loop running forever. It must
// now settle the run as plan_complete.
func TestPlanComplete_MaybeStartNextSettlesDrainedPlan(t *testing.T) {
	svc, runID := vibePlanCompleteService(t, 2)
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running"})

	svc.maybeStartNextVibeSprint(runID)

	st := svc.agentOrchestrator.loopStateFor(runID)
	if st.Status != "done" || st.CompletionKind != "plan_complete" {
		t.Fatalf("drained plan must settle done+plan_complete, got status=%q kind=%q", st.Status, st.CompletionKind)
	}
	svc.mu.Lock()
	status := svc.runs[runID].status
	svc.mu.Unlock()
	if status != RunStatusCompleted {
		t.Fatalf("plan-complete settle must terminalize the run, got status=%q", status)
	}
}

// Reopen wedge: a finished vibe run whose Task files all moved out of todo/
// must not park "Resume cp_writer → task_slicer" — plan complete means there
// is nothing to resume.
func TestPlanComplete_JoinResumeSkippedWhenDrained(t *testing.T) {
	svc, runID := vibePlanCompleteService(t, 2)
	cwd := t.TempDir()
	svc.mu.Lock()
	svc.runs[runID].workspaceCwd = cwd
	svc.mu.Unlock()
	seedVibePlanCompleteArtifacts(t, cwd)

	if svc.maybeParkVibeCpJoinResume(runID) {
		t.Fatal("plan-complete run must not park a cp_writer→task_slicer resume-confirm")
	}
	svc.mu.Lock()
	confirm := svc.runs[runID].vibeResumeConfirm
	svc.mu.Unlock()
	if confirm {
		t.Fatal("plan-complete run must not arm vibeResumeConfirm")
	}
}

// A late/stale slicer completion landing on a run whose plan is already
// delivered must settle plan_complete, not re-park the finished run behind a
// "produced no Task files" failure card.
func TestPlanComplete_ZeroTaskSlicerOnDrainedPlanSettles(t *testing.T) {
	svc, runID := vibePlanCompleteService(t, 2)
	cwd := t.TempDir()
	svc.mu.Lock()
	svc.runs[runID].workspaceCwd = cwd
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running"})

	svc.onVibeCpNodeDone(runID, vibeTaskSlicerNodeID)

	st := svc.agentOrchestrator.loopStateFor(runID)
	if st.Status != "done" || st.CompletionKind != "plan_complete" {
		t.Fatalf("stale slicer completion on drained plan must settle plan_complete, got status=%q kind=%q", st.Status, st.CompletionKind)
	}
}

// True slicer failure is preserved: a fresh run whose slicer produced zero
// Task files still parks on the retryable blocked/requirement path.
func TestPlanComplete_ZeroTaskSlicerFreshRunStillBlocks(t *testing.T) {
	svc, runID := vibePlanCompleteService(t, 0)
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.vibeTaskPlan = nil
	rs.workspaceCwd = t.TempDir()
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running"})

	svc.onVibeCpNodeDone(runID, vibeTaskSlicerNodeID)

	st := svc.agentOrchestrator.loopStateFor(runID)
	if st.Status != "blocked" || st.BlockReason != "requirement" {
		t.Fatalf("genuine zero-task slicer must stay on the blocked/requirement retry path, got status=%q reason=%q", st.Status, st.BlockReason)
	}
	if st.CompletionKind != "" {
		t.Fatalf("slicer failure must not stamp completionKind, got %q", st.CompletionKind)
	}
}

// An operator-stopped loop must not be flipped to done by a late drain
// detection — the sweep is conditional like every other settle seam.
func TestPlanComplete_SettleDoesNotFlipStoppedLoop(t *testing.T) {
	svc, runID := vibePlanCompleteService(t, 2)
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "stopped"})

	svc.maybeStartNextVibeSprint(runID)

	st := svc.agentOrchestrator.loopStateFor(runID)
	if st.Status != "stopped" {
		t.Fatalf("stopped loop must win over plan-complete settle, got %q", st.Status)
	}
	if st.CompletionKind != "" {
		t.Fatalf("stopped loop must not carry completionKind, got %q", st.CompletionKind)
	}
}

// seedVibePlanCompleteArtifacts plants the CP+SS artifact presence that
// maybeParkVibeCpJoinResume requires before it reaches the task-list check.
// todo/ holds no Task files — the drained-plan state under test.
func seedVibePlanCompleteArtifacts(t *testing.T, cwd string) {
	t.Helper()
	writeVibePlanTestFile(t, cwd, "requirements/07-Coding-Plan/todo/CP-1-demo.md", "# CP\nDocument ID: CP-1\n")
	writeVibePlanTestFile(t, cwd, "requirements/05-System-Specs/SS-1-demo.md", "# SS\n")
}

func writeVibePlanTestFile(t *testing.T, cwd, rel, body string) {
	t.Helper()
	abs := filepath.Join(cwd, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatalf("MkdirAll %s: %v", filepath.Dir(abs), err)
	}
	if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", abs, err)
	}
}
