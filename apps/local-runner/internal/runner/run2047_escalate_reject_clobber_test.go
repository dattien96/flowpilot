package runner

import (
	"testing"
	"time"
)

// TestTransitionRejectedDoesNotClobberBlocked is run-2047: after escalate parks
// the flow (loop=blocked), cancelled children emit TurnFailed → transition
// "rejected". That must not wipe blocked/escalate or hub_stalled replaces the form.
func TestTransitionRejectedDoesNotClobberBlocked(t *testing.T) {
	o := newAgentOrchestrator()
	o.setLoop("run-1", AgentLoopState{
		Status: "blocked", BlockReason: "escalate", GateReason: "Reviewers reported: continue needed", Cap: 3,
	})
	snap := o.transition("run-1", "rejected")
	if snap.LoopState.Status != "blocked" {
		t.Fatalf("Status = %q, want blocked preserved", snap.LoopState.Status)
	}
	if snap.LoopState.BlockReason != "escalate" {
		t.Fatalf("BlockReason = %q, want escalate", snap.LoopState.BlockReason)
	}
	if snap.LoopState.GateReason != "Reviewers reported: continue needed" {
		t.Fatalf("GateReason clobbered to %q", snap.LoopState.GateReason)
	}
}

func TestTransitionRejectedStillWorksWhenRunning(t *testing.T) {
	o := newAgentOrchestrator()
	o.setLoop("run-1", AgentLoopState{Status: "running", Cap: 3})
	snap := o.transition("run-1", "rejected")
	if snap.LoopState.Status != "rejected" {
		t.Fatalf("Status = %q, want rejected", snap.LoopState.Status)
	}
}

func TestHubStallDoesNotFireWhenBlocked(t *testing.T) {
	svc := bug289Service(t)
	runID := "run-blocked"
	rs := &interactiveRun{
		id:                runID,
		flowEngineDriven:  true,
		status:            RunStatusRunning,
		hubLastProgressAt: time.Now().UTC().Add(-10 * time.Minute),
		stallTimeout:      time.Second,
		subs:              map[int64]chan ProviderEvent{},
	}
	svc.mu.Lock()
	svc.runs[runID] = rs
	svc.mu.Unlock()
	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = "escalate"
		st.GateReason = "coder failed"
		return st
	})
	if svc.checkAndBlockStalledHub(runID) {
		t.Fatal("hub_stalled must not fire over escalate blocked form")
	}
}
