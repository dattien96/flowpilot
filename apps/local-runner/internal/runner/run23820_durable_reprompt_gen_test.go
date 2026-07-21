package runner

import (
	"fmt"
	"testing"
)

// run-23820: second durable gate reprompt on the same child must not reuse
// gen=1 idempotency key of the completed first reprompt. clearIntentFields
// keeps gen as a high-water mark so gate_hook's gen++ yields a fresh key.
//
// Cross-provider parity: Case 1 — clearIntentFieldsLocked / startTurn durable
// idempotency take no providerKey and do not branch on one. Codex is the
// representative provider for createRun; the same path serves Claude and Grok.
func TestRun23820SecondDurableRepromptUsesNextGenNotFirstKeyReplay(t *testing.T) {
	svc, _ := newTestServer(t)
	run, err := svc.createRun(StartRunInput{
		ProjectID:   "p-23820",
		ChatMode:    "normal_chat",
		ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatal(err)
	}
	runID := run.RunID

	// --- Reprompt #1 (gen=1) ---
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.pendingGateRepromptPrompt = "Missing Change Contract remediation #1"
	rs.pendingGateRepromptStepID = "chat-" + runID
	rs.pendingGateRepromptGen = 1
	rs.stepID = "chat-" + runID
	rs.lastTurnStepID = "chat-" + runID
	svc.mu.Unlock()

	key1 := fmt.Sprintf("durable-%s-reprompt-%020d", runID, 1)
	tid1, apiErr := svc.startTurn(runID, TurnInput{
		StepID: "chat-" + runID,
		Prompt: "Missing Change Contract remediation #1",
	}, "", key1)
	if apiErr != nil {
		t.Fatalf("first durable reprompt startTurn: %v", apiErr)
	}
	if tid1 == "" {
		t.Fatal("first reprompt returned empty turn id")
	}

	// Simulate accept + turn completion (provider finished; gate will re-queue).
	svc.mu.Lock()
	rs = svc.runs[runID]
	rs.turnInFlight = false
	rs.currentTurnID = ""
	rs.lastTurnID = tid1
	// Delivery clear: must keep gen high-water (was the bug when gen→0).
	clearIntentFieldsLocked(rs, "reprompt")
	if rs.pendingGateRepromptPrompt != "" {
		t.Fatal("prompt must clear after delivery")
	}
	if rs.pendingGateRepromptGen != 1 {
		t.Fatalf("gen high-water after clear = %d, want 1 (must not reset to 0)", rs.pendingGateRepromptGen)
	}
	// Gate re-queues second reprompt: gen++ from high-water.
	rs.pendingGateRepromptPrompt = "Missing Change Contract remediation #2"
	rs.pendingGateRepromptStepID = "chat-" + runID
	rs.pendingGateRepromptGen++
	gen2 := rs.pendingGateRepromptGen
	svc.mu.Unlock()

	if gen2 != 2 {
		t.Fatalf("second queue gen = %d, want 2", gen2)
	}

	key2 := fmt.Sprintf("durable-%s-reprompt-%020d", runID, gen2)
	if key2 == key1 {
		t.Fatal("second durable key must differ from first")
	}

	// Old key must short-circuit as completed-turn replay (lastTurnID==tid1).
	tidReplay, apiErr := svc.startTurn(runID, TurnInput{
		StepID: "chat-" + runID,
		Prompt: "should replay first key only",
	}, "", key1)
	if apiErr != nil {
		t.Fatalf("replay of first key: %v", apiErr)
	}
	if tidReplay != tid1 {
		t.Fatalf("first key replay turn = %q, want %q", tidReplay, tid1)
	}
	// Ensure we did not start a live second flight via the old key.
	svc.mu.Lock()
	if svc.runs[runID].turnInFlight && svc.runs[runID].currentTurnID != tid1 {
		svc.mu.Unlock()
		t.Fatal("old key must not launch a different live turn")
	}
	// Drop any residual in-flight from create path before second key.
	svc.runs[runID].turnInFlight = false
	svc.runs[runID].currentTurnID = ""
	svc.mu.Unlock()

	// New key must open a real new turn (not silent replay of tid1).
	tid2, apiErr := svc.startTurn(runID, TurnInput{
		StepID: "chat-" + runID,
		Prompt: "Missing Change Contract remediation #2",
	}, "", key2)
	if apiErr != nil {
		t.Fatalf("second durable reprompt startTurn: %v", apiErr)
	}
	if tid2 == "" {
		t.Fatal("second reprompt returned empty turn id")
	}
	if tid2 == tid1 {
		t.Fatalf("second reprompt reused turn %q; want a new turn (gen-wrap regression)", tid1)
	}
	svc.mu.Lock()
	rs = svc.runs[runID]
	if !rs.turnInFlight || rs.currentTurnID != tid2 {
		t.Fatalf("second reprompt not live: inFlight=%v current=%q want %q",
			rs.turnInFlight, rs.currentTurnID, tid2)
	}
	svc.mu.Unlock()
}

// Prove the pre-fix failure mode: zeroing gen after clear makes the next
// queue reuse key ...0001 and startTurn returns the completed turn without a
// new live flight (the run-23820 strand).
func TestRun23820ZeroingGenWouldReplayCompletedRepromptKey(t *testing.T) {
	svc, _ := newTestServer(t)
	// Case 1 path — any registered provider; Codex is available in unit tests.
	// Live run-23820 used Grok; gen/idempotency is not provider-keyed.
	run, err := svc.createRun(StartRunInput{
		ProjectID:   "p-23820-legacy",
		ChatMode:    "normal_chat",
		ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatal(err)
	}
	runID := run.RunID

	key1 := fmt.Sprintf("durable-%s-reprompt-%020d", runID, 1)
	tid1, apiErr := svc.startTurn(runID, TurnInput{StepID: "s", Prompt: "reprompt-1"}, "", key1)
	if apiErr != nil {
		t.Fatalf("first start: %v", apiErr)
	}
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.turnInFlight = false
	rs.currentTurnID = ""
	rs.lastTurnID = tid1
	// Legacy bug: zero gen then ++ → same key.
	rs.pendingGateRepromptGen = 0
	rs.pendingGateRepromptGen++
	if rs.pendingGateRepromptGen != 1 {
		svc.mu.Unlock()
		t.Fatalf("setup gen=%d", rs.pendingGateRepromptGen)
	}
	svc.mu.Unlock()

	tid2, apiErr := svc.startTurn(runID, TurnInput{StepID: "s", Prompt: "reprompt-2"}, "", key1)
	if apiErr != nil {
		t.Fatalf("legacy-key start: %v", apiErr)
	}
	if tid2 != tid1 {
		t.Fatalf("legacy zero-gen path should replay tid1, got %q vs %q", tid2, tid1)
	}
	svc.mu.Lock()
	// Replay of a completed turn is "safe" — must not leave a distinct live turn.
	if svc.runs[runID].currentTurnID != "" && svc.runs[runID].currentTurnID != tid1 && svc.runs[runID].turnInFlight {
		t.Fatalf("unexpected new live turn on gen-wrap key")
	}
	svc.mu.Unlock()
}
