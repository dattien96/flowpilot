package runner

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// TestStartTurnRejectsWhenLoopBlocked is run-1675: while the escalate / Continue
// form is up (loop.Status=blocked), neither hub nor child may start a new turn
// (gate reprompt, hub reinvoke, freeform coding).
func TestStartTurnRejectsWhenLoopBlocked(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "should not run"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	parentID := parent.RunID
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{
		Status: "blocked", BlockReason: "escalate", Cap: 3, RoundCap: 3, Mode: "explicit",
		GateReason: "Coder sub-agent failed: no Claude account",
	})
	svc.mu.Lock()
	svc.runs[parentID].flowEngineDriven = true
	svc.mu.Unlock()

	_, apiErr := svc.startTurn(parentID, TurnInput{StepID: "chat-step", Prompt: "gate reprompt: create BUG doc"}, "", "")
	if apiErr == nil {
		t.Fatal("expected startTurn to reject while loop is blocked")
	}
	if apiErr.code != "flow_awaiting_user" {
		t.Fatalf("code = %q, want flow_awaiting_user (got %s)", apiErr.code, apiErr.msg)
	}
}

// TestParkFlowForAwaitingUserCancelsTurnAndDropsReprompt: escalate freezes
// in-flight hub work and drops pending gate reprompt so main cannot continue.
func TestParkFlowForAwaitingUserCancelsTurnAndDropsReprompt(t *testing.T) {
	svc, parentID := newFlowTestRun(t)
	var cancelled atomic.Bool
	svc.mu.Lock()
	rs := svc.runs[parentID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.turnInFlight = true
	rs.pendingGateRepromptPrompt = "The flow gate is asking you to add a required document..."
	rs.pendingGateRepromptStepID = "chat-step"
	rs.pendingGateRepromptGen = 1
	rs.pendingHubReinvoke = true
	rs.pendingFlowGateSettle = true
	ctx, cancel := context.WithCancel(context.Background())
	rs.turnCancel = func() {
		cancelled.Store(true)
		cancel()
	}
	_ = ctx
	svc.mu.Unlock()

	svc.parkFlowForAwaitingUser(parentID)

	if !cancelled.Load() {
		t.Fatal("expected in-flight hub turnCancel to fire")
	}
	svc.mu.Lock()
	rs = svc.runs[parentID]
	if rs.pendingGateRepromptPrompt != "" || rs.pendingGateRepromptStepID != "" {
		t.Fatalf("gate reprompt intent must be dropped, got prompt=%q step=%q",
			rs.pendingGateRepromptPrompt, rs.pendingGateRepromptStepID)
	}
	if rs.pendingHubReinvoke {
		t.Fatal("pendingHubReinvoke must clear")
	}
	if rs.pendingFlowGateSettle {
		t.Fatal("pendingFlowGateSettle must clear so gate cannot re-queue")
	}
	if rs.turnInFlight && rs.turnCancel != nil {
		// turnInFlight may still be true until finishTurn; cancel must have been called.
	}
	svc.mu.Unlock()
}

// TestEscalateParksFlowAndBlocksStartTurn: applyFlowControl escalate → form
// invariant end-to-end.
func TestEscalateParksFlowAndBlocksStartTurn(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "nope"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	parentID := parent.RunID
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3, Mode: "explicit"})
	nodes := []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.delegate"},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	svc.mu.Lock()
	svc.runs[parentID].flowEngineDriven = true
	svc.runs[parentID].autoOrchestrate = true
	svc.runs[parentID].activeFlowNodes = nodes
	svc.runs[parentID].pendingGateRepromptPrompt = "Missing BugFix document..."
	svc.runs[parentID].pendingGateRepromptStepID = "chat-step"
	var cancelled atomic.Bool
	svc.runs[parentID].turnInFlight = true
	svc.runs[parentID].turnCancel = func() { cancelled.Store(true) }
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parentID, nodes)

	result, aerr := svc.applyFlowControl(parentID, FlowControlInput{
		Status:  "escalate",
		Summary: `Coder sub-agent (provider: claude) failed: no connected local account`,
	})
	if aerr != nil {
		t.Fatalf("applyFlowControl: %v", aerr)
	}
	if result.NextAction != "awaiting_user" {
		t.Fatalf("NextAction = %q, want awaiting_user", result.NextAction)
	}
	loop := svc.agentOrchestrator.loopStateFor(parentID)
	if loop.Status != "blocked" {
		t.Fatalf("loop.Status = %q, want blocked", loop.Status)
	}
	if !cancelled.Load() {
		t.Fatal("escalate must cancel in-flight hub turn")
	}
	svc.mu.Lock()
	if svc.runs[parentID].pendingGateRepromptPrompt != "" {
		t.Fatal("escalate must drop pending gate reprompt (run-1675 hang class)")
	}
	svc.mu.Unlock()

	_, apiErr := svc.startTurn(parentID, TurnInput{StepID: "chat-step", Prompt: "create bugfix now"}, "", "")
	if apiErr == nil || apiErr.code != "flow_awaiting_user" {
		t.Fatalf("startTurn after escalate: err=%v, want flow_awaiting_user", apiErr)
	}
}

// TestFlushDurableTurnIntentsSkippedWhenBlocked: notifyTurnIdle must not
// start a gate reprompt while the Continue form is showing.
func TestFlushDurableTurnIntentsSkippedWhenBlocked(t *testing.T) {
	reg := newProviderRegistry()
	var turns atomic.Int32
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				turns.Add(1)
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "should not"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	parentID := parent.RunID
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "blocked", BlockReason: "escalate", Cap: 3})
	svc.mu.Lock()
	svc.runs[parentID].pendingGateRepromptPrompt = "create BUG-906 doc"
	svc.runs[parentID].pendingGateRepromptStepID = "chat-step"
	svc.runs[parentID].pendingGateRepromptGen = 1
	svc.mu.Unlock()

	svc.flushDurableTurnIntents(parentID)
	time.Sleep(100 * time.Millisecond)
	if turns.Load() != 0 {
		t.Fatalf("flush while blocked started %d turn(s); want 0", turns.Load())
	}
}
