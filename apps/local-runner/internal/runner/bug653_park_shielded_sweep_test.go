package runner

import (
	"context"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// BUG-653 (live run-523131): an armed pendingFlowGateSettle parked the
// synthesis hub on a missing-verdict card while the tdd step underneath was
// RUNNING-dead (its leg had already completed). The sweep's run-level guard
// skipped the whole run for the armed settle — the park shielded the wedge
// that caused it, and the card's missing input could never arrive. The
// settle owns the parked node only; dead siblings still heal.
func TestBug653_ParkedHubDoesNotShieldDeadStep(t *testing.T) {
	svc, store, runID := seedDeadDispatchRun(t, []agentpack.FlowNode{
		{ID: "tdd", Behavior: "agent.scaffold", Agent: "agents/scaffold-architect.md"},
		{ID: "synthesis", Behavior: "hub.inline"},
	})
	svc.mu.Lock()
	svc.runs[runID].pendingFlowGateSettle = true
	svc.mu.Unlock()
	svc.setFlowStepStatus(context.Background(), runID, "tdd", StepStatusRunning)
	ageStepStartedAt(t, svc, runID, "tdd", 3*time.Minute)
	svc.setFlowStepStatus(context.Background(), runID, "synthesis", StepStatusWaitingUserApr)
	// The wedge shape: tdd's leg completed durably but its completion was
	// consumed before the mirror settled (pre-BUG-647 class loss).
	store.sessions["run-tdd-leg"] = ProviderSessionState{
		RunID:       "run-tdd-leg",
		ParentRunID: runID,
		Label:       "tdd",
		Status:      RunStatusCompleted,
	}

	svc.sweepWedgedFlowWork()

	if got := flowStepStatus(t, svc, runID, "tdd"); got != StepStatusDone {
		t.Fatalf("park-shielded dead step never healed: tdd=%v, want DONE (leg completed)", got)
	}
	if got := flowStepStatus(t, svc, runID, "synthesis"); got != StepStatusWaitingUserApr {
		t.Fatalf("sweep disturbed the parked node: synthesis=%v, want WAITING_USER_APPROVAL", got)
	}
}

// The parked hub node itself must never be re-dispatched while its settle is
// armed — the card awaits a human answer and a hub reinvoke would double
// the decision surface. Even a RUNNING-stamped hub under an armed settle
// stays put; the settle's resolution owns it.
func TestBug653_ParkedNodeNotRedispatched(t *testing.T) {
	svc, _, runID := seedDeadDispatchRun(t, []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
		{ID: "synthesis", Behavior: "hub.inline"},
	})
	svc.mu.Lock()
	svc.runs[runID].pendingFlowGateSettle = true
	svc.mu.Unlock()
	svc.setFlowStepStatus(context.Background(), runID, "synthesis", StepStatusRunning)
	ageStepStartedAt(t, svc, runID, "synthesis", 3*time.Minute)

	svc.sweepWedgedFlowWork()

	svc.mu.Lock()
	armed := svc.runs[runID].reinvokeInFlight || svc.runs[runID].pendingHubReinvoke
	svc.mu.Unlock()
	if armed {
		t.Fatal("parked hub node was re-dispatched while its gate settle is armed — the card's answer must own the release")
	}
}
