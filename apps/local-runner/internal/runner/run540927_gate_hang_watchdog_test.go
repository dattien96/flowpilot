// BUG-354 (run-540927): a post-turn gate whose oracle never returned kept the
// hub busy forever — parent postTurnGateCancel + child active
// (hasActiveFlowChild) both treated a live cancel func as unbounded busy, so
// hub_stalled never fired and implement stayed RUNNING with no actionable
// card. These tests lock the bounded-busy contract: gateCancelLive ages out
// after postTurnGateBusyBound; fresh gates and zero stamps keep legacy
// behavior (CA-361/CA-355 intact).
//
// Provider-agnostic (R2 Case 1): hub watchdog + active-child predicate take no
// providerKey and never branch on one — grep evidence in CA-742.
package runner

import (
	"context"
	"testing"
	"time"
)

func new540927HubRun(t *testing.T, svc *InteractiveService, runID string, startedAt time.Time, cancel context.CancelFunc) {
	t.Helper()
	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:                    runID,
		flowEngineDriven:      true,
		status:                RunStatusRunning,
		hubLastProgressAt:     time.Now().UTC().Add(-10 * time.Minute),
		stallTimeout:          time.Second,
		pendingFlowGateSettle: true,
		postTurnGateCancel:    cancel,
		postTurnGateStartedAt: startedAt,
		subs:                  map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		return st
	})
}

// Repro shape: the gate armed at stamp-time long ago and never returned — the
// watchdog must park hub_stalled instead of re-arming forever.
func Test540927HubStallFiresWhenGateCancelStale(t *testing.T) {
	svc := bug289Service(t)
	runID := "run-540927-stale-gate"
	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	new540927HubRun(t, svc, runID, time.Now().UTC().Add(-10*time.Minute), cancel)

	if !svc.checkAndBlockStalledHub(runID) {
		t.Fatal("expected hub_stalled: stale gate cancel must age out of busy")
	}
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.Status != "blocked" || loop.BlockReason != "hub_stalled" {
		t.Fatalf("loop = %+v, want blocked/hub_stalled", loop)
	}
}

// Near-miss: a freshly armed gate (legit long suite up to the oracle's 5m
// deadline) is still busy — must NOT stall. Mirrors the legacy contract with
// the new stamp present.
func Test540927HubStallBusyWhileGateCancelFresh(t *testing.T) {
	svc := bug289Service(t)
	runID := "run-540927-fresh-gate"
	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	new540927HubRun(t, svc, runID, time.Now().UTC().Add(-30*time.Second), cancel)

	if svc.checkAndBlockStalledHub(runID) {
		t.Fatal("fresh gate cancel must stay busy (no hub_stalled)")
	}
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.Status == "blocked" {
		t.Fatalf("loop should stay running during live gate, got %+v", loop)
	}
}

// The exact run-540927 topology: the stuck gate belongs to the CODER CHILD.
// The child's stale gate cancel must stop counting as active so the hub
// watchdog can fire.
func Test540927ChildGateStaleDoesNotKeepHubBusy(t *testing.T) {
	svc := bug289Service(t)
	parentID := "run-540927-hub"
	childID := "run-540927-coder"
	_, cancel := context.WithCancel(context.Background())
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
		id:                    childID,
		parentRunID:           parentID,
		label:                 "implement",
		status:                RunStatusRunning,
		agentStatus:           string(RunStatusRunning),
		pendingFlowGateSettle: true,
		postTurnGateCancel:    cancel,
		postTurnGateStartedAt: time.Now().UTC().Add(-10 * time.Minute),
		subs:                  map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, childID)
	svc.agentOrchestrator.mutateLoop(parentID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		return st
	})

	if !svc.checkAndBlockStalledHub(parentID) {
		t.Fatal("expected hub_stalled: stale CHILD gate must not keep hub busy forever")
	}
}

// CA-361 guard: a child still holding a live provider turn stays active and is
// never cancelled by the watchdog — the BUG-354 bound only applies to the
// gate-cancel signal, not turnInFlight.
func Test540927ChildTurnInFlightStillBusy(t *testing.T) {
	svc := bug289Service(t)
	parentID := "run-540927-ca361"
	childID := "run-540927-reviewer"
	cancelled := false
	_, cancel := context.WithCancel(context.Background())
	wrapped := func() {
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
		turnCancel:   wrapped,
		subs:         map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, childID)
	svc.agentOrchestrator.mutateLoop(parentID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		return st
	})

	if svc.checkAndBlockStalledHub(parentID) {
		t.Fatal("child turnInFlight without a gate cancel must keep hub busy (CA-361)")
	}
	if cancelled {
		t.Fatal("running child was cancelled by hub stall watchdog")
	}
}

// F1 review probe (V9-03 shape): the LIVE run-540927 hang — V9-03 keeps
// turnInFlight true for the entire post-turn gate window, so the child holds
// turnInFlight=true while the gate's oracle never returns. A stale gate stamp
// must own the busy signal and let hub_stalled fire anyway.
func Test540927ChildTurnInFlightStaleGateStillFires(t *testing.T) {
	svc := bug289Service(t)
	parentID := "run-540927-f1-stale"
	childID := "run-540927-f1-coder"
	_, cancel := context.WithCancel(context.Background())
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
		id:                    childID,
		parentRunID:           parentID,
		label:                 "implement",
		status:                RunStatusRunning,
		agentStatus:           string(RunStatusRunning),
		turnInFlight:          true, // V9-03: held for the whole gate window
		pendingFlowGateSettle: true,
		postTurnGateCancel:    cancel,
		postTurnGateStartedAt: time.Now().UTC().Add(-10 * time.Minute),
		subs:                  map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, childID)
	svc.agentOrchestrator.mutateLoop(parentID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		return st
	})

	if !svc.checkAndBlockStalledHub(parentID) {
		t.Fatal("expected hub_stalled: turnInFlight held by a stale gate must age out (F1)")
	}
}

// F1 near-miss: same V9-03 shape but the gate is fresh — the legit 5-minute
// oracle window stays busy.
func Test540927ChildTurnInFlightFreshGateStillBusy(t *testing.T) {
	svc := bug289Service(t)
	parentID := "run-540927-f1-fresh"
	childID := "run-540927-f1-fresh-coder"
	_, cancel := context.WithCancel(context.Background())
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
		id:                    childID,
		parentRunID:           parentID,
		label:                 "implement",
		status:                RunStatusRunning,
		agentStatus:           string(RunStatusRunning),
		turnInFlight:          true,
		pendingFlowGateSettle: true,
		postTurnGateCancel:    cancel,
		postTurnGateStartedAt: time.Now().UTC().Add(-30 * time.Second),
		subs:                  map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, childID)
	svc.agentOrchestrator.mutateLoop(parentID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		return st
	})

	if svc.checkAndBlockStalledHub(parentID) {
		t.Fatal("fresh gate + V9-03 turnInFlight must stay busy (no hub_stalled)")
	}
}

// F1 parent-self probe: the hub's own turn in the same V9-03 gate window with
// a stale gate stamp must not shield the hub from hub_stalled.
func Test540927HubTurnStaleGateStillFires(t *testing.T) {
	svc := bug289Service(t)
	runID := "run-540927-f1-hub"
	_, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:                    runID,
		flowEngineDriven:      true,
		status:                RunStatusRunning,
		hubLastProgressAt:     time.Now().UTC().Add(-10 * time.Minute),
		stallTimeout:          time.Second,
		turnInFlight:          true, // V9-03: held during the hub's own gate
		pendingFlowGateSettle: true,
		postTurnGateCancel:    cancel,
		postTurnGateStartedAt: time.Now().UTC().Add(-10 * time.Minute),
		subs:                  map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		return st
	})

	if !svc.checkAndBlockStalledHub(runID) {
		t.Fatal("expected hub_stalled: hub turnInFlight held by a stale gate must age out (F1)")
	}
}
