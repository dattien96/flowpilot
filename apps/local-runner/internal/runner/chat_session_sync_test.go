package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

type fakeChatDriveFile struct {
	ID            string
	Name          string
	ParentID      string
	MimeType      string
	AppProperties map[string]string
	Content       []byte
}

type fakeChatDriveAPI struct {
	nextID      int
	files       map[string]fakeChatDriveFile
	uploadOrder []string
	failUpload  bool
}

func newFakeChatDriveAPI(rootFolderID string) *fakeChatDriveAPI {
	return &fakeChatDriveAPI{
		nextID: 1,
		files: map[string]fakeChatDriveFile{
			rootFolderID: {ID: rootFolderID, Name: "root", MimeType: googleDriveFolderMimeType},
		},
	}
}

func (api *fakeChatDriveAPI) handle(_ context.Context, method, endpoint string, headers map[string]string, body []byte) (int, []byte, error) {
	switch {
	case endpoint == "https://oauth2.googleapis.com/token":
		return 200, []byte(`{"access_token":"drive-access-token"}`), nil
	case method == http.MethodGet && strings.HasPrefix(endpoint, "https://www.googleapis.com/drive/v3/files/") && strings.Contains(endpoint, "?alt=media"):
		return api.handleDownload(endpoint)
	case method == http.MethodGet && strings.HasPrefix(endpoint, "https://www.googleapis.com/drive/v3/files?"):
		return api.handleList(endpoint)
	case strings.HasPrefix(endpoint, "https://www.googleapis.com/upload/drive/v3/files"):
		return api.handleUpload(method, endpoint, headers, body)
	default:
		return 500, []byte(`{"error":"unexpected endpoint"}`), nil
	}
}

func (api *fakeChatDriveAPI) handleDownload(endpoint string) (int, []byte, error) {
	id := pathBaseWithoutQuery(endpoint)
	file, ok := api.files[id]
	if !ok {
		return 404, []byte(`{"error":"missing"}`), nil
	}
	return 200, file.Content, nil
}

func (api *fakeChatDriveAPI) handleList(endpoint string) (int, []byte, error) {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return 500, nil, err
	}
	query := parsed.Query().Get("q")
	switch {
	case strings.Contains(query, "name = "):
		name, parentID := parseGoogleDriveListQuery(query)
		for _, file := range api.files {
			if file.Name == name && file.ParentID == parentID {
				payload, err := api.fileListPayload([]fakeChatDriveFile{file})
				return 200, payload, err
			}
		}
		return 200, []byte(`{"files":[]}`), nil
	case strings.Contains(query, "in parents"):
		parentID := strings.TrimPrefix(strings.Split(query, " ")[0], "'")
		parentID = strings.TrimSuffix(parentID, "'")
		children := make([]fakeChatDriveFile, 0)
		for _, file := range api.files {
			if file.ParentID == parentID {
				children = append(children, file)
			}
		}
		payload, err := api.fileListPayload(children)
		return 200, payload, err
	default:
		return 200, []byte(`{"files":[]}`), nil
	}
}

func (api *fakeChatDriveAPI) fileListPayload(files []fakeChatDriveFile) ([]byte, error) {
	items := make([]map[string]any, 0, len(files))
	for _, file := range files {
		items = append(items, map[string]any{
			"id":            file.ID,
			"name":          file.Name,
			"mimeType":      file.MimeType,
			"webViewLink":   "https://drive.google.com/file/d/" + file.ID + "/view",
			"appProperties": file.AppProperties,
		})
	}
	return json.Marshal(map[string]any{"files": items})
}

func (api *fakeChatDriveAPI) handleUpload(method, endpoint string, headers map[string]string, body []byte) (int, []byte, error) {
	if api.failUpload {
		return 500, []byte(`{"error":"upload failed"}`), nil
	}
	mediaType := headers["content-type"]
	boundary := strings.TrimPrefix(mediaType[strings.Index(mediaType, "boundary="):], "boundary=")
	reader := multipart.NewReader(bytes.NewReader(body), boundary)
	var metadata struct {
		Name          string            `json:"name"`
		MimeType      string            `json:"mimeType"`
		Parents       []string          `json:"parents"`
		AppProperties map[string]string `json:"appProperties"`
	}
	var content []byte
	for i := 0; i < 2; i++ {
		part, err := reader.NextPart()
		if err != nil {
			break
		}
		data, err := io.ReadAll(part)
		if err != nil {
			return 500, nil, err
		}
		if i == 0 {
			if err := json.Unmarshal(data, &metadata); err != nil {
				return 400, nil, err
			}
		} else {
			content = data
		}
	}
	parentID := ""
	if len(metadata.Parents) > 0 {
		parentID = metadata.Parents[0]
	}
	targetID := ""
	if method == http.MethodPatch {
		targetID = pathBaseWithoutQuery(endpoint)
	}
	if targetID == "" {
		api.nextID++
		targetID = "chat-drive-" + strconv.Itoa(api.nextID)
	}
	file := fakeChatDriveFile{
		ID:            targetID,
		Name:          metadata.Name,
		ParentID:      parentID,
		MimeType:      metadata.MimeType,
		AppProperties: metadata.AppProperties,
		Content:       content,
	}
	if file.MimeType == "" {
		file.MimeType = "application/octet-stream"
	}
	if existing, ok := api.files[targetID]; ok && file.ParentID == "" {
		file.ParentID = existing.ParentID
	}
	api.files[targetID] = file
	api.uploadOrder = append(api.uploadOrder, metadata.Name)
	payload, err := json.Marshal(map[string]any{
		"id":            file.ID,
		"name":          file.Name,
		"mimeType":      file.MimeType,
		"webViewLink":   "https://drive.google.com/file/d/" + file.ID + "/view",
		"appProperties": file.AppProperties,
	})
	return 200, payload, err
}

func newChatSyncService(t *testing.T) (*InteractiveService, *Runner, *localFileSessionStore, *fakeChatDriveAPI, string, string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GOOGLE_DRIVE_CLIENT_ID", "client-id")
	t.Setenv("GOOGLE_DRIVE_CLIENT_SECRET", "client-secret")
	t.Setenv("GOOGLE_DRIVE_REDIRECT_URI", "http://localhost/callback")
	t.Setenv("GOOGLE_PICKER_API_KEY", "picker-key")

	workspace := t.TempDir()
	instance, err := New(workspace)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	instance.secretStore = newMemorySecretStore()
	accountHome := filepath.Join(home, "codex-home")
	if err := os.MkdirAll(filepath.Join(accountHome, ".codex"), 0o755); err != nil {
		t.Fatalf("mkdir auth dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(accountHome, ".codex", "auth.json"), []byte(`{"id_token":"token"}`), 0o644); err != nil {
		t.Fatalf("write auth: %v", err)
	}
	if err := instance.saveProviderAccountState(providerAccountState{
		Accounts: []ProviderAccount{{
			ID:          "acct-sync",
			ProviderKey: "codex",
			DisplayName: "Account 1",
			HomePath:    accountHome,
			SlotIndex:   1,
			IsActive:    true,
			AuthStatus:  "connected",
			CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		}},
	}); err != nil {
		t.Fatalf("saveProviderAccountState: %v", err)
	}
	if err := instance.saveGoogleDriveCredentialByProject("project-1", googleDriveCredential{RefreshToken: "refresh-token-1"}); err != nil {
		t.Fatalf("saveGoogleDriveCredentialByProject: %v", err)
	}
	if err := instance.saveArtifactStorageGoogleDriveState(func(current *artifactStorageGoogleDriveState) {
		current.Connections["project-1"] = artifactStorageGoogleDriveConnectionRecord{
			ProjectID:   "project-1",
			Status:      "connected",
			FolderID:    "drive-root",
			ConnectedAt: time.Now().UTC().Format(time.RFC3339Nano),
		}
	}); err != nil {
		t.Fatalf("saveArtifactStorageGoogleDriveState: %v", err)
	}
	store, err := NewLocalFileSessionStore(filepath.Join(workspace, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), nil, store)
	svc.AttachRunner(instance)
	svc.SetActiveAccount("acct-sync")

	api := newFakeChatDriveAPI("drive-root")
	originalHTTPRequest := httpRequestFn
	httpRequestFn = api.handle
	t.Cleanup(func() { httpRequestFn = originalHTTPRequest })
	return svc, instance, store, api, workspace, accountHome
}

func remoteProviderFileID(api *fakeChatDriveAPI, fileName string) string {
	for _, file := range api.files {
		if file.Name == fileName {
			return file.ID
		}
	}
	return ""
}

func remoteManifestFileID(api *fakeChatDriveAPI) string {
	for _, file := range api.files {
		if file.Name == "manifest.json" {
			return file.ID
		}
	}
	return ""
}

func rootAncestorID(api *fakeChatDriveAPI, fileID string) string {
	currentID := fileID
	visited := map[string]bool{}
	for currentID != "" && !visited[currentID] {
		visited[currentID] = true
		file, ok := api.files[currentID]
		if !ok || file.ParentID == "" {
			return currentID
		}
		currentID = file.ParentID
	}
	return currentID
}

func seedLocalChatRun(t *testing.T, store *localFileSessionStore, accountHome, workspace, runID string, fileBody []byte) ProviderSessionState {
	t.Helper()
	sessionID := "session-" + runID
	sessionPath := filepath.Join(accountHome, "sessions", "2026", "06", "17", "rollout-local-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(sessionPath), 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	if err := os.WriteFile(sessionPath, fileBody, 0o644); err != nil {
		t.Fatalf("write session file: %v", err)
	}
	state := ProviderSessionState{
		RunID:             runID,
		ProjectID:         "project-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: sessionID,
		ProviderAccountID: "acct-sync",
		WorkingDirectory:  workspace,
		Status:            RunStatusCompleted,
		LastPrompt:        "summarize the branch",
		LastMessage:       "done",
		StartedAt:         "2026-06-17T10:00:00Z",
		UpdatedAt:         "2026-06-17T10:05:00Z",
		RunKind:           "chat",
	}
	if err := store.UpsertProviderSession(context.Background(), state); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}
	return state
}

func TestMergeChatSessionDriveIndexLastWinsBySourceIdentity(t *testing.T) {
	existing := []byte(`{"source_machine_id":"m1","source_run_id":"run-1","project_id":"project-1","synced_at":"old","manifest_path":"old"}` + "\n" +
		`{"source_machine_id":"m2","source_run_id":"run-2","project_id":"project-1","synced_at":"same","manifest_path":"keep"}` + "\n")
	merged := parseChatSessionDriveIndex(mergeChatSessionDriveIndex(existing, chatSessionDriveIndexRecord{
		ProjectID:       "project-1",
		SourceMachineID: "m1",
		SourceRunID:     "run-1",
		SyncedAt:        "new",
		ManifestPath:    "new",
	}))
	if len(merged) != 2 {
		t.Fatalf("merged records = %d, want 2", len(merged))
	}
	for _, record := range merged {
		if record.SourceMachineID == "m1" && record.ManifestPath != "new" {
			t.Fatalf("last-wins merge failed: %#v", record)
		}
	}
}

func TestMergeChatSessionDriveIndexIgnoresMalformedLines(t *testing.T) {
	merged := parseChatSessionDriveIndex(mergeChatSessionDriveIndex([]byte("{bad}\n"+`{"source_machine_id":"m1","source_run_id":"run-1","project_id":"project-1","manifest_path":"old","synced_at":"old"}`+"\n"), chatSessionDriveIndexRecord{
		ProjectID:       "project-1",
		SourceMachineID: "m2",
		SourceRunID:     "run-2",
		ManifestPath:    "new",
		SyncedAt:        "new",
	}))
	if len(merged) != 2 {
		t.Fatalf("merged valid records = %d, want 2", len(merged))
	}
}

func TestChatSessionDrivePathsUseSafeSegments(t *testing.T) {
	path := chatSessionManifestPath("mch/a:b", "run/one")
	if strings.Contains(path, ":") || strings.Contains(path, "//") {
		t.Fatalf("unsafe manifest path: %q", path)
	}
	if got := chatSessionProviderLogicalPath("mch/a:b", "run/one", ProviderKeyCodex, "../rollout.jsonl"); strings.Contains(got, "..") {
		t.Fatalf("unsafe provider path: %q", got)
	}
}

func TestBuildChatSessionSyncManifestHashesProviderFile(t *testing.T) {
	svc, _, store, _, workspace, accountHome := newChatSyncService(t)
	state := seedLocalChatRun(t, store, accountHome, workspace, "run-1", []byte("session-body"))
	manifest, _, apiErr := svc.BuildChatSessionSyncManifest(context.Background(), state.RunID)
	if apiErr != nil {
		t.Fatalf("BuildChatSessionSyncManifest() failed: %v", apiErr)
	}
	if manifest.ProviderKey != ProviderKeyCodex || manifest.ProviderSessionID != state.ProviderSessionID {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
	if manifest.ProviderFile.SizeBytes != int64(len("session-body")) {
		t.Fatalf("unexpected size: %d", manifest.ProviderFile.SizeBytes)
	}
	if manifest.ProviderFile.SHA256 != hashBytesSHA256([]byte("session-body")) {
		t.Fatalf("unexpected hash: %q", manifest.ProviderFile.SHA256)
	}
}

func TestBuildChatSessionSyncManifestMissingRun(t *testing.T) {
	svc, _, _, _, _, _ := newChatSyncService(t)
	_, _, apiErr := svc.BuildChatSessionSyncManifest(context.Background(), "missing")
	if apiErr == nil || apiErr.code != "run_not_found" {
		t.Fatalf("expected run_not_found, got %#v", apiErr)
	}
}

func TestBuildChatSessionSyncManifestRejectsNonChatRun(t *testing.T) {
	svc, _, store, _, workspace, accountHome := newChatSyncService(t)
	state := seedLocalChatRun(t, store, accountHome, workspace, "run-workflow", []byte("session-body"))
	state.RunKind = "workflow"
	if err := store.UpsertProviderSession(context.Background(), state); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}
	_, _, apiErr := svc.BuildChatSessionSyncManifest(context.Background(), state.RunID)
	if apiErr == nil || apiErr.code != "resume_unsupported" {
		t.Fatalf("expected resume_unsupported, got %#v", apiErr)
	}
}

func TestBuildChatSessionSyncManifestMissingProviderFile(t *testing.T) {
	svc, _, store, _, workspace, _ := newChatSyncService(t)
	state := ProviderSessionState{
		RunID:             "run-missing-file",
		ProjectID:         "project-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: "session-missing",
		ProviderAccountID: "acct-sync",
		WorkingDirectory:  workspace,
		Status:            RunStatusCompleted,
		RunKind:           "chat",
	}
	if err := store.UpsertProviderSession(context.Background(), state); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}
	_, _, apiErr := svc.BuildChatSessionSyncManifest(context.Background(), state.RunID)
	if apiErr == nil || apiErr.code != "session_unavailable" {
		t.Fatalf("expected session_unavailable, got %#v", apiErr)
	}
}

func TestSyncChatRunToDriveUploadOrdering(t *testing.T) {
	svc, _, store, api, workspace, accountHome := newChatSyncService(t)
	seedLocalChatRun(t, store, accountHome, workspace, "run-sync", []byte("session-body"))
	if _, apiErr := svc.syncChatRunToDrive(context.Background(), "run-sync", ChatSessionSyncRequest{}); apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}
	if len(api.uploadOrder) < 3 {
		t.Fatalf("upload order too short: %#v", api.uploadOrder)
	}
	providerIndex := slices.Index(api.uploadOrder, "rollout-local-session-run-sync.jsonl")
	manifestIndex := slices.Index(api.uploadOrder, "manifest.json")
	indexUploadIndex := slices.Index(api.uploadOrder, "sessions.ndjson")
	if providerIndex < 0 || manifestIndex < 0 || indexUploadIndex < 0 || !(providerIndex < manifestIndex && manifestIndex < indexUploadIndex) {
		t.Fatalf("unexpected upload order: %#v", api.uploadOrder)
	}
}

func TestSyncChatRunToDriveUpdatesLocalSyncStatus(t *testing.T) {
	svc, _, store, _, workspace, accountHome := newChatSyncService(t)
	seedLocalChatRun(t, store, accountHome, workspace, "run-status", []byte("session-body"))
	if _, apiErr := svc.syncChatRunToDrive(context.Background(), "run-status", ChatSessionSyncRequest{}); apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}
	session, found, err := store.GetProviderSession(context.Background(), "run-status")
	if err != nil || !found {
		t.Fatalf("GetProviderSession() failed: found=%v err=%v", found, err)
	}
	if session.SyncStatus != "synced" || session.SyncUpdatedAt == "" || session.SourceMachineID == "" || session.SourceRunID != "run-status" {
		t.Fatalf("local sync metadata not updated: %#v", session)
	}
}

func TestSyncChatRunToDriveFailurePreservesPriorMetadata(t *testing.T) {
	svc, _, store, api, workspace, accountHome := newChatSyncService(t)
	state := seedLocalChatRun(t, store, accountHome, workspace, "run-fail", []byte("session-body"))
	state.SyncStatus = "synced"
	state.SyncUpdatedAt = "2026-06-17T11:00:00Z"
	state.SourceMachineID = "mch_old"
	state.SourceRunID = "run-fail"
	if err := store.UpsertProviderSession(context.Background(), state); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}
	api.failUpload = true
	if _, apiErr := svc.syncChatRunToDrive(context.Background(), "run-fail", ChatSessionSyncRequest{}); apiErr == nil {
		t.Fatal("expected sync error")
	}
	session, found, err := store.GetProviderSession(context.Background(), "run-fail")
	if err != nil || !found {
		t.Fatalf("GetProviderSession() failed: found=%v err=%v", found, err)
	}
	if session.SourceMachineID != "mch_old" || session.SyncUpdatedAt != "2026-06-17T11:00:00Z" {
		t.Fatalf("prior sync metadata was not preserved: %#v", session)
	}
}

func TestSyncChatRunToDriveRequiresGoogleDriveConnection(t *testing.T) {
	svc, instance, store, _, workspace, accountHome := newChatSyncService(t)
	seedLocalChatRun(t, store, accountHome, workspace, "run-no-drive", []byte("session-body"))
	if err := instance.saveArtifactStorageGoogleDriveState(func(current *artifactStorageGoogleDriveState) {
		delete(current.Connections, "project-1")
	}); err != nil {
		t.Fatalf("saveArtifactStorageGoogleDriveState: %v", err)
	}
	if _, apiErr := svc.syncChatRunToDrive(context.Background(), "run-no-drive", ChatSessionSyncRequest{}); apiErr == nil || apiErr.code != "google_drive_not_connected" {
		t.Fatalf("expected google_drive_not_connected, got %#v", apiErr)
	}
}

func TestSyncChatRunToDriveRejectsFolderOverride(t *testing.T) {
	svc, _, store, _, workspace, accountHome := newChatSyncService(t)
	seedLocalChatRun(t, store, accountHome, workspace, "run-folder-override", []byte("session-body"))

	if _, apiErr := svc.syncChatRunToDrive(context.Background(), "run-folder-override", ChatSessionSyncRequest{
		GoogleDriveFolderID: "folder-override",
	}); apiErr == nil || apiErr.code != "invalid_request" {
		t.Fatalf("expected invalid_request for folder override, got %#v", apiErr)
	}
}

func TestChatSessionSyncPrefersDedicatedChatFolderOverArtifactLegacyBinding(t *testing.T) {
	svc, instance, store, api, workspace, accountHome := newChatSyncService(t)
	if err := instance.saveGoogleDriveCredentialByAccount("drive-account-1", googleDriveCredential{
		RefreshToken: "refresh-token-1",
		AccountEmail: "owner@example.com",
	}); err != nil {
		t.Fatalf("saveGoogleDriveCredentialByAccount: %v", err)
	}
	if err := instance.saveArtifactStorageGoogleDriveState(func(current *artifactStorageGoogleDriveState) {
		current.ChatSyncConnections["project-1"] = artifactStorageGoogleDriveConnectionRecord{
			ProjectID:    "project-1",
			AccountID:    "drive-account-1",
			AccountEmail: "owner@example.com",
			FolderID:     "chat-root",
			FolderName:   "Chat Sync",
			Status:       "connected",
		}
	}); err != nil {
		t.Fatalf("saveArtifactStorageGoogleDriveState: %v", err)
	}
	api.files["chat-root"] = fakeChatDriveFile{ID: "chat-root", Name: "chat-root", MimeType: googleDriveFolderMimeType}

	seedLocalChatRun(t, store, accountHome, workspace, "run-chat-root", []byte("session-body"))
	if _, apiErr := svc.syncChatRunToDrive(context.Background(), "run-chat-root", ChatSessionSyncRequest{}); apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}

	summaries, apiErr := svc.listRemoteChatSessions(context.Background(), "project-1")
	if apiErr != nil {
		t.Fatalf("listRemoteChatSessions() failed: %v", apiErr)
	}
	if len(summaries) != 1 || summaries[0].SourceRunID != "run-chat-root" {
		t.Fatalf("expected chat-root remote summary, got %#v", summaries)
	}

	manifestID := remoteManifestFileID(api)
	if manifestID == "" {
		t.Fatal("expected manifest upload")
	}
	if got := rootAncestorID(api, manifestID); got != "chat-root" {
		t.Fatalf("expected manifest to upload under chat sync root, got root %q", got)
	}
}

func TestListRemoteChatSessionsReadsDriveIndex(t *testing.T) {
	svc, _, store, _, workspace, accountHome := newChatSyncService(t)
	seedLocalChatRun(t, store, accountHome, workspace, "run-remote", []byte("session-body"))
	if _, apiErr := svc.syncChatRunToDrive(context.Background(), "run-remote", ChatSessionSyncRequest{}); apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}
	summaries, apiErr := svc.listRemoteChatSessions(context.Background(), "project-1")
	if apiErr != nil {
		t.Fatalf("listRemoteChatSessions() failed: %v", apiErr)
	}
	if len(summaries) != 1 || summaries[0].SourceRunID != "run-remote" {
		t.Fatalf("unexpected summaries: %#v", summaries)
	}
}

func TestListRemoteChatSessionsIncludesRecordsFromSameDriveRootWithDifferentProjectIDs(t *testing.T) {
	svc, _, store, api, workspace, accountHome := newChatSyncService(t)
	seedLocalChatRun(t, store, accountHome, workspace, "run-codex", []byte("codex-session"))
	if _, apiErr := svc.syncChatRunToDrive(context.Background(), "run-codex", ChatSessionSyncRequest{}); apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}

	indexFileID := ""
	for id, file := range api.files {
		if file.Name == "sessions.ndjson" {
			indexFileID = id
			break
		}
	}
	if indexFileID == "" {
		t.Fatal("expected Drive index")
	}
	indexFile := api.files[indexFileID]
	indexFile.Content = mergeChatSessionDriveIndex(indexFile.Content, chatSessionDriveIndexRecord{
		RunID:           "run-claude",
		ProjectID:       "project-id-from-another-pc",
		ProviderKey:     string(ProviderKeyClaude),
		RunKind:         "chat",
		SourceMachineID: "mch_remote",
		SourceRunID:     "run-claude",
		UpdatedAt:       "2026-06-19T10:00:00Z",
		ManifestPath:    "chat-sessions/runs/mch_remote/run-claude/manifest.json",
	})
	api.files[indexFileID] = indexFile

	summaries, apiErr := svc.listRemoteChatSessions(context.Background(), "project-1")
	if apiErr != nil {
		t.Fatalf("listRemoteChatSessions() failed: %v", apiErr)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected both provider records from the selected Drive root, got %#v", summaries)
	}
	if summaries[0].ProviderKey != ProviderKeyClaude || summaries[1].ProviderKey != ProviderKeyCodex {
		t.Fatalf("expected Claude and Codex summaries, got %#v", summaries)
	}
}

func TestListRemoteChatSessionsMissingIndexIsEmpty(t *testing.T) {
	svc, _, _, _, _, _ := newChatSyncService(t)
	summaries, apiErr := svc.listRemoteChatSessions(context.Background(), "project-1")
	if apiErr != nil {
		t.Fatalf("listRemoteChatSessions() failed: %v", apiErr)
	}
	if len(summaries) != 0 {
		t.Fatalf("expected no remote summaries, got %#v", summaries)
	}
}

func TestRestoreChatRunFromDriveValidatesManifest(t *testing.T) {
	svc, _, store, api, workspace, accountHome := newChatSyncService(t)
	seedLocalChatRun(t, store, accountHome, workspace, "run-manifest", []byte("session-body"))
	result, apiErr := svc.syncChatRunToDrive(context.Background(), "run-manifest", ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}
	manifestID := remoteManifestFileID(api)
	if manifestID == "" {
		t.Fatal("expected manifest upload")
	}
	file := api.files[manifestID]
	var manifest ChatSessionSyncManifest
	if err := json.Unmarshal(file.Content, &manifest); err != nil {
		t.Fatalf("manifest json invalid: %v", err)
	}
	if manifest.SchemaVersion != chatSessionManifestSchemaVersion || manifest.SourceMachineID != result.SourceMachineID || manifest.SourceRunID != result.SourceRunID || manifest.ProviderKey != ProviderKeyCodex {
		t.Fatalf("unexpected manifest: %#v", manifest)
	}
}

func TestRestoreChatRunFromDriveMissingRemoteFile(t *testing.T) {
	svc, _, store, api, workspace, accountHome := newChatSyncService(t)
	seedLocalChatRun(t, store, accountHome, workspace, "run-missing-remote", []byte("session-body"))
	result, apiErr := svc.syncChatRunToDrive(context.Background(), "run-missing-remote", ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}
	fileID := remoteProviderFileID(api, "rollout-local-session-run-missing-remote.jsonl")
	if fileID == "" {
		t.Fatal("expected provider upload")
	}
	delete(api.files, fileID)
	_, apiErr = svc.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
		ProjectID:       "project-1",
		SourceMachineID: result.SourceMachineID,
		SourceRunID:     result.SourceRunID,
		Cwd:             workspace,
	})
	if apiErr == nil || apiErr.code != "sync_remote_not_found" {
		t.Fatalf("expected sync_remote_not_found, got %#v", apiErr)
	}
}

func TestRestoreChatRunFromDriveRejectsHashMismatch(t *testing.T) {
	svc, _, store, api, workspace, accountHome := newChatSyncService(t)
	seedLocalChatRun(t, store, accountHome, workspace, "run-restore", []byte("session-body"))
	result, apiErr := svc.syncChatRunToDrive(context.Background(), "run-restore", ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}
	fileID := remoteProviderFileID(api, "rollout-local-session-run-restore.jsonl")
	file := api.files[fileID]
	file.Content = []byte("tampered")
	api.files[fileID] = file
	_, apiErr = svc.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
		ProjectID:       "project-1",
		SourceMachineID: result.SourceMachineID,
		SourceRunID:     result.SourceRunID,
		Cwd:             workspace,
	})
	if apiErr == nil || apiErr.code != "sync_integrity_failed" {
		t.Fatalf("expected sync_integrity_failed, got %#v", apiErr)
	}
}

func TestRestoreChatRunFromDriveUsesRequestCwd(t *testing.T) {
	svc, _, store, _, _, accountHome := newChatSyncService(t)
	originalWorkspace := filepath.Join(t.TempDir(), "deleted-later")
	seedLocalChatRun(t, store, accountHome, originalWorkspace, "run-remap-request", []byte("session-body"))
	result, apiErr := svc.syncChatRunToDrive(context.Background(), "run-remap-request", ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}
	_ = os.RemoveAll(originalWorkspace)
	restoredWorkspace := filepath.Join(t.TempDir(), "remapped-project")
	if err := os.MkdirAll(restoredWorkspace, 0o755); err != nil {
		t.Fatalf("MkdirAll remapped workspace: %v", err)
	}
	restored, apiErr := svc.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
		ProjectID:       "project-1",
		SourceMachineID: result.SourceMachineID,
		SourceRunID:     result.SourceRunID,
		Cwd:             restoredWorkspace,
	})
	if apiErr != nil {
		t.Fatalf("restoreChatRunFromDrive() failed: %v", apiErr)
	}
	session, found, err := store.GetProviderSession(context.Background(), restored.RunID)
	if err != nil || !found {
		t.Fatalf("GetProviderSession() failed: found=%v err=%v", found, err)
	}
	if session.WorkingDirectory != restoredWorkspace {
		t.Fatalf("working directory = %q, want %q", session.WorkingDirectory, restoredWorkspace)
	}
}

func TestRestoreChatRunFromDriveMissingActiveAccountHome(t *testing.T) {
	svc, instance, store, _, workspace, accountHome := newChatSyncService(t)
	seedLocalChatRun(t, store, accountHome, workspace, "run-no-home", []byte("session-body"))
	result, apiErr := svc.syncChatRunToDrive(context.Background(), "run-no-home", ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}
	if err := instance.saveProviderAccountState(providerAccountState{}); err != nil {
		t.Fatalf("clear provider accounts: %v", err)
	}
	svc.SetActiveAccount("missing-account")
	_, apiErr = svc.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
		ProjectID:       "project-1",
		SourceMachineID: result.SourceMachineID,
		SourceRunID:     result.SourceRunID,
		Cwd:             workspace,
	})
	if apiErr == nil || apiErr.code != "account_unavailable" {
		t.Fatalf("expected account_unavailable, got %#v", apiErr)
	}
}

func TestRestoreChatRunFromDriveMissingActiveAccountAuth(t *testing.T) {
	svc, _, store, _, workspace, accountHome := newChatSyncService(t)
	seedLocalChatRun(t, store, accountHome, workspace, "run-no-auth", []byte("session-body"))
	result, apiErr := svc.syncChatRunToDrive(context.Background(), "run-no-auth", ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}
	if err := os.Remove(filepath.Join(accountHome, ".codex", "auth.json")); err != nil {
		t.Fatalf("Remove auth.json: %v", err)
	}
	_, apiErr = svc.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
		ProjectID:       "project-1",
		SourceMachineID: result.SourceMachineID,
		SourceRunID:     result.SourceRunID,
		Cwd:             workspace,
	})
	if apiErr == nil || apiErr.code != "account_not_signed_in" {
		t.Fatalf("expected account_not_signed_in, got %#v", apiErr)
	}
}

func TestRestoreChatRunFromDriveRejectsOverwriteConflict(t *testing.T) {
	svc, _, store, _, workspace, accountHome := newChatSyncService(t)
	seedLocalChatRun(t, store, accountHome, workspace, "run-conflict", []byte("session-body"))
	result, apiErr := svc.syncChatRunToDrive(context.Background(), "run-conflict", ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}
	conflictPath := filepath.Join(accountHome, "sessions", "2026", "06", "17", "rollout-local-session-run-conflict.jsonl")
	if err := os.WriteFile(conflictPath, []byte("different"), 0o644); err != nil {
		t.Fatalf("WriteFile conflict: %v", err)
	}
	_, apiErr = svc.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
		ProjectID:       "project-1",
		SourceMachineID: result.SourceMachineID,
		SourceRunID:     result.SourceRunID,
		Cwd:             workspace,
	})
	if apiErr == nil || apiErr.code != "session_file_conflict" {
		t.Fatalf("expected session_file_conflict, got %#v", apiErr)
	}
}

func TestRestoreChatRunFromDriveAcceptsExistingIdenticalFile(t *testing.T) {
	svc, _, store, _, workspace, accountHome := newChatSyncService(t)
	seedLocalChatRun(t, store, accountHome, workspace, "run-identical", []byte("session-body"))
	result, apiErr := svc.syncChatRunToDrive(context.Background(), "run-identical", ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}
	targetPath := filepath.Join(accountHome, "sessions", "2026", "06", "17", "rollout-local-session-run-identical.jsonl")
	if err := os.WriteFile(targetPath, []byte("session-body"), 0o644); err != nil {
		t.Fatalf("WriteFile identical target: %v", err)
	}
	restored, apiErr := svc.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
		ProjectID:       "project-1",
		SourceMachineID: result.SourceMachineID,
		SourceRunID:     result.SourceRunID,
		Cwd:             workspace,
	})
	if apiErr != nil {
		t.Fatalf("restoreChatRunFromDrive() failed: %v", apiErr)
	}
	if restored.RunID != "run-identical" {
		t.Fatalf("unexpected restored run id: %q", restored.RunID)
	}
}

func TestRestoreChatRunFromDriveOverwritesWhenRemoteExtendsLocal(t *testing.T) {
	svc, _, store, _, workspace, accountHome := newChatSyncService(t)
	extended := "line1\nline2\nline3\n"
	seedLocalChatRun(t, store, accountHome, workspace, "run-remote-ext", []byte(extended))
	result, apiErr := svc.syncChatRunToDrive(context.Background(), "run-remote-ext", ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}
	targetPath := filepath.Join(accountHome, "sessions", "2026", "06", "17", "rollout-local-session-run-remote-ext.jsonl")
	// Local holds an older prefix of the synced (remote) content.
	if err := os.WriteFile(targetPath, []byte("line1\n"), 0o644); err != nil {
		t.Fatalf("WriteFile shorter local: %v", err)
	}
	restored, apiErr := svc.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
		ProjectID:       "project-1",
		SourceMachineID: result.SourceMachineID,
		SourceRunID:     result.SourceRunID,
		Cwd:             workspace,
	})
	if apiErr != nil {
		t.Fatalf("restoreChatRunFromDrive() failed: %v", apiErr)
	}
	if restored.RunID != "run-remote-ext" {
		t.Fatalf("unexpected restored run id: %q", restored.RunID)
	}
	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("ReadFile target: %v", err)
	}
	if string(got) != extended {
		t.Fatalf("local file not overwritten with newer remote extension: got %q", got)
	}
}

func TestRestoreChatRunFromDriveKeepsLocalWhenLocalExtendsRemote(t *testing.T) {
	svc, _, store, _, workspace, accountHome := newChatSyncService(t)
	seedLocalChatRun(t, store, accountHome, workspace, "run-local-ext", []byte("line1\n"))
	result, apiErr := svc.syncChatRunToDrive(context.Background(), "run-local-ext", ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}
	targetPath := filepath.Join(accountHome, "sessions", "2026", "06", "17", "rollout-local-session-run-local-ext.jsonl")
	localExtended := "line1\nline2\n"
	// Local is ahead of the synced snapshot (more turns appended locally).
	if err := os.WriteFile(targetPath, []byte(localExtended), 0o644); err != nil {
		t.Fatalf("WriteFile longer local: %v", err)
	}
	restored, apiErr := svc.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
		ProjectID:       "project-1",
		SourceMachineID: result.SourceMachineID,
		SourceRunID:     result.SourceRunID,
		Cwd:             workspace,
	})
	if apiErr != nil {
		t.Fatalf("restoreChatRunFromDrive() failed: %v", apiErr)
	}
	if restored.RunID != "run-local-ext" {
		t.Fatalf("unexpected restored run id: %q", restored.RunID)
	}
	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("ReadFile target: %v", err)
	}
	if string(got) != localExtended {
		t.Fatalf("local file should be untouched when local is ahead: got %q", got)
	}
}

func TestRestoreChatRunFromDrivePreservesLocalMetadataWhenLocalAhead(t *testing.T) {
	svc, _, store, _, workspace, accountHome := newChatSyncService(t)
	seedLocalChatRun(t, store, accountHome, workspace, "run-meta", []byte("line1\n"))
	result, apiErr := svc.syncChatRunToDrive(context.Background(), "run-meta", ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}
	targetPath := filepath.Join(accountHome, "sessions", "2026", "06", "17", "rollout-local-session-run-meta.jsonl")
	if err := os.WriteFile(targetPath, []byte("line1\nline2\n"), 0o644); err != nil {
		t.Fatalf("WriteFile longer local: %v", err)
	}
	// Local conversation has advanced past the synced snapshot; the restore must
	// not downgrade these fields to the older remote manifest. (BUG-091)
	row, found, err := store.GetProviderSession(context.Background(), "run-meta")
	if err != nil || !found {
		t.Fatalf("GetProviderSession(run-meta) found=%v err=%v", found, err)
	}
	row.LastPrompt = "newer local prompt"
	row.LastMessage = "newer local reply"
	row.UpdatedAt = "2026-06-18T12:00:00Z"
	if err := store.UpsertProviderSession(context.Background(), row); err != nil {
		t.Fatalf("UpsertProviderSession newer local: %v", err)
	}
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
	if persisted.LastPrompt != "newer local prompt" {
		t.Fatalf("local LastPrompt was downgraded to remote manifest: got %q", persisted.LastPrompt)
	}
	if persisted.LastMessage != "newer local reply" {
		t.Fatalf("local LastMessage was downgraded to remote manifest: got %q", persisted.LastMessage)
	}
}

func TestResolveRestoredRunIDUsesSourceWhenUnused(t *testing.T) {
	svc, _, _, _, _, _ := newChatSyncService(t)
	if got := svc.resolveRestoredRunID(context.Background(), "mch-source", "run-1"); got != "run-1" {
		t.Fatalf("resolveRestoredRunID() = %q, want %q", got, "run-1")
	}
}

func TestResolveRestoredRunIDReusesSameSourceIdentity(t *testing.T) {
	svc, _, store, _, workspace, accountHome := newChatSyncService(t)
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "sync-existing",
		ProjectID:         "project-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: "session-existing",
		ProviderAccountID: "acct-sync",
		WorkingDirectory:  workspace,
		Status:            RunStatusCompleted,
		RunKind:           "chat",
		SourceMachineID:   "mch-source",
		SourceRunID:       "run-1",
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}
	seedLocalChatRun(t, store, accountHome, workspace, "irrelevant", []byte("session-body"))
	if got := svc.resolveRestoredRunID(context.Background(), "mch-source", "run-1"); got != "sync-existing" {
		t.Fatalf("resolveRestoredRunID() = %q, want %q", got, "sync-existing")
	}
}

func TestRestoreChatRunFromDriveResolvesRunIDCollision(t *testing.T) {
	svc, _, store, _, workspace, accountHome := newChatSyncService(t)
	seedLocalChatRun(t, store, accountHome, workspace, "run-collision", []byte("session-body"))
	result, apiErr := svc.syncChatRunToDrive(context.Background(), "run-collision", ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:     "run-collision",
		ProjectID: "project-1",
		Status:    RunStatusCompleted,
		RunKind:   "chat",
	}); err != nil {
		t.Fatalf("UpsertProviderSession collision: %v", err)
	}
	restored, apiErr := svc.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
		ProjectID:       "project-1",
		SourceMachineID: result.SourceMachineID,
		SourceRunID:     result.SourceRunID,
		Cwd:             filepath.Join(workspace, "other"),
	})
	if apiErr != nil {
		t.Fatalf("restoreChatRunFromDrive() failed: %v", apiErr)
	}
	if restored.RunID == "run-collision" || !strings.HasPrefix(restored.RunID, "sync-") {
		t.Fatalf("expected collision-safe run id, got %q", restored.RunID)
	}
}

func TestRestoreChatRunFromDriveUpsertsOneLocalSession(t *testing.T) {
	svc, _, store, _, workspace, accountHome := newChatSyncService(t)
	seedLocalChatRun(t, store, accountHome, workspace, "run-upsert", []byte("session-body"))
	result, apiErr := svc.syncChatRunToDrive(context.Background(), "run-upsert", ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}
	restoredWorkspace := filepath.Join(t.TempDir(), "restored-project")
	if err := os.MkdirAll(restoredWorkspace, 0o755); err != nil {
		t.Fatalf("MkdirAll restored project: %v", err)
	}
	restored, apiErr := svc.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
		ProjectID:       "project-1",
		SourceMachineID: result.SourceMachineID,
		SourceRunID:     result.SourceRunID,
		Cwd:             restoredWorkspace,
	})
	if apiErr != nil {
		t.Fatalf("restoreChatRunFromDrive() failed: %v", apiErr)
	}
	session, found, err := store.GetProviderSession(context.Background(), restored.RunID)
	if err != nil || !found {
		t.Fatalf("GetProviderSession() failed: found=%v err=%v", found, err)
	}
	if session.SourceMachineID != result.SourceMachineID || session.SourceRunID != result.SourceRunID || session.SyncStatus != "restored" {
		t.Fatalf("unexpected restored session: %#v", session)
	}
}

func TestRestoreChatRunFromDriveRequiresCwdRemap(t *testing.T) {
	svc, _, store, _, _, accountHome := newChatSyncService(t)
	workspace := filepath.Join(t.TempDir(), "deleted-later")
	seedLocalChatRun(t, store, accountHome, workspace, "run-remap", []byte("session-body"))
	result, apiErr := svc.syncChatRunToDrive(context.Background(), "run-remap", ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}
	_ = os.RemoveAll(workspace)
	_, apiErr = svc.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
		ProjectID:       "project-1",
		SourceMachineID: result.SourceMachineID,
		SourceRunID:     result.SourceRunID,
	})
	if apiErr == nil || apiErr.code != "cwd_remap_required" {
		t.Fatalf("expected cwd_remap_required, got %#v", apiErr)
	}
}

func TestSyncChatHandlerReturnsTypedError(t *testing.T) {
	svc, instance, store, _, workspace, accountHome := newChatSyncService(t)
	seedLocalChatRun(t, store, accountHome, workspace, "run-http-sync", []byte("session-body"))
	if err := instance.saveArtifactStorageGoogleDriveState(func(current *artifactStorageGoogleDriveState) {
		delete(current.Connections, "project-1")
	}); err != nil {
		t.Fatalf("saveArtifactStorageGoogleDriveState: %v", err)
	}
	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()
	status, body := doJSON(t, http.MethodPost, server.URL+"/client/workflow-runs/run-http-sync/sync-chat", map[string]any{}, nil)
	if status != http.StatusConflict || !strings.Contains(string(body), `"google_drive_not_connected"`) {
		t.Fatalf("unexpected response: status=%d body=%s", status, body)
	}
}

func TestSyncChatHandlerReturnsSyncResult(t *testing.T) {
	svc, _, store, _, workspace, accountHome := newChatSyncService(t)
	seedLocalChatRun(t, store, accountHome, workspace, "run-http-sync-ok", []byte("session-body"))
	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()
	status, body := doJSON(t, http.MethodPost, server.URL+"/client/workflow-runs/run-http-sync-ok/sync-chat", map[string]any{}, nil)
	if status != http.StatusOK {
		t.Fatalf("unexpected response: status=%d body=%s", status, body)
	}
	if !strings.Contains(string(body), `"runId":"run-http-sync-ok"`) || !strings.Contains(string(body), `"syncStatus":"synced"`) {
		t.Fatalf("unexpected response body: %s", body)
	}
}

func TestRestoreChatHandlerReturnsTypedError(t *testing.T) {
	svc, _, store, _, _, accountHome := newChatSyncService(t)
	workspace := filepath.Join(t.TempDir(), "deleted")
	seedLocalChatRun(t, store, accountHome, workspace, "run-http-restore", []byte("session-body"))
	syncResult, apiErr := svc.syncChatRunToDrive(context.Background(), "run-http-restore", ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}
	_ = os.RemoveAll(workspace)
	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()
	status, body := doJSON(t, http.MethodPost, server.URL+"/client/chat-sessions/restore", map[string]any{
		"projectId":       "project-1",
		"sourceMachineId": syncResult.SourceMachineID,
		"sourceRunId":     syncResult.SourceRunID,
	}, nil)
	if status != http.StatusConflict || !strings.Contains(string(body), `"cwd_remap_required"`) {
		t.Fatalf("unexpected response: status=%d body=%s", status, body)
	}
}

func TestRestoreChatHandlerReturnsLocalRunID(t *testing.T) {
	svc, _, store, _, workspace, accountHome := newChatSyncService(t)
	seedLocalChatRun(t, store, accountHome, workspace, "run-http-restore-ok", []byte("session-body"))
	syncResult, apiErr := svc.syncChatRunToDrive(context.Background(), "run-http-restore-ok", ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}
	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()
	status, body := doJSON(t, http.MethodPost, server.URL+"/client/chat-sessions/restore", map[string]any{
		"projectId":       "project-1",
		"sourceMachineId": syncResult.SourceMachineID,
		"sourceRunId":     syncResult.SourceRunID,
		"cwd":             workspace,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("unexpected response: status=%d body=%s", status, body)
	}
	if !strings.Contains(string(body), `"runId":"run-http-restore-ok"`) || !strings.Contains(string(body), `"restoreStatus":"restored"`) {
		t.Fatalf("unexpected response body: %s", body)
	}
}

func TestListRemoteChatSessionsHandlerReturnsSummaries(t *testing.T) {
	svc, _, store, _, workspace, accountHome := newChatSyncService(t)
	seedLocalChatRun(t, store, accountHome, workspace, "run-http-list", []byte("session-body"))
	if _, apiErr := svc.syncChatRunToDrive(context.Background(), "run-http-list", ChatSessionSyncRequest{}); apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}
	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	server := httptest.NewServer(mux)
	defer server.Close()
	status, body := doJSON(t, http.MethodGet, server.URL+"/client/projects/project-1/chat-sessions/remote", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", status, body)
	}
	if !strings.Contains(string(body), `"sourceRunId":"run-http-list"`) || strings.Contains(string(body), "session-body") {
		t.Fatalf("unexpected response body: %s", body)
	}
}

// TestSyncChatRunWithStaleAccountIDBeforeOpening verifies that Drive sync succeeds
// even when the session's stored ProviderAccountID no longer exists in
// provider-accounts.json (e.g. after a missing-config restart regenerated IDs).
// This is the BUG-093 scenario: the session file is under "acct-current"'s home but
// the stored account ID is the stale "acct-stale". Sync must NOT require opening the
// chat first, and the repaired account ID must be persisted in the session store.
func TestSyncChatRunWithStaleAccountIDBeforeOpening(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GOOGLE_DRIVE_CLIENT_ID", "client-id")
	t.Setenv("GOOGLE_DRIVE_CLIENT_SECRET", "client-secret")
	t.Setenv("GOOGLE_DRIVE_REDIRECT_URI", "http://localhost/callback")
	t.Setenv("GOOGLE_PICKER_API_KEY", "picker-key")

	workspace := t.TempDir()
	instance, err := New(workspace)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	instance.secretStore = newMemorySecretStore()

	// provider-accounts.json contains "acct-current", not the stale "acct-stale".
	accountHome := filepath.Join(home, "codex-home")
	if err := os.MkdirAll(filepath.Join(accountHome, ".codex"), 0o755); err != nil {
		t.Fatalf("mkdir auth dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(accountHome, ".codex", "auth.json"), []byte(`{"id_token":"token"}`), 0o644); err != nil {
		t.Fatalf("write auth: %v", err)
	}
	if err := instance.saveProviderAccountState(providerAccountState{
		Accounts: []ProviderAccount{{
			ID:          "acct-current",
			ProviderKey: "codex",
			DisplayName: "Account 1",
			HomePath:    accountHome,
			SlotIndex:   1,
			IsActive:    true,
			AuthStatus:  "connected",
			CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		}},
	}); err != nil {
		t.Fatalf("saveProviderAccountState: %v", err)
	}
	if err := instance.saveGoogleDriveCredentialByProject("project-1", googleDriveCredential{RefreshToken: "refresh-token-1"}); err != nil {
		t.Fatalf("saveGoogleDriveCredentialByProject: %v", err)
	}
	if err := instance.saveArtifactStorageGoogleDriveState(func(current *artifactStorageGoogleDriveState) {
		current.Connections["project-1"] = artifactStorageGoogleDriveConnectionRecord{
			ProjectID:   "project-1",
			Status:      "connected",
			FolderID:    "drive-root",
			ConnectedAt: time.Now().UTC().Format(time.RFC3339Nano),
		}
	}); err != nil {
		t.Fatalf("saveArtifactStorageGoogleDriveState: %v", err)
	}

	store, err := NewLocalFileSessionStore(filepath.Join(workspace, ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}

	// Session file lives under "acct-current"'s home.
	sessionID := "session-stale-sync"
	sessionPath := filepath.Join(accountHome, "sessions", "2026", "06", "19", "rollout-local-"+sessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(sessionPath), 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	if err := os.WriteFile(sessionPath, []byte("{\"type\":\"session_meta\"}\n"), 0o644); err != nil {
		t.Fatalf("write session file: %v", err)
	}

	// Session store has the stale account ID.
	if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID:             "run-stale-sync",
		ProjectID:         "project-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: sessionID,
		ProviderAccountID: "acct-stale", // stale — not in provider-accounts.json
		WorkingDirectory:  workspace,
		Status:            RunStatusCompleted,
		RunKind:           "chat",
		StartedAt:         time.Now().UTC().Format(time.RFC3339Nano),
		UpdatedAt:         time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	svc := NewInteractiveServiceWithStore(DefaultProviderRegistry(), nil, store)
	svc.AttachRunner(instance)

	api := newFakeChatDriveAPI("drive-root")
	originalHTTPRequest := httpRequestFn
	httpRequestFn = api.handle
	t.Cleanup(func() { httpRequestFn = originalHTTPRequest })

	// Sync WITHOUT opening first — this is the BUG-093 scenario.
	result, apiErr := svc.syncChatRunToDrive(context.Background(), "run-stale-sync", ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive with stale account ID: code=%s message=%s", apiErr.code, apiErr.msg)
	}
	if result.RunID != "run-stale-sync" {
		t.Fatalf("syncChatRunToDrive RunID = %q, want run-stale-sync", result.RunID)
	}

	// Account ID must be repaired in the store after recovery.
	session, found, err := store.GetProviderSession(context.Background(), "run-stale-sync")
	if err != nil || !found {
		t.Fatalf("GetProviderSession = (%v, %v, %v)", session, found, err)
	}
	if session.ProviderAccountID != "acct-current" {
		t.Fatalf("repaired ProviderAccountID = %q, want acct-current", session.ProviderAccountID)
	}
}
