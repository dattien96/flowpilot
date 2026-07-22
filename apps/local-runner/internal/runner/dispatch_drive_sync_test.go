package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// task_d80cf120: syncDispatchLogToDrive is called once per run inside a batch
// chat-sync (syncChatRunToDrive runs once per run; every call unconditionally
// re-exported and re-uploaded the entire project-wide dispatch.ndjson) — a live
// 17-chat batch confirmed 9 redundant re-uploads of the same 3.4MB file.
func newDispatchSyncTestService(t *testing.T) (*InteractiveService, *Runner, *fakeChatDriveAPI, string) {
	t.Helper()
	svc, instance, _, api, workspace, _ := newChatSyncService(t)
	chatsRoot := filepath.Join(workspace, ".flowpilot", "chats")
	svc.SetDispatchStore(newMultiProjectDispatchStore(chatsRoot))
	return svc, instance, api, chatsRoot
}

// connectGoogleDriveProject mirrors newChatSyncService's own project-1 wiring
// (chat_session_sync_test.go) so a second project can also resolve a Drive root.
func connectGoogleDriveProject(t *testing.T, instance *Runner, projectID string) {
	t.Helper()
	if err := instance.saveGoogleDriveCredentialByProject(projectID, googleDriveCredential{RefreshToken: "refresh-token-1"}); err != nil {
		t.Fatalf("saveGoogleDriveCredentialByProject(%s): %v", projectID, err)
	}
	if err := instance.saveArtifactStorageGoogleDriveState(func(current *artifactStorageGoogleDriveState) {
		current.Connections[projectID] = artifactStorageGoogleDriveConnectionRecord{
			ProjectID:   projectID,
			Status:      "connected",
			FolderID:    "drive-root",
			ConnectedAt: nowRFC3339Nano(),
		}
	}); err != nil {
		t.Fatalf("saveArtifactStorageGoogleDriveState(%s): %v", projectID, err)
	}
}

func seedProjectDispatchLog(t *testing.T, chatsRoot, projectID string, content []byte) {
	t.Helper()
	dir := filepath.Join(chatsRoot, projectID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "dispatch.ndjson"), content, 0o644); err != nil {
		t.Fatalf("seed dispatch.ndjson: %v", err)
	}
}

func TestSyncDispatchLogToDrive_SkipsReuploadWhenContentUnchanged(t *testing.T) {
	svc, _, api, chatsRoot := newDispatchSyncTestService(t)
	const projectID = "project-1"
	seedProjectDispatchLog(t, chatsRoot, projectID, []byte(`{"kind":"record","seq":1}`+"\n"))

	ctx := context.Background()
	rootFolderID, accessToken, apiErr := svc.ensureChatSessionDriveRoot(projectID)
	if apiErr != nil {
		t.Fatalf("ensureChatSessionDriveRoot: %v", apiErr)
	}

	if err := svc.syncDispatchLogToDrive(ctx, projectID, accessToken, rootFolderID); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	firstUploads := len(api.uploadOrder)
	if firstUploads == 0 {
		t.Fatal("expected the first sync to actually upload")
	}

	// Same content, called again (simulating the next run in the same batch) —
	// must not perform a second Drive upload.
	for i := 0; i < 3; i++ {
		if err := svc.syncDispatchLogToDrive(ctx, projectID, accessToken, rootFolderID); err != nil {
			t.Fatalf("repeat sync #%d: %v", i, err)
		}
	}
	if got := len(api.uploadOrder); got != firstUploads {
		t.Fatalf("expected no additional uploads for unchanged content: before=%d after=%d", firstUploads, got)
	}
}

func TestSyncDispatchLogToDrive_ReuploadsWhenContentChanges(t *testing.T) {
	svc, _, api, chatsRoot := newDispatchSyncTestService(t)
	const projectID = "project-1"
	seedProjectDispatchLog(t, chatsRoot, projectID, []byte(`{"kind":"record","seq":1}`+"\n"))

	ctx := context.Background()
	rootFolderID, accessToken, apiErr := svc.ensureChatSessionDriveRoot(projectID)
	if apiErr != nil {
		t.Fatalf("ensureChatSessionDriveRoot: %v", apiErr)
	}
	if err := svc.syncDispatchLogToDrive(ctx, projectID, accessToken, rootFolderID); err != nil {
		t.Fatalf("first sync: %v", err)
	}
	firstUploads := len(api.uploadOrder)

	// Dispatch log grew (a new turn happened) — this MUST still upload, or a
	// genuinely newer dispatch log would never reach Drive.
	seedProjectDispatchLog(t, chatsRoot, projectID, []byte(`{"kind":"record","seq":1}`+"\n"+`{"kind":"record","seq":2}`+"\n"))
	if err := svc.syncDispatchLogToDrive(ctx, projectID, accessToken, rootFolderID); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if got := len(api.uploadOrder); got <= firstUploads {
		t.Fatalf("expected a new upload after content changed: before=%d after=%d", firstUploads, got)
	}
}

// A failed upload must not poison the cache — otherwise the NEXT attempt (even
// with the exact same unchanged content) would wrongly see "already uploaded"
// and silently skip retrying, permanently losing that dispatch log on Drive.
func TestSyncDispatchLogToDrive_FailedUploadDoesNotPoisonCache(t *testing.T) {
	svc, _, api, chatsRoot := newDispatchSyncTestService(t)
	const projectID = "project-1"
	seedProjectDispatchLog(t, chatsRoot, projectID, []byte(`{"kind":"record","seq":1}`+"\n"))

	ctx := context.Background()
	rootFolderID, accessToken, apiErr := svc.ensureChatSessionDriveRoot(projectID)
	if apiErr != nil {
		t.Fatalf("ensureChatSessionDriveRoot: %v", apiErr)
	}

	api.failUpload = true
	if err := svc.syncDispatchLogToDrive(ctx, projectID, accessToken, rootFolderID); err == nil {
		t.Fatal("expected the upload to fail")
	}
	if got := len(api.uploadOrder); got != 0 {
		t.Fatalf("expected zero recorded uploads after a failed attempt, got %d", got)
	}

	// Same, still-unchanged content — if the failed attempt above had wrongly
	// cached a hash, this retry would incorrectly no-op instead of uploading.
	api.failUpload = false
	if err := svc.syncDispatchLogToDrive(ctx, projectID, accessToken, rootFolderID); err != nil {
		t.Fatalf("retry after transient failure: %v", err)
	}
	if got := len(api.uploadOrder); got == 0 {
		t.Fatal("expected the retry to actually upload — a failed attempt must not poison the cache")
	}
}

func TestSyncDispatchLogToDrive_SeparateProjectsUploadIndependently(t *testing.T) {
	svc, instance, api, chatsRoot := newDispatchSyncTestService(t)
	connectGoogleDriveProject(t, instance, "project-2")
	// Deliberately IDENTICAL content for both projects: a cache keyed globally
	// by content-hash instead of per-project would wrongly treat project-2 as
	// "already uploaded" once project-1's identical bytes land — this is the
	// exact scoping bug this test must catch.
	seedProjectDispatchLog(t, chatsRoot, "project-1", []byte(`{"kind":"record","seq":1}`+"\n"))
	seedProjectDispatchLog(t, chatsRoot, "project-2", []byte(`{"kind":"record","seq":1}`+"\n"))

	ctx := context.Background()
	sync := func(projectID string) {
		t.Helper()
		rootFolderID, accessToken, apiErr := svc.ensureChatSessionDriveRoot(projectID)
		if apiErr != nil {
			t.Fatalf("ensureChatSessionDriveRoot(%s): %v", projectID, apiErr)
		}
		if err := svc.syncDispatchLogToDrive(ctx, projectID, accessToken, rootFolderID); err != nil {
			t.Fatalf("sync(%s): %v", projectID, err)
		}
	}

	sync("project-1")
	afterProject1 := len(api.uploadOrder)
	if afterProject1 == 0 {
		t.Fatal("expected project-1's first sync to upload")
	}

	// Same bytes as project-1, but a DIFFERENT project — must still upload
	// (proves the cache is keyed by project, not just by content hash).
	sync("project-2")
	afterProject2 := len(api.uploadOrder)
	if afterProject2 <= afterProject1 {
		t.Fatalf("expected project-2 to upload independently of project-1's identical content: after project-1=%d after project-2=%d", afterProject1, afterProject2)
	}

	// Repeat both, unchanged — neither should upload again.
	sync("project-1")
	sync("project-2")
	if got := len(api.uploadOrder); got != afterProject2 {
		t.Fatalf("expected no additional uploads once both projects are cached: before=%d after=%d", afterProject2, got)
	}
}
