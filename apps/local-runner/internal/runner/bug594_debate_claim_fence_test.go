package runner

import (
	"context"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// BUG-594 (live run-139670): the owner-debate topology claim
// (vibeParkedNodes non-empty) has no mutual exclusion with new flow mounts.
// While the debate overlay was mounted, a still-running ingest leg
// (task_plan_reader) completed under the parked chain, advanced into the
// sprint start, and startResolvedFlow overwrote activeFlowNodes/chatFlowRef
// — the debate graph was clobbered mid-deliberation, the owner cohort join
// stamped the sprint's `synthesis` node, debate_synthesis never ran, and
// every later gate divert was swallowed by "debate already active".
//
// The contract: while the claim holds, non-debate flow mounts defer —
// either refused through the caller's own rollback (vibe-sprint via
// vibeSprintStartBlocked / the boundary take) or recorded durably and
// drained after restoreVibeFlowAfterDebate. A foreign-shape claim
// (parked nodes detached from a mounted non-debate topology) is stale
// corruption, not a live debate — it must drop so a fresh divert can
// actually park + mount.

// bug594SeedClaimedDebate seeds a run mid-debate: sprint topology parked,
// debate graph mounted. gatedRunID may be "".
func bug594SeedClaimedDebate(t *testing.T) (*InteractiveService, *fakeWorkflowStore, string) {
	t.Helper()
	store := newFakeWorkflowStore()
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)
	runID := "run-bug594-claimed"
	store.seed(runID, []RuntimeWorkflowStep{
		{NodeID: "debate_trigger", Status: StepStatusDone},
		{NodeID: "owner_1", Status: StepStatusRunning},
		{NodeID: "owner_2", Status: StepStatusRunning},
		{NodeID: "debate_synthesis", Status: StepStatusPending},
	})
	rs := &interactiveRun{
		id:                runID,
		projectID:         "proj",
		status:            RunStatusRunning,
		agentStatus:       string(RunStatusRunning),
		workingMode:       workingmode.Vibe,
		chatFlowRef:       workingmode.PackPrefix + vibeOwnerDebateFlowID,
		autoOrchestrate:   true,
		flowEngineDriven:  true,
		activeFlowNodes:   bug567DebateGraph(),
		vibeParkedNodes:   bug567SprintGraph(),
		vibeParkedFlowRef: workingmode.PackPrefix + vibeSprintFlowID,
		vibeTaskPlan:      []string{"task-a", "task-b"},
		vibeSprintIndex:   0,
	}
	svc.mu.Lock()
	svc.runs[runID] = rs
	svc.mu.Unlock()
	return svc, store, runID
}

// The live sequence: a parked-chain leg completes mid-debate and the chain
// advance calls maybeStartNextVibeSprint. The sprint index must NOT be
// consumed and nothing may mount — the parked chain re-derives the start
// once the debate restores.
func TestBUG594ChainSprintStartBlockedWhileDebateClaimed(t *testing.T) {
	svc, _, runID := bug594SeedClaimedDebate(t)

	svc.maybeStartNextVibeSprint(runID)

	svc.mu.Lock()
	rs := svc.runs[runID]
	gotIndex := rs.vibeSprintIndex
	gotRef := rs.chatFlowRef
	isDebate := vibeOwnerDebateGraph(rs.activeFlowNodes)
	svc.mu.Unlock()
	if gotIndex != 0 {
		t.Fatalf("vibeSprintIndex consumed under the debate claim: got %d want 0 — a skipped task follows", gotIndex)
	}
	if !isDebate {
		t.Fatalf("debate topology clobbered by a sprint start under the claim: chatFlowRef=%q", gotRef)
	}
}

// A non-sprint flow start under the claim must not clobber the debate
// topology either — it queues durably instead of mounting.
func TestBUG594ForeignFlowStartDefersWhileClaimed(t *testing.T) {
	svc, _, runID := bug594SeedClaimedDebate(t)

	svc.startResolvedFlow(context.Background(), runID, workingmode.PackPrefix+"rag-harness", "ctx prompt")

	svc.mu.Lock()
	rs := svc.runs[runID]
	gotRef := rs.chatFlowRef
	isDebate := vibeOwnerDebateGraph(rs.activeFlowNodes)
	deferred := len(rs.vibeDeferredFlowStarts)
	svc.mu.Unlock()
	if !isDebate {
		t.Fatalf("debate topology clobbered by a foreign flow start under the claim: chatFlowRef=%q", gotRef)
	}
	if deferred != 1 {
		t.Fatalf("deferred flow start not recorded: got %d want 1", deferred)
	}
}

// Deferred starts drain once the debate restores: the queued mount fires on
// the restored topology.
func TestBUG594DeferredFlowStartDrainsOnRestore(t *testing.T) {
	svc, store, runID := bug594SeedClaimedDebate(t)

	svc.startResolvedFlow(context.Background(), runID, workingmode.PackPrefix+"rag-harness", "ctx prompt")
	svc.mu.Lock()
	if n := len(svc.runs[runID].vibeDeferredFlowStarts); n != 1 {
		svc.mu.Unlock()
		t.Fatalf("setup: deferred start missing (got %d)", n)
	}
	svc.mu.Unlock()

	// Debate concluded: synthesis done, owners settled → restore replays.
	store.seed(runID, []RuntimeWorkflowStep{
		{NodeID: "debate_trigger", Status: StepStatusDone},
		{NodeID: "owner_1", Status: StepStatusDone},
		{NodeID: "owner_2", Status: StepStatusDone},
		{NodeID: "debate_synthesis", Status: StepStatusDone},
	})
	if !svc.restoreVibeFlowAfterDebate(runID) {
		t.Fatal("restoreVibeFlowAfterDebate refused a valid claim")
	}

	waitLoop(t, "deferred flow start drained after restore", 5*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		rs := svc.runs[runID]
		return rs != nil && runHasFlowNode(rs, "test_signatures")
	})
}

// The corrupted live shape (run-139670 durable record): parked ingest
// topology + mounted sprint topology + chatFlowRef=sprint. The claim is
// detached from the live graph — a divert must drop the stale claim and
// park the LIVE sprint, not suppress forever.
func TestBUG594StaleForeignClaimDropsOnDivert(t *testing.T) {
	store := newFakeWorkflowStore()
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)
	runID := "run-bug594-corrupt"
	store.seed(runID, []RuntimeWorkflowStep{
		{NodeID: "coder", Status: StepStatusRunning},
	})
	rs := &interactiveRun{
		id:               runID,
		projectID:        "proj",
		status:           RunStatusRunning,
		agentStatus:      string(RunStatusRunning),
		workingMode:      workingmode.Vibe,
		chatFlowRef:      workingmode.PackPrefix + vibeSprintFlowID, // clobbered onto the sprint
		autoOrchestrate:  true,
		flowEngineDriven: true,
		activeFlowNodes:  bug567SprintGraph(),
		// Stale claim: the ingest graph the dead debate parked before the
		// sprint mount clobbered the overlay.
		vibeParkedNodes: []agentpack.FlowNode{
			{ID: "task_plan_reader", Behavior: "agent.delegate", Agent: "agents/planner.md"},
		},
		vibeParkedFlowRef: workingmode.PackPrefix + "vibe-tasks",
	}
	svc.mu.Lock()
	svc.runs[runID] = rs
	svc.mu.Unlock()

	svc.startVibeOwnerDebate(runID, "run-gated-coder", "gate block: r-tests")

	waitLoop(t, "stale claim dropped; sprint re-parked and debate mounted", 5*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		rs := svc.runs[runID]
		return rs != nil &&
			rs.vibeParkedFlowRef == workingmode.PackPrefix+vibeSprintFlowID &&
			vibeOwnerDebateGraph(rs.activeFlowNodes)
	})
}

// A mount that dies between stash and topology swap (resolve/dispatch
// failure) must release the claim — otherwise the parked claim suppresses
// every divert forever with no debate ever mounted.
func TestBUG594MountDeathReleasesClaim(t *testing.T) {
	store := newFakeWorkflowStore()
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)
	runID := "run-bug594-mountdied"
	rs := &interactiveRun{
		id:               runID,
		projectID:        "proj",
		status:           RunStatusRunning,
		agentStatus:      string(RunStatusRunning),
		workingMode:      workingmode.Vibe,
		chatFlowRef:      workingmode.PackPrefix + vibeSprintFlowID,
		autoOrchestrate:  true,
		flowEngineDriven: true,
		activeFlowNodes:  bug567SprintGraph(),
		// Claimed but the debate never mounted (startResolvedFlow died after
		// the stash): parked topology still holds the sprint, active still
		// shows it, chatFlowRef is not the debate.
		vibeParkedNodes:   bug567SprintGraph(),
		vibeParkedFlowRef: workingmode.PackPrefix + vibeSprintFlowID,
	}
	svc.mu.Lock()
	svc.runs[runID] = rs
	svc.mu.Unlock()

	svc.maybeReleaseVibeDebateClaimIfMountDied(runID)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	got := svc.runs[runID]
	if len(got.vibeParkedNodes) != 0 {
		t.Fatalf("dead claim not released: vibeParkedNodes still %d nodes", len(got.vibeParkedNodes))
	}
}

// A live debate (mounted overlay + claim) must NOT be released by the
// mount-death check — suppression stays suppression.
func TestBUG594LiveDebateClaimNotReleased(t *testing.T) {
	svc, _, runID := bug594SeedClaimedDebate(t)

	svc.maybeReleaseVibeDebateClaimIfMountDied(runID)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	if len(rs.vibeParkedNodes) == 0 {
		t.Fatal("live debate claim released by the mount-death check")
	}
}
