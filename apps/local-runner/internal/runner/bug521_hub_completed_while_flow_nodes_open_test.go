package runner

import (
	"context"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// BUG-521 (live run-60145): after the BUG-520 wedge parked a tournament flow
// (loop=blocked/hub_stalled, child candidate-b WAITING_USER_APPROVAL, nodes
// tournament_arbiter/merge_and_audit PENDING), an operator `continue` dispatched
// a plain hub turn which settled and stamped the run status=completed — a
// dishonest terminal surface while the flow graph still had required nodes
// non-terminal.
//
// BUG-507 already withholds `completed` for a run hosting an open flow loop on
// the non-gated completion path; this is the sibling seam — the post-turn gate
// pass path publishes Completed unconditionally, so a flowEngineDriven hub turn
// (which ALWAYS arms pendingFlowGateSettle) leaked `completed` while the loop
// was open.

// TestBug521_GatePassWithholdsCompletedWhileLoopOpen pins that a flow hub's
// clean turn whose post-turn gate passes must NOT publish status=completed
// while the mounted flow loop is still open. The gate verifies only the
// turn's diff — never the graph — so the run stays running and the loop's own
// seal/escalation publishes the honest terminal later.
//
// Both live shapes from run-60145 are covered:
//   - "running": the post-continue hub turn drained with the loop unblocked
//     but still open (required nodes still PENDING/WAITING).
//   - "blocked": the turn was admitted while running, then the flow parked
//     mid-turn (escalate/hub_stalled) — the preserved turn drains onto an
//     already-blocked loop.
func TestBug521_GatePassWithholdsCompletedWhileLoopOpen(t *testing.T) {
	for _, midTurnLoopStatus := range []string{"", "blocked"} {
		name := "loop_running"
		if midTurnLoopStatus != "" {
			name = "loop_blocked_mid_turn"
		}
		t.Run(name, func(t *testing.T) {
			reg := newProviderRegistry()
			var svc *InteractiveService
			var runID string
			reg.register(ProviderRegistration{
				Key: ProviderKeyGrok, Status: ProviderStatusAvailable,
				Capabilities: ProviderCapabilities{Streaming: true},
				newAdapter: func() ProviderRuntimeAdapter {
					return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
						if midTurnLoopStatus != "" && svc != nil {
							// Mid-turn park: the flow escalated while this
							// preserved turn was in flight (BUG-437 park
							// semantics — the hub's own turn drains onto the
							// now-blocked loop).
							svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
								st.Status = midTurnLoopStatus
								st.BlockReason = "escalate"
								st.GateReason = "merge decision parked for operator"
								return st
							})
						}
						b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "hub turn settled"})
						return nil
					})
				},
			})
			svc = newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
			run, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyGrok})
			if err != nil {
				t.Fatalf("createRun: %v", err)
			}
			runID = run.RunID
			// Mark the run as a flow hub (the post-turn gate deferral keys off
			// this) and mount an open loop — the live shape of run-60145's
			// continue-dispatched turn.
			svc.mu.Lock()
			svc.runs[run.RunID].flowEngineDriven = true
			svc.mu.Unlock()
			svc.agentOrchestrator.setLoop(run.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

			if _, apiErr := svc.startTurn(run.RunID, TurnInput{StepID: "turn-1", Prompt: "synthesize"}, "", ""); apiErr != nil {
				t.Fatalf("startTurn: %s", apiErr.msg)
			}
			waitLoop(t, "turn settled", 3*time.Second, func() bool {
				svc.mu.Lock()
				defer svc.mu.Unlock()
				return !svc.runs[run.RunID].turnInFlight
			})

			svc.mu.Lock()
			defer svc.mu.Unlock()
			if svc.runs[run.RunID].status == RunStatusCompleted {
				t.Fatalf("flow hub must not report completed while its loop is %q (nodes may still be PENDING/WAITING)", svc.agentOrchestrator.loopStateFor(run.RunID).Status)
			}
		})
	}
}

// TestBug521_ResumedGatePassWithholdsCompletedWhileLoopOpen covers the restart
// seam: a persisted pendingFlowGateSettle on a flow hub whose loop is still
// "running" must not publish completed when the re-run post-turn gate passes —
// the restart path (resumePendingFlowGate) is the second place the dishonest
// terminal could be stamped.
func TestBug521_ResumedGatePassWithholdsCompletedWhileLoopOpen(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyGrok, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "drained"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	run, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyGrok})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[run.RunID]
	rs.flowEngineDriven = true
	rs.status = RunStatusRunning
	rs.agentStatus = string(RunStatusRunning)
	// Durable armed settle — the shape a restart reconstructs when the hub's
	// turn drained after kill before the post-turn gate ran.
	rs.pendingFlowGateSettle = true
	rs.pendingFlowGateFinalMsg = "drained turn"
	rs.pendingFlowGateOccurredAt = time.Now().UTC().Format(time.RFC3339Nano)
	rs.pendingFlowGateTurnID = "turn-1"
	rs.lastTurnID = "turn-1"
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(run.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	svc.resumePendingFlowGate(run.RunID)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.runs[run.RunID].status == RunStatusCompleted {
		t.Fatal("resumed flow hub must not publish completed while its loop is still running")
	}
	if svc.runs[run.RunID].pendingFlowGateSettle {
		t.Fatal("gate settle must be consumed after a passing gate")
	}
}

// TestBug521_SealedLoopGatePassStillCompletes guards the regression direction:
// once the loop is sealed (done), the hub's settle path must still publish
// status=completed like before.
func TestBug521_SealedLoopGatePassStillCompletes(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyGrok, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "flow done"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	run, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyGrok})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	svc.runs[run.RunID].flowEngineDriven = true
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(run.RunID, AgentLoopState{Status: "done", Cap: 3, RoundCap: 3})

	if _, apiErr := svc.startTurn(run.RunID, TurnInput{StepID: "turn-1", Prompt: "hi"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn: %s", apiErr.msg)
	}
	waitLoop(t, "turn settled", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[run.RunID].turnInFlight
	})
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.runs[run.RunID].status != RunStatusCompleted {
		t.Fatalf("sealed-loop hub turn must still complete, got %q", svc.runs[run.RunID].status)
	}
}

// TestBug521_LiveFlowRunStaysRunningWhileLoopOpen drives the REAL entry path —
// startTurn(FlowRef) on a first turn (the call handleStartTurn makes after
// resolving the ref) → startResolvedFlow → delegate children → edge advance →
// hub reinvoke turn → post-turn gate — and asserts the run never reports
// completed while the mounted loop is still open. On the buggy code the hub's
// drained turn published completed the moment its diff gate passed.
func TestBug521_LiveFlowRunStaysRunningWhileLoopOpen(t *testing.T) {
	const liveFlowRef = "66666666-6666-6666-6666-666666666666"

	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				if autoAnswerPreflightContractPlanTurn(req, b) {
					return nil
				}
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	catalog := newInteractiveCatalog()
	catalog.steps[liveFlowRef] = []Step{
		{ID: "drafting", WorkflowID: liveFlowRef, Name: "drafting", Order: 1, Model: "gpt-5.4"},
	}
	svc := newInteractiveService(reg, catalog, newFakeWorkflowStore())

	store := newFakeFlowDefinitionStore()
	store.byRef[liveFlowRef] = FlowDefinitionRecord{
		FlowRef:   liveFlowRef,
		Name:      "Live Bug-521 Flow",
		Source:    "supabase_user_definition",
		Editable:  true,
		Cloneable: true,
		Definition: agentpack.FlowDefinition{
			ID: "bug521-live-two-step",
			Nodes: []agentpack.FlowNode{
				{ID: "drafting", Behavior: "agent.delegate", Agent: "agents/coder.md"},
				{ID: "final_check", Behavior: "agent.delegate", Agent: "agents/reviewer.md", DependsOn: []string{"drafting"}},
			},
			Edges: []agentpack.FlowEdge{
				{From: "drafting", To: "final_check", When: "done", Kind: "forward"},
			},
		},
	}
	svc.SetFlowDefinitionStore(store)

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].workflowID = liveFlowRef
	svc.mu.Unlock()

	// First turn carries the resolved flowRef — exactly what handleStartTurn
	// hands startTurn after resolveWorkflowFlowRef/explicitFlowRefResolves.
	if _, apiErr := svc.startTurn(parent.RunID, TurnInput{StepID: "turn-1", Prompt: "draft the feature", FlowRef: liveFlowRef}, "", ""); apiErr != nil {
		t.Fatalf("startTurn: %s", apiErr.msg)
	}

	// Wait for the flow to mount and the entry child to spawn.
	waitLoop(t, "entry child spawned", 5*time.Second, func() bool {
		return countChildrenWithLabel(svc, parent.RunID, "drafting") == 1
	})
	// Both children run to completion on the fake adapter; the edge advance
	// spawns final_check, then the engine notifies/reinvokes the hub. Wait for
	// a REAL hub turn (turnCount>=2 — the first turn is the synthetic
	// flow-start handoff, which suppresses the provider turn) to drain through
	// the post-turn gate.
	waitLoop(t, "hub turn drained through post-turn gate", 10*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		rs := svc.runs[parent.RunID]
		return rs != nil && rs.turnCount >= 2 && !rs.turnInFlight && !rs.pendingFlowGateSettle
	})

	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	status := rs.status
	svc.mu.Unlock()
	loop := svc.agentOrchestrator.loopStateFor(parent.RunID)
	t.Logf("post-drain: run=%q loop=%q", status, loop.Status)

	// The model never emitted flow_control("done") — the loop is still open —
	// so the run must not read completed even though every spawned child is
	// terminal and the hub's own turn passed its gate.
	if loop.Status == "done" || loop.Status == "stopped" {
		t.Skipf("loop unexpectedly sealed (%q); scenario can't assert the withhold", loop.Status)
	}
	if status == RunStatusCompleted {
		t.Fatalf("flow hub reported completed while loop=%q — dishonest terminal (BUG-521)", loop.Status)
	}
	if status != RunStatusRunning {
		t.Fatalf("withheld hub must report running, got %q", status)
	}
}
