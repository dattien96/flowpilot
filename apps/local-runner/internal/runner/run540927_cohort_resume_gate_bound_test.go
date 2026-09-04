// BUG-354 P2 (run-540927): the remaining unbounded postTurnGateCancel busy
// consumers — the cohort member stall watchdog's gate shield and restart park,
// and notifyTurnIdle's reinvoke drain — must age a DEAD gate out of busy with
// the same postTurnGateBusyBound contract as the hub watchdog (CA-742). A gate
// cancel past the bound can no longer shield a cohort member from
// member_stalled, park a restart intent forever, or strand a hub reinvoke.
package runner

import (
	"context"
	"testing"
	"time"
)

// Cohort member holding a STALE gate cancel with no recent provider events:
// the dead gate must stop shielding the member — member_stalled fires.
func Test540927CohortMemberStaleGateStillStalls(t *testing.T) {
	svc := bug289Service(t)
	parentID := "run-540927-cohort-stale"
	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	svc.mu.Lock()
	svc.runs[parentID] = &interactiveRun{
		id:                parentID,
		flowEngineDriven:  true,
		status:            RunStatusRunning,
		stallTimeout:      time.Second,
		hubLastProgressAt: time.Now().UTC(),
		subs:              map[int64]chan ProviderEvent{},
	}
	svc.runs["run-540927-cohort-member"] = &interactiveRun{
		id:                    "run-540927-cohort-member",
		parentRunID:           parentID,
		label:                 "my-reviewer",
		status:                RunStatusRunning,
		agentStatus:           string(RunStatusRunning),
		turnInFlight:          true,
		flowCohortId:          "flow-cohort-540927",
		lastProviderEventAt:   time.Now().UTC().Add(-10 * time.Minute),
		postTurnGateCancel:    cancel,
		postTurnGateStartedAt: time.Now().UTC().Add(-10 * time.Minute),
		subs:                  map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, "run-540927-cohort-member")
	svc.agentOrchestrator.preRegisterCohort(parentID, "flow-cohort-540927", 2)
	svc.agentOrchestrator.mutateLoop(parentID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		return st
	})

	if !svc.checkAndBlockStalledMembers(parentID) {
		t.Fatal("expected member_stalled: stale gate cancel must not shield the member")
	}
	loop := svc.agentOrchestrator.loopStateFor(parentID)
	if loop.Status != "blocked" || loop.BlockReason != "member_stalled" {
		t.Fatalf("loop = %+v, want blocked/member_stalled", loop)
	}
}

// Near-miss: a FRESH gate cancel (legit oracle window) still shields the
// member — the T-11(a)/R13-06 wait-forever contract stays for live gates.
func Test540927CohortMemberFreshGateStillShielded(t *testing.T) {
	svc := bug289Service(t)
	parentID := "run-540927-cohort-fresh"
	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	svc.mu.Lock()
	svc.runs[parentID] = &interactiveRun{
		id:                parentID,
		flowEngineDriven:  true,
		status:            RunStatusRunning,
		stallTimeout:      time.Second,
		hubLastProgressAt: time.Now().UTC(),
		subs:              map[int64]chan ProviderEvent{},
	}
	svc.runs["run-540927-cohort-member-fresh"] = &interactiveRun{
		id:                    "run-540927-cohort-member-fresh",
		parentRunID:           parentID,
		label:                 "my-reviewer",
		status:                RunStatusRunning,
		agentStatus:           string(RunStatusRunning),
		turnInFlight:          true,
		flowCohortId:          "flow-cohort-540927-fresh",
		lastProviderEventAt:   time.Now().UTC().Add(-10 * time.Minute),
		postTurnGateCancel:    cancel,
		postTurnGateStartedAt: time.Now().UTC().Add(-30 * time.Second),
		subs:                  map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, "run-540927-cohort-member-fresh")
	svc.agentOrchestrator.preRegisterCohort(parentID, "flow-cohort-540927-fresh", 2)
	svc.agentOrchestrator.mutateLoop(parentID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		return st
	})

	if svc.checkAndBlockStalledMembers(parentID) {
		t.Fatal("fresh gate cancel must keep shielding the member (no member_stalled)")
	}
}

// notifyTurnIdle drain: a hub with a STALE gate cancel and a pending reinvoke
// must drain the reinvoke instead of stranding it forever; a fresh gate keeps
// the drain deferred.
func Test540927NotifyTurnIdleDrainsReinvokePastStaleGate(t *testing.T) {
	svc := bug289Service(t)
	staleID := "run-540927-drain-stale"
	freshID := "run-540927-drain-fresh"
	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	svc.mu.Lock()
	svc.runs[staleID] = &interactiveRun{
		id:                       staleID,
		flowEngineDriven:         true,
		status:                   RunStatusRunning,
		pendingHubReinvoke:       true,
		pendingHubReinvokePrompt: "continue the flow",
		postTurnGateCancel:       cancel,
		postTurnGateStartedAt:    time.Now().UTC().Add(-10 * time.Minute),
		subs:                     map[int64]chan ProviderEvent{},
	}
	svc.runs[freshID] = &interactiveRun{
		id:                       freshID,
		flowEngineDriven:         true,
		status:                   RunStatusRunning,
		pendingHubReinvoke:       true,
		pendingHubReinvokePrompt: "continue the flow",
		postTurnGateCancel:       cancel,
		postTurnGateStartedAt:    time.Now().UTC().Add(-30 * time.Second),
		subs:                     map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()

	svc.notifyTurnIdle(staleID)
	svc.mu.Lock()
	drained := !svc.runs[staleID].pendingHubReinvoke
	svc.mu.Unlock()
	if !drained {
		t.Fatal("stale gate cancel must not strand the pending hub reinvoke")
	}

	svc.notifyTurnIdle(freshID)
	svc.mu.Lock()
	deferred := svc.runs[freshID].pendingHubReinvoke
	svc.mu.Unlock()
	if !deferred {
		t.Fatal("fresh gate cancel must keep deferring the reinvoke drain")
	}
}
