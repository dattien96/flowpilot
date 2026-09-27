package runner

import (
	"testing"
)

// Live run-45 (second restart): a flow hub whose durable session row ended at
// status=running + loop_state=done (the mid-turn settle clobber wedge) must
// still read back as completed on the single-run endpoint — same as the
// project history list, which already projects through
// normalizeResumedFlowStatus (BUG-StaleCancel, interactive_handlers.go:1591).
// durableRunSnapshot returned the RAW persisted status, so the run detail view
// disagreed with the list view: history showed completed, GET showed running.
//
// Pin: GET /client/workflow-runs/{id} on a persisted-only run normalizes
// loop-done → completed instead of surfacing the stale in-flight status.
func TestDurableRunSnapshotNormalizesLoopDoneToCompleted(t *testing.T) {
	fws := newFakeWorkflowStore()
	fws.sessions["run-x"] = ProviderSessionState{
		RunID:     "run-x",
		ProjectID: "p1",
		RunKind:   "chat",
		Status:    RunStatusRunning,
		LoopState: AgentLoopState{Status: "done"},
	}
	svc := &InteractiveService{
		runs:          map[string]*interactiveRun{},
		workflowStore: fws,
	}
	view, e := svc.runSnapshot("run-x")
	if e != nil {
		t.Fatalf("runSnapshot: %v", e)
	}
	if view.Status != RunStatusCompleted {
		t.Fatalf("durable snapshot must normalize loop=done to completed, got %q", view.Status)
	}
}
