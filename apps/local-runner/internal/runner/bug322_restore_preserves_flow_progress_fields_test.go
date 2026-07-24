package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// BUG-322 (CP-51 C6 gap): BUG-091's localAhead branch in restoreChatRunFromDrive
// only preserves LastPrompt/LastMessage/Status/UpdatedAt from the local session
// when the local Codex rollout file is a byte-prefix-extension of the restored
// remote snapshot (i.e. local genuinely progressed further since the last
// sync-up). Fields added to ProviderSessionState later -- TurnCount (BUG-315),
// LoopState, ActiveFlowNodes/ActiveFlowEdges, AgentStatus, DependsOn,
// PendingAgentContext, FlowCohortID, ChatSubMode, ChatFlowRef -- are set
// unconditionally from the (older) remote manifest regardless of localAhead,
// so a stale restore silently regresses genuine local flow progress. This is
// the CP-51 companion doc's own C6 "no empty-overwrite of newer local" anti-
// regression property, narrowed to the flow-runtime fields BUG-091 predates.
//
// TestRestoreChatRunFromDrivePreservesFlowProgressFieldsWhenLocalAhead is RED
// before the fix: it reproduces exactly BUG-315's own scenario (turnCount
// regression -> flow re-runs from scratch on a follow-up) via a second stale
// restore instead of a fresh one.
func TestRestoreChatRunFromDrivePreservesFlowProgressFieldsWhenLocalAhead(t *testing.T) {
	svc, _, store, _, workspace, accountHome := newChatSyncService(t)
	seedLocalChatRun(t, store, accountHome, workspace, "run-progress", []byte("line1\n"))
	result, apiErr := svc.syncChatRunToDrive(context.Background(), "run-progress", ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}

	// Local continues past the synced snapshot: more rollout turns (file-level
	// localAhead) AND real flow progress recorded in the session row -- exactly
	// what happens between two restores of the same never-deleted local chat.
	targetPath := filepath.Join(accountHome, "sessions", "2026", "06", "17", "rollout-local-session-run-progress.jsonl")
	if err := os.WriteFile(targetPath, []byte("line1\nline2\n"), 0o644); err != nil {
		t.Fatalf("WriteFile longer local: %v", err)
	}
	row, found, err := store.GetProviderSession(context.Background(), "run-progress")
	if err != nil || !found {
		t.Fatalf("GetProviderSession(run-progress) found=%v err=%v", found, err)
	}
	row.TurnCount = 5
	row.LoopState = AgentLoopState{Status: "running", Round: 2}
	if err := store.UpsertProviderSession(context.Background(), row); err != nil {
		t.Fatalf("UpsertProviderSession newer local: %v", err)
	}

	// A second restore of the same chat (e.g. re-clicked from Remote Chats, or a
	// batch "Restore all" re-run) must not downgrade the local flow progress it
	// just advanced past the synced snapshot.
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
	if persisted.TurnCount != 5 {
		t.Fatalf("local TurnCount was regressed to the stale remote manifest: got %d, want 5 (BUG-315 regression risk)", persisted.TurnCount)
	}
	if persisted.LoopState.Round != 2 || persisted.LoopState.Status != "running" {
		t.Fatalf("local LoopState was regressed to the stale remote manifest: got %+v", persisted.LoopState)
	}
}
