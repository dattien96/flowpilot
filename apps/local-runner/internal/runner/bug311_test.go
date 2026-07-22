package runner

// BUG-311: a chat run with no resumable session file on this machine (e.g. a
// turn cancelled before the provider ever wrote one) was retried forever.
//
// Root cause: BuildChatSessionSyncManifest returns "session_unavailable" for
// this case but never persisted that fact anywhere. Since it can never later
// gain a session file, every future "Sync all" batch re-attempted it and
// re-failed it silently -- the Navigator's unsynced-count badge could never
// reach zero for a project containing such a run.

import (
	"context"
	"testing"
)

// TestBuildChatSessionSyncManifestPersistsUnsyncableOnMissingSession reproduces
// BUG-311.
//
// Expected (pre-fix): session_unavailable is returned, but the persisted
// SyncStatus stays "" -- the exact same failure would repeat forever.
// Expected (post-fix): SyncStatus is persisted as "unsyncable" so the caller
// (and the frontend's isSyncableRun) can permanently stop retrying.
func TestBuildChatSessionSyncManifestPersistsUnsyncableOnMissingSession(t *testing.T) {
	svc, _, store, _, workspace, _ := newChatSyncService(t)
	state := ProviderSessionState{
		RunID:             "run-cancelled-no-session",
		ProjectID:         "project-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: "thread-0",
		ProviderAccountID: "acct-sync",
		WorkingDirectory:  workspace,
		Status:            RunStatusCancelled,
		RunKind:           "chat",
	}
	if err := store.UpsertProviderSession(context.Background(), state); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	_, _, apiErr := svc.BuildChatSessionSyncManifest(context.Background(), state.RunID)
	if apiErr == nil || apiErr.code != "session_unavailable" {
		t.Fatalf("expected session_unavailable, got %#v", apiErr)
	}

	persisted, found, err := store.GetProviderSession(context.Background(), state.RunID)
	if err != nil || !found {
		t.Fatalf("GetProviderSession after failed sync: found=%v err=%v", found, err)
	}
	if persisted.SyncStatus != "unsyncable" {
		t.Errorf("BUG-311: SyncStatus = %q after session_unavailable, want \"unsyncable\" (must persist so this run stops being retried forever)", persisted.SyncStatus)
	}

	// A second attempt (simulating the next "Sync all" batch) must behave
	// identically -- this is a permanent fact about the run, not a one-shot
	// side effect.
	_, _, apiErr2 := svc.BuildChatSessionSyncManifest(context.Background(), state.RunID)
	if apiErr2 == nil || apiErr2.code != "session_unavailable" {
		t.Fatalf("second attempt: expected session_unavailable, got %#v", apiErr2)
	}
	persisted2, _, _ := store.GetProviderSession(context.Background(), state.RunID)
	if persisted2.SyncStatus != "unsyncable" {
		t.Errorf("SyncStatus after second attempt = %q, want \"unsyncable\" to remain stable", persisted2.SyncStatus)
	}
}
