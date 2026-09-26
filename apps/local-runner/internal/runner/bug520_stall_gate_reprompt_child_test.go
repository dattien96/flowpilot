package runner

import (
	"testing"
	"time"
)

// BUG-520 (live run-60145): the hub stall watchdog fired while a cohort
// member sat in its post-turn gate window with an armed gate-reprompt
// intent — hasActiveFlowChild did not count pendingGateRepromptPrompt /
// pendingFlowGateSettle as activity, read the child as a ghost, and
// parkFlowForAwaitingUser wiped the armed reprompt, orphaning the child
// (waiting_user_approval forever) and parking a healthy tournament.
func TestBug520_StallDoesNotFireOnArmedGateReprompt(t *testing.T) {
	svc := bug289Service(t)
	parentID := "run-520p"
	childID := "run-520c"

	svc.mu.Lock()
	svc.runs[parentID] = &interactiveRun{
		id:                parentID,
		flowEngineDriven:  true,
		status:            RunStatusRunning,
		hubLastProgressAt: time.Now().UTC().Add(-10 * time.Minute),
		stallTimeout:      time.Second,
		subs:              map[int64]chan ProviderEvent{},
	}
	// Exact post-turn shape observed live: turn over (turnInFlight=false),
	// gate evaluated and armed a reprompt intent, but the reprompt turn has
	// not dispatched yet (postTurnGateCancel already cleared).
	svc.runs[childID] = &interactiveRun{
		id:                        childID,
		parentRunID:               parentID,
		label:                     "candidate-b",
		status:                    RunStatusRunning,
		agentStatus:               string(RunStatusRunning),
		flowCohortId:              "flow-auto-parallel_rollout-round-0",
		pendingGateRepromptPrompt: "[flow-gate] fix your change-contract scope",
		pendingGateRepromptStepID: "step-x",
		pendingGateRepromptGen:    1,
		subs:                      map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, childID)
	svc.agentOrchestrator.preRegisterCohort(parentID, "flow-auto-parallel_rollout-round-0", 2)
	svc.agentOrchestrator.mutateLoop(parentID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		st.Cap = 3
		return st
	})

	if svc.checkAndBlockStalledHub(parentID) {
		t.Fatal("hub_stalled must not fire while a child has an armed gate-reprompt intent")
	}
	if !svc.hasActiveFlowChild(parentID) {
		t.Fatal("child with armed pendingGateRepromptPrompt must count as active")
	}
}

// BUG-520 guard: a child that is genuinely a ghost (running, no turn, no
// gate, no armed reprompt — the BUG-354 shape) still must NOT shield the
// hub — the watchdog has to keep firing for real stalls.
func TestBug520_TrueGhostChildStillStalls(t *testing.T) {
	svc := bug289Service(t)
	parentID := "run-522p"
	childID := "run-522c"

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
		label:        "candidate-b",
		status:       RunStatusRunning,
		agentStatus:  string(RunStatusRunning),
		flowCohortId: "flow-auto-parallel_rollout-round-0",
		subs:         map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, childID)
	svc.agentOrchestrator.preRegisterCohort(parentID, "flow-auto-parallel_rollout-round-0", 2)
	svc.agentOrchestrator.mutateLoop(parentID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		st.Cap = 3
		return st
	})

	if !svc.checkAndBlockStalledHub(parentID) {
		t.Fatal("a true ghost child (no armed work of any kind) must still let hub_stalled fire")
	}
}
