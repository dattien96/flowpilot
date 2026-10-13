package runner

import (
	"context"
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// BUG-674 (live run-816751 / run-752742 leg5): spawnChildRun's sealed-loop
// guards read loopStateFor(parentRunID) — the relay leg's own phantom loop.
// When that loop reported "stopped" (watchdog park on the phantom, or a
// deliberate neutralize), spawn_agent refused with "loop is stopped" even
// though the real flow loop on the launcher leg was alive — the engine
// expected that leg to spawn the review cohort. Fix: both guards consult
// the launcher leg's loop via flowOwnerLoopStateForLocked.

func bug674LauncherAndRelay(t *testing.T, svc *InteractiveService) (string, string) {
	t.Helper()
	launcher, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun launcher: %v", err)
	}
	relay, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun relay: %v", err)
	}
	svc.mu.Lock()
	lr := svc.runs[launcher.RunID]
	lr.chatID = "cht-bug674"
	lr.legState = LegStateActive
	lr.legSeq = 1
	lr.flowEngineDriven = true
	lr.activeFlowNodes = []agentpack.FlowNode{{ID: "reviewer_a", Behavior: "agent.delegate", Cohort: "review"}}
	rr := svc.runs[relay.RunID]
	rr.chatID = "cht-bug674"
	rr.legState = LegStateActive
	rr.legSeq = 2
	rr.flowEngineDriven = true
	svc.mu.Unlock()
	return launcher.RunID, relay.RunID
}

func TestBug674_SpawnOnRelayLegConsultsLauncherLoop(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	launcherID, relayID := bug674LauncherAndRelay(t, svc)

	// The relay leg's own (phantom) loop is stopped — the live leg5 shape.
	// The launcher leg has no parked loop (zero value = not sealed).
	svc.agentOrchestrator.stop(relayID)

	_, err := svc.spawnChildRun(context.Background(), relayID, SpawnAgentInput{
		Agent:  "reviewer",
		Prompt: "review the diff",
		Label:  "reviewer_a",
	})
	if err != nil && strings.Contains(err.Error(), "loop is") {
		t.Fatalf("spawn under relay leg must consult the launcher leg's loop, not the relay's stopped phantom: %v", err)
	}

	// Control arm stays: once the OWNER loop seals, spawn must refuse.
	svc.agentOrchestrator.stop(launcherID)
	_, err = svc.spawnChildRun(context.Background(), relayID, SpawnAgentInput{
		Agent:  "reviewer",
		Prompt: "review the diff",
		Label:  "reviewer_a",
	})
	if err == nil || !strings.Contains(err.Error(), "loop is") {
		t.Fatalf("sealed owner loop must still refuse spawn on the relay leg, got %v", err)
	}
}
