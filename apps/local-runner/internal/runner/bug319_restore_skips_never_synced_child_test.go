package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// BUG-319/BUG-320: a review-loop chat with a reviewer child whose turn was
// interrupted before it ever wrote a real transcript (e.g. the provider's MCP
// connection never became ready) is correctly SKIPPED during sync-up
// (BUG-311's session_unavailable path, syncChatRunToDrive's own best-effort
// "skip child" branch, chat_session_sync.go:920) -- it never gets an index
// row or a manifest blob on Drive at all. Deleting the chat locally and
// restoring it back from Drive used to hard-fail the ENTIRE tree:
// restoreChatRunTreeFromDrive recursed into every entry of
// manifest.ChildAgents unconditionally, and ANY child restore error --
// including "this child was never synced" -- aborted the whole restore, even
// though the hub and every other child had perfectly good, fully-uploaded
// transcripts. Live repro: run-55348 (hub) / run-55467 (reviewer, Claude MCP
// connection timeout, turn_failed with 0 real events) -- confirmed via
// runner.log 2026-07-24 08:12:38-08:24:59.
//
// BUG-319's first cut fully DROPPED the never-synced child from the restored
// tree (no persisted record at all). That reopened a second, live-verified
// defect: the flow step-timeline reconstruction (resumedFlowStepRows,
// interactive_resume.go) only knows a node's real per-run outcome from that
// child's OWN persisted session record -- with none at all, it fell back to
// defaulting every evidence-less node (the dropped reviewer, and any
// inline-hub/synthesis node) to DONE, because the hub's own last CHAT TURN
// had completed normally even though the FLOW inside it was stopped mid-round
// (BUG-320). The tests below assert the superseding TOMBSTONE contract: a
// never-synced child gets a minimal, metadata-only, terminal-status
// ProviderSessionState record (no transcript, SyncStatus=unsyncable) instead
// of vanishing entirely, so the step timeline can tell "this node failed/was
// canceled" apart from "this node has literally no evidence".

// seedUnsyncableChildRun creates a local child ProviderSessionState with no
// resumable session file anywhere on disk, mirroring
// TestBuildChatSessionSyncManifestMissingProviderFile: BuildChatSessionSyncManifest
// returns session_unavailable for it, so syncChatRunToDrive's per-child loop
// logs and skips it -- it is never uploaded, gets no index row and no
// manifest blob. label mirrors the flow node id (e.g. "my-reviewer-claude")
// the way a live spawn sets interactiveRun.label -- matchFlowNodeForSession
// (interactive_resume.go) matches on it before falling back to AgentName/Role.
func seedUnsyncableChildRun(t *testing.T, store *localFileSessionStore, parentRunID, runID, label, agentName string, status RunStatus, provider ProviderKey) ProviderSessionState {
	t.Helper()
	state := ProviderSessionState{
		RunID:             runID,
		ProjectID:         "project-1",
		ProviderKey:       provider,
		ProviderSessionID: "session-missing-" + runID,
		ProviderAccountID: "acct-sync",
		Status:            status,
		RunKind:           "chat",
		ParentRunID:       parentRunID,
		AgentName:         agentName,
		Role:              agentName,
		Label:             label,
		StartedAt:         "2026-07-24T08:12:38Z",
		UpdatedAt:         "2026-07-24T08:12:40Z",
	}
	if err := store.UpsertProviderSession(context.Background(), state); err != nil {
		t.Fatalf("UpsertProviderSession unsyncable child: %v", err)
	}
	return state
}

// findSummaryByLabel is a small test-only lookup used across this file's
// tombstone assertions.
func findSummaryByLabel(t *testing.T, summaries []AgentRunSummary, label string) AgentRunSummary {
	t.Helper()
	for _, s := range summaries {
		if s.Label == label {
			return s
		}
	}
	t.Fatalf("no summary with label %q in %#v", label, summaries)
	return AgentRunSummary{}
}

// TestRestoreParentChatTombstonesNeverSyncedChildRestoresRest is the
// reported-repro regression: hub + a synced coder + a never-synced reviewer.
// Restoring must succeed, restoring the hub and the coder for real, and
// giving the reviewer a terminal-status TOMBSTONE record instead of
// hard-failing the whole tree or leaving it with no trace at all.
func TestRestoreParentChatTombstonesNeverSyncedChildRestoresRest(t *testing.T) {
	svc, instance, sourceStore, _, workspace, accountHome := newChatSyncService(t)
	parent := seedLocalChatRun(t, sourceStore, accountHome, workspace, "run-hub-319", []byte("hub-session"))
	coder := seedLocalChatRun(t, sourceStore, accountHome, workspace, "run-coder-319", []byte("coder-session"))
	coder.ParentRunID = parent.RunID
	coder.AgentName = "coder"
	coder.Role = "coder"
	coder.Label = "my-coder"
	if err := sourceStore.UpsertProviderSession(context.Background(), coder); err != nil {
		t.Fatalf("UpsertProviderSession coder: %v", err)
	}
	seedUnsyncableChildRun(t, sourceStore, parent.RunID, "run-reviewer-319", "my-reviewer-claude", "reviewer", RunStatusFailed, ProviderKeyCodex)

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
		t.Fatalf("BUG-320: restoreChatRunFromDrive() failed = %#v, want success tombstoning the never-synced reviewer", apiErr)
	}

	restartedService := NewInteractiveServiceWithStore(DefaultProviderRegistry(), nil, restoredStore)
	summaries := restartedService.listAgentRunSummaries(restored.RunID)
	if len(summaries) != 2 {
		t.Fatalf("agent summaries = %#v, want exactly 2 (coder + reviewer tombstone)", summaries)
	}
	coderSummary := findSummaryByLabel(t, summaries, "my-coder")
	if coderSummary.AgentName != "coder" {
		t.Fatalf("restored coder = %#v, want AgentName coder", coderSummary)
	}
	reviewerSummary := findSummaryByLabel(t, summaries, "my-reviewer-claude")
	if reviewerSummary.AgentName != "reviewer" {
		t.Fatalf("tombstoned reviewer = %#v, want AgentName reviewer", reviewerSummary)
	}
	if reviewerSummary.Status != RunStatusFailed {
		t.Fatalf("tombstone status = %q, want failed (exact terminal status, not a loose done/failed disjunction)", reviewerSummary.Status)
	}
	if reviewerSummary.ParentRunID != restored.RunID {
		t.Fatalf("tombstone ParentRunID = %q, want %q", reviewerSummary.ParentRunID, restored.RunID)
	}

	sessions, err := restoredStore.ListAllProviderSessions(context.Background())
	if err != nil {
		t.Fatalf("ListAllProviderSessions: %v", err)
	}
	var tombstoneRecord ProviderSessionState
	found := false
	for _, s := range sessions {
		if s.RunID == reviewerSummary.RunID {
			tombstoneRecord = s
			found = true
		}
	}
	if !found {
		t.Fatalf("tombstone record for %q not persisted locally", reviewerSummary.RunID)
	}
	if tombstoneRecord.SyncStatus != "unsyncable" {
		t.Fatalf("tombstone SyncStatus = %q, want unsyncable (BUG-311 semantics -- can never be synced from this machine either)", tombstoneRecord.SyncStatus)
	}
	if tombstoneRecord.ProviderSessionID != "" {
		t.Fatalf("tombstone ProviderSessionID = %q, want empty -- no transcript exists for this child", tombstoneRecord.ProviderSessionID)
	}
	if tombstoneRecord.RestoredFrom != "google_drive" {
		t.Fatalf("tombstone RestoredFrom = %q, want google_drive", tombstoneRecord.RestoredFrom)
	}
	if tombstoneRecord.StartedAt == "" || tombstoneRecord.UpdatedAt == "" {
		t.Fatalf("tombstone timestamps = %#v, want non-empty (ordering + 90-day pruning depend on them)", tombstoneRecord)
	}
}

// TestRestoreParentChatTombstonesAllNeverSyncedChildren covers the
// degraded-input edge where EVERY child is never-synced (e.g. a flow stopped
// before any node produced real output). The hub still restores, and both
// children get terminal tombstones rather than the tree silently ending up
// with zero children.
func TestRestoreParentChatTombstonesAllNeverSyncedChildren(t *testing.T) {
	svc, instance, sourceStore, _, workspace, accountHome := newChatSyncService(t)
	parent := seedLocalChatRun(t, sourceStore, accountHome, workspace, "run-hub-319-allskip", []byte("hub-session"))
	seedUnsyncableChildRun(t, sourceStore, parent.RunID, "run-coder-319-allskip", "my-coder", "coder", RunStatusCancelled, ProviderKeyCodex)
	seedUnsyncableChildRun(t, sourceStore, parent.RunID, "run-reviewer-319-allskip", "my-reviewer-claude", "reviewer", RunStatusFailed, ProviderKeyCodex)

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
		t.Fatalf("BUG-320: restoreChatRunFromDrive() failed = %#v, want success with two tombstones", apiErr)
	}
	summaries := restoredService.listAgentRunSummaries(restored.RunID)
	if len(summaries) != 2 {
		t.Fatalf("agent summaries = %#v, want 2 tombstones (every child was never synced)", summaries)
	}
	coderSummary := findSummaryByLabel(t, summaries, "my-coder")
	if coderSummary.Status != RunStatusCancelled {
		t.Fatalf("coder tombstone status = %q, want cancelled (exact snapshot preserved)", coderSummary.Status)
	}
	reviewerSummary := findSummaryByLabel(t, summaries, "my-reviewer-claude")
	if reviewerSummary.Status != RunStatusFailed {
		t.Fatalf("reviewer tombstone status = %q, want failed (exact snapshot preserved)", reviewerSummary.Status)
	}
}

// TestRestoreParentChatTerminalizesNonTerminalTombstoneStatus proves a
// never-synced child snapshot that was still RUNNING (or any other
// non-terminal status) at sync time collapses to Cancelled on the tombstone
// -- normalizeResumedStatus alone would have preserved waiting_approval/
// waiting_question for real restart gate-rehydration, but a tombstone has no
// synced approval/question record to rehydrate from, so leaving it
// non-terminal would show a permanently "waiting" card nothing can resolve.
func TestRestoreParentChatTerminalizesNonTerminalTombstoneStatus(t *testing.T) {
	svc, instance, sourceStore, _, workspace, accountHome := newChatSyncService(t)
	parent := seedLocalChatRun(t, sourceStore, accountHome, workspace, "run-hub-320-term", []byte("hub-session"))
	seedUnsyncableChildRun(t, sourceStore, parent.RunID, "run-reviewer-320-term", "my-reviewer-claude", "reviewer", RunStatusRunning, ProviderKeyCodex)

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
	summaries := restoredService.listAgentRunSummaries(restored.RunID)
	reviewerSummary := findSummaryByLabel(t, summaries, "my-reviewer-claude")
	if reviewerSummary.Status != RunStatusCancelled {
		t.Fatalf("tombstone status for a RUNNING snapshot = %q, want cancelled (terminalized, not left in-flight)", reviewerSummary.Status)
	}
}

// TestRestoreParentChatRemapsDependsOnToTombstoneID proves a restored
// sibling's DependsOn reference to a tombstoned child's original remote id is
// remapped to the tombstone's actual LOCAL id (which can differ from the raw
// remote id when that raw id collides with an unrelated existing local run --
// resolveRestoredRunID's collision handling, BUG-320), not left dangling.
func TestRestoreParentChatRemapsDependsOnToTombstoneID(t *testing.T) {
	svc, instance, sourceStore, _, workspace, accountHome := newChatSyncService(t)
	parent := seedLocalChatRun(t, sourceStore, accountHome, workspace, "run-hub-319-dep", []byte("hub-session"))
	skippedReviewer := seedUnsyncableChildRun(t, sourceStore, parent.RunID, "run-reviewer-319-dep", "my-reviewer-claude", "reviewer", RunStatusFailed, ProviderKeyCodex)

	reviewerB := seedLocalChatRun(t, sourceStore, accountHome, workspace, "run-reviewerb-319-dep", []byte("reviewerb-session"))
	reviewerB.ParentRunID = parent.RunID
	reviewerB.AgentName = "reviewer-b"
	reviewerB.Role = "reviewer"
	reviewerB.Label = "my-reviewer-b"
	reviewerB.DependsOn = []string{skippedReviewer.RunID}
	if err := sourceStore.UpsertProviderSession(context.Background(), reviewerB); err != nil {
		t.Fatalf("UpsertProviderSession reviewerB: %v", err)
	}

	synced, apiErr := svc.syncChatRunToDrive(context.Background(), parent.RunID, ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}

	restoredStore, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore restored: %v", err)
	}
	// BUG-320: force a collision -- an UNRELATED local run on the restoring
	// machine already happens to use the tombstone's raw source run id, so
	// resolveRestoredRunID must derive a different local id for the tombstone
	// instead of overwriting this record.
	unrelated := ProviderSessionState{
		RunID:       skippedReviewer.RunID,
		ProjectID:   "project-1",
		ProviderKey: ProviderKeyCodex,
		Status:      RunStatusCompleted,
		RunKind:     "chat",
		LastMessage: "unrelated local run, must survive untouched",
	}
	if err := restoredStore.UpsertProviderSession(context.Background(), unrelated); err != nil {
		t.Fatalf("UpsertProviderSession unrelated collider: %v", err)
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
		t.Fatalf("BUG-320: restoreChatRunFromDrive() failed = %#v", apiErr)
	}
	summaries := restoredService.listAgentRunSummaries(restored.RunID)
	if len(summaries) != 2 {
		t.Fatalf("agent summaries = %#v, want exactly 2 (reviewer-b + tombstoned reviewer)", summaries)
	}
	tombstone := findSummaryByLabel(t, summaries, "my-reviewer-claude")
	if tombstone.RunID == skippedReviewer.RunID {
		t.Fatalf("tombstone RunID = %q, want a DERIVED id -- the raw id collides with an unrelated local run", tombstone.RunID)
	}
	reviewerBSummary := findSummaryByLabel(t, summaries, "my-reviewer-b")
	if len(reviewerBSummary.DependsOn) != 1 || reviewerBSummary.DependsOn[0] != tombstone.RunID {
		t.Fatalf("reviewer-b DependsOn = %#v, want remapped to the tombstone's actual local id %q", reviewerBSummary.DependsOn, tombstone.RunID)
	}
	// The unrelated collider must survive completely untouched.
	stillThere, found, err := restoredStore.GetProviderSession(context.Background(), skippedReviewer.RunID)
	if err != nil || !found {
		t.Fatalf("GetProviderSession(unrelated collider): found=%v err=%v", found, err)
	}
	if stillThere.LastMessage != "unrelated local run, must survive untouched" {
		t.Fatalf("unrelated collider was overwritten: %#v", stillThere)
	}
}

// TestRestoreParentChatHardFailsOnTransientDriveErrorInsteadOfTombstoning
// proves a transient Drive lookup failure (network blip, API error) while
// checking whether a child was ever synced is NOT treated the same as
// "confirmed never synced" -- it must hard-fail the whole restore instead of
// silently tombstoning a child that might still have real, recoverable data
// (the exact case the CA-404 guard test below exists to catch).
func TestRestoreParentChatHardFailsOnTransientDriveErrorInsteadOfTombstoning(t *testing.T) {
	svc, instance, sourceStore, _, workspace, accountHome := newChatSyncService(t)
	parent := seedLocalChatRun(t, sourceStore, accountHome, workspace, "run-hub-320-transient", []byte("hub-session"))
	seedUnsyncableChildRun(t, sourceStore, parent.RunID, "run-reviewer-320-transient", "my-reviewer-claude", "reviewer", RunStatusFailed, ProviderKeyCodex)

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

	// Simulate a transient failure specifically for the never-synced child's
	// own manifest-path lookup (the only Drive call BUG-320's
	// chatSessionRemoteRunKnown makes for it) -- everything else (index,
	// parent manifest, coder's real files) goes through untouched.
	original := httpRequestFn
	httpRequestFn = func(ctx context.Context, method, endpoint string, headers map[string]string, body []byte) (int, []byte, error) {
		if strings.Contains(endpoint, "run-reviewer-320-transient") {
			return 0, nil, errors.New("simulated transient drive lookup error")
		}
		return original(ctx, method, endpoint, headers, body)
	}
	t.Cleanup(func() { httpRequestFn = original })

	_, apiErr = restoredService.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
		ProjectID:       "project-1",
		SourceMachineID: synced.SourceMachineID,
		SourceRunID:     synced.SourceRunID,
		Cwd:             workspace,
	})
	if apiErr == nil {
		t.Fatalf("restoreChatRunFromDrive() succeeded, want hard-fail on transient Drive lookup error")
	}
	history, err := restoredStore.ListProviderSessionsByProject(context.Background(), "project-1")
	if err != nil {
		t.Fatalf("ListProviderSessionsByProject: %v", err)
	}
	if len(history) != 0 {
		t.Fatalf("main history = %#v, want empty -- a transient lookup error must not tombstone or restore anything", history)
	}
}

// TestRestoreParentChatStillFailsWhenChildManifestSurvivesWithoutIndexRow is
// the CA-404 regression guard: a child manifest can survive on Drive while its
// index row is lost to a concurrent-sync last-write-wins race (see
// TestSyncChatRunToDriveConcurrentIndexMergesPreserveAllParentRows and
// listRemoteChatSessions' own index-repair logic). That child WAS synced --
// it just isn't currently discoverable via the index -- and must NOT be
// treated the same as "never synced": restore must still hard-fail the
// parent rather than silently tombstoning a child that has real data.
func TestRestoreParentChatStillFailsWhenChildManifestSurvivesWithoutIndexRow(t *testing.T) {
	svc, instance, sourceStore, drive, workspace, accountHome := newChatSyncService(t)
	parent := seedLocalChatRun(t, sourceStore, accountHome, workspace, "run-hub-319-404", []byte("hub-session"))
	seedUnsyncableChildRun(t, sourceStore, parent.RunID, "run-orphan-319-404", "my-reviewer-claude", "reviewer", RunStatusFailed, ProviderKeyCodex)

	synced, apiErr := svc.syncChatRunToDrive(context.Background(), parent.RunID, ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}

	// Plant a manifest blob directly (no index row) for the child -- models the
	// Drive state left behind by a CA-404 index-merge race, whatever its exact
	// cause: real data present, index silent about it.
	plantDriveRunManifest(t, drive, "drive-root", ChatSessionSyncManifest{
		SchemaVersion:   chatSessionManifestSchemaVersion,
		SourceMachineID: synced.SourceMachineID,
		SourceRunID:     "run-orphan-319-404",
		ProjectID:       "project-1",
		ProviderKey:     ProviderKeyCodex,
		RunKind:         "chat",
		Status:          "completed",
		SyncedAt:        "2026-07-24T08:14:35Z",
		UpdatedAt:       "2026-07-24T08:14:35Z",
	})

	restoredStore, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore restored: %v", err)
	}
	restoredService := NewInteractiveServiceWithStore(DefaultProviderRegistry(), nil, restoredStore)
	restoredService.AttachRunner(instance)
	restoredService.SetActiveAccount("acct-sync")

	_, apiErr = restoredService.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
		ProjectID:       "project-1",
		SourceMachineID: synced.SourceMachineID,
		SourceRunID:     synced.SourceRunID,
		Cwd:             workspace,
	})
	if apiErr == nil || apiErr.code != "sync_remote_not_found" {
		t.Fatalf("BUG-319 CA-404 guard: restoreChatRunFromDrive() = %#v, want sync_remote_not_found (must not silently tombstone a child with a surviving manifest)", apiErr)
	}
	history, err := restoredStore.ListProviderSessionsByProject(context.Background(), "project-1")
	if err != nil {
		t.Fatalf("ListProviderSessionsByProject: %v", err)
	}
	if len(history) != 0 {
		t.Fatalf("main history = %#v, want empty until every real child restores", history)
	}
}

// TestRestoreParentChatTombstoneSurvivesServiceRestart proves the tombstone is
// genuinely DURABLE (written to disk, not just held in the restoring
// service's in-memory cache) -- a THIRD, fresh InteractiveService instance
// pointed at the same store (simulating a full runner restart after the
// restore) must still see it.
func TestRestoreParentChatTombstoneSurvivesServiceRestart(t *testing.T) {
	svc, instance, sourceStore, _, workspace, accountHome := newChatSyncService(t)
	parent := seedLocalChatRun(t, sourceStore, accountHome, workspace, "run-hub-320-restart", []byte("hub-session"))
	seedUnsyncableChildRun(t, sourceStore, parent.RunID, "run-reviewer-320-restart", "my-reviewer-claude", "reviewer", RunStatusFailed, ProviderKeyCodex)

	synced, apiErr := svc.syncChatRunToDrive(context.Background(), parent.RunID, ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}
	storeDir := t.TempDir()
	restoredStore, err := NewLocalFileSessionStore(storeDir)
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

	// Simulate a full process restart: brand-new store instance re-reading the
	// SAME on-disk directory, and a brand-new service on top of it.
	reloadedStore, err := NewLocalFileSessionStore(storeDir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore reloaded: %v", err)
	}
	restartedService := NewInteractiveServiceWithStore(DefaultProviderRegistry(), nil, reloadedStore)
	summaries := restartedService.listAgentRunSummaries(restored.RunID)
	if len(summaries) != 1 {
		t.Fatalf("agent summaries after restart = %#v, want 1 durable tombstone", summaries)
	}
	if summaries[0].Status != RunStatusFailed {
		t.Fatalf("tombstone status after restart = %q, want failed", summaries[0].Status)
	}
}

// TestRestoreParentChatSkipsNeverSyncedChildAcrossProviders proves the
// tombstone decision is provider-agnostic (R2): it is made purely from
// SourceMachineID+SourceRunID index/manifest-path membership
// (chatSessionManifestPath has no provider segment), before any
// provider-specific restore code ever runs. Exercised for Codex, Claude, and
// Grok using the same 3-account harness as
// TestRestoreChatRunFromDriveRebuildsTurnLogSidecarAllProviders (BUG-313).
func TestRestoreParentChatSkipsNeverSyncedChildAcrossProviders(t *testing.T) {
	svc, instance, store, _, workspace, accountHome := newChatSyncService(t)

	claudeHome := filepath.Join(filepath.Dir(accountHome), "claude-home")
	if err := os.MkdirAll(filepath.Join(claudeHome, ".claude"), 0o755); err != nil {
		t.Fatalf("MkdirAll claude home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(claudeHome, ".claude", ".credentials.json"), []byte(`{"claudeAiOauth":{"accessToken":"tok"}}`), 0o644); err != nil {
		t.Fatalf("write claude credentials: %v", err)
	}
	grokHome := filepath.Join(filepath.Dir(accountHome), "grok-home")
	if err := os.MkdirAll(grokHome, 0o755); err != nil {
		t.Fatalf("MkdirAll grok home: %v", err)
	}
	if err := os.WriteFile(filepath.Join(grokHome, "auth.json"), []byte(`{"issuer::user":{"refresh_token":"rt","email":"g@example.com"}}`), 0o644); err != nil {
		t.Fatalf("write grok auth: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := instance.saveProviderAccountState(providerAccountState{Accounts: []ProviderAccount{
		{ID: "acct-sync", ProviderKey: "codex", DisplayName: "Account 1", HomePath: accountHome, SlotIndex: 1, IsActive: true, AuthStatus: "connected", CreatedAt: now},
		{ID: "acct-claude", ProviderKey: "claude", DisplayName: "Claude 1", HomePath: claudeHome, SlotIndex: 1, IsActive: true, AuthStatus: "connected", CreatedAt: now},
		{ID: "acct-grok", ProviderKey: "grok", DisplayName: "Grok 1", HomePath: grokHome, SlotIndex: 1, IsActive: true, AuthStatus: "connected", CreatedAt: now},
	}}); err != nil {
		t.Fatalf("saveProviderAccountState: %v", err)
	}

	cases := []struct {
		provider   ProviderKey
		accountID  string
		hubRunID   string
		childRunID string
	}{
		{ProviderKeyCodex, "acct-sync", "run-hub-319-codex", "run-reviewer-319-codex"},
		{ProviderKeyClaude, "acct-claude", "run-hub-319-claude", "run-reviewer-319-claude"},
		{ProviderKeyGrok, "acct-grok", "run-hub-319-grok", "run-reviewer-319-grok"},
	}
	for _, tc := range cases {
		t.Run(string(tc.provider), func(t *testing.T) {
			hub := ProviderSessionState{
				RunID:             tc.hubRunID,
				ProjectID:         "project-1",
				ProviderKey:       tc.provider,
				ProviderSessionID: "thread-1",
				ProviderAccountID: tc.accountID,
				WorkingDirectory:  workspace,
				Status:            RunStatusCompleted,
				RunKind:           "workflow",
			}
			if err := store.UpsertProviderSession(context.Background(), hub); err != nil {
				t.Fatalf("UpsertProviderSession hub: %v", err)
			}
			seedUnsyncableChildRun(t, store, tc.hubRunID, tc.childRunID, "my-reviewer-claude", "reviewer", RunStatusFailed, tc.provider)

			synced, apiErr := svc.syncChatRunToDrive(context.Background(), tc.hubRunID, ChatSessionSyncRequest{})
			if apiErr != nil {
				t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
			}
			restoredStore, err := NewLocalFileSessionStore(t.TempDir())
			if err != nil {
				t.Fatalf("NewLocalFileSessionStore restored: %v", err)
			}
			restoredService := NewInteractiveServiceWithStore(DefaultProviderRegistry(), nil, restoredStore)
			restoredService.AttachRunner(instance)
			restoredService.SetActiveAccount(tc.accountID)

			restored, apiErr := restoredService.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
				ProjectID:       "project-1",
				SourceMachineID: synced.SourceMachineID,
				SourceRunID:     synced.SourceRunID,
				Cwd:             workspace,
			})
			if apiErr != nil {
				t.Fatalf("BUG-320 (%s): restoreChatRunFromDrive() failed = %#v", tc.provider, apiErr)
			}
			summaries := restoredService.listAgentRunSummaries(restored.RunID)
			if len(summaries) != 1 {
				t.Fatalf("BUG-320 (%s): agent summaries = %#v, want 1 tombstone", tc.provider, summaries)
			}
			if summaries[0].Status != RunStatusFailed {
				t.Fatalf("BUG-320 (%s): tombstone status = %q, want failed", tc.provider, summaries[0].Status)
			}
		})
	}
}
