package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// BUG-1185 residual — the orphan-cure in resumeFlowWithFeedback only cured
// parked children carrying pendingGateCodePaths (BUG-520's gate-debt seed).
// A child parked waiting_user_approval with NO gate debt but an UNFINISHED
// flow step (Running/WaitingUserApr) was skipped — orphaned forever behind
// the same park (live CP-03: children frozen by the hub-stall park without
// any gate violation). The unfinished step row is the owed-work evidence.
func TestBug1185_ContinueReDrivesParkedChildWithUnfinishedStep(t *testing.T) {
	reg := newProviderRegistry()
	adapterCh := make(chan TurnRequest, 2)
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				adapterCh <- req
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc, srv := newTestServerWith(t, reg, nil, nil)

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	parentID := parent.RunID
	childID := "run-1185-orphan"

	svc.mu.Lock()
	rs := svc.runs[parentID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.status = RunStatusRunning
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "rollout", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke"},
		{ID: "candidate-b", Behavior: "agent.delegate", Agent: "agents/coder.md", Lifecycle: "spawn"},
	}
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "rollout", To: "candidate-b", When: "done", Kind: "forward"},
	}
	svc.runs[childID] = &interactiveRun{
		id:                childID,
		parentRunID:       parentID,
		label:             "candidate-b",
		providerKey:       ProviderKeyCodex,
		providerAccountID: rs.providerAccountID,
		status:            RunStatusWaitingUserApr,
		agentStatus:       "waiting_user_approval",
		stepID:            "step-candidate-b",
		// NO pendingGateCodePaths — the park froze this child mid-step with
		// no gate violation; the unfinished step row is the only evidence.
		subs: map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, childID)
	svc.agentOrchestrator.upsertSummary(parentID, AgentRunSummary{
		RunID: childID, ParentRunID: parentID, AgentName: "coder", Label: "candidate-b",
		Status: RunStatusWaitingUserApr, AgentStatus: "waiting_user_approval",
	})
	// The child's flow step is still mid-flight — the owed-work marker the
	// cure must read.
	store, ok := svc.workflowStore.(*fakeWorkflowStore)
	if !ok {
		t.Fatalf("expected fakeWorkflowStore, got %T", svc.workflowStore)
	}
	store.seed(parentID, []RuntimeWorkflowStep{
		{ID: "rollout", NodeID: "rollout", Status: StepStatusDone},
		{ID: "candidate-b", NodeID: "candidate-b", Status: StepStatusRunning},
	})
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "blocked", Cap: 5, RoundCap: 5, BlockReason: "hub_stalled"})

	body, _ := json.Marshal(map[string]string{"feedback": "continue"})
	resp, herr := http.Post(srv.URL+"/client/workflow-runs/"+parentID+"/agent-loop/continue", "application/json", strings.NewReader(string(body)))
	if herr != nil {
		t.Fatalf("continue POST: %v", herr)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("continue POST status=%d", resp.StatusCode)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		st := svc.runs[childID].status
		svc.mu.Unlock()
		if st != RunStatusWaitingUserApr {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	svc.mu.Lock()
	st := svc.runs[childID].status
	svc.mu.Unlock()
	if st == RunStatusWaitingUserApr {
		t.Fatal("Continue must re-drive a parked child whose step is unfinished — still waiting_user_approval")
	}
	childTurn := false
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !childTurn {
		select {
		case req := <-adapterCh:
			if req.RunID == childID {
				childTurn = true
			}
		case <-time.After(100 * time.Millisecond):
		}
	}
	if !childTurn {
		t.Fatal("orphan re-drive never dispatched a provider turn to the child")
	}
}

// Guard: a parked child whose step is DONE (or terminal) stays parked — the
// cure must only touch members that still owe work.
func TestBug1185_ParkedChildWithDoneStepStaysPut(t *testing.T) {
	reg := newProviderRegistry()
	adapterCh := make(chan TurnRequest, 2)
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				adapterCh <- req
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc, srv := newTestServerWith(t, reg, nil, nil)

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	parentID := parent.RunID
	childID := "run-1185-done"

	svc.mu.Lock()
	rs := svc.runs[parentID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.status = RunStatusRunning
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "rollout", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke"},
		{ID: "candidate-b", Behavior: "agent.delegate", Agent: "agents/coder.md", Lifecycle: "spawn"},
	}
	svc.runs[childID] = &interactiveRun{
		id:                childID,
		parentRunID:       parentID,
		label:             "candidate-b",
		providerKey:       ProviderKeyCodex,
		providerAccountID: rs.providerAccountID,
		status:            RunStatusWaitingUserApr,
		agentStatus:       "waiting_user_approval",
		subs:              map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, childID)
	store2, ok := svc.workflowStore.(*fakeWorkflowStore)
	if !ok {
		t.Fatalf("expected fakeWorkflowStore, got %T", svc.workflowStore)
	}
	store2.seed(parentID, []RuntimeWorkflowStep{
		{ID: "candidate-b", NodeID: "candidate-b", Status: StepStatusDone},
	})
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "blocked", Cap: 5, RoundCap: 5, BlockReason: "hub_stalled"})

	body, _ := json.Marshal(map[string]string{"feedback": "continue"})
	resp, herr := http.Post(srv.URL+"/client/workflow-runs/"+parentID+"/agent-loop/continue", "application/json", strings.NewReader(string(body)))
	if herr != nil {
		t.Fatalf("continue POST: %v", herr)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("continue POST status=%d", resp.StatusCode)
	}

	time.Sleep(400 * time.Millisecond)
	svc.mu.Lock()
	st := svc.runs[childID].status
	svc.mu.Unlock()
	if st != RunStatusWaitingUserApr {
		t.Fatalf("child with a DONE step must not be re-driven as an orphan, status=%q", st)
	}
	// The hub's own generic-reinvoke turn is legitimate — only a turn
	// targeting the child would be the orphan cure misfiring.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		select {
		case req := <-adapterCh:
			if req.RunID == childID {
				t.Fatalf("a DONE-step child must not receive a re-drive turn, got %q", req.Prompt)
			}
		case <-time.After(50 * time.Millisecond):
		}
	}
}
