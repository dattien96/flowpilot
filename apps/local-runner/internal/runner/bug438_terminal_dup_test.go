package runner

import (
	"context"
	"testing"
	"time"
)

// BUG-438 (live lt-verify-devin run-1): a chat run whose turn emitted a real
// file_changed event arms pendingFlowGateSettle; emitLocked does NOT defer the
// live terminal broadcast for plain chat roots (deferGateCompleted=false), so
// subscribers already saw turn_completed — then the armed-settle pass branch
// materialized and broadcast a SECOND identical turn_completed.
//
// Expected: exactly one turn_completed reaches a live subscriber.

func TestBug438_ArmedSettleChatRunBroadcastsTerminalOnce(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventFileChanged, Path: "main.go"})
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "done"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	run, apiE := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if apiE != nil {
		t.Fatalf("createRun: %v", apiE)
	}

	subID, ch, _, ok := svc.subscribe(run.RunID, 0)
	if !ok {
		t.Fatal("subscribe failed")
	}
	defer svc.unsubscribe(run.RunID, subID)

	if _, err := svc.startTurn(run.RunID, TurnInput{StepID: "chat-" + run.RunID, Prompt: "change main.go"}, "", ""); err != nil {
		t.Fatalf("startTurn: %v", err)
	}

	// Drain until the run settles (post-gate materialization included) or timeout.
	deadline := time.Now().Add(8 * time.Second)
	terminals := 0
	for time.Now().Before(deadline) {
		select {
		case ev, ok := <-ch:
			if !ok {
				goto drained
			}
			if ev.Type == EventTurnCompleted {
				terminals++
			}
		case <-time.After(1500 * time.Millisecond):
			goto drained
		}
	}
drained:
	if terminals != 1 {
		t.Fatalf("subscriber saw %d turn_completed events, want exactly 1 (dup terminal on armed-settle chat run)", terminals)
	}
}
