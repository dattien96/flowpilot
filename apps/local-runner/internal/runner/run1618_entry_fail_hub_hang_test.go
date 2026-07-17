package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// TestNonCohortEntryFailSettlesStepAndClearsHubGateSettle is the run-1618 / A1
// regression: when a flow entry node (Wait=false, no cohort) fails — e.g. coder
// with no connected Claude account — the hub must not hang "active" forever.
//
// Before the fix:
//   - coder step stayed RUNNING (only cohort failures stamped FAILED)
//   - hub kept pendingFlowGateSettle from the flowStartOnly synthetic turn
//   - no hub reinvoke → main looked stuck while child was already failed
func TestNonCohortEntryFailSettlesStepAndClearsHubGateSettle(t *testing.T) {
	// Registry with a no-op adapter so the hub reinvoke startTurn can succeed
	// (or at least get past provider construction).
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "hub saw the failure"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun(parent): %v", err)
	}
	parentID := parent.RunID
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 3, RoundCap: 3})

	nodes := []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.delegate", Agent: "agents/coder.md"},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md"},
	}
	svc.mu.Lock()
	p := svc.runs[parentID]
	p.activeFlowNodes = nodes
	p.autoOrchestrate = true
	p.flowEngineDriven = true
	// Stale gate settle as left by flowStartOnly synthetic EventTurnCompleted.
	p.pendingFlowGateSettle = true
	p.pendingFlowGateTurnID = "turn-synthetic-entry"
	p.pendingFlowGateFinalMsg = ""
	p.postTurnGateCancel = nil
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parentID, nodes)
	svc.setFlowStepStatus(context.Background(), parentID, "coder", StepStatusRunning)

	// Child mirrors a Wait=false flow entry spawn (review-loop coder).
	child, err := svc.createRun(StartRunInput{
		ProjectID:   "proj",
		ChatMode:    "normal_chat",
		ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("createRun(child): %v", err)
	}
	svc.mu.Lock()
	crs := svc.runs[child.RunID]
	crs.parentRunID = parentID
	crs.agentName = "coder"
	crs.label = "coder"
	crs.role = "coder"
	crs.waitForResult = false
	crs.uiInitiated = false
	crs.status = RunStatusRunning
	crs.agentStatus = string(RunStatusRunning)
	svc.mu.Unlock()

	// Emit the same terminal failure EventTurnFailed path live used for
	// "no connected local account found for provider claude".
	svc.mu.Lock()
	svc.emitLocked(crs, ProviderEvent{
		Type:  EventTurnFailed,
		Error: `no connected local account found for provider "claude"`,
	})
	pendingSettle := svc.runs[parentID].pendingFlowGateSettle
	var notes []string
	if pr := svc.runs[parentID]; pr != nil {
		notes = append([]string(nil), pr.pendingAgentContext...)
	}
	reinvokeInFlight := svc.runs[parentID].reinvokeInFlight
	svc.mu.Unlock()

	if got := flowStepStatus(t, svc, parentID, "coder"); got != StepStatusFailed {
		t.Errorf("coder step = %v, want FAILED after non-cohort entry failure", got)
	}
	if pendingSettle {
		t.Error("hub pendingFlowGateSettle still true; must clear stale flowStartOnly settle so reinvoke is not gate_in_progress")
	}
	joined := strings.Join(notes, "\n")
	if !strings.Contains(joined, "failed") || !strings.Contains(joined, "claude") {
		t.Errorf("pendingAgentContext missing failure note, got %#v", notes)
	}

	// Reinvoke is scheduled with reinvokeInFlight=true then go startTurn.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		pr := svc.runs[parentID]
		armed := pr != nil && (pr.reinvokeInFlight || pr.pendingHubReinvoke || pr.turnInFlight || pr.hubReinvokeStartFailCount > 0 || pr.turnCount > 0)
		svc.mu.Unlock()
		if armed || reinvokeInFlight {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	svc.mu.Lock()
	pr := svc.runs[parentID]
	armed := pr != nil && (pr.reinvokeInFlight || pr.pendingHubReinvoke || pr.turnInFlight || pr.hubReinvokeStartFailCount > 0 || pr.turnCount > 0)
	svc.mu.Unlock()
	if !armed && !reinvokeInFlight {
		t.Error("expected hub reinvoke to arm (reinvokeInFlight / pendingHubReinvoke / turnInFlight) after entry node failure")
	}
}

// TestNonCohortPreflightStartTurnFailReinvokesHub is H-A: startTurn fails after
// the child run exists (spawnChildRun async path) — never hits EventTurnFailed
// emitLocked. handleChildStartTurnFailure must settle step FAILED and reinvoke hub.
func TestNonCohortPreflightStartTurnFailReinvokesHub(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "hub recovery"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun(parent): %v", err)
	}
	parentID := parent.RunID
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 3, RoundCap: 3})
	nodes := []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.delegate", Agent: "agents/coder.md"},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md"},
	}
	svc.mu.Lock()
	p := svc.runs[parentID]
	p.activeFlowNodes = nodes
	p.autoOrchestrate = true
	p.flowEngineDriven = true
	p.pendingFlowGateSettle = true
	p.pendingFlowGateTurnID = "turn-stale"
	p.postTurnGateCancel = nil
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parentID, nodes)
	svc.setFlowStepStatus(context.Background(), parentID, "coder", StepStatusRunning)

	// Child already exists (post-createRun, pre-adapter) — mirrors the state
	// right before spawnChildRun's async startTurn fails.
	child, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun(child): %v", err)
	}
	svc.mu.Lock()
	crs := svc.runs[child.RunID]
	crs.parentRunID = parentID
	crs.agentName = "coder"
	crs.label = "coder"
	crs.role = "coder"
	crs.waitForResult = false
	crs.uiInitiated = false
	crs.status = RunStatusRunning
	crs.agentStatus = string(RunStatusRunning)
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, child.RunID)

	svc.handleChildStartTurnFailure(child.RunID, parentID, `no connected local account found for provider "claude"`)

	if got := flowStepStatus(t, svc, parentID, "coder"); got != StepStatusFailed {
		t.Fatalf("coder step = %v, want FAILED after preflight startTurn fail", got)
	}
	svc.mu.Lock()
	settle := svc.runs[parentID].pendingFlowGateSettle
	notes := append([]string(nil), svc.runs[parentID].pendingAgentContext...)
	childStatus := svc.runs[child.RunID].status
	svc.mu.Unlock()
	if settle {
		t.Fatal("stale pendingFlowGateSettle must clear on preflight fail")
	}
	if childStatus != RunStatusFailed {
		t.Fatalf("child status = %v, want failed", childStatus)
	}
	if !strings.Contains(strings.Join(notes, "\n"), "failed") {
		t.Fatalf("expected failure note in pendingAgentContext, got %#v", notes)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		armed := svc.runs[parentID].reinvokeInFlight || svc.runs[parentID].pendingHubReinvoke ||
			svc.runs[parentID].turnInFlight || svc.runs[parentID].hubReinvokeStartFailCount > 0 ||
			svc.runs[parentID].turnCount > 0
		svc.mu.Unlock()
		if armed {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("expected hub reinvoke to arm after non-cohort preflight startTurn fail")
}

// TestFlowStartAllEntrySpawnsFailedReinvokesHub is H-B: spawnChildRun errors for
// every entry node — hub must not sit active with zero children.
func TestFlowStartAllEntrySpawnsFailedReinvokesHub(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "hub saw spawn fail"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	// Force child createRun path to fail by... actually spawnChildRun fails on
	// persist or adapter. Easier: call notifyHubOfFlowEntrySpawnFailure directly
	// for unit contract, plus integration via startResolvedFlow with broken agent.
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	parentID := parent.RunID
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 3, RoundCap: 3})
	nodes := []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.delegate", Agent: "agents/coder.md"},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	svc.mu.Lock()
	svc.runs[parentID].activeFlowNodes = nodes
	svc.runs[parentID].autoOrchestrate = true
	svc.runs[parentID].flowEngineDriven = true
	svc.runs[parentID].pendingFlowGateSettle = true
	svc.runs[parentID].postTurnGateCancel = nil
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parentID, nodes)
	svc.setFlowStepStatus(context.Background(), parentID, "coder", StepStatusFailed)

	svc.notifyHubOfFlowEntrySpawnFailure(parentID, "flowpilot-core-flow-pack/review-loop",
		[]string{"coder"}, "persist child failed: disk full")

	svc.mu.Lock()
	settle := svc.runs[parentID].pendingFlowGateSettle
	notes := append([]string(nil), svc.runs[parentID].pendingAgentContext...)
	svc.mu.Unlock()
	if settle {
		t.Fatal("H-B must clear stale pendingFlowGateSettle")
	}
	if !strings.Contains(strings.Join(notes, "\n"), "Failed to start flow entry") {
		t.Fatalf("expected entry-spawn failure note, got %#v", notes)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		armed := svc.runs[parentID].reinvokeInFlight || svc.runs[parentID].pendingHubReinvoke ||
			svc.runs[parentID].turnInFlight || svc.runs[parentID].hubReinvokeStartFailCount > 0 ||
			svc.runs[parentID].turnCount > 0
		svc.mu.Unlock()
		if armed {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Error("expected hub reinvoke after all entry spawns failed")
}

// TestHubStallFiresDespiteStalePendingGateSettle is H-C: F-0 must not treat
// pendingFlowGateSettle alone (no live postTurnGateCancel) as forever-busy.
func TestHubStallFiresDespiteStalePendingGateSettle(t *testing.T) {
	svc := bug289Service(t)
	runID := "run-hc-stale-settle"
	rs := &interactiveRun{
		id:                    runID,
		flowEngineDriven:      true,
		status:                RunStatusRunning,
		hubLastProgressAt:     time.Now().UTC().Add(-10 * time.Minute),
		stallTimeout:          time.Second,
		pendingFlowGateSettle: true, // stale — no postTurnGateCancel
		pendingFlowGateTurnID: "turn-stale-synthetic",
		subs:                  map[int64]chan ProviderEvent{},
	}
	svc.mu.Lock()
	svc.runs[runID] = rs
	svc.mu.Unlock()
	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		return st
	})

	if !svc.checkAndBlockStalledHub(runID) {
		t.Fatal("expected hub_stalled: stale pendingFlowGateSettle alone must not be busy forever")
	}
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.Status != "blocked" || loop.BlockReason != "hub_stalled" {
		t.Fatalf("loop = %+v, want blocked/hub_stalled", loop)
	}
}

// TestHubStallStillBusyDuringLivePostTurnGate ensures H-C does not break the
// legitimate "gate evaluating" busy window (postTurnGateCancel armed).
func TestHubStallStillBusyDuringLivePostTurnGate(t *testing.T) {
	svc := bug289Service(t)
	runID := "run-hc-live-gate"
	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	rs := &interactiveRun{
		id:                runID,
		flowEngineDriven:  true,
		status:            RunStatusRunning,
		hubLastProgressAt: time.Now().UTC().Add(-10 * time.Minute),
		stallTimeout:      time.Second,
		// Live gate: cancel func set.
		pendingFlowGateSettle: true,
		postTurnGateCancel:    cancel,
		subs:                  map[int64]chan ProviderEvent{},
	}
	svc.mu.Lock()
	svc.runs[runID] = rs
	svc.mu.Unlock()
	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		return st
	})

	if svc.checkAndBlockStalledHub(runID) {
		t.Fatal("must NOT hub_stalled while postTurnGateCancel is live (gate in progress)")
	}
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.Status == "blocked" {
		t.Fatalf("loop should stay running during live gate, got %+v", loop)
	}
}

// TestFlowStartOnlyClearsSyntheticPendingGateSettle locks the flowStartOnly
// handoff path: synthetic EventTurnCompleted must not leave the hub with
// pendingFlowGateSettle=true (run-1618 hang precondition).
func TestFlowStartOnlyClearsSyntheticPendingGateSettle(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				// Must not be invoked for flowStartOnly.
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
	svc.mu.Lock()
	if rs := svc.runs[parentID]; rs != nil {
		rs.turnCount = 0
	}
	svc.mu.Unlock()

	_, apiErr := svc.startTurn(parentID, TurnInput{
		StepID:  "chat-step-1",
		Prompt:  "fix bug 1+1 != 2",
		FlowRef: "flowpilot-core-flow-pack/review-loop",
	}, "", "")
	if apiErr != nil {
		t.Fatalf("startTurn(flowStartOnly): %v", apiErr)
	}

	// Give async startResolvedFlow a moment; settle flags must already be clear.
	time.Sleep(50 * time.Millisecond)

	svc.mu.Lock()
	rs := svc.runs[parentID]
	if rs == nil {
		svc.mu.Unlock()
		t.Fatal("parent run missing")
	}
	if rs.pendingFlowGateSettle {
		svc.mu.Unlock()
		t.Fatal("flowStartOnly left pendingFlowGateSettle=true; hub would hang on gate_in_progress")
	}
	if rs.turnInFlight {
		svc.mu.Unlock()
		t.Fatal("flowStartOnly left turnInFlight=true")
	}
	if !rs.flowEngineDriven {
		svc.mu.Unlock()
		t.Fatal("expected flowEngineDriven after FlowRef start")
	}
	svc.mu.Unlock()
}
