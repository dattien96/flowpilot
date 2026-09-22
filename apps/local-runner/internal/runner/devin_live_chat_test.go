package runner

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// Live Devin tests — gated behind FLOWPILOT_LIVE_DEVIN=1. They spawn the REAL
// `devin acp` process (ambient credentials from the logged-in CLI) and drive
// the devinAdapter end to end. Model: SWE-2 (swe-2-high).

func requireLiveDevinOptIn(t *testing.T) {
	t.Helper()
	if os.Getenv("FLOWPILOT_LIVE_DEVIN") == "" {
		t.Skip("set FLOWPILOT_LIVE_DEVIN=1 to run the live e2e against a real logged-in devin account")
	}
}

// TestLiveRealDevinChatStreamAndResume drives the actual devinAdapter through
// a real `devin acp` process on SWE-2: fresh session/new, streamed answer,
// token usage, then a real session/load resume that must keep conversational
// context (the model recalls the word from turn 1).
func TestLiveRealDevinChatStreamAndResume(t *testing.T) {
	requireLiveDevinOptIn(t)

	scratch := t.TempDir()
	r := &Runner{workspace: scratch}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	h, err := r.ensureDevinProcess(ctx, "live-manual-scope", scratch, nil, "swe-2-high", "")
	if err != nil {
		t.Fatalf("ensureDevinProcess: %v", err)
	}
	defer h.close()

	store := &liveTestSessionStore{}
	h.adapter.sessionStore = store

	fmt.Println("--- turn 1: fresh session (swe-2-high) ---")
	bridge1 := &liveManualBridge{approveDecision: "approve"}
	err = h.adapter.SendTurn(ctx, TurnRequest{
		RunID:     "live-devin-1",
		Prompt:    "Reply with exactly the single word: PINEAPPLE. Do not call any tools.",
		ModelName: "swe-2-high",
		Cwd:       scratch,
	}, bridge1)
	if err != nil {
		t.Fatalf("SendTurn (turn 1): %v", err)
	}
	if !bridge1.hasEvent(EventTurnCompleted) {
		t.Fatal("turn 1: expected a turn_completed event")
	}
	fmt.Println("turn 1 final message:", bridge1.finalMessage())

	sessionID := store.latestSessionID()
	if sessionID == "" {
		t.Fatal("expected a real ACP sessionId to have been persisted")
	}
	fmt.Println("captured real sessionId:", sessionID)

	fmt.Println("--- turn 2: resume via session/load, same process ---")
	bridge2 := &liveManualBridge{approveDecision: "approve"}
	err = h.adapter.SendTurn(ctx, TurnRequest{
		RunID:             "live-devin-1",
		ProviderSessionID: sessionID,
		Prompt:            "What single word did I just ask you to reply with? Answer with only that one word.",
		ModelName:         "swe-2-high",
		Cwd:               scratch,
	}, bridge2)
	if err != nil {
		t.Fatalf("SendTurn (turn 2, resume): %v", err)
	}
	final2 := bridge2.finalMessage()
	fmt.Println("turn 2 (resumed) final message:", final2)
	if final2 == "" {
		t.Fatal("resumed turn produced no final message")
	}
	if !strings.Contains(strings.ToLower(final2), "pineapple") {
		t.Fatalf("resumed turn lost context: final=%q", final2)
	}
}
