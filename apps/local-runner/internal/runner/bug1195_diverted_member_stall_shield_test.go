package runner

import (
	"testing"
	"time"
)

// BUG-1195 (live run-225691, gated leg run-258141): an owner member whose
// post-turn gate diverted into an ALREADY-MOUNTED debate is recorded in the
// parent's vibeParkedGatedRunIDs — parked on purpose until the debate's
// restore reprompts it. Its settle was consumed by the divert so it stays
// Running with a held cohort seat and no provider events. The stall sweep
// counted exactly that shape as "silent member" and parked the hub
// (member_stalled) while the debate was still live — and the follow-on
// respawn is what BUG-1194 had to re-bind. A diverted member is queued
// remediation (same contract as the BUG-539 armed-intent shield), not a
// silent leg: it must never fire member_stalled.
func TestBug1195_DivertedMemberDoesNotStall(t *testing.T) {
	svc := bug289Service(t)
	parentID, childID := "run-1195p", "run-1195c"
	cohortID := "flow-auto-debate_trigger-round-4"

	svc.mu.Lock()
	svc.runs[parentID] = &interactiveRun{
		id:               parentID,
		flowEngineDriven: true,
		status:           RunStatusRunning,
		stallTimeout:     time.Second,
		// Live shape: sprint topology parked for the mounted debate and the
		// diverted child recorded for its post-debate reprompt.
		vibeParkedNodes:       reviewLoopTestNodes(),
		vibeParkedGatedRunIDs: []string{childID},
		subs:                  map[int64]chan ProviderEvent{},
	}
	svc.runs[childID] = &interactiveRun{
		id:                  childID,
		parentRunID:         parentID,
		label:               "owner_1",
		status:              RunStatusRunning,
		flowCohortId:        cohortID,
		lastProviderEventAt: time.Now().UTC().Add(-10 * time.Minute),
		subs:                map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, childID)
	svc.agentOrchestrator.preRegisterCohort(parentID, cohortID, 2)
	svc.agentOrchestrator.mutateLoop(parentID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		st.Cap = 3
		return st
	})

	if svc.checkAndBlockStalledMembers(parentID) {
		t.Fatal("member_stalled fired on a debate-diverted member — it is parked for the post-debate reprompt, not silent")
	}
	loop := svc.agentOrchestrator.loopStateFor(parentID)
	if loop.Status == "blocked" && loop.BlockReason == "member_stalled" {
		t.Fatalf("diverted member parked the hub: %+v", loop)
	}
}

// The shield ends where the divert ends: once the parked gated-run list no
// longer holds the member (debate resolved / claim dropped) a still-silent
// member stalls normally.
func TestBug1195_NonDivertedSilentMemberStillStalls(t *testing.T) {
	svc := bug289Service(t)
	parentID, childID := "run-1195sp", "run-1195sc"
	cohortID := "flow-auto-debate_trigger-round-4"

	svc.mu.Lock()
	svc.runs[parentID] = &interactiveRun{
		id:               parentID,
		flowEngineDriven: true,
		status:           RunStatusRunning,
		stallTimeout:     time.Second,
		subs:             map[int64]chan ProviderEvent{},
	}
	svc.runs[childID] = &interactiveRun{
		id:                  childID,
		parentRunID:         parentID,
		label:               "owner_1",
		status:              RunStatusRunning,
		flowCohortId:        cohortID,
		lastProviderEventAt: time.Now().UTC().Add(-10 * time.Minute),
		subs:                map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, childID)
	svc.agentOrchestrator.preRegisterCohort(parentID, cohortID, 2)
	svc.agentOrchestrator.mutateLoop(parentID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		st.Cap = 3
		return st
	})

	if !svc.checkAndBlockStalledMembers(parentID) {
		t.Fatal("silent member outside the diverted set must still fire member_stalled")
	}
}
