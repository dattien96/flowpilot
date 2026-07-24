package runner

import (
	"sync/atomic"
	"testing"
)

// Provider-agnostic (run-45103): applyFlowControl / parkFlowForAwaitingUser have
// no providerKey branch; SubmitFlowControl is shared for Claude/Codex/Grok.

// TestRun45103CapContinueFromSubmittingHubTurnDoesNotCancelParentTurn locks the
// live hang: synthesis turn submits continue at Round+1 >= Cap; park must freeze
// the flow but must NOT cancel the submitting hub turn (context canceled /
// Thinking... UX). Cap block + WAITING_USER_APPROVAL contracts stay intact.
func TestRun45103CapContinueFromSubmittingHubTurnDoesNotCancelParentTurn(t *testing.T) {
	svc, runID := newFlowEngineTestRun(t)
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{
		Status: "running", Cap: 3, Round: 2, RoundCap: 3, Mode: "explicit",
	})

	var parentCancelled atomic.Bool
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.currentTurnID = "turn-synthesis-cap"
	rs.turnInFlight = true
	rs.pendingHubReinvoke = true
	rs.pendingGateRepromptPrompt = "should clear"
	rs.pendingGateRepromptStepID = "synthesis"
	rs.pendingFlowGateSettle = true
	rs.turnCancel = func() { parentCancelled.Store(true) }
	// lastFlowControlTurnID empty — applyFlowControl stamps it to currentTurnID.
	svc.mu.Unlock()

	result, err := svc.applyFlowControl(runID, FlowControlInput{
		Status:  "continue",
		Summary: "still open issues",
		Payload: map[string]any{"issues": []any{map[string]any{"title": "suite_regressed"}}},
	})
	if err != nil {
		t.Fatalf("applyFlowControl: %v", err)
	}
	if result.Status != "blocked" || result.NextAction != "awaiting_user" {
		t.Fatalf("result = %+v, want blocked/awaiting_user", result)
	}
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.Status != "blocked" || loop.BlockReason != "cap" {
		t.Fatalf("loop = %+v, want blocked/cap", loop)
	}
	if loop.GateReason == "" {
		t.Fatal("GateReason must describe cap reached")
	}
	if got := flowStepStatus(t, svc, runID, "synthesis"); got != StepStatusWaitingUserApr {
		t.Fatalf("synthesis step = %v, want WAITING_USER_APPROVAL", got)
	}
	if parentCancelled.Load() {
		t.Fatal("submitting hub turn must NOT be cancelled by park when continue hits cap (run-45103)")
	}
	svc.mu.Lock()
	rs = svc.runs[runID]
	if rs.pendingHubReinvoke || rs.pendingGateRepromptPrompt != "" || rs.pendingFlowGateSettle {
		svc.mu.Unlock()
		t.Fatalf("auto-intents must still clear: reinvoke=%v reprompt=%q settle=%v",
			rs.pendingHubReinvoke, rs.pendingGateRepromptPrompt, rs.pendingFlowGateSettle)
	}
	svc.mu.Unlock()
}

// TestRun45103CapContinueStillCancelsChildrenAndExternalHubWork ensures freeze
// semantics stay strict for everyone except the same-turn submitting hub turn.
func TestRun45103CapContinueStillCancelsChildrenAndExternalHubWork(t *testing.T) {
	svc, runID := newFlowEngineTestRun(t)
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{
		Status: "running", Cap: 3, Round: 2, RoundCap: 3, Mode: "explicit",
	})

	var parentCancelled, childCancelled atomic.Bool
	childID := "run-child-cap"
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.currentTurnID = "turn-synthesis-cap"
	rs.turnInFlight = true
	rs.turnCancel = func() { parentCancelled.Store(true) }
	svc.runs[childID] = &interactiveRun{
		id:           childID,
		parentRunID:  runID,
		label:        "reviewer_security",
		status:       RunStatusRunning,
		turnInFlight: true,
		turnCancel:   func() { childCancelled.Store(true) },
		subs:         map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(runID, childID)

	// Cap from submitting turn: parent preserved, child cancelled.
	if _, err := svc.applyFlowControl(runID, FlowControlInput{
		Status:  "continue",
		Summary: "still open",
		Payload: map[string]any{"issues": []any{map[string]any{"title": "x"}}},
	}); err != nil {
		t.Fatalf("applyFlowControl: %v", err)
	}
	if parentCancelled.Load() {
		t.Fatal("parent submitting turn must be preserved")
	}
	if !childCancelled.Load() {
		t.Fatal("child in-flight turn must still be cancelled by park")
	}

	// External freeze (no matching lastFlowControlTurnID / empty currentTurnID):
	// park without preserve must still cancel parent (hub_stall / escalate class).
	var externalCancelled atomic.Bool
	svc.mu.Lock()
	rs = svc.runs[runID]
	rs.currentTurnID = "turn-external"
	rs.lastFlowControlTurnID = "" // not the submitting decision for this park
	rs.turnInFlight = true
	rs.turnCancel = func() { externalCancelled.Store(true) }
	svc.mu.Unlock()
	svc.parkFlowForAwaitingUser(runID)
	if !externalCancelled.Load() {
		t.Fatal("default park must still cancel parent when no preserveParentTurnID")
	}
}

// TestRun45103ParkPreserveRequiresMatchingTurnID guards the option: wrong turn id
// must not suppress cancel.
func TestRun45103ParkPreserveRequiresMatchingTurnID(t *testing.T) {
	svc, runID := newFlowEngineTestRun(t)
	var cancelled atomic.Bool
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.currentTurnID = "turn-A"
	rs.turnInFlight = true
	rs.turnCancel = func() { cancelled.Store(true) }
	svc.mu.Unlock()

	svc.parkFlowForAwaitingUser(runID, parkFlowForAwaitingUserOptions{
		preserveParentTurnID: "turn-B", // mismatch
	})
	if !cancelled.Load() {
		t.Fatal("mismatched preserveParentTurnID must not skip parent cancel")
	}
}
