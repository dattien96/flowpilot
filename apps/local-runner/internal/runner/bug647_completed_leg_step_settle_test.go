package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// BUG-647 (live run-523131): a non-cohort leg whose turn completed and passed
// the post-turn gate reached RunStatusCompleted, but its parent flow-step
// mirror stayed RUNNING — the DONE stamp was deferred to the async edge
// advance (tryAdvanceFlowFromNode), which can be consumed by a gate divert /
// loop-not-advancing / missing target and then never runs. The stale RUNNING
// step blocked `tdd --done--> coder` for ~4h and wedged resume (BUG-656).
//
// Contract: a leg that settles Completed stamps its own step mirror DONE in
// the same settle, gated on no same-label sibling still in flight.
func TestBug647_CompletedNonCohortLegSettlesStepMirror(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc, _ := newTestServerWith(t, reg, nil, nil)

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	parentID := parent.RunID

	svc.mu.Lock()
	rs := svc.runs[parentID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.status = RunStatusRunning
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "tdd", Behavior: "agent.scaffold", Agent: "agents/scaffold-architect.md"},
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
	}
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "tdd", To: "coder", When: "done", Kind: "forward"},
	}
	// The TDD leg: completed turn, no cohort (sprint legs are not cohort
	// members), waiting on the parent's advance.
	leg := &interactiveRun{
		id:            "run-tdd-leg",
		parentRunID:   parentID,
		label:         "tdd",
		agentName:     "scaffold-architect",
		role:          "coder",
		providerKey:   ProviderKeyCodex,
		status:        RunStatusCompleted,
		agentStatus:   string(RunStatusCompleted),
		waitForResult: true,
		subs:          map[int64]chan ProviderEvent{},
	}
	svc.runs[leg.id] = leg
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 5, RoundCap: 5})
	svc.mu.Unlock()

	store, ok := svc.workflowStore.(*fakeWorkflowStore)
	if !ok {
		t.Fatalf("expected fakeWorkflowStore, got %T", svc.workflowStore)
	}
	// Live shape: leg durably Completed, step mirror stuck RUNNING (its settle
	// stamp was deferred to an advance that never fired).
	store.seed(parentID, []RuntimeWorkflowStep{
		{ID: "tdd", NodeID: "tdd", Status: StepStatusRunning},
		{ID: "coder", NodeID: "coder", Status: StepStatusPending},
	})

	svc.mu.Lock()
	svc.settleFlowChildTurnCompletedLocked(leg, "tdd leg wrote tests", ProviderEvent{})
	svc.mu.Unlock()

	if st := svc.lookupFlowStepStatus(parentID, "tdd"); st != StepStatusDone {
		t.Fatalf("completed leg's step mirror must settle DONE at settle time — got %q", st)
	}
}

// Guard: a same-label sibling still holding live work must keep the node's
// RUNNING stamp (duplicate-label class, BUG-639) — completing one leg does
// not stamp over its sibling's in-flight step.
func TestBug647_CompletedLegLeavesLiveSiblingStamp(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc, _ := newTestServerWith(t, reg, nil, nil)

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	parentID := parent.RunID

	svc.mu.Lock()
	rs := svc.runs[parentID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.status = RunStatusRunning
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "tdd", Behavior: "agent.scaffold", Agent: "agents/scaffold-architect.md"},
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
	}
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "tdd", To: "coder", When: "done", Kind: "forward"},
	}
	leg := &interactiveRun{
		id:            "run-tdd-leg",
		parentRunID:   parentID,
		label:         "tdd",
		agentName:     "scaffold-architect",
		role:          "coder",
		providerKey:   ProviderKeyCodex,
		status:        RunStatusCompleted,
		agentStatus:   string(RunStatusCompleted),
		waitForResult: true,
		subs:          map[int64]chan ProviderEvent{},
	}
	svc.runs[leg.id] = leg
	// Duplicate-label sibling still running under the same node id.
	svc.runs["run-tdd-leg-live"] = &interactiveRun{
		id:          "run-tdd-leg-live",
		parentRunID: parentID,
		label:       "tdd",
		providerKey: ProviderKeyCodex,
		status:      RunStatusRunning,
		subs:        map[int64]chan ProviderEvent{},
	}
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 5, RoundCap: 5})
	svc.mu.Unlock()

	store, ok := svc.workflowStore.(*fakeWorkflowStore)
	if !ok {
		t.Fatalf("expected fakeWorkflowStore, got %T", svc.workflowStore)
	}
	store.seed(parentID, []RuntimeWorkflowStep{
		{ID: "tdd", NodeID: "tdd", Status: StepStatusRunning},
		{ID: "coder", NodeID: "coder", Status: StepStatusPending},
	})

	svc.mu.Lock()
	svc.settleFlowChildTurnCompletedLocked(leg, "tdd leg wrote tests", ProviderEvent{})
	svc.mu.Unlock()

	if st := svc.lookupFlowStepStatus(parentID, "tdd"); st != StepStatusRunning {
		t.Fatalf("step mirror must stay RUNNING while a same-label sibling holds live work — got %q", st)
	}
}
