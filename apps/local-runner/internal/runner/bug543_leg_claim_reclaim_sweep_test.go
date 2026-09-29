package runner

import (
	"context"
	"testing"
	"time"
)

// BUG-543 residual: cancelled/completed children of a sealed (stopped/done)
// or deleted parent keep leg_state=active forever — durable garbage claims a
// live leg cannot reuse and reconstruction surfaces as stale residue. The
// sweep is conservative by design (see BUG-543 doc): the leg claim is a
// re-drivable provider pin, so a claim may only be reclaimed when the row is
// terminal, carries no armed intents or pending cards, is older than the
// reclaim window, and the parent loop is provably sealed (stopped/done) or
// gone. Every reclaim emits a durable leg_claim_reclaimed audit event.
func bug543ChildRow(runID, parentID string, status RunStatus, ageHours float64) ProviderSessionState {
	return ProviderSessionState{
		RunID:       runID,
		ProjectID:   "proj",
		ParentRunID: parentID,
		ProviderKey: ProviderKeyCodex,
		Status:      status,
		LegState:    LegStateActive,
		StartedAt:   time.Now().Add(-72 * time.Hour).UTC().Format(time.RFC3339Nano),
		UpdatedAt:   time.Now().Add(-time.Duration(ageHours * float64(time.Hour))).UTC().Format(time.RFC3339Nano),
	}
}

func bug543ParentRow(parentID string, loopStatus string) ProviderSessionState {
	return ProviderSessionState{
		RunID:       parentID,
		ProjectID:   "proj",
		ProviderKey: ProviderKeyCodex,
		Status:      RunStatusCancelled,
		LegState:    LegStateActive,
		StartedAt:   time.Now().Add(-72 * time.Hour).UTC().Format(time.RFC3339Nano),
		UpdatedAt:   time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339Nano),
		LoopState:   AgentLoopState{Status: loopStatus},
	}
}

func bug543Store(t *testing.T, svc *InteractiveService) *fakeWorkflowStore {
	t.Helper()
	store, ok := svc.workflowStore.(*fakeWorkflowStore)
	if !ok {
		t.Fatalf("workflowStore is %T, want *fakeWorkflowStore", svc.workflowStore)
	}
	return store
}

func bug543LegOf(t *testing.T, store *fakeWorkflowStore, runID string) ProviderSessionState {
	t.Helper()
	row, found, err := store.GetProviderSession(context.Background(), runID)
	if err != nil || !found {
		t.Fatalf("GetProviderSession(%s): found=%v err=%v", runID, found, err)
	}
	return row
}

func bug543AuditEvents(store *fakeWorkflowStore, runID string) []ProviderEvent {
	store.mu.Lock()
	defer store.mu.Unlock()
	var out []ProviderEvent
	for _, ev := range store.events[runID] {
		if ev.Type == EventLegClaimReclaimed {
			out = append(out, ev)
		}
	}
	return out
}

// The headline case: a stale cancelled child whose parent loop is sealed
// stopped will never be re-driven — its leg claim is durable garbage and the
// sweep must close it with an auditable reason.
func TestBug543_SweepReclaimsStaleClaimOfSealedParent(t *testing.T) {
	svc, _ := clusterFService(t)
	store := bug543Store(t, svc)
	ctx := context.Background()

	if err := store.UpsertProviderSession(ctx, bug543ParentRow("run-parent", "stopped")); err != nil {
		t.Fatalf("upsert parent: %v", err)
	}
	if err := store.UpsertProviderSession(ctx, bug543ChildRow("run-child", "run-parent", RunStatusCancelled, 48)); err != nil {
		t.Fatalf("upsert child: %v", err)
	}

	svc.sweepStaleLegClaims(ctx)

	row := bug543LegOf(t, store, "run-child")
	if row.LegState != LegStateClosed || row.LegClosedReason != LegClosedReasonReclaimed {
		t.Fatalf("stale claim under sealed parent must be reclaimed: got leg_state=%q reason=%q", row.LegState, row.LegClosedReason)
	}
	if evs := bug543AuditEvents(store, "run-child"); len(evs) != 1 {
		t.Fatalf("expected exactly one durable leg_claim_reclaimed audit event, got %d", len(evs))
	}
}

// Same child, parent loop still running — the claim is live contract (review
// retry edges / orphan-cure can re-drive the child on this leg).
func TestBug543_SweepKeepsClaimOfLiveParent(t *testing.T) {
	svc, _ := clusterFService(t)
	store := bug543Store(t, svc)
	ctx := context.Background()

	if err := store.UpsertProviderSession(ctx, bug543ParentRow("run-parent", "running")); err != nil {
		t.Fatalf("upsert parent: %v", err)
	}
	if err := store.UpsertProviderSession(ctx, bug543ChildRow("run-child", "run-parent", RunStatusCancelled, 48)); err != nil {
		t.Fatalf("upsert child: %v", err)
	}

	svc.sweepStaleLegClaims(ctx)

	row := bug543LegOf(t, store, "run-child")
	if row.LegState != LegStateActive {
		t.Fatalf("claim under a running parent must survive, got leg_state=%q", row.LegState)
	}
}

// A sealed parent alone is not proof — the row must also be older than the
// reclaim window so a just-stopped loop's children survive for resume.
func TestBug543_SweepKeepsFreshClaim(t *testing.T) {
	svc, _ := clusterFService(t)
	store := bug543Store(t, svc)
	ctx := context.Background()

	if err := store.UpsertProviderSession(ctx, bug543ParentRow("run-parent", "stopped")); err != nil {
		t.Fatalf("upsert parent: %v", err)
	}
	if err := store.UpsertProviderSession(ctx, bug543ChildRow("run-child", "run-parent", RunStatusCancelled, 0.1)); err != nil {
		t.Fatalf("upsert child: %v", err)
	}

	svc.sweepStaleLegClaims(ctx)

	row := bug543LegOf(t, store, "run-child")
	if row.LegState != LegStateActive {
		t.Fatalf("fresh claim must survive even under a sealed parent, got leg_state=%q", row.LegState)
	}
}

// An armed durable intent is an explicit re-drive promise — never reclaim it.
func TestBug543_SweepKeepsArmedIntent(t *testing.T) {
	svc, _ := clusterFService(t)
	store := bug543Store(t, svc)
	ctx := context.Background()

	if err := store.UpsertProviderSession(ctx, bug543ParentRow("run-parent", "stopped")); err != nil {
		t.Fatalf("upsert parent: %v", err)
	}
	child := bug543ChildRow("run-child", "run-parent", RunStatusCancelled, 48)
	child.PendingResumePrompt = "retry me"
	if err := store.UpsertProviderSession(ctx, child); err != nil {
		t.Fatalf("upsert child: %v", err)
	}

	svc.sweepStaleLegClaims(ctx)

	row := bug543LegOf(t, store, "run-child")
	if row.LegState != LegStateActive {
		t.Fatalf("claim with an armed resume intent must survive, got leg_state=%q", row.LegState)
	}
}

// A pending approval card is a re-drive surface (orphan-cure revives armed
// children) — conservatively keep the claim while the card is unresolved.
func TestBug543_SweepKeepsPendingCard(t *testing.T) {
	svc, _ := clusterFService(t)
	store := bug543Store(t, svc)
	ctx := context.Background()

	if err := store.UpsertProviderSession(ctx, bug543ParentRow("run-parent", "stopped")); err != nil {
		t.Fatalf("upsert parent: %v", err)
	}
	if err := store.UpsertProviderSession(ctx, bug543ChildRow("run-child", "run-parent", RunStatusCancelled, 48)); err != nil {
		t.Fatalf("upsert child: %v", err)
	}
	if err := store.UpsertApproval(ctx, ProviderApprovalState{ApprovalID: "ap-1", RunID: "run-child", Status: "pending"}); err != nil {
		t.Fatalf("upsert approval: %v", err)
	}

	svc.sweepStaleLegClaims(ctx)

	row := bug543LegOf(t, store, "run-child")
	if row.LegState != LegStateActive {
		t.Fatalf("claim with a pending approval card must survive, got leg_state=%q", row.LegState)
	}
}

// Deleted/corrupt parent: the durable row is gone, so nothing can ever
// re-drive the child — the claim is orphan garbage even without a loop row.
func TestBug543_SweepReclaimsOrphanedClaim(t *testing.T) {
	svc, _ := clusterFService(t)
	store := bug543Store(t, svc)
	ctx := context.Background()

	if err := store.UpsertProviderSession(ctx, bug543ChildRow("run-child", "run-parent-gone", RunStatusCompleted, 48)); err != nil {
		t.Fatalf("upsert child: %v", err)
	}

	svc.sweepStaleLegClaims(ctx)

	row := bug543LegOf(t, store, "run-child")
	if row.LegState != LegStateClosed || row.LegClosedReason != LegClosedReasonReclaimed {
		t.Fatalf("orphaned stale claim must be reclaimed: got leg_state=%q reason=%q", row.LegState, row.LegClosedReason)
	}
}

// A resident in-memory run matching the predicate gets the same treatment:
// claim closed in memory, durable row flipped, audit event emitted.
func TestBug543_SweepReclaimsResidentClaim(t *testing.T) {
	svc, _ := clusterFService(t)
	store := bug543Store(t, svc)
	ctx := context.Background()

	if err := store.UpsertProviderSession(ctx, bug543ParentRow("run-parent", "stopped")); err != nil {
		t.Fatalf("upsert parent: %v", err)
	}
	handle, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if apiErr != nil {
		t.Fatalf("createRun child: %v", apiErr)
	}
	svc.mu.Lock()
	child := svc.runs[handle.RunID]
	child.parentRunID = "run-parent"
	child.legState = LegStateActive
	child.status = RunStatusCancelled
	child.agentStatus = string(RunStatusCancelled)
	child.updatedAt = time.Now().Add(-48 * time.Hour).UTC().Format(time.RFC3339Nano)
	snap := sessionStateOf(child)
	svc.mu.Unlock()
	if err := store.UpsertProviderSession(ctx, snap); err != nil {
		t.Fatalf("upsert child: %v", err)
	}

	svc.sweepStaleLegClaims(ctx)

	svc.mu.Lock()
	legState, reason := child.legState, child.legClosedReason
	svc.mu.Unlock()
	if legState != LegStateClosed || reason != LegClosedReasonReclaimed {
		t.Fatalf("resident stale claim must be reclaimed: got leg_state=%q reason=%q", legState, reason)
	}
	if row := bug543LegOf(t, store, handle.RunID); row.LegState != LegStateClosed {
		t.Fatalf("resident reclaim must persist: durable leg_state=%q", row.LegState)
	}
}

// Remote-origin rows describe another machine's legs — reclaiming here would
// poison a leg the source machine may still re-drive. Never touch them.
func TestBug543_SweepKeepsRemoteOriginClaim(t *testing.T) {
	svc, _ := clusterFService(t)
	store := bug543Store(t, svc)
	ctx := context.Background()

	if err := store.UpsertProviderSession(ctx, bug543ParentRow("run-parent", "stopped")); err != nil {
		t.Fatalf("upsert parent: %v", err)
	}
	child := bug543ChildRow("run-child", "run-parent", RunStatusCancelled, 48)
	child.SourceMachineID = "mch_other"
	if err := store.UpsertProviderSession(ctx, child); err != nil {
		t.Fatalf("upsert child: %v", err)
	}

	svc.sweepStaleLegClaims(ctx)

	row := bug543LegOf(t, store, "run-child")
	if row.LegState != LegStateActive {
		t.Fatalf("remote-origin claim must survive — the source machine owns the leg, got leg_state=%q", row.LegState)
	}
}

// A terminal parent with no live loop is sealed: nothing remains that could
// re-drive its children. Covers parents whose LoopState was never populated.
func TestBug543_SweepReclaimsClaimOfTerminalLooplessParent(t *testing.T) {
	svc, _ := clusterFService(t)
	store := bug543Store(t, svc)
	ctx := context.Background()

	parent := bug543ParentRow("run-parent", "")
	parent.Status = RunStatusCancelled
	if err := store.UpsertProviderSession(ctx, parent); err != nil {
		t.Fatalf("upsert parent: %v", err)
	}
	if err := store.UpsertProviderSession(ctx, bug543ChildRow("run-child", "run-parent", RunStatusCancelled, 48)); err != nil {
		t.Fatalf("upsert child: %v", err)
	}

	svc.sweepStaleLegClaims(ctx)

	row := bug543LegOf(t, store, "run-child")
	if row.LegState != LegStateClosed {
		t.Fatalf("claim under a terminal loopless parent must be reclaimed, got leg_state=%q", row.LegState)
	}
}
