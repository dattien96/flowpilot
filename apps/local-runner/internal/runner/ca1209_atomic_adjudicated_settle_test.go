package runner

import (
	"context"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// CA-1209 (live run-262417): settleAdjudicatedDonePredecessors evaluated the
// durable-leg evidence for a predecessor, then stamped DONE later — with the
// sprint boundary's index++ (under s.mu) landing in between. The sprint-041
// audit's post-settle advance carried "synthesis has a completed leg"
// evidence across the take and stamped sprint-042's reseeded synthesis row
// DONE; the stale pred then auto-fired audit → the sprint sealed
// false-complete while its tdd leg was still writing the contract.
//
// Contract: evidence evaluation and the stamp are atomic under s.mu — if the
// take won first, the new sprint index scopes the session scan to the new
// sprint's legs and the stale stamp is refused.

// gatedSessionStore wraps the fake store with a controllable gate inside
// ListAllProviderSessions, so a test can park the settle between the index
// read (eval-time) and the stamp (write-time) — the exact live race window.
type gatedSessionStore struct {
	*fakeWorkflowStore
	entered chan struct{}
	gate    chan struct{}
}

func (g *gatedSessionStore) ListAllProviderSessions(ctx context.Context) ([]ProviderSessionState, error) {
	if g.entered != nil {
		close(g.entered)
		g.entered = nil
	}
	if g.gate != nil {
		<-g.gate
	}
	return g.fakeWorkflowStore.ListAllProviderSessions(ctx)
}

func armAdjudicatedSettleRun(t *testing.T, svc *InteractiveService, sprintIndex int) (string, *fakeWorkflowStore) {
	t.Helper()
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	parentID := parent.RunID
	svc.mu.Lock()
	rs := svc.runs[parentID]
	rs.flowEngineDriven = true
	rs.vibeSprintIndex = sprintIndex
	svc.mu.Unlock()

	store, ok := svc.workflowStore.(*fakeWorkflowStore)
	if !ok {
		t.Fatalf("expected fakeWorkflowStore, got %T", svc.workflowStore)
	}
	store.seed(parentID, []RuntimeWorkflowStep{
		{ID: "synthesis", NodeID: "synthesis", Status: StepStatusPending},
		{ID: "audit", NodeID: "audit", Status: StepStatusPending},
	})
	return parentID, store
}

var adjudicatedSettleEdges = []agentpack.FlowEdge{
	{From: "synthesis", To: "audit", When: "done", Kind: "forward"},
}

func TestCA1209_SettleVsBoundaryTakeMustNotStampNewSprintRow(t *testing.T) {
	svc, _ := newTestServer(t)
	parentID, store := armAdjudicatedSettleRun(t, svc, 1)
	// Sprint-041 evidence: a durably Completed synthesis leg stamped to the
	// PRIOR sprint index.
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:         "run-syn-s1",
		ParentRunID:   parentID,
		ProjectID:     "proj",
		ProviderKey:   ProviderKeyCodex,
		Label:         "synthesis",
		Status:        RunStatusCompleted,
		RunKind:       "delegate",
		VibeTaskIndex: 1,
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	entered := make(chan struct{})
	gate := make(chan struct{})
	svc.workflowStore = &gatedSessionStore{fakeWorkflowStore: store, entered: entered, gate: gate}

	settleDone := make(chan struct{})
	go func() {
		defer close(settleDone)
		svc.settleAdjudicatedDonePredecessors(parentID, adjudicatedSettleEdges, "audit")
	}()

	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("settle never reached the session-index read")
	}
	// The settle is parked mid-evaluation (its index read already happened
	// with sprint 1 live). Now the boundary take advances + the mount reseeds.
	svc.mu.Lock()
	svc.runs[parentID].vibeSprintIndex = 2
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parentID, []agentpack.FlowNode{
		{ID: "synthesis"}, {ID: "audit"},
	})
	close(gate)

	select {
	case <-settleDone:
	case <-time.After(5 * time.Second):
		t.Fatal("settle never returned")
	}
	if st := svc.lookupFlowStepStatus(parentID, "synthesis"); st == StepStatusDone {
		t.Fatal("sprint-041's late settle stamped sprint-042's synthesis row DONE — eval and stamp must share one s.mu hold")
	}
}

// The same settle must still stamp when the index has NOT advanced — the
// evidence belongs to the current sprint.
func TestCA1209_SettleStampsWithinSameSprint(t *testing.T) {
	svc, _ := newTestServer(t)
	parentID, store := armAdjudicatedSettleRun(t, svc, 1)
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:         "run-syn-s1",
		ParentRunID:   parentID,
		ProjectID:     "proj",
		ProviderKey:   ProviderKeyCodex,
		Label:         "synthesis",
		Status:        RunStatusCompleted,
		RunKind:       "delegate",
		VibeTaskIndex: 1,
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	svc.settleAdjudicatedDonePredecessors(parentID, adjudicatedSettleEdges, "audit")

	if st := svc.lookupFlowStepStatus(parentID, "synthesis"); st != StepStatusDone {
		t.Fatalf("adjudicated predecessor with a durably completed same-sprint leg must stamp DONE, got %q", st)
	}
}
