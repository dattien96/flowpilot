package runner

import (
	"context"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// BUG-320: a Claude flow-claude review-loop chat was stopped while its
// reviewer's turn was still starting (live repro run-55348/run-55467 -- see
// BUG-319/CA-418). After BUG-319 tombstoned the never-synced reviewer instead
// of dropping it, the restored chat's step-timeline panel still showed
// "Round 1/3, 3/3 steps" with my-reviewer-claude and synthesis both DONE --
// even though the source machine's own diag log
// (.flowpilot/logs/features/agent-flow-engine/run-55348.ndjson) proved the
// reviewer step actually went RUNNING -> CANCELED -> FAILED and synthesis
// never transitioned at all.
//
// Root cause: resumedFlowStepRows (interactive_resume.go) defaults any
// evidence-less node (an inline hub, or -- pre-tombstone -- a dropped child)
// to DONE whenever `flowComplete` is true. `flowComplete` was derived from
// `resumedFlowRunIncomplete`, which (via terminalFlowLoopStatus) treated a
// STOPPED loop exactly like a genuinely DONE one. Since the hub's own last
// CHAT TURN completed normally (the user's "return (claude-flow-stop)..."
// follow-up got an ordinary reply) even though the FLOW inside it stopped
// mid-round, flowComplete came out true, and every node with no evidence
// (synthesis; the tombstoned reviewer's own step, once matched, is NOT
// affected by this default -- but before the tombstone existed at all it had
// no evidence either) defaulted to DONE.
//
// Fix: resumedFlowStepsComplete (interactive_resume.go) treats "stopped" as
// NOT complete, used by both the evidence-walk default AND the
// transition-log-replay hub-promotion check, so a stopped flow's
// evidence-less steps stay PENDING regardless of whether the run has a local
// step-transition-log sidecar (same-machine restart) or not (Drive restore).

func bug320ReviewLoopNodes() []agentpack.FlowNode {
	return []agentpack.FlowNode{
		{ID: "my-coder", Behavior: "agent.delegate", Agent: "coder-agent"},
		{ID: "my-reviewer-claude", Behavior: "agent.delegate", Agent: "reviewer", DependsOn: []string{"my-coder"}},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "synthesizer", DependsOn: []string{"my-reviewer-claude"}},
	}
}

// TestResumedFlowStepsCompleteStoppedFlowLeavesEvidencelessNodesPending is the
// focused, non-Drive unit proof for P2: a stopped loop whose reviewer child
// genuinely failed must show coder=DONE (real evidence), reviewer=FAILED
// (real evidence), and synthesis=PENDING (no evidence at all, and the loop
// never reached synthesis) -- not the pre-fix all-DONE default.
func TestResumedFlowStepsCompleteStoppedFlowLeavesEvidencelessNodesPending(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	svc := newInteractiveService(newProviderRegistry(), newInteractiveCatalog(), store)
	ctx := context.Background()
	nodes := bug320ReviewLoopNodes()
	now := "2026-07-24T08:12:40Z"
	for _, session := range []ProviderSessionState{
		{
			RunID: "run-coder-320a", ParentRunID: "run-hub-320a", Label: "my-coder",
			ProviderKey: ProviderKeyClaude, RunKind: "chat",
			AgentName: "coder-agent", Role: "coder-agent",
			Status: RunStatusCompleted, StartedAt: now, UpdatedAt: now,
		},
		{
			// BUG-320: this stands in for the tombstone a Drive restore would
			// create -- from resumedFlowStepRows' point of view a tombstone IS
			// just a ProviderSessionState with ParentRunID+Label set, so this unit
			// test exercises the SAME evidence-matching path without going
			// through chat_session_sync.go at all.
			RunID: "run-reviewer-320a", ParentRunID: "run-hub-320a", Label: "my-reviewer-claude",
			ProviderKey: ProviderKeyClaude, RunKind: "chat",
			AgentName: "reviewer", Role: "reviewer",
			Status: RunStatusFailed, StartedAt: now, UpdatedAt: now,
		},
	} {
		if err := store.UpsertProviderSession(ctx, session); err != nil {
			t.Fatalf("UpsertProviderSession(%s): %v", session.RunID, err)
		}
	}

	rs, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID:           "run-hub-320a",
		ProjectID:       "proj",
		ProviderKey:     ProviderKeyClaude,
		WorkflowID:      "wf-320a",
		RunKind:         "workflow",
		Status:          RunStatusCompleted, // the hub's own last chat turn DID complete
		StartedAt:       now,
		UpdatedAt:       now,
		ActiveFlowNodes: nodes,
		AutoOrchestrate: true,
		LoopState:       AgentLoopState{Status: "stopped", Round: 0, Cap: 3, RoundCap: 3, GateReason: "stopped", Mode: "explicit"},
	})
	if apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	_ = rs
	steps, err := svc.workflowStore.LoadRunSteps(ctx, "run-hub-320a")
	if err != nil {
		t.Fatalf("LoadRunSteps: %v", err)
	}
	if len(steps) != len(nodes) {
		t.Fatalf("restored steps = %d, want %d", len(steps), len(nodes))
	}
	if got := flowStepStatus(t, svc, "run-hub-320a", "my-coder"); got != StepStatusDone {
		t.Fatalf("my-coder = %q, want DONE (real evidence)", got)
	}
	if got := flowStepStatus(t, svc, "run-hub-320a", "my-reviewer-claude"); got != StepStatusFailed {
		t.Fatalf("my-reviewer-claude = %q, want FAILED (real evidence) -- BUG-320 repro: this showed DONE pre-fix", got)
	}
	if got := flowStepStatus(t, svc, "run-hub-320a", "synthesis"); got != StepStatusPending {
		t.Fatalf("synthesis = %q, want PENDING (no evidence, flow stopped before it ever ran) -- BUG-320 repro: this showed DONE pre-fix", got)
	}
}

// TestResumedFlowStepsCompleteDoneFlowStillDefaultsEvidencelessToDone is the
// BUG-260 parity guard: P2 must only change the STOPPED case. A genuinely
// DONE flow still defaults its evidence-less inline hub to DONE, and a
// failed cohort member's own real evidence still wins over that default.
func TestResumedFlowStepsCompleteDoneFlowStillDefaultsEvidencelessToDone(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	svc := newInteractiveService(newProviderRegistry(), newInteractiveCatalog(), store)
	ctx := context.Background()
	nodes := bug320ReviewLoopNodes()
	now := "2026-07-24T08:12:40Z"
	for _, session := range []ProviderSessionState{
		{
			RunID: "run-coder-320b", ParentRunID: "run-hub-320b", Label: "my-coder",
			ProviderKey: ProviderKeyClaude, RunKind: "chat",
			AgentName: "coder-agent", Role: "coder-agent",
			Status: RunStatusCompleted, StartedAt: now, UpdatedAt: now,
		},
		{
			// BUG-260: the hub can declare the round done off surviving cohort
			// members even when one member genuinely FAILED -- that member's own
			// real outcome must never be silently promoted to DONE.
			RunID: "run-reviewer-320b", ParentRunID: "run-hub-320b", Label: "my-reviewer-claude",
			ProviderKey: ProviderKeyClaude, RunKind: "chat",
			AgentName: "reviewer", Role: "reviewer",
			Status: RunStatusFailed, StartedAt: now, UpdatedAt: now,
		},
	} {
		if err := store.UpsertProviderSession(ctx, session); err != nil {
			t.Fatalf("UpsertProviderSession(%s): %v", session.RunID, err)
		}
	}

	_, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID:           "run-hub-320b",
		ProjectID:       "proj",
		ProviderKey:     ProviderKeyClaude,
		WorkflowID:      "wf-320b",
		RunKind:         "workflow",
		Status:          RunStatusCompleted,
		StartedAt:       now,
		UpdatedAt:       now,
		ActiveFlowNodes: nodes,
		AutoOrchestrate: true,
		LoopState:       AgentLoopState{Status: "done", Round: 1, Cap: 3, RoundCap: 3, Mode: "explicit"},
	})
	if apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	if got := flowStepStatus(t, svc, "run-hub-320b", "my-coder"); got != StepStatusDone {
		t.Fatalf("my-coder = %q, want DONE", got)
	}
	if got := flowStepStatus(t, svc, "run-hub-320b", "my-reviewer-claude"); got != StepStatusFailed {
		t.Fatalf("my-reviewer-claude = %q, want FAILED -- BUG-260: real evidence must survive a DONE flow", got)
	}
	if got := flowStepStatus(t, svc, "run-hub-320b", "synthesis"); got != StepStatusDone {
		t.Fatalf("synthesis = %q, want DONE -- a genuinely done flow still defaults evidence-less nodes to DONE (P2 must not change this case)", got)
	}
}

// TestResumedFlowStepsCompleteStoppedFlowTransitionLogReplayAgreesWithEvidenceWalk
// proves the SAME predicate now governs the transition-log replay branch
// (same-machine restart, local sidecar present) as the evidence-walk default
// (Drive restore, no sidecar) -- before this fix, a same-machine restart of a
// stopped flow with logged coder/reviewer transitions but no synthesis
// transition promoted synthesis PENDING->DONE via
// `normalizeResumedFlowStatus(st)==Completed` (true for a stopped loop whose
// hub chat turn completed -- BUG-308), while the identical Drive-restored
// copy (no sidecar, evidence-walk only) left it PENDING -- two displays
// disagreeing about the same underlying fact.
func TestResumedFlowStepsCompleteStoppedFlowTransitionLogReplayAgreesWithEvidenceWalk(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	svc := newInteractiveService(newProviderRegistry(), newInteractiveCatalog(), store)
	ctx := context.Background()
	nodes := bug320ReviewLoopNodes()
	now := "2026-07-24T08:12:40Z"
	for _, session := range []ProviderSessionState{
		{
			RunID: "run-coder-320c", ParentRunID: "run-hub-320c", Label: "my-coder",
			ProviderKey: ProviderKeyClaude, RunKind: "chat",
			AgentName: "coder-agent", Role: "coder-agent",
			Status: RunStatusCompleted, StartedAt: now, UpdatedAt: now,
		},
		{
			RunID: "run-reviewer-320c", ParentRunID: "run-hub-320c", Label: "my-reviewer-claude",
			ProviderKey: ProviderKeyClaude, RunKind: "chat",
			AgentName: "reviewer", Role: "reviewer",
			Status: RunStatusFailed, StartedAt: now, UpdatedAt: now,
		},
	} {
		if err := store.UpsertProviderSession(ctx, session); err != nil {
			t.Fatalf("UpsertProviderSession(%s): %v", session.RunID, err)
		}
	}
	// Local step-transition-log sidecar, exactly like a same-machine restart
	// would have (BUG-289/Task-239) -- coder and reviewer transitioned; synthesis
	// never did, because the flow was stopped before it ever ran.
	for _, line := range []stepTransitionLine{
		{RunID: "run-hub-320c", NodeID: "my-coder", Status: "RUNNING", TS: now},
		{RunID: "run-hub-320c", NodeID: "my-coder", Status: "DONE", TS: now},
		// BUG-288 #21: the transition log is full-terminal-monotonic -- once a
		// node logs ANY terminal status (DONE/FAILED/CANCELED) it can never move
		// to a DIFFERENT terminal status afterward (TestStepTransitionReplayKeepsFailedDespiteFlowDone).
		// FAILED is logged directly (no earlier CANCELED) so this test isolates
		// the P2 question -- does synthesis stay PENDING -- without also
		// exercising that separate, pre-existing monotonic rule.
		{RunID: "run-hub-320c", NodeID: "my-reviewer-claude", Status: "RUNNING", TS: now},
		{RunID: "run-hub-320c", NodeID: "my-reviewer-claude", Status: "FAILED", TS: now},
	} {
		if err := store.AppendStepTransition(ctx, "run-hub-320c", line); err != nil {
			t.Fatalf("AppendStepTransition: %v", err)
		}
	}

	_, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID:           "run-hub-320c",
		ProjectID:       "proj",
		ProviderKey:     ProviderKeyClaude,
		WorkflowID:      "wf-320c",
		RunKind:         "workflow",
		Status:          RunStatusCompleted,
		StartedAt:       now,
		UpdatedAt:       now,
		ActiveFlowNodes: nodes,
		AutoOrchestrate: true,
		LoopState:       AgentLoopState{Status: "stopped", Round: 0, Cap: 3, RoundCap: 3, GateReason: "stopped", Mode: "explicit"},
	})
	if apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	if got := flowStepStatus(t, svc, "run-hub-320c", "my-reviewer-claude"); got != StepStatusFailed {
		t.Fatalf("my-reviewer-claude (replay) = %q, want FAILED", got)
	}
	if got := flowStepStatus(t, svc, "run-hub-320c", "synthesis"); got != StepStatusPending {
		t.Fatalf("synthesis (replay) = %q, want PENDING -- same-machine restart (sidecar present) must agree with the Drive-restore evidence-walk default", got)
	}
}

// TestRestoreThenResumeStoppedFlowShowsReviewerFailedAndSynthesisPending is
// the full end-to-end regression proof, tying P1c (tombstone) and P2
// (stopped-aware predicate) together exactly as the live repro hit them:
// sync a stopped hub with a real completed coder and a never-synced (failed)
// reviewer, delete locally, restore from Drive, then actually OPEN the
// restored chat (resumeRun -- the real desktop code path) and check the
// resulting step timeline.
func TestRestoreThenResumeStoppedFlowShowsReviewerFailedAndSynthesisPending(t *testing.T) {
	svc, instance, sourceStore, _, workspace, accountHome := newChatSyncService(t)
	nodes := bug320ReviewLoopNodes()
	parent := ProviderSessionState{
		RunID:             "run-hub-320e2e",
		ProjectID:         "project-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: "thread-hub-320e2e",
		ProviderAccountID: "acct-sync",
		WorkingDirectory:  workspace,
		Status:            RunStatusCompleted,
		RunKind:           "workflow",
		AutoOrchestrate:   true,
		ActiveFlowNodes:   nodes,
		LoopState:         AgentLoopState{Status: "stopped", Round: 0, Cap: 3, RoundCap: 3, GateReason: "stopped", Mode: "explicit"},
	}
	if err := sourceStore.UpsertProviderSession(context.Background(), parent); err != nil {
		t.Fatalf("UpsertProviderSession parent: %v", err)
	}
	coder := seedLocalChatRun(t, sourceStore, accountHome, workspace, "run-coder-320e2e", []byte("coder-session"))
	coder.ParentRunID = parent.RunID
	coder.AgentName = "coder-agent"
	coder.Role = "coder-agent"
	coder.Label = "my-coder"
	coder.Status = RunStatusCompleted
	if err := sourceStore.UpsertProviderSession(context.Background(), coder); err != nil {
		t.Fatalf("UpsertProviderSession coder: %v", err)
	}
	seedUnsyncableChildRun(t, sourceStore, parent.RunID, "run-reviewer-320e2e", "my-reviewer-claude", "reviewer", RunStatusFailed, ProviderKeyCodex)

	synced, apiErr := svc.syncChatRunToDrive(context.Background(), parent.RunID, ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}

	restoredStore, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore restored: %v", err)
	}
	restoredService := NewInteractiveServiceWithStore(DefaultProviderRegistry(), nil, restoredStore)
	restoredService.AttachRunner(instance)
	restoredService.SetActiveAccount("acct-sync")

	restored, apiErr := restoredService.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
		ProjectID:       "project-1",
		SourceMachineID: synced.SourceMachineID,
		SourceRunID:     synced.SourceRunID,
		Cwd:             workspace,
	})
	if apiErr != nil {
		t.Fatalf("restoreChatRunFromDrive() failed: %#v", apiErr)
	}

	// The real desktop code path: open/resume the restored chat (BUG-178's
	// reconstructRun is reached through here when the run is not in memory).
	if _, apiErr := restoredService.resumeRun(restored.RunID); apiErr != nil {
		t.Fatalf("resumeRun() failed: %#v", apiErr)
	}

	if got := flowStepStatus(t, restoredService, restored.RunID, "my-coder"); got != StepStatusDone {
		t.Fatalf("my-coder = %q, want DONE", got)
	}
	if got := flowStepStatus(t, restoredService, restored.RunID, "my-reviewer-claude"); got != StepStatusFailed {
		t.Fatalf("my-reviewer-claude = %q, want FAILED -- live repro showed DONE (\"Round 1/3, 3/3 steps\") before this fix", got)
	}
	if got := flowStepStatus(t, restoredService, restored.RunID, "synthesis"); got != StepStatusPending {
		t.Fatalf("synthesis = %q, want PENDING -- the flow never reached synthesis; live repro showed DONE before this fix", got)
	}
}
