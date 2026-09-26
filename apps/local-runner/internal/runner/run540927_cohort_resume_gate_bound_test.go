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
	if loop.Status != "tournament_escalation" || loop.BlockReason != "member_stalled" {
		t.Fatalf("loop = %+v, want tournament_escalation/member_stalled (escalation always on)", loop)
	}
	awaitTournamentChildIdle(t, svc, parentID)
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

// P2-R2 review probes (F1/F3): the LIVE run-540927 / V9-03 shape —
// pendingFlowGateSettle AND turnInFlight stay true for the whole gate window,
// so the member holds both while the gate's oracle never returns. A stale gate
// stamp must own the gate-visible decision and let member_stalled fire.
func Test540927CohortMemberV9ShapeStaleGateStalls(t *testing.T) {
	svc, _ := newTestServer(t)
	parentID := "run-540927-v9-cohort-parent"
	memberID := "run-540927-v9-cohort-member"
	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	svc.mu.Lock()
	svc.runs[parentID] = &interactiveRun{
		id:               parentID,
		flowEngineDriven: true,
		status:           RunStatusRunning,
		stallTimeout:     time.Second,
		subs:             map[int64]chan ProviderEvent{},
	}
	svc.runs[memberID] = &interactiveRun{
		id:                    memberID,
		parentRunID:           parentID,
		label:                 "reviewer_c",
		flowCohortId:          "run-540927-v9-cohort",
		status:                RunStatusRunning,
		agentStatus:           string(RunStatusRunning),
		turnInFlight:          true, // V9-03: held for the whole gate window
		pendingFlowGateSettle: true, // stamped by the flow child turn path
		lastProviderEventAt:   time.Now().UTC().Add(-10 * time.Minute),
		postTurnGateCancel:    cancel,
		postTurnGateStartedAt: time.Now().UTC().Add(-10 * time.Minute), // stale
		subs:                  map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, memberID)
	svc.agentOrchestrator.preRegisterCohort(parentID, "run-540927-v9-cohort", 1)
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Cap: 3})

	if !svc.checkAndBlockStalledMembers(parentID) {
		t.Fatal("expected member_stalled: V9-03 settle+turnInFlight must age out with the stale gate")
	}
	st := svc.agentOrchestrator.loopStateFor(parentID)
	if st.Status != "tournament_escalation" || st.BlockReason != "member_stalled" {
		t.Fatalf("loop = %+v, want tournament_escalation/member_stalled (escalation always on)", st)
	}
	awaitTournamentChildIdle(t, svc, parentID)
}

// Near-miss: same V9-03 shape, fresh gate — the member stays shielded.
func Test540927CohortMemberV9ShapeFreshGateShielded(t *testing.T) {
	svc, _ := newTestServer(t)
	parentID := "run-540927-v9-cohort-fresh-parent"
	memberID := "run-540927-v9-cohort-fresh-member"
	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	svc.mu.Lock()
	svc.runs[parentID] = &interactiveRun{
		id:               parentID,
		flowEngineDriven: true,
		status:           RunStatusRunning,
		stallTimeout:     time.Second,
		subs:             map[int64]chan ProviderEvent{},
	}
	svc.runs[memberID] = &interactiveRun{
		id:                    memberID,
		parentRunID:           parentID,
		label:                 "reviewer_d",
		flowCohortId:          "run-540927-v9-cohort-fresh",
		status:                RunStatusRunning,
		agentStatus:           string(RunStatusRunning),
		turnInFlight:          true,
		pendingFlowGateSettle: true,
		lastProviderEventAt:   time.Now().UTC().Add(-10 * time.Minute),
		postTurnGateCancel:    cancel,
		postTurnGateStartedAt: time.Now().UTC().Add(-30 * time.Second), // fresh
		subs:                  map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, memberID)
	svc.agentOrchestrator.preRegisterCohort(parentID, "run-540927-v9-cohort-fresh", 1)
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Cap: 3})

	if svc.checkAndBlockStalledMembers(parentID) {
		t.Fatal("V9-03 shape with a live gate must stay shielded")
	}
}

// P2-R2 review probes (F2/F3): notifyTurnIdle under the V9-03 shape —
// turnInFlight held by the gate window + settle stamped. A stale gate must let
// the reinvoke drain; a live gate must keep deferring it. Fidelity note
// (round-3 nit N-2): with autoOrchestrate=false the assertion covers flag
// consumption only — the launched reinvoke goroutine cannot re-arm; with
// autoOrchestrate=true the M7 guard re-stashes flag+prompt and forward
// progress completes via the hub watchdog recovery / gate settle.
func Test540927NotifyTurnIdleV9ShapeDrainsPastStaleGate(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := "run-540927-v9-drain-stale"
	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:                       runID,
		flowEngineDriven:         true,
		parentRunID:              "",
		status:                   RunStatusRunning,
		turnInFlight:             true, // V9-03: held during the gate window
		pendingFlowGateSettle:    true,
		pendingHubReinvoke:       true,
		pendingHubReinvokePrompt: "hub.notify retry",
		postTurnGateCancel:       cancel,
		postTurnGateStartedAt:    time.Now().UTC().Add(-10 * time.Minute), // stale
		subs:                     map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()

	svc.notifyTurnIdle(runID)

	svc.mu.Lock()
	drained := !svc.runs[runID].pendingHubReinvoke
	svc.mu.Unlock()
	if !drained {
		t.Fatal("V9-03 shape + stale gate must drain the pending hub reinvoke")
	}
}

func Test540927NotifyTurnIdleV9ShapeFreshGateDefers(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := "run-540927-v9-drain-fresh"
	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:                       runID,
		flowEngineDriven:         true,
		parentRunID:              "",
		status:                   RunStatusRunning,
		turnInFlight:             true,
		pendingFlowGateSettle:    true,
		pendingHubReinvoke:       true,
		pendingHubReinvokePrompt: "hub.notify retry",
		postTurnGateCancel:       cancel,
		postTurnGateStartedAt:    time.Now().UTC().Add(-30 * time.Second), // fresh
		subs:                     map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()

	svc.notifyTurnIdle(runID)

	svc.mu.Lock()
	kept := svc.runs[runID].pendingHubReinvoke
	svc.mu.Unlock()
	if !kept {
		t.Fatal("V9-03 shape with a live gate must keep deferring the drain")
	}
}
