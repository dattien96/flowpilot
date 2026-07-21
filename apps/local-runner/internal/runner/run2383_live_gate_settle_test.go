package runner

import (
	"context"
	"testing"
	"time"
)

type run2383TerminalAdapter struct{ key ProviderKey }

func (a run2383TerminalAdapter) Key() ProviderKey { return a.key }

func (a run2383TerminalAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Streaming: true}
}

func (a run2383TerminalAdapter) SendTurn(_ context.Context, _ TurnRequest, bridge TurnBridge) error {
	bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "flow control completed"})
	return nil
}

func TestRun2383LiveGatePassFinalizesTerminalSettleForEverySupportedProvider(t *testing.T) {
	for _, providerKey := range []ProviderKey{ProviderKeyCodex, ProviderKeyClaude, ProviderKeyGrok} {
		t.Run(string(providerKey), func(t *testing.T) {
			ctx := context.Background()
			store := NewMemoryDispatchStore()
			svc, _ := newTestServer(t)
			svc.dispatchStore = store

			handle, createErr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
			if createErr != nil {
				t.Fatalf("createRun: %v", createErr)
			}
			const turnID = "turn-2383-live-gate"
			record := testPrepared(handle.RunID, turnID)
			record.SettleOwed = true
			if err := store.CreatePrepared(ctx, record, testEnvelope(handle.RunID, turnID)); err != nil {
				t.Fatalf("CreatePrepared: %v", err)
			}
			current, revision, getErr := store.Get(ctx, handle.RunID, turnID)
			if getErr != nil {
				t.Fatalf("Get prepared: %v", getErr)
			}
			revision, advanceErr := store.CASAdvance(ctx, handle.RunID, turnID, revision, DispatchPrepared, DispatchSendClaimed, nil)
			if advanceErr != nil {
				t.Fatalf("send_claimed: %v", advanceErr)
			}
			revision, advanceErr = store.CASAdvance(ctx, handle.RunID, turnID, revision, DispatchSendClaimed, DispatchSendStarted, nil)
			if advanceErr != nil {
				t.Fatalf("send_started: %v", advanceErr)
			}
			proofPayload := []byte(`{"type":"turn_completed"}`)
			if _, err := store.CommitTerminalAndSettleIntent(ctx, handle.RunID, turnID, revision, TerminalEvidence{
				ProviderKey:          string(providerKey),
				EvidenceKind:         string(EventTurnCompleted),
				Outcome:              "completed",
				PayloadCanonicalJSON: proofPayload,
				PayloadSHA256:        HashBytes(proofPayload),
				ObservedAt:           nowRFC3339Nano(),
			}, current.IntentOwnerRunID, current.OuterIntentKey, current.OuterIntentGen); err != nil {
				t.Fatalf("CommitTerminalAndSettleIntent: %v", err)
			}

			svc.mu.Lock()
			rs := svc.runs[handle.RunID]
			// The dispatch/gate path is shared. Use the test adapter for each
			// supported provider without depending on a locally installed runtime.
			rs.providerKey = providerKey
			rs.flowEngineDriven = true
			// Model a completed synthesis outcome: it has no child-style deferred
			// settle flag, but it still must drive the durable terminal settlement
			// after its live gate passes.
			rs.status = RunStatusCompleted
			rs.agentStatus = string(RunStatusCompleted)
			rs.currentTurnID = turnID
			rs.turnInFlight = true
			svc.mu.Unlock()
			svc.agentOrchestrator.setLoop(handle.RunID, AgentLoopState{Status: "done", Round: 1, Cap: 3, Mode: "explicit"})
			svc.runTurn(ctx, rs, run2383TerminalAdapter{key: providerKey}, TurnInput{Prompt: "complete"}, "normal", turnID, nil)
			// This is the production seam reached only after a successful live gate.
			// The fake adapter supplies terminal evidence but does not implement the
			// flow-control tool that drives a real synthesis turn to this point.
			svc.scheduleSettleAfterGatePass(handle.RunID, turnID)

			deadline := time.Now().Add(2 * time.Second)
			for time.Now().Before(deadline) {
				settled, _, getErr := store.Get(ctx, handle.RunID, turnID)
				if getErr == nil && settled.SettlePhase == SettleFinalized {
					return
				}
				time.Sleep(time.Millisecond)
			}
			settled, _, getErr := store.Get(ctx, handle.RunID, turnID)
			if getErr != nil {
				t.Fatalf("Get settled record: %v", getErr)
			}
			t.Fatalf("settle phase = %s, want %s", settled.SettlePhase, SettleFinalized)
		})
	}
}
