package runner

import (
	"context"
	"testing"
	"time"
)

// BUG-623 (live run-150388): a leg spawned through tryAdvanceFlowFromNode
// always carries a flow-auto-<node>-round-<N> cohort id, so its completion
// settles through the cohort-join path. When the cohort has exactly one
// member and that member's forward "done" target is a DELEGATE node
// (vibe-sprint tdd -> coder), there is no shared inline join target to
// dispatch — the code fell through to a hub reinvoke, and the hub cannot
// express that edge: its only flow-control instrument resolves the HUB
// node's own edges (synthesis -> coder), and that back-edge only reinvokes
// an existing child ("matched no existing child and did not spawn one").
// The hub then tried `done`, was correctly blocked by the missing reviewer
// verdict, escalated, and parked the run WAITING_USER_APPROVAL with the
// sprint stuck at tdd DONE forever.
//
// The fix: a single-member cohort whose forward done-targets are all
// delegate-spawnable must fire the member's own done-edge through
// tryAdvanceFlowFromNode — the same advance a non-cohorted leg gets via
// advanceOrNotifyHub.
func TestBUG623_SingleMemberCohortJoinAdvancesDelegateTarget(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "done"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	runID := armVibeSprintRun(t, svc)

	child, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("createRun(child): %v", err)
	}

	// Mirror the live wedge: tdd leg spawned via tryAdvanceFlowFromNode carries
	// a size-1 flow-auto cohort, so its completion goes through the join path.
	const cohortID = "flow-auto-context-round-16"
	svc.agentOrchestrator.preRegisterCohort(runID, cohortID, 1)
	svc.mu.Lock()
	rs := svc.runs[child.RunID]
	rs.parentRunID = runID
	rs.label = "tdd"
	rs.flowCohortId = cohortID
	rs.status = RunStatusCompleted
	rs.pendingFlowGateSettle = false
	svc.settleFlowChildTurnCompletedLocked(rs, "scaffold ready",
		ProviderEvent{Type: EventTurnCompleted, FinalMessage: "scaffold ready"})
	svc.mu.Unlock()

	waitLoop(t, "tdd -> coder member-edge advance spawns a coder leg", 6*time.Second, func() bool {
		return countChildrenWithLabel(svc, runID, "coder") >= 1
	})
}
