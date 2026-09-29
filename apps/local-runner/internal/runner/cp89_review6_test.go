package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// CP-89 review pass 6 (R6-*): post-R5 deep audit. Two confirmed findings.
//
// ---- R6-1: local-ahead Drive restore must not regress the latch ------------
//
// applyLocalAheadSessionFields keeps live-progress fields (TurnCount,
// LoopState, topology) when the local rollout file is a byte-prefix-extension
// of the synced snapshot, but it does NOT preserve the CP-89 latch + pin
// fields. A chat that synced while pending and then forwarded locally
// (flowArm=started, possibly with a forward-adopted sourceDocID) restores
// from the STALE manifest's pending latch — the next turn can double-launch
// the flow even though the local run already has children.
func TestR6_LocalAheadRestoreKeepsFlowArmAndPin(t *testing.T) {
	svc, _, store, _, workspace, accountHome := newChatSyncService(t)
	seedLocalChatRun(t, store, accountHome, workspace, "run-arm", []byte("line1\n"))

	// The run synced while still pending: manifest carries pending + the pin.
	row, found, err := store.GetProviderSession(context.Background(), "run-arm")
	if err != nil || !found {
		t.Fatalf("GetProviderSession found=%v err=%v", found, err)
	}
	row.FlowArm = "pending"
	row.ChatFlowRef = "flowpilot-core-flow-pack/vibe-cp-ingest"
	row.SourceDocID = "requirements/07-Coding-Plan/CP-89.md"
	row.WorkingMode = "vibe"
	row.ChangeType = "feature"
	row.ChatSubMode = "vibe"
	if err := store.UpsertProviderSession(context.Background(), row); err != nil {
		t.Fatalf("UpsertProviderSession pin: %v", err)
	}
	result, apiErr := svc.syncChatRunToDrive(context.Background(), "run-arm", ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}

	// Local progressed past the snapshot: more rollout turns (file-level
	// localAhead) AND the forward committed — latch started, and the forward
	// turn adopted a corrected source pin.
	targetPath := filepath.Join(accountHome, "sessions", "2026", "06", "17", "rollout-local-session-run-arm.jsonl")
	if err := os.WriteFile(targetPath, []byte("line1\nline2\n"), 0o644); err != nil {
		t.Fatalf("WriteFile longer local: %v", err)
	}
	row, found, err = store.GetProviderSession(context.Background(), "run-arm")
	if err != nil || !found {
		t.Fatalf("GetProviderSession found=%v err=%v", found, err)
	}
	row.FlowArm = "started"
	row.SourceDocID = "requirements/07-Coding-Plan/CP-90.md"
	if err := store.UpsertProviderSession(context.Background(), row); err != nil {
		t.Fatalf("UpsertProviderSession newer local: %v", err)
	}

	// A re-restore (re-clicked Remote Chats / batch restore) of the same chat
	// must not roll the latch back to the stale manifest's pending.
	restored, apiErr := svc.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
		ProjectID:       "project-1",
		SourceMachineID: result.SourceMachineID,
		SourceRunID:     result.SourceRunID,
		Cwd:             workspace,
	})
	if apiErr != nil {
		t.Fatalf("restoreChatRunFromDrive() failed: %v", apiErr)
	}
	persisted, found, err := store.GetProviderSession(context.Background(), restored.RunID)
	if err != nil || !found {
		t.Fatalf("GetProviderSession(%q) found=%v err=%v", restored.RunID, found, err)
	}
	if persisted.FlowArm != "started" {
		t.Fatalf("local started latch regressed to stale manifest value: got %q, want started — a re-forward would double-launch", persisted.FlowArm)
	}
	if persisted.SourceDocID != "requirements/07-Coding-Plan/CP-90.md" {
		t.Fatalf("forward-adopted source pin regressed to stale manifest value: got %q", persisted.SourceDocID)
	}
}

// ---- R6-2: transition evidence without a child must not wedge --------------
//
// R5-5 made step-transition/step-row evidence keep the latch started. That is
// backwards for the zero-children case: a child row IS the launch commit (the
// spawn persists it synchronously), so "started + topology + transitions +
// NO child" means the executor engaged (inline dispatch wrote DONE before the
// delegate spawn, or RUNNING before the spawn commit) but produced nothing
// resumable. Keeping started wedges the run: nothing re-spawns the entry and
// forward retry answers flow_already_started. The durable contract is
// safely-retryable: heal to pending so the forward retry relaunches; the
// inline dispatch it re-runs is deterministic context production.
func TestR6_TransitionEvidenceWithoutChildHealsPending(t *testing.T) {
	store := &r5TransitionStore{fakeWorkflowStore: newFakeWorkflowStore()}
	svc := task451ServiceWithStore(t, store)
	// Executor progress the crash left behind: the entry node transitioned
	// RUNNING, but the child row is absent (kill between the transition and
	// the spawn commit).
	store.lines = map[string][]stepTransitionLine{
		"run-r6s": {{RunID: "run-r6s", NodeID: "n1", Status: string(StepStatusRunning), TS: "2026-01-01T00:00:00Z"}},
	}
	rs, err := svc.reconstructRun(ProviderSessionState{
		RunID: "run-r6s", ProjectID: "proj", RunKind: "chat",
		ProviderKey: ProviderKeyCodex, Status: RunStatusCompleted,
		ProviderAccountID: "default",
		ChatFlowRef:     "task-harness",
		FlowArm:         "started", TurnCount: 1,
		ActiveFlowNodes: []agentpack.FlowNode{{ID: "n1"}},
	})
	if err != nil {
		t.Fatalf("reconstructRun: %v", err)
	}
	if rs.flowArm != FlowArmPending {
		t.Fatalf("transition evidence with NO child row must heal to pending — staying started wedges with nothing to resume, arm=%q", rs.flowArm)
	}
	// And the healed run must actually be retryable: forward relaunches.
	if _, e := svc.startTurn(rs.id, TurnInput{StepID: "chat", ForwardFlow: true, Prompt: "go"}, "", ""); e != nil {
		t.Fatalf("forward retry on the healed run must succeed, got %v", e)
	}
	svc.mu.Lock()
	arm := svc.runs[rs.id].flowArm
	svc.mu.Unlock()
	if arm != FlowArmStarted {
		t.Fatalf("forward retry must flip healed pending→started, arm=%q", arm)
	}
}
