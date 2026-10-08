package runner

import (
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// BUG-654 (live run-523131): agent-loop/continue on a run parked at a
// routing hub re-dispatched the PARKED hub turn — burning a round to
// re-submit the identical submit_review_outcome and re-block on the same
// gate — while flow-control{continue} on the same park traversed the
// intended synthesis→coder back-edge. The operator's "continue" must resolve
// to edge traversal on a routing park, not a hub re-run whose recorded
// outcome cannot change.
func TestBug654_ContinueOnRoutingParkTraversesBackEdge(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = "vibe"
	rs.flowEngineDriven = true
	rs.status = RunStatusRunning
	rs.agentStatus = string(RunStatusRunning)
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "coder", Behavior: "code.writer"},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "coder", To: "synthesis", When: "done", Kind: "forward"},
		{From: "synthesis", To: "coder", When: "continue", Kind: "back"},
	}
	svc.mu.Unlock()
	fake, _ := svc.workflowStore.(*fakeWorkflowStore)
	fake.seed(parent.RunID, []RuntimeWorkflowStep{
		{ID: "coder", NodeID: "coder", Status: StepStatusDone},
		{ID: "synthesis", NodeID: "synthesis", Status: StepStatusWaitingUserApr},
	})
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Status: "blocked", BlockReason: "escalate",
		GateReason: "done blocked: member verdict not approved",
		Cap: 5, RoundCap: 5, Round: 1,
	})

	if _, err := svc.resumeFlowWithFeedback(parent.RunID, "continue"); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}

	// Edge traversal proof: the continue back-edge's re-entry node (coder) is
	// stamped RUNNING by applyFlowControl's loop-reset. The pre-fix path
	// (hub reinvoke) never touches the coder step.
	deadline := time.Now().Add(3 * time.Second)
	for {
		if got := svc.lookupFlowStepStatus(parent.RunID, "coder"); got == StepStatusRunning {
			return
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("continue on routing park never traversed the back-edge: coder=%s",
		svc.lookupFlowStepStatus(parent.RunID, "coder"))
}

// A writer-node escalate park is NOT a routing park — continue must keep the
// existing member/node redrive semantics, not traverse a hub back-edge.
func TestBug654_WriterEscalateParkDoesNotTraverse(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = "vibe"
	rs.flowEngineDriven = true
	rs.status = RunStatusRunning
	rs.lastEscalatedInlineNodeID = "coder" // escalate owns the writer node
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "coder", Behavior: "code.writer"},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "coder", To: "synthesis", When: "done", Kind: "forward"},
		{From: "synthesis", To: "coder", When: "continue", Kind: "back"},
	}
	svc.mu.Unlock()
	fake, _ := svc.workflowStore.(*fakeWorkflowStore)
	fake.seed(parent.RunID, []RuntimeWorkflowStep{
		{ID: "coder", NodeID: "coder", Status: StepStatusWaitingUserApr},
		{ID: "synthesis", NodeID: "synthesis", Status: StepStatusDone},
	})
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Status: "blocked", BlockReason: "escalate",
		GateReason: "writer gate violation",
		Cap: 5, RoundCap: 5, Round: 1,
	})

	if _, err := svc.resumeFlowWithFeedback(parent.RunID, "continue"); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}

	// The writer park's escalate node is not the hub: the routing-park seam
	// must not fire — coder stays at its parked stamp.
	if got := svc.lookupFlowStepStatus(parent.RunID, "coder"); got != StepStatusWaitingUserApr {
		t.Fatalf("writer escalate park was treated as a routing park: coder=%s", got)
	}
}
