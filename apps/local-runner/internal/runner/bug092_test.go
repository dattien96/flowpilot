package runner

import (
	"context"
	"path/filepath"
	"testing"
)

func TestSyncedChatCanResumeAfterServiceRestart(t *testing.T) {
	svc, instance, store, _, workspace, accountHome := newChatSyncService(t)
	seedLocalChatRun(t, store, accountHome, workspace, "run-synced-restart", []byte("{\"type\":\"session_meta\"}\n"))

	if _, apiErr := svc.syncChatRunToDrive(context.Background(), "run-synced-restart", ChatSessionSyncRequest{}); apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}

	reloadedStore, err := NewLocalFileSessionStore(filepath.Join(workspace, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore after restart: %v", err)
	}
	restarted := NewInteractiveServiceWithStore(DefaultProviderRegistry(), nil, reloadedStore)
	restarted.AttachRunner(instance)

	handle, apiErr := restarted.resumeRun("run-synced-restart")
	if apiErr != nil {
		t.Fatalf("resumeRun() after sync and restart failed: code=%s message=%s", apiErr.code, apiErr.msg)
	}
	if handle.RunID != "run-synced-restart" {
		t.Fatalf("resumeRun() run id = %q", handle.RunID)
	}
}

func TestSyncedChatCanRestoreFromRemoteAfterServiceRestart(t *testing.T) {
	svc, instance, store, _, workspace, accountHome := newChatSyncService(t)
	seedLocalChatRun(t, store, accountHome, workspace, "run-remote-restart", []byte("{\"type\":\"session_meta\"}\n"))

	synced, apiErr := svc.syncChatRunToDrive(context.Background(), "run-remote-restart", ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}

	reloadedStore, err := NewLocalFileSessionStore(filepath.Join(workspace, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore after restart: %v", err)
	}
	restarted := NewInteractiveServiceWithStore(DefaultProviderRegistry(), nil, reloadedStore)
	restarted.AttachRunner(instance)

	restored, apiErr := restarted.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
		ProjectID:       "project-1",
		SourceMachineID: synced.SourceMachineID,
		SourceRunID:     synced.SourceRunID,
		Cwd:             workspace,
	})
	if apiErr != nil {
		t.Fatalf("restoreChatRunFromDrive() after restart failed: code=%s message=%s", apiErr.code, apiErr.msg)
	}
	session, found, err := reloadedStore.GetProviderSession(context.Background(), restored.RunID)
	if err != nil || !found {
		t.Fatalf("GetProviderSession(%q) found=%v err=%v", restored.RunID, found, err)
	}
	if session.ProviderAccountID != "acct-sync" {
		t.Fatalf("restored provider account = %q, want acct-sync", session.ProviderAccountID)
	}
}
