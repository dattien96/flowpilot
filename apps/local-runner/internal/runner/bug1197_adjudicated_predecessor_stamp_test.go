package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// BUG-1197 (live run-225691): tryAdvanceFlowFromNode stamps the completing
// node DONE and dispatches its successor, but a strict done-edge predecessor
// whose own leg already went durably Completed — its settle consumed by a
// gate divert / debate stash / stale-completion claim — keeps RUNNING
// forever. Live: tdd stayed RUNNING while coder, validate and spec_align all
// stamped DONE; the adjudicated node leaked a mid-spine RUNNING badge.
//
// Contract: advancing PAST a node whose leg durably completed means the flow
// adjudicated its outcome — the step row must stamp DONE atomically with the
// forward advance. A predecessor with no completed leg (in-flight parallel
// work, never-dispatched spine) is left alone.
func TestBug1197_AdvanceSettlesAdjudicatedPredecessor(t *testing.T) {
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
		{ID: "validate", Behavior: "agent.delegate", Agent: "agents/validator.md"},
	}
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "tdd", To: "coder", When: "done", Kind: "forward"},
		{From: "coder", To: "validate", When: "done", Kind: "forward"},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Cap: 5, RoundCap: 5})

	store, ok := svc.workflowStore.(*fakeWorkflowStore)
	if !ok {
		t.Fatalf("expected fakeWorkflowStore, got %T", svc.workflowStore)
	}
	// Live shape: tdd's leg finished and its outcome was adjudicated, but the
	// settle that would have stamped the step was consumed — the durable
	// completed session is the evidence (persistedCompletedChildExists).
	store.seed(parentID, []RuntimeWorkflowStep{
		{ID: "tdd", NodeID: "tdd", Status: StepStatusRunning},
		{ID: "coder", NodeID: "coder", Status: StepStatusRunning},
		{ID: "validate", NodeID: "validate", Status: StepStatusPending},
	})
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:       "run-tdd-leg",
		ParentRunID: parentID,
		ProjectID:   "proj",
		ProviderKey: ProviderKeyCodex,
		Label:       "tdd",
		Status:      RunStatusCompleted,
		RunKind:     "delegate",
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	svc.tryAdvanceFlowFromNode(parentID, "coder", "coder leg completed")

	if st := svc.lookupFlowStepStatus(parentID, "coder"); st != StepStatusDone {
		t.Fatalf("coder step must stamp DONE on its own completion, got %q", st)
	}
	if st := svc.lookupFlowStepStatus(parentID, "tdd"); st != StepStatusDone {
		t.Fatalf("adjudicated predecessor tdd (leg durably completed) must stamp DONE when the flow advances past it — got %q", st)
	}
}

// Guard: a predecessor whose leg never durably completed (still running, or
// never spawned) is NOT ours to stamp — parallel in-flight work and the
// never-dispatched spine keep their own stamps.
func TestBug1197_AdvanceLeavesUnfinishedPredecessor(t *testing.T) {
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
		{ID: "validate", Behavior: "agent.delegate", Agent: "agents/validator.md"},
	}
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "tdd", To: "coder", When: "done", Kind: "forward"},
		{From: "coder", To: "validate", When: "done", Kind: "forward"},
	}
	// tdd leg is still genuinely in flight — a live child run plus a
	// completed PRIOR-round session row (round re-entry reuses node ids).
	svc.runs["run-tdd-leg-live"] = &interactiveRun{
		id:          "run-tdd-leg-live",
		parentRunID: parentID,
		label:       "tdd",
		providerKey: ProviderKeyCodex,
		status:      RunStatusRunning,
		subs:        map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Cap: 5, RoundCap: 5})

	store, ok := svc.workflowStore.(*fakeWorkflowStore)
	if !ok {
		t.Fatalf("expected fakeWorkflowStore, got %T", svc.workflowStore)
	}
	store.seed(parentID, []RuntimeWorkflowStep{
		{ID: "tdd", NodeID: "tdd", Status: StepStatusRunning},
		{ID: "coder", NodeID: "coder", Status: StepStatusRunning},
		{ID: "validate", NodeID: "validate", Status: StepStatusPending},
	})
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:       "run-tdd-leg-prior",
		ParentRunID: parentID,
		ProjectID:   "proj",
		ProviderKey: ProviderKeyCodex,
		Label:       "tdd",
		Status:      RunStatusCompleted,
		RunKind:     "delegate",
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	svc.tryAdvanceFlowFromNode(parentID, "coder", "coder leg completed")

	if st := svc.lookupFlowStepStatus(parentID, "coder"); st != StepStatusDone {
		t.Fatalf("coder step must stamp DONE on its own completion, got %q", st)
	}
	if st := svc.lookupFlowStepStatus(parentID, "tdd"); st != StepStatusRunning {
		t.Fatalf("predecessor tdd whose leg is still in flight must stay RUNNING — got %q", st)
	}
}
