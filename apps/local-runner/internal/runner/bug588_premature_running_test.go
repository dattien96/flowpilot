package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// BUG-588 (live run-139670): a continue back-edge stamped the coder step
// RUNNING while the only live leg was still tdd — the step claimed work that
// had not spawned. When the re-entry dispatch finds no child AND another
// sibling leg is still in flight (the upstream node hasn't finished feeding
// the re-entry), the premature RUNNING must fall back to PENDING so the
// timeline never claims a leg that does not exist.
func TestBUG588_ContinueBackEdgeNoChildWithLiveSiblingRevertsStamp(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	nodes := []agentpack.FlowNode{
		{ID: "tdd", Behavior: "agent.code"},
		{ID: "coder", Behavior: "agent.code"},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	edges := []agentpack.FlowEdge{
		{From: "tdd", To: "coder", When: "done", Kind: "forward"},
		{From: "coder", To: "synthesis", When: "done", Kind: "forward"},
		{From: "synthesis", To: "coder", When: "continue", Kind: "back"},
	}
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.activeFlowNodes = nodes
	rs.activeFlowEdges = edges
	rs.activeHubNodeID = "synthesis"
	rs.flowEngineDriven = true
	// A live sibling leg — tdd is still running and will eventually spawn
	// coder through the normal advance.
	childID := "run-bug588-tdd"
	svc.runs[childID] = &interactiveRun{
		id:           childID,
		parentRunID:  runID,
		providerKey:  ProviderKeyCodex,
		label:        "tdd",
		status:       RunStatusRunning,
		turnInFlight: true,
		subs:         map[int64]chan ProviderEvent{},
	}
	svc.agentOrchestrator.registerChild(runID, childID)
	svc.mu.Unlock()
	svc.markFlowEngineDriven(runID)
	svc.reseedFlowStepRuntime(runID, nodes)
	// Mirror the round-reset stamp the live run carried.
	svc.setFlowStepStatus(context.Background(), runID, "coder", StepStatusRunning)

	svc.maybeReinvokeCoderForContinue(runID, "[flow-engine] continue")

	steps, err := svc.workflowStore.LoadRunSteps(context.Background(), runID)
	if err != nil {
		t.Fatalf("LoadRunSteps: %v", err)
	}
	st, _ := stepByID(steps, "coder")
	if st.Status == StepStatusRunning {
		t.Fatal("coder stamped RUNNING with no leg while tdd is still live — " +
			"revert to PENDING; the upstream advance will spawn and stamp it for real")
	}
}

// Guard the legitimate path: a no-child continue while NO sibling leg is
// in flight keeps the RUNNING stamp (the reset contract, Task-304/BUG-286 —
// the leg may still be provisioning or the run is awaiting the next dispatch).
func TestBUG588_ContinueBackEdgeNoChildWithoutLiveSiblingKeepsStamp(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	nodes := []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.code"},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	edges := []agentpack.FlowEdge{
		{From: "coder", To: "synthesis", When: "done", Kind: "forward"},
		{From: "synthesis", To: "coder", When: "continue", Kind: "back"},
	}
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.activeFlowNodes = nodes
	rs.activeFlowEdges = edges
	rs.activeHubNodeID = "synthesis"
	rs.flowEngineDriven = true
	svc.mu.Unlock()
	svc.markFlowEngineDriven(runID)
	svc.reseedFlowStepRuntime(runID, nodes)
	svc.setFlowStepStatus(context.Background(), runID, "coder", StepStatusRunning)

	svc.maybeReinvokeCoderForContinue(runID, "[flow-engine] continue")

	steps, _ := svc.workflowStore.LoadRunSteps(context.Background(), runID)
	st, _ := stepByID(steps, "coder")
	if st.Status != StepStatusRunning {
		t.Fatalf("coder = %q, want RUNNING preserved when no live sibling exists", st.Status)
	}
}
