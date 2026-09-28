package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// CP-89 review pass 7 (R7-*): post-R6 deep audit.
//
// ---- R7-1: byte-identical provider file must not disable the local merge ----
//
// R6-1 gated the local-ahead merge on the rollout FILE extending the synced
// snapshot. A CP-89 forward turn is synthetic: it commits durable latch state
// (flowArm=started, adopted sourceDocID, TurnCount++) without appending a
// single provider byte. A chat that synced while pending and then forwarded
// locally therefore re-restores with a byte-IDENTICAL provider file —
// localAhead stays false, the merge never runs, and the stale manifest's
// pending latch is persisted over the local started latch. The next forward
// double-launches a flow that already has children.
//
// resolveRestoredRunID only maps a manifest onto an existing local row when
// the row's (SourceMachineID, SourceRunID) match the manifest's — i.e. the
// row is this machine's own lineage. For that self-re-restore the local
// record is never behind the manifest (syncs snapshot local state; local
// only advances afterward), so the mutable-field merge must run whether or
// not the provider file changed.
func TestR7_SameFileReRestoreKeepsLocalAheadFlowArm(t *testing.T) {
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

	// Local forwarded AFTER the snapshot — a synthetic turn that appends NO
	// provider bytes, so the rollout file stays byte-identical to the
	// manifest's (the R6-1 file-extension localAhead signal never fires).
	row, found, err = store.GetProviderSession(context.Background(), "run-arm")
	if err != nil || !found {
		t.Fatalf("GetProviderSession found=%v err=%v", found, err)
	}
	row.FlowArm = "started"
	row.SourceDocID = "requirements/07-Coding-Plan/CP-90.md"
	row.TurnCount++
	if err := store.UpsertProviderSession(context.Background(), row); err != nil {
		t.Fatalf("UpsertProviderSession newer local: %v", err)
	}

	// A re-restore of the same machine's own snapshot must keep the local
	// latch — the local row can only be AHEAD of a manifest this machine
	// wrote, and identical provider bytes do not mean identical local state.
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
		t.Fatalf("byte-identical re-restore regressed the latch: got %q, want started — the next forward would double-launch a flow that already has children", persisted.FlowArm)
	}
	if persisted.SourceDocID != "requirements/07-Coding-Plan/CP-90.md" {
		t.Fatalf("forward-adopted source pin regressed to stale manifest value: got %q", persisted.SourceDocID)
	}
	if persisted.WorkingMode != "vibe" {
		t.Fatalf("working mode regressed to stale manifest value: got %q", persisted.WorkingMode)
	}
}

// ---- R7-2: a minted-but-never-engaged child row is not launch proof --------
//
// spawnChildRun persists the child row BEFORE scheduling the child's first
// turn goroutine. A kill in that window leaves a durable row with
// status=idle, TurnCount=0, agentStatus=spawned and no durable intent — the
// launch never produced resumable work. flowEntryChildExists counted the bare
// row as evidence, so the parent stays "started" forever: reconstructPending
// ChildSessions only re-drives children carrying pending gate/cohort/
// continuation markers, so nothing ever runs the child's first turn and
// forward retry answers flow_already_started. Heal to pending instead: the
// retry re-packs and re-spawns a fresh entry child.
func TestR7_IdleEntryChildRowDoesNotProveLaunch(t *testing.T) {
	store := newFakeWorkflowStore()
	svc := task451ServiceWithStore(t, store)
	// Kill between the row commit and the first-turn goroutine: the row
	// exists but carries no engagement evidence whatsoever.
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: "run-c-entry", ProjectID: "proj", ParentRunID: "run-i1",
		Label: "n1", Status: RunStatusIdle, AgentStatus: "spawned",
	}); err != nil {
		t.Fatal(err)
	}
	rs, err := svc.reconstructRun(ProviderSessionState{
		RunID: "run-i1", ProjectID: "proj", RunKind: "chat",
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
		t.Fatalf("a minted idle child row is not launch proof — nothing ever ran; arm=%q must heal to pending for a safe retry", rs.flowArm)
	}
	// The heal must leave the run retryable.
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

// Control: a dependency-parked child IS a real launch — the flow deliberately
// holds it (agentStatus=waiting_dependency + durable DependsOn) until sibling
// dependencies settle. Healing the parent would double-spawn it on retry.
func TestR7_WaitingDependencyChildKeepsStarted(t *testing.T) {
	store := newFakeWorkflowStore()
	svc := task451ServiceWithStore(t, store)
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: "run-c-dep", ProjectID: "proj", ParentRunID: "run-d1",
		Label: "n1", Status: RunStatusIdle, AgentStatus: "waiting_dependency",
		DependsOn: []string{"n0"},
	}); err != nil {
		t.Fatal(err)
	}
	rs, err := svc.reconstructRun(ProviderSessionState{
		RunID: "run-d1", ProjectID: "proj", RunKind: "chat",
		ProviderKey: ProviderKeyCodex, Status: RunStatusCompleted,
		ProviderAccountID: "default",
		ChatFlowRef:     "task-harness",
		FlowArm:         "started", TurnCount: 1,
		ActiveFlowNodes: []agentpack.FlowNode{{ID: "n1"}},
	})
	if err != nil {
		t.Fatalf("reconstructRun: %v", err)
	}
	if rs.flowArm != FlowArmStarted {
		t.Fatalf("a dependency-parked child is an engaged launch — arm=%q must stay started", rs.flowArm)
	}
}
