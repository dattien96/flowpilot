package runner

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// BUG-672 (live run-752742): an armed pendingHubReinvoke is drained only
// when (a) resumeFlowWithFeedback took the wasBlocked path AND the pending
// carried a prompt, or (b) the hub-stall tick saw failCount<=3. A Continue
// landing on a non-blocked loop (rejected — the post-verdict state) early-
// returned past BOTH drains while the saturated fail budget
// (hubReinvokeStartFailCount>3) kept the watchdog's drain gate closed —
// the armed intent stayed armed forever and every stall tick re-parked the
// flow. Operator Continue is a fresh budget: consume the armed pending and
// reset the fail count regardless of the loop's current status.

func TestBug672_ContinueDrainsArmedReinvokeOnNonBlockedLoop(t *testing.T) {
	var mu sync.Mutex
	var turns int
	t.Setenv("FLOWPILOT_QUOTA_ROUTING_STATE_FILE", filepath.Join(t.TempDir(), "quota-state.json"))
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				mu.Lock()
				turns++
				mu.Unlock()
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.activeFlowNodes = []agentpack.FlowNode{{ID: "synthesis", Behavior: "hub.inline"}}
	// Armed prompt-less pending + saturated budget — the stranded shape.
	rs.pendingHubReinvoke = true
	rs.pendingHubReinvokePrompt = ""
	rs.hubReinvokeStartFailCount = 4
	rs.providerAccountID = svc.activeAccountForProvider(rs.providerKey)
	svc.mu.Unlock()
	// Post-verdict loop state — NOT blocked, so the wasBlocked drain is
	// skipped entirely.
	svc.agentOrchestrator.transition(parent.RunID, "rejected")

	if _, err := svc.resumeFlowWithFeedback(parent.RunID, "continue"); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}
	waitLoop(t, "reinvoke turn dispatched", 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return turns > 0
	})
	svc.mu.Lock()
	defer svc.mu.Unlock()
	got := svc.runs[parent.RunID]
	if got.pendingHubReinvoke {
		t.Fatal("armed pendingHubReinvoke must be consumed by an operator Continue")
	}
	if got.hubReinvokeStartFailCount != 0 {
		t.Fatalf("operator Continue must reset the consecutive-fail budget, got %d", got.hubReinvokeStartFailCount)
	}
	if turns == 0 {
		t.Fatal("armed reinvoke intent was never dispatched")
	}
}
