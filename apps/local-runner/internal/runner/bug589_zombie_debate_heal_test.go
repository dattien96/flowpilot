package runner

import (
	"testing"
	"time"

	"flowpilot-runner/internal/workingmode"
)

// BUG-589 (live run-139670): an owner-debate overlay can end up claimed but
// dead — vibeParkedNodes holds the real flow, both owner legs settled, yet
// debate_synthesis never ran (its cohort join mis-dispatched into the sprint's
// `synthesis` step and the hub reinvoke died). On the live path nothing
// re-drives it: every subsequent gate divert hits "debate already active" and
// the gated child's outcome is silently orphaned — the diverting node stays
// RUNNING forever even though its work finished.
//
// The suppressed-divert path must therefore run the zombie check instead of
// swallowing the outcome: owners settled + synthesis pending → re-drive the
// synthesis hub; synthesis already done → replay the (idempotent) restore.
// A genuinely live debate (running owners/synthesis) must be left untouched.

func bug589SeedZombieDebate(t *testing.T, synDone bool) (*InteractiveService, string) {
	t.Helper()
	store := newFakeWorkflowStore()
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)
	runID := "run-zombie-debate"

	// Seed the step store: owners DONE, debate_synthesis pending-or-done.
	synStatus := StepStatusPending
	if synDone {
		synStatus = StepStatusDone
	}
	store.seed(runID, []RuntimeWorkflowStep{
		{NodeID: "debate_trigger", Status: StepStatusDone},
		{NodeID: "owner_1", Status: StepStatusDone},
		{NodeID: "owner_2", Status: StepStatusDone},
		{NodeID: "debate_synthesis", Status: synStatus},
	})

	rs := &interactiveRun{
		id:               runID,
		projectID:        "proj",
		status:           RunStatusRunning,
		agentStatus:      string(RunStatusRunning),
		workingMode:      workingmode.Vibe,
		chatFlowRef:      workingmode.PackPrefix + vibeOwnerDebateFlowID,
		autoOrchestrate:  true,
		flowEngineDriven: true,
		activeFlowNodes:  bug567DebateGraph(),
		vibeParkedNodes:  bug567SprintGraph(),
	}
	svc.mu.Lock()
	svc.runs[runID] = rs
	svc.mu.Unlock()
	return svc, runID
}

// Suppressed divert + owners settled + synthesis never ran → must re-drive the
// synthesis hub (the same redrive the restart path performs), not drop the
// gated child's outcome into the void.
func TestBUG589SuppressedDivertRedrivesDeadDebateSynthesis(t *testing.T) {
	svc, runID := bug589SeedZombieDebate(t, false)

	svc.startVibeOwnerDebate(runID, "run-gated-coder", "gate block: r-tests")

	waitLoop(t, "zombie debate re-driven after suppressed divert", 5*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		rs := svc.runs[runID]
		if rs == nil {
			return false
		}
		return rs.reinvokeInFlight || rs.pendingHubReinvoke || rs.turnInFlight
	})
}

// Suppressed divert + debate_synthesis already DONE → the missed restore is
// replayed: parked topology comes back and every gated child owes a reprompt.
func TestBUG589SuppressedDivertRestoresWhenSynthesisDone(t *testing.T) {
	svc, runID := bug589SeedZombieDebate(t, true)

	svc.startVibeOwnerDebate(runID, "run-gated-coder", "gate block: r-tests")

	waitLoop(t, "parked topology restored after suppressed divert", 5*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		rs := svc.runs[runID]
		return rs != nil && len(rs.vibeParkedNodes) == 0 && runHasFlowNode(rs, "coder")
	})
}

// A debate that is actually live (an owner still running) must NOT be
// restored by a concurrent divert — suppression stays suppression.
func TestBUG589SuppressedDivertLeavesLiveDebateAlone(t *testing.T) {
	svc, runID := bug589SeedZombieDebate(t, false)
	svc.workflowStore.(*fakeWorkflowStore).seed(runID, []RuntimeWorkflowStep{
		{NodeID: "debate_trigger", Status: StepStatusDone},
		{NodeID: "owner_1", Status: StepStatusDone},
		{NodeID: "owner_2", Status: StepStatusRunning},
		{NodeID: "debate_synthesis", Status: StepStatusPending},
	})

	svc.startVibeOwnerDebate(runID, "run-gated-coder", "gate block: r-tests")

	// Give the heal a chance to fire if it wrongly did.
	time.Sleep(150 * time.Millisecond)
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	if rs == nil || len(rs.vibeParkedNodes) == 0 {
		t.Fatal("live debate overlay was unmounted by a suppressed divert — restored while an owner was still running")
	}
}
