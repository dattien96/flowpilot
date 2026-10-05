package runner

import (
	"context"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// seedDeadDispatchRun builds a vibe flow parent mid-sprint: flowEngineDriven,
// loop running, nothing in flight. Nodes default to the caller's graph; step
// rows are seeded PENDING so the test stamps the shape it needs.
func seedDeadDispatchRun(t *testing.T, nodes []agentpack.FlowNode) (*InteractiveService, *fakeWorkflowStore, string) {
	t.Helper()
	store := newFakeWorkflowStore()
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 5, RoundCap: 5})
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.status = RunStatusRunning
	rs.agentStatus = string(RunStatusRunning)
	rs.workingMode = workingmode.Vibe
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.activeFlowNodes = nodes
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	return svc, store, parent.RunID
}

// CA-1202 (live run-183756): coder was stamped RUNNING after a batch reset
// but zero provider turn ever dispatched — the activation dead-ended between
// stamp and spawn, and no sweep class owned the shape. An aged RUNNING
// delegate step with no live leg and no driver must get a fresh leg.
func TestCA1202_DeadDispatchRespawnsDelegateLeg(t *testing.T) {
	svc, _, runID := seedDeadDispatchRun(t, []agentpack.FlowNode{
		{ID: "tdd", Behavior: "agent.scaffold"},
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
		{ID: "synthesis", Behavior: "hub.inline"},
	})
	svc.setFlowStepStatus(context.Background(), runID, "coder", StepStatusRunning)
	ageStepStartedAt(t, svc, runID, "coder", 3*time.Minute)

	svc.sweepWedgedFlowWork()

	svc.mu.Lock()
	spawned := false
	for _, r := range svc.runs {
		if r != nil && r.parentRunID == runID && r.label == "coder" {
			spawned = true
		}
	}
	svc.mu.Unlock()
	if !spawned {
		t.Fatal("dead-dispatched coder step got no fresh leg — dispatch wedge still invisible")
	}
}

// Same class on a hub.inline node (run-183756 synthesis): the reinvoke was
// consumed but no turn materialized — re-drive through the hub reinvoke path.
func TestCA1202_DeadDispatchReinvokesHubNode(t *testing.T) {
	svc, _, runID := seedDeadDispatchRun(t, []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
		{ID: "synthesis", Behavior: "hub.inline"},
	})
	svc.setFlowStepStatus(context.Background(), runID, "synthesis", StepStatusRunning)
	ageStepStartedAt(t, svc, runID, "synthesis", 3*time.Minute)

	svc.sweepWedgedFlowWork()

	svc.mu.Lock()
	armed := svc.runs[runID].reinvokeInFlight || svc.runs[runID].pendingHubReinvoke
	svc.mu.Unlock()
	if !armed {
		t.Fatal("dead-dispatched hub node must re-drive a hub turn (reinvoke or armed pending)")
	}
}

// A fresh RUNNING stamp is inside the legitimate dispatch window — the
// spawn/settle is mid-flight. Only an aged stamp with no live work counts.
func TestCA1202_FreshRunningStepIsNotDeadDispatch(t *testing.T) {
	svc, _, runID := seedDeadDispatchRun(t, []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
	})
	svc.setFlowStepStatus(context.Background(), runID, "coder", StepStatusRunning)

	svc.sweepWedgedFlowWork()

	svc.mu.Lock()
	n := 0
	for _, r := range svc.runs {
		if r != nil && r.parentRunID == runID {
			n++
		}
	}
	svc.mu.Unlock()
	if n != 0 {
		t.Fatalf("fresh RUNNING stamp spawned %d legs — dispatch window must hold", n)
	}
}

// A step RUNNING with a live leg is real work — never respawn over it. The
// waiting_user_approval leg is the sharpest case: a gate-parked child is a
// visible wait, not a dead dispatch.
func TestCA1202_WaitingUserLegIsNotDeadDispatch(t *testing.T) {
	svc, _, runID := seedDeadDispatchRun(t, []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
	})
	svc.setFlowStepStatus(context.Background(), runID, "coder", StepStatusRunning)
	ageStepStartedAt(t, svc, runID, "coder", 3*time.Minute)
	svc.mu.Lock()
	svc.runs["run-parked-leg"] = &interactiveRun{
		id:          "run-parked-leg",
		parentRunID: runID,
		label:       "coder",
		status:      RunStatusWaitingUserApr,
	}
	svc.mu.Unlock()

	svc.sweepWedgedFlowWork()

	svc.mu.Lock()
	n := 0
	for _, r := range svc.runs {
		if r != nil && r.parentRunID == runID && r.label == "coder" && r.id != "run-parked-leg" {
			n++
		}
	}
	svc.mu.Unlock()
	if n != 0 {
		t.Fatalf("respawned over a waiting_user_approval leg — park is a visible wait, not residue")
	}
}

// A step RUNNING whose leg already completed is a settle/advance gap, not a
// dead dispatch — respawning would double the node's work. BUG-1197's
// predecessor settle owns that shape.
func TestCA1202_CompletedLegIsNotDeadDispatch(t *testing.T) {
	svc, store, runID := seedDeadDispatchRun(t, []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
	})
	svc.setFlowStepStatus(context.Background(), runID, "coder", StepStatusRunning)
	ageStepStartedAt(t, svc, runID, "coder", 3*time.Minute)
	store.sessions["run-done-leg"] = ProviderSessionState{
		RunID:       "run-done-leg",
		ParentRunID: runID,
		Label:       "coder",
		Status:      RunStatusCompleted,
	}

	svc.sweepWedgedFlowWork()

	svc.mu.Lock()
	n := 0
	for _, r := range svc.runs {
		if r != nil && r.parentRunID == runID && r.label == "coder" {
			n++
		}
	}
	svc.mu.Unlock()
	if n != 0 {
		t.Fatalf("respawned over a completed leg — the settle gap is a different class")
	}
}
