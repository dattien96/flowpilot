package runner

// ============================================================================
// CA-1087 — vibe handoff consumed-node claim + stale hub-done guard
// (live run-2302): task_plan_reader completion armed the sprint plan and
// swapped activeFlowNodes to vibe-sprint BEFORE tryAdvanceFlowFromNode
// resolved edges — the reader has no targets in the sprint topology →
// returned false → advanceOrNotifyHub reinvoked the hub → its
// submit_review_outcome(done) fell through to the fallback hub resolver,
// which picked the NEW topology's first hub.inline ("synthesis", never
// dispatched, still PENDING) → synthesis --done--> audit fired with no
// validation run → blocked_validation_failed → the escalate park cancelled
// the in-flight contract-planner child mid-turn (MidTurnAbort).
// ============================================================================

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// ca1087VibeTasksSprintBed stands up the exact post-lock shape run-2302 was
// in when task_plan_reader completed: CP-02 admitted, cp_lock gate cleared,
// 2 CP-02-parented tasks on disk, flow-engine driven vibe-tasks topology.
func ca1087VibeTasksSprintBed(t *testing.T) (*InteractiveService, RunHandle) {
	t.Helper()
	svc, _ := newTestServer(t)
	dir := t.TempDir()
	vibeTasksSeedBed(t, dir)
	parent := newVibeTasksRun(t, svc, dir, "CP-02")
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.autoOrchestrate = true
	rs.vibeSprintBudget = defaultVibeSprintBudget
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Status: "running", Mode: "explicit", Cap: 3, RoundCap: 3,
	})
	// Admission arms the CP Preview & Lock gate; the operator's lock is the
	// cp_lock completion that clears it (same as VT-A2x).
	svc.onVibeCpNodeDone(parent.RunID, vibeCpLockNodeID)
	return svc, parent
}

// The reader's completion is consumed by onVibeCpNodeDone (plan armed,
// topology swapped to vibe-sprint, sprint-1 child dispatched). Returning
// false from tryAdvanceFlowFromNode sent advanceOrNotifyHub down the
// note+reinvoke path that produced the misattributed hub done.
func TestCA1087_TaskPlanReaderCompletionConsumedByChainIsClaimed(t *testing.T) {
	svc, parent := ca1087VibeTasksSprintBed(t)

	if !svc.tryAdvanceFlowFromNode(parent.RunID, vibeTaskPlanReaderNodeID, "found Task-21, Task-22") {
		t.Fatal("task_plan_reader completion was consumed by the sprint chain handoff; it must be claimed (no hub reinvoke)")
	}

	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	idx := rs.vibeSprintIndex
	plan := append([]string(nil), rs.vibeTaskPlan...)
	topo := hubInlineNodeID(rs.activeFlowNodes)
	pendingReinvoke := rs.pendingHubReinvoke
	svc.mu.Unlock()

	if len(plan) != 2 || idx != 1 {
		t.Fatalf("sprint plan=%v index=%d, want 2 tasks armed with sprint-1 in flight", plan, idx)
	}
	if topo != "synthesis" {
		t.Fatalf("active topology hub=%q, want synthesis (vibe-sprint swapped in)", topo)
	}
	if pendingReinvoke {
		t.Fatal("pendingHubReinvoke armed — a stale synthesis turn would fire after this turn")
	}
}

// The stale outcome half: a hub "done" submitted by a synthesis turn that
// was never dispatched for this node must be consumed as advancing — never
// resolved against a PENDING hub's forward edge. run-2302's stale done
// dispatched audit with zero validation and its escalate park cancelled the
// contract-planner child.
func TestCA1087_StaleHubDoneOnUndispatchedSprintHubDoesNotDispatchAudit(t *testing.T) {
	svc, parent := ca1087VibeTasksSprintBed(t)

	// Topology swap + sprint-1 dispatch exactly as the live run.
	svc.onVibeCpNodeDone(parent.RunID, vibeTaskPlanReaderNodeID)
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	if got := hubInlineNodeID(rs.activeFlowNodes); got != "synthesis" {
		svc.mu.Unlock()
		t.Fatalf("topology hub=%q, want synthesis after swap", got)
	}
	if rs.activeHubNodeID != "" {
		svc.mu.Unlock()
		t.Fatalf("activeHubNodeID=%q, want empty (cleared at cp_lock seal, never re-stamped)", rs.activeHubNodeID)
	}
	svc.mu.Unlock()

	if got := flowStepStatus(t, svc, parent.RunID, "synthesis"); got != StepStatusPending {
		t.Fatalf("synthesis step=%v, want PENDING (hub never dispatched)", got)
	}

	res, handled := svc.advanceHubDoneThroughEdge(parent.RunID, FlowControlInput{
		Status:         "done",
		Summary:        "task_plan_reader synthesized: 2 scoped tasks",
		agentInitiated: true,
	})
	if !handled || res.NextAction != "advancing" {
		t.Fatalf("stale hub done = (handled=%v %+v), want consumed as done/advancing without dispatching audit", handled, res)
	}
	if got := flowStepStatus(t, svc, parent.RunID, "synthesis"); got != StepStatusPending {
		t.Fatalf("synthesis step=%v, want PENDING — a stale done must not settle an undispatched hub", got)
	}
	if got := flowStepStatus(t, svc, parent.RunID, "audit"); got == StepStatusRunning || got == StepStatusDone || got == StepStatusWaitingUserApr {
		t.Fatalf("audit step=%v — stale done dispatched audit with no validation (run-2302 repro)", got)
	}
	if st := svc.agentOrchestrator.loopStateFor(parent.RunID); st.Status == "blocked" {
		t.Fatalf("loop=%+v — stale done parked the run (would cancel the in-flight sprint entry child)", st)
	}
}

// Regression lock for the legitimate path: when synthesis WAS dispatched
// (RUNNING + activeHubNodeID stamped by dispatchHubNotifyNode), its own
// done must still resolve its declared edge and reach audit.
func TestCA1087_LegitDispatchedSynthesisDoneStillAdvances(t *testing.T) {
	svc, parent := ca1087VibeTasksSprintBed(t)
	svc.onVibeCpNodeDone(parent.RunID, vibeTaskPlanReaderNodeID)

	// Simulate the real dispatch: dispatchHubNotifyNode stamps
	// activeHubNodeID + RUNNING before the hub turn runs.
	svc.dispatchHubNotifyNode(parent.RunID, mustFindFlowNode(t, svc, parent.RunID, "synthesis"))
	if got := flowStepStatus(t, svc, parent.RunID, "synthesis"); got != StepStatusRunning {
		t.Fatalf("synthesis step=%v, want RUNNING after dispatchHubNotifyNode", got)
	}

	res, handled := svc.advanceHubDoneThroughEdge(parent.RunID, FlowControlInput{
		Status:         "done",
		Summary:        "review approved",
		agentInitiated: true,
	})
	if !handled {
		t.Fatal("dispatched synthesis done must still resolve its edge")
	}
	// audit runs inline; on a bed without validation output it escalates —
	// the point is the run MOVED (synthesis consumed its own edge), it did
	// not get eaten by the stale-outcome guard.
	if got := flowStepStatus(t, svc, parent.RunID, "synthesis"); got != StepStatusDone {
		t.Fatalf("synthesis step=%v, want DONE (its own done-edge consumed), res=%+v", got, res)
	}
}

func mustFindFlowNode(t *testing.T, svc *InteractiveService, runID, id string) agentpack.FlowNode {
	t.Helper()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	for _, n := range rs.activeFlowNodes {
		if n.ID == id {
			return n
		}
	}
	t.Fatalf("node %q not in activeFlowNodes", id)
	return agentpack.FlowNode{}
}
