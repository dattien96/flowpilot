package runner

import (
	"context"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// bug551Service builds a service with a completing fake Codex adapter plus a
// real createRun'd parent that has been marked flow-driven — the shape of the
// wedged live run (running loop, mounted flow nodes, no live work).
func bug551Service(t *testing.T) (*InteractiveService, string) {
	t.Helper()
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "done"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	handle, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	runID := handle.RunID
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.flowEngineDriven = true
	rs.status = RunStatusRunning
	rs.activeFlowNodes = []agentpack.FlowNode{{ID: "synthesis", Behavior: "hub.inline"}}
	svc.mu.Unlock()
	return svc, runID
}

func bug551WaitForTurn(t *testing.T, svc *InteractiveService, runID string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		rs := svc.runs[runID]
		fired := rs.reinvokeInFlight || rs.turnInFlight || rs.turnCount > 0
		svc.mu.Unlock()
		if fired {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("no hub turn dispatched — quiet wedge persists")
}

// BUG-551: live run-15525 wedged silently after an escalate-park cancelled an
// in-flight turn and the next hub reinvoke hit the round cap. The loop record
// stayed `running` but no turn, intent or watchdog survived — every recovery
// surface believed the loop was busy.
//
// Seam 1: resumeFlowWithFeedback returned early when the loop was not durably
// blocked — "running" was read as "nothing to do", so extend-cap + continue
// never re-drove anything. With the fix, a quiet running loop is redriven.
func TestBug551_ContinueOnQuietRunningLoopRedrives(t *testing.T) {
	svc, runID := bug551Service(t)
	// Post-extend-cap shape: round below cap, nothing in flight — the wedge.
	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		st.Round = 4
		st.Cap = 7
		return st
	})

	if _, err := svc.resumeFlowWithFeedback(runID, "continue"); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}
	bug551WaitForTurn(t, svc, runID)
}

// Seam 2: pendingAgentContext is input a future hub turn must consume — it is
// NOT proof a turn exists. The cancelled-turn orphan note must not make
// redriveQuietFlowLoop consider the loop busy.
func TestBug551_OrphanedPendingContextDoesNotSuppressRedrive(t *testing.T) {
	svc, runID := bug551Service(t)
	svc.mu.Lock()
	// Orphaned by the turn the escalate-park cancelled — nothing will ever
	// drain this unless redrive treats the loop as quiet.
	svc.runs[runID].pendingAgentContext = []string{"[cohort note] candidate-b: stale"}
	svc.mu.Unlock()
	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		st.Round = 4
		st.Cap = 7
		return st
	})

	svc.redriveQuietFlowLoop(runID)
	bug551WaitForTurn(t, svc, runID)
}

// Seam 3: the hub-stall watchdog is an in-memory timer — a durable `running`
// flow root rehydrated after restart must re-arm it, or the quiet wedge is
// invisible again until the next continue. The live shape: a parked child
// keeps the root `running` through normalize (fail-closed cancel path
// otherwise applies by design — cancelled roots must NOT arm).
func TestBug551_RehydratedRunningFlowRearmsWatchdog(t *testing.T) {
	nodes := []agentpack.FlowNode{
		{ID: "tdd", Behavior: "agent.code"},
		{ID: "coder", Behavior: "agent.code"},
	}

	t.Run("running root re-arms", func(t *testing.T) {
		svc, _ := newTestServer(t)
		runID := "run-551c"
		// A parked gated child in memory is what keeps the root `running`
		// through normalizeResumedFlowRun (parentHasLivePendingChildren) —
		// this is the live run-15525 shape, and no other reconstruct path
		// arms the parent's watchdog on its own.
		svc.mu.Lock()
		svc.runs["child-551c"] = &interactiveRun{
			id:                   "child-551c",
			parentRunID:          runID,
			pendingFlowGateSettle: true,
			status:               RunStatusWaitingApproval,
			subs:                 map[int64]chan ProviderEvent{},
		}
		svc.mu.Unlock()
		_, apiErr := svc.reconstructRun(ProviderSessionState{
			RunID:           runID,
			ProjectID:       "proj",
			ProviderKey:     ProviderKeyCodex,
			RunKind:         "chat",
			Status:          RunStatusRunning,
			StartedAt:       "2026-09-09T00:00:00Z",
			UpdatedAt:       "2026-09-09T00:05:00Z",
			ActiveFlowNodes: nodes,
			LoopState:       AgentLoopState{Status: "running"},
		})
		if apiErr != nil {
			t.Fatalf("reconstructRun: %v", apiErr)
		}
		svc.mu.Lock()
		status := svc.runs[runID].status
		svc.mu.Unlock()
		if status != RunStatusRunning {
			t.Fatalf("parent kept running via parked child, got %q", status)
		}
		hubStallTimerMu.Lock()
		_, armed := hubStallTimers[hubStallTimerKey(svc, runID)]
		hubStallTimerMu.Unlock()
		if !armed {
			t.Fatal("rehydrated running flow root did not re-arm hub-stall watchdog — quiet wedge invisible after restart")
		}
	})

	t.Run("cancelled root stays disarmed", func(t *testing.T) {
		svc, _ := newTestServer(t)
		runID := "run-551d"
		_, apiErr := svc.reconstructRun(ProviderSessionState{
			RunID:           runID,
			ProjectID:       "proj",
			ProviderKey:     ProviderKeyCodex,
			RunKind:         "chat",
			Status:          RunStatusCancelled,
			StartedAt:       "2026-09-09T00:00:00Z",
			UpdatedAt:       "2026-09-09T00:05:00Z",
			ActiveFlowNodes: nodes,
		})
		if apiErr != nil {
			t.Fatalf("reconstructRun: %v", apiErr)
		}
		hubStallTimerMu.Lock()
		_, armed := hubStallTimers[hubStallTimerKey(svc, runID)]
		hubStallTimerMu.Unlock()
		if armed {
			t.Fatal("cancelled root must not arm the hub-stall watchdog")
		}
	})
}

// Adjacent hole in the same wedge class: a hub reinvoke silently dropped at
// the round cap left the loop running-but-dead with no watchdog — the exact
// live sequence before extend-cap was tried. The cap-blocked path must arm
// the watchdog so a still-dead loop surfaces as hub_stalled.
func TestBug551_CapBlockedReinvokeArmsWatchdog(t *testing.T) {
	svc := bug289Service(t)
	runID := "run-551e"

	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:               runID,
		flowEngineDriven: true,
		autoOrchestrate:  true,
		status:           RunStatusRunning,
		activeFlowNodes:  []agentpack.FlowNode{{ID: "synthesis", Behavior: "hub.inline"}},
		subs:             map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		st.Round = 4
		st.Cap = 4 // at cap
		return st
	})

	svc.maybeAutoReinvokeHub(runID)

	hubStallTimerMu.Lock()
	_, armed := hubStallTimers[hubStallTimerKey(svc, runID)]
	hubStallTimerMu.Unlock()
	if !armed {
		t.Fatal("cap-blocked hub reinvoke armed no watchdog — loop stays running-but-dead silently")
	}
}
