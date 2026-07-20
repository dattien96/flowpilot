package runner

import (
	"context"
	"path/filepath"
	"testing"
)

// CP-51 A1: continue-delegate suppression is turn-scoped. A valid hub gate
// reprompt on turn N+1 must not be suppressed by a prior continue on turn N.
func TestRun9437ContinueDelegateDoesNotSuppressLaterHubGateReprompt(t *testing.T) {
	const turnN = "turn-N-continue"
	const turnN1 = "turn-N1-gate"

	rs := &interactiveRun{
		id:                         "hub",
		hubContinueDelegatedTurnID: turnN,
		pendingGateRepromptPrompt:  "same-turn forbidden",
		pendingGateRepromptStepID:  "synthesis",
		pendingGateRepromptGen:     1,
	}
	if !clearHubGateRepromptIfContinueDelegatedLocked(rs, turnN) {
		t.Fatal("same-turn N must suppress")
	}
	if rs.pendingGateRepromptPrompt != "" || rs.hubContinueDelegatedTurnID != "" {
		t.Fatalf("same-turn suppress must clear reprompt and consume marker, got prompt=%q marker=%q",
			rs.pendingGateRepromptPrompt, rs.hubContinueDelegatedTurnID)
	}

	// After consume, a later N+1 reprompt is free.
	rs.pendingGateRepromptPrompt = "valid N+1 reprompt"
	rs.pendingGateRepromptStepID = "synthesis"
	rs.pendingGateRepromptGen = 2
	if clearHubGateRepromptIfContinueDelegatedLocked(rs, turnN1) {
		t.Fatal("N+1 must not suppress after marker consumed")
	}
	if rs.pendingGateRepromptPrompt != "valid N+1 reprompt" {
		t.Fatalf("N+1 reprompt lost: %q", rs.pendingGateRepromptPrompt)
	}

	// Leftover marker for N must not suppress a gate for N+1.
	rs.hubContinueDelegatedTurnID = turnN
	rs.pendingGateRepromptPrompt = "valid N+1 while marker still N"
	if clearHubGateRepromptIfContinueDelegatedLocked(rs, turnN1) {
		t.Fatal("N+1 must not suppress under marker for N")
	}
	if rs.pendingGateRepromptPrompt != "valid N+1 while marker still N" {
		t.Fatalf("N+1 reprompt cleared under mismatched marker: %q", rs.pendingGateRepromptPrompt)
	}
	if rs.hubContinueDelegatedTurnID != turnN {
		t.Fatalf("mismatched suppress must not consume marker, got %q", rs.hubContinueDelegatedTurnID)
	}

	// New hub turn start clears a stale continue-delegate marker.
	clearHubContinueDelegatedMarkerLocked(rs, turnN1)
	if rs.hubContinueDelegatedTurnID != "" {
		t.Fatal("new hub turn must clear stale continue-delegate marker")
	}
}

func TestRun9437SameTurnSuppressConsumesMarkerDurably(t *testing.T) {
	root := t.TempDir()
	store, err := NewLocalFileSessionStore(filepath.Join(root, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), newInteractiveCatalog(), store)
	const hubID = "run-9437-consume"
	const turnN = "turn-N"

	svc.mu.Lock()
	svc.runs[hubID] = &interactiveRun{
		id:                         hubID,
		flowEngineDriven:           true,
		status:                     RunStatusRunning,
		hubContinueDelegatedTurnID: turnN,
		pendingGateRepromptPrompt:  "same-turn forbidden",
		pendingGateRepromptStepID:  "synthesis",
		pendingGateRepromptGen:     3,
		lastTurnID:                 turnN,
		pendingFlowGateTurnID:      turnN,
		subs:                       map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()

	if !svc.durableSuppressHubGateRepromptAfterContinue(hubID, turnN) {
		t.Fatal("same-turn suppress expected")
	}

	// N+1 reprompt after consume must not be suppressed.
	svc.mu.Lock()
	svc.runs[hubID].pendingGateRepromptPrompt = "valid later reprompt"
	svc.runs[hubID].pendingGateRepromptStepID = "synthesis"
	svc.runs[hubID].pendingGateRepromptGen = 4
	svc.runs[hubID].lastTurnID = "turn-N1"
	svc.runs[hubID].pendingFlowGateTurnID = "turn-N1"
	svc.mu.Unlock()
	if svc.durableSuppressHubGateRepromptAfterContinue(hubID, "turn-N1") {
		t.Fatal("N+1 must not suppress after marker consumed")
	}
	svc.mu.Lock()
	if svc.runs[hubID].pendingGateRepromptPrompt != "valid later reprompt" {
		svc.mu.Unlock()
		t.Fatal("N+1 reprompt was cleared")
	}
	svc.mu.Unlock()

	loaded, ok, getErr := store.GetProviderSession(context.Background(), hubID)
	if getErr != nil || !ok {
		t.Fatalf("GetProviderSession: ok=%v err=%v", ok, getErr)
	}
	if loaded.HubContinueDelegatedTurnID != "" {
		t.Fatalf("disk marker not consumed: %q", loaded.HubContinueDelegatedTurnID)
	}
}

func TestRun9437FlushDoesNotSuppressMismatchedGateTurn(t *testing.T) {
	svc := bug289Service(t)
	const hubID = "run-9437-flush-scope"
	const turnN = "turn-N"
	const turnN1 = "turn-N1"

	svc.mu.Lock()
	svc.runs[hubID] = &interactiveRun{
		id:                         hubID,
		flowEngineDriven:           true,
		status:                     RunStatusRunning,
		hubContinueDelegatedTurnID: turnN,
		pendingGateRepromptPrompt:  "valid N+1",
		pendingGateRepromptStepID:  "synthesis",
		pendingGateRepromptGen:     5,
		lastTurnID:                 turnN1,
		pendingFlowGateTurnID:      turnN1,
		subs:                       map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.mutateLoop(hubID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		return st
	})

	svc.flushDurableTurnIntents(hubID)

	svc.mu.Lock()
	rs := svc.runs[hubID]
	if rs.hubContinueDelegatedTurnID != turnN {
		svc.mu.Unlock()
		t.Fatalf("flush must not consume marker for mismatched gate turn, got %q", rs.hubContinueDelegatedTurnID)
	}
	// Intent may still be present (startTurn not fully wired in bug289Service)
	// or claimed — but must not be continue-suppress-cleared.
	if rs.pendingGateRepromptPrompt == "" && rs.pendingGateRepromptGen == 0 && rs.turnInFlight {
		// started — OK
	} else if rs.pendingGateRepromptPrompt != "valid N+1" && !rs.turnInFlight {
		// If start failed without clearIntent, prompt should remain.
		if rs.pendingGateRepromptPrompt == "" {
			svc.mu.Unlock()
			t.Fatal("N+1 reprompt cleared by flush without matching continue turn")
		}
	}
	svc.mu.Unlock()
}
