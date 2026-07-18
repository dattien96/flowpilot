package runner

import (
	"context"
	"testing"
	"time"
)

func TestRun333HubStallDoesNotCancelRunningChild(t *testing.T) {
	svc := bug289Service(t)
	parentID := "run-333"
	childID := "run-940"

	cancelled := false
	_, cancel := context.WithCancel(context.Background())
	wrappedCancel := func() {
		cancelled = true
		cancel()
	}
	t.Cleanup(cancel)

	svc.mu.Lock()
	svc.runs[parentID] = &interactiveRun{
		id:                parentID,
		flowEngineDriven:  true,
		status:            RunStatusRunning,
		hubLastProgressAt: time.Now().UTC().Add(-10 * time.Minute),
		stallTimeout:      time.Second,
		subs:              map[int64]chan ProviderEvent{},
	}
	svc.runs[childID] = &interactiveRun{
		id:           childID,
		parentRunID:  parentID,
		label:        "my-reviewer",
		status:       RunStatusRunning,
		agentStatus:  string(RunStatusRunning),
		turnInFlight: true,
		turnCancel:   wrappedCancel,
		flowCohortId: "flow-auto-grok-coder-round-0",
		lastProviderEventAt: time.Now().UTC().
			Add(-30 * time.Second),
		subs: map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, childID)
	svc.agentOrchestrator.preRegisterCohort(parentID, "flow-auto-grok-coder-round-0", 2)
	svc.agentOrchestrator.mutateLoop(parentID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		st.Cap = 3
		return st
	})

	if svc.checkAndBlockStalledHub(parentID) {
		t.Fatal("hub_stalled must not fire while a child reviewer is still running")
	}
	if cancelled {
		t.Fatal("running child was cancelled by hub stall watchdog")
	}
	loop := svc.agentOrchestrator.loopStateFor(parentID)
	if loop.Status == "blocked" || loop.BlockReason == "hub_stalled" {
		t.Fatalf("loop = %+v, want still running and not hub_stalled", loop)
	}
}
