package runner

import (
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// TestRun43831ChildCompletionRefreshesHubProgressBeforeAutoAdvance locks the
// residual after CA-361/run-333: a long-running child keeps F-0 re-armed via
// hasActiveFlowChild, but the instant it becomes terminal DONE there is a gap
// before the next spawn is active. Without stamping hubLastProgressAt on the
// accepted child-completion handoff, checkAndBlockStalledHub sees age > 2m and
// falsely parks the hub mid auto-advance (live run-43831).
//
// Provider-agnostic: hub_stall has no providerKey branch; one representative
// regression test covers Codex/Claude/Grok.
func TestRun43831ChildCompletionRefreshesHubProgressBeforeAutoAdvance(t *testing.T) {
	svc := bug289Service(t)
	parentID := "run-43831"
	childID := "run-43836"

	stale := time.Now().UTC().Add(-10 * time.Minute)
	svc.mu.Lock()
	svc.runs[parentID] = &interactiveRun{
		id:                parentID,
		flowEngineDriven:  true,
		status:            RunStatusRunning,
		hubLastProgressAt: stale,
		stallTimeout:      time.Second,
		subs:              map[int64]chan ProviderEvent{},
		// Minimal tracked topology so settle takes the explicit advance path.
		activeFlowNodes: []agentpack.FlowNode{
			{ID: "coder", Behavior: "agent.delegate", Agent: "agents/coder.md"},
			{ID: "reviewer_correctness", Behavior: "agent.delegate", Agent: "agents/reviewer.md"},
			{ID: "reviewer_security", Behavior: "agent.delegate", Agent: "agents/reviewer.md"},
		},
		activeFlowEdges: []agentpack.FlowEdge{
			{From: "coder", To: "reviewer_correctness", When: "done", Kind: "forward"},
			{From: "coder", To: "reviewer_security", When: "done", Kind: "forward"},
		},
	}
	svc.runs[childID] = &interactiveRun{
		id:          childID,
		parentRunID: parentID,
		label:       "coder",
		agentName:   "coder",
		role:        "coder",
		status:      RunStatusRunning,
		agentStatus: string(RunStatusRunning),
		subs:        map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, childID)
	svc.agentOrchestrator.mutateLoop(parentID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		st.Mode = "explicit"
		st.Cap = 3
		return st
	})

	// Drive the accepted child-completion settlement path under s.mu — this is
	// the handoff that must refresh parent progress before any advance goroutine.
	svc.mu.Lock()
	child := svc.runs[childID]
	svc.settleFlowChildTurnCompletedLocked(child, "fixed Add()", ProviderEvent{
		Type:         EventTurnCompleted,
		FinalMessage: "fixed Add()",
	})
	parent := svc.runs[parentID]
	if parent == nil {
		svc.mu.Unlock()
		t.Fatal("parent missing after settle")
	}
	if !parent.hubLastProgressAt.After(stale) {
		svc.mu.Unlock()
		t.Fatalf("hubLastProgressAt not refreshed on child completion: got %v want after %v",
			parent.hubLastProgressAt, stale)
	}
	// Force the watchdog age window: even if the stamp is recent, prove that a
	// check with stale-age math would still re-arm rather than stall once stamped.
	// Simulate the race window: child is no longer active (completed) and no next
	// child is registered yet — only the refreshed timestamp protects the hub.
	child.status = RunStatusCompleted
	child.agentStatus = string(RunStatusCompleted)
	child.turnInFlight = false
	svc.mu.Unlock()

	if svc.checkAndBlockStalledHub(parentID) {
		t.Fatal("hub_stalled must not fire after child completion refreshed hub progress (run-43831 gap)")
	}
	loop := svc.agentOrchestrator.loopStateFor(parentID)
	if loop.Status == "blocked" || loop.BlockReason == "hub_stalled" {
		t.Fatalf("loop = %+v, want still running and not hub_stalled", loop)
	}
}

// TestRun43831ChildFailureAlsoRefreshesHubProgress covers the symmetric failure
// handoff (notifyHubOfFlowChildFailureLocked) so a failed entry/delegate cannot
// leave hubLastProgressAt stale either.
func TestRun43831ChildFailureAlsoRefreshesHubProgress(t *testing.T) {
	svc := bug289Service(t)
	parentID := "run-43831-fail"
	childID := "run-43836-fail"

	stale := time.Now().UTC().Add(-10 * time.Minute)
	svc.mu.Lock()
	svc.runs[parentID] = &interactiveRun{
		id:                parentID,
		flowEngineDriven:  true,
		status:            RunStatusRunning,
		hubLastProgressAt: stale,
		stallTimeout:      time.Second,
		subs:              map[int64]chan ProviderEvent{},
	}
	svc.runs[childID] = &interactiveRun{
		id:          childID,
		parentRunID: parentID,
		label:       "coder",
		agentName:   "coder",
		role:        "coder",
		providerKey: ProviderKeyCodex,
		status:      RunStatusFailed,
		agentStatus: string(RunStatusFailed),
		subs:        map[int64]chan ProviderEvent{},
	}
	child := svc.runs[childID]
	svc.notifyHubOfFlowChildFailureLocked(child, "simulated start failure")
	parent := svc.runs[parentID]
	if parent == nil || !parent.hubLastProgressAt.After(stale) {
		svc.mu.Unlock()
		t.Fatalf("hubLastProgressAt not refreshed on child failure: parent=%v", parent)
	}
	svc.mu.Unlock()

	if svc.checkAndBlockStalledHub(parentID) {
		t.Fatal("hub_stalled must not fire immediately after child failure stamped hub progress")
	}
}

// TestRun43831CohortPreflightFailureRefreshesHubProgress covers handleChildStartTurnFailure
// for a cohort member: pre-adapter start failure never emits EventTurnFailed, but still
// joins the cohort / may reinvoke the hub — so hub progress must refresh under s.mu
// before that handoff (Codex walkthrough NEEDS_FIX #1).
func TestRun43831CohortPreflightFailureRefreshesHubProgress(t *testing.T) {
	svc := bug289Service(t)
	parentID := "run-43831-preflight"
	childID := "run-44949-preflight"
	cohortID := "flow-auto-coder-round-0"

	stale := time.Now().UTC().Add(-10 * time.Minute)
	svc.mu.Lock()
	svc.runs[parentID] = &interactiveRun{
		id:                parentID,
		flowEngineDriven:  true,
		status:            RunStatusRunning,
		hubLastProgressAt: stale,
		stallTimeout:      time.Second,
		subs:              map[int64]chan ProviderEvent{},
	}
	svc.runs[childID] = &interactiveRun{
		id:           childID,
		parentRunID:  parentID,
		label:        "reviewer_correctness",
		agentName:    "reviewer",
		role:         "reviewer",
		providerKey:  ProviderKeyCodex,
		flowCohortId: cohortID,
		status:       RunStatusStarting,
		agentStatus:  "spawned",
		subs:         map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, childID)
	svc.agentOrchestrator.preRegisterCohort(parentID, cohortID, 1)
	svc.agentOrchestrator.mutateLoop(parentID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		st.Mode = "explicit"
		st.Cap = 3
		return st
	})

	svc.handleChildStartTurnFailure(childID, parentID, "simulated pre-adapter start failure")

	svc.mu.Lock()
	parent := svc.runs[parentID]
	if parent == nil || !parent.hubLastProgressAt.After(stale) {
		svc.mu.Unlock()
		t.Fatalf("hubLastProgressAt not refreshed on cohort pre-flight failure: parent=%v", parent)
	}
	svc.mu.Unlock()

	if svc.checkAndBlockStalledHub(parentID) {
		t.Fatal("hub_stalled must not fire after cohort pre-flight failure stamped hub progress")
	}
	loop := svc.agentOrchestrator.loopStateFor(parentID)
	if loop.BlockReason == "hub_stalled" {
		t.Fatalf("loop = %+v, want not hub_stalled", loop)
	}
}
