package runner

import (
	"context"
	"testing"
)

// Live run-945 / run-400 class: a leg closed by provider switch / quota stop /
// worktree sweep kept its durable armed state — pending_resume_*,
// pending_gate_reprompt_*, pendingFlowGateSettle, waiting_user_approval — so
// the row still reads actionable. Worse, reconstructPendingChildSessions'
// needSessionWithIntent and rehydratePendingGatesLocked do not filter
// legState, so a closed-but-armed row RESURRECTS on parent resume and
// re-drives intent the closed leg can never flush.
//
// Fix contract: closing a leg clears transient pending fields (gen high-water
// marks and legClosedReason stay for audit), waiting-* status collapses to
// cancelled, and the read paths never re-drive or rehydrate a closed leg.

func findSession(t *testing.T, svc *InteractiveService, runID string) ProviderSessionState {
	t.Helper()
	idx, ok := svc.workflowStore.(SessionIndexReader)
	if !ok {
		t.Fatal("store lacks SessionIndexReader")
	}
	sessions, err := idx.ListAllProviderSessions(context.Background())
	if err != nil {
		t.Fatalf("ListAllProviderSessions: %v", err)
	}
	for _, s := range sessions {
		if s.RunID == runID {
			return s
		}
	}
	t.Fatalf("session %q not found", runID)
	return ProviderSessionState{}
}

// Mutation side: closeLegsBoundToWorktree must drop armed residue on BOTH the
// resident (in-memory) leg and the durable-only row sweep.
func TestClosedLegClearsArmedPendingStateOnClose(t *testing.T) {
	svc, _ := newTestServer(t)
	dir := t.TempDir()
	ctx := context.Background()

	// Resident leg armed with durable intents + a waiting card status.
	resident := &interactiveRun{
		id:                        "leg-resident",
		chatID:                    "cht-x",
		projectID:                 "proj-x",
		providerKey:               ProviderKeyCodex,
		status:                    RunStatusWaitingUserApr,
		agentStatus:               string(RunStatusWaitingUserApr),
		workspaceCwd:              dir,
		legState:                  LegStateActive,
		pendingResumePrompt:       "dispatch after unblock",
		pendingResumeStepID:       "chat-leg-resident",
		pendingResumeGen:          3,
		pendingResumeDeliveredGen: 2,
		pendingResumeApprovalID:   "appr-1",
		pendingResumeDecision:     "approve",
		pendingGateRepromptPrompt: "reprompt the gate",
		pendingGateRepromptStepID: "chat-leg-resident",
		pendingGateRepromptGen:    4,
		pendingFlowGateSettle:     true,
		pendingFlowGateFinalMsg:   "settle msg",
		pendingTurnPrompt:         "buffered",
		pendingApprovalID:         "appr-live",
	}
	svc.mu.Lock()
	svc.runs[resident.id] = resident
	svc.mu.Unlock()

	// Durable-only sibling row (dead run not resident in memory).
	if err := svc.workflowStore.(InteractiveStateStore).UpsertProviderSession(ctx, ProviderSessionState{
		RunID: "leg-nonresident", ChatID: "cht-x", ProjectID: "proj-x",
		ProviderKey: ProviderKeyCodex,
		Status:      RunStatusWaitingApproval, AgentStatus: string(RunStatusWaitingApproval),
		WorkingDirectory: dir, LegState: LegStateActive,
		PendingResumePrompt: "armed", PendingResumeStepID: "chat-leg-nonresident",
		PendingResumeGen: 5, PendingFlowGateSettle: true,
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	svc.closeLegsBoundToWorktree(dir)

	for _, runID := range []string{"leg-resident", "leg-nonresident"} {
		row := findSession(t, svc, runID)
		if row.LegState != LegStateClosed || row.LegClosedReason != LegClosedReasonWorktreeSwept {
			t.Fatalf("%s leg close fields = %q/%q, want closed/%s", runID, row.LegState, row.LegClosedReason, LegClosedReasonWorktreeSwept)
		}
		if row.PendingResumePrompt != "" || row.PendingResumeStepID != "" ||
			row.PendingGateRepromptPrompt != "" || row.PendingGateRepromptStepID != "" {
			t.Fatalf("%s kept armed intent fields: %+v", runID, row)
		}
		if row.PendingFlowGateSettle || row.PendingFlowGateFinalMsg != "" {
			t.Fatalf("%s kept pending flow-gate settle", runID)
		}
		if row.Status == RunStatusWaitingApproval || row.Status == RunStatusWaitingQuestion || row.Status == RunStatusWaitingUserApr {
			t.Fatalf("%s Status = %q, want non-waiting on a closed leg", runID, row.Status)
		}
		// Audit/idempotency fields must survive.
		if row.PendingResumeGen == 0 {
			t.Fatalf("%s lost PendingResumeGen high-water mark", runID)
		}
	}
}

// Read side: parent resume must not reconstruct a closed-but-armed child row —
// needSessionWithIntent never consulted legState.
func TestReconstructSkipsClosedLegWithArmedIntent(t *testing.T) {
	svc, _ := newTestServer(t)
	ctx := context.Background()
	if err := svc.workflowStore.(InteractiveStateStore).UpsertProviderSession(ctx, ProviderSessionState{
		RunID: "child-closed-armed", ParentRunID: "parent-x",
		ProjectID: "proj-x", ProviderKey: ProviderKeyCodex,
		Status: RunStatusRunning, LegState: LegStateClosed,
		LegClosedReason: LegClosedReasonProviderSwitch,
		PendingResumePrompt: "armed", PendingResumeStepID: "s", PendingResumeGen: 2,
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}
	if err := svc.reconstructPendingChildSessions("parent-x"); err != nil {
		t.Fatalf("reconstructPendingChildSessions: %v", err)
	}
	svc.mu.Lock()
	got := svc.runs["child-closed-armed"]
	svc.mu.Unlock()
	if got != nil {
		t.Fatal("closed leg resurrected into s.runs — closed legs never re-drive")
	}
}

// Projection honesty for pre-fix stale rows already on disk: a closed leg with
// armed fields + waiting status must read terminal, not armed/actionable.
func TestClosedLegProjectsTerminalNotArmed(t *testing.T) {
	svc, _ := newTestServer(t)
	row := ProviderSessionState{
		RunID: "child-closed-waiting", ParentRunID: "parent-y",
		ProjectID: "proj-y", ProviderKey: ProviderKeyCodex,
		Status: RunStatusWaitingUserApr, AgentStatus: string(RunStatusWaitingUserApr),
		LegState:            LegStateClosed,
		LegClosedReason:     LegClosedReasonProviderSwitch,
		PendingResumePrompt: "armed", PendingResumeStepID: "s", PendingResumeGen: 9,
	}
	if err := svc.workflowStore.(InteractiveStateStore).UpsertProviderSession(context.Background(), row); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}
	summaries, err := svc.listAgentRunSummaries("parent-y")
	if err != nil {
		t.Fatalf("listAgentRunSummaries: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("summaries = %d, want 1", len(summaries))
	}
	if summaries[0].Status == RunStatusRunning || summaries[0].Status == RunStatusWaitingUserApr {
		t.Fatalf("Status = %q, want terminal (closed leg armed residue must not render running/waiting)", summaries[0].Status)
	}
	if step := resumedChildRunStepStatus(row); step != StepStatusCanceled {
		t.Fatalf("resumedChildRunStepStatus = %q, want %q for a closed leg", step, StepStatusCanceled)
	}
}

// A closed leg's durable pending cards must not rehydrate as actionable —
// the card died with the leg.
func TestClosedLegDoesNotRehydratePendingCards(t *testing.T) {
	svc, _ := newTestServer(t)
	ctx := context.Background()
	runID := "leg-closed-card"
	if err := svc.workflowStore.(InteractiveStateStore).UpsertApproval(ctx, ProviderApprovalState{
		ApprovalID: "appr-dead", RunID: runID, Status: "pending",
		Command: "rm -rf /tmp/x",
	}); err != nil {
		t.Fatalf("UpsertApproval: %v", err)
	}
	rs := &interactiveRun{
		id: runID, status: RunStatusWaitingApproval, agentStatus: string(RunStatusWaitingApproval),
		legState: LegStateClosed, legClosedReason: LegClosedReasonProviderSwitch,
	}
	svc.mu.Lock()
	svc.runs[runID] = rs
	svc.rehydratePendingGatesLocked(runID)
	got := svc.approvals["appr-dead"]
	svc.mu.Unlock()
	if got != nil {
		t.Fatalf("closed leg rehydrated an actionable card: %+v", got.details)
	}
}
