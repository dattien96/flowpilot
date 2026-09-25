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
	"sync"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
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
	mu          sync.Mutex
	nextID      int
	files       map[string]fakeChatDriveFile
	uploadOrder []string
	failUpload  bool
}

type recordingChatSessionStore struct {
	*localFileSessionStore
	upsertOrder []string
}

func (s *recordingChatSessionStore) UpsertProviderSession(ctx context.Context, session ProviderSessionState) error {
	if err := s.localFileSessionStore.UpsertProviderSession(ctx, session); err != nil {
		return err
	}
	s.upsertOrder = append(s.upsertOrder, session.RunID)
	return nil
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
	api.mu.Lock()
	defer api.mu.Unlock()
	switch {
	case endpoint == "https://oauth2.googleapis.com/token":
		return 200, []byte(`{"access_token":"drive-access-token"}`), nil
	case method == http.MethodGet && strings.HasPrefix(endpoint, "https://www.googleapis.com/drive/v3/files/") && strings.Contains(endpoint, "?alt=media"):
		return api.handleDownloadLocked(endpoint)
	case method == http.MethodGet && strings.HasPrefix(endpoint, "https://www.googleapis.com/drive/v3/files?"):
		return api.handleListLocked(endpoint)
	case strings.HasPrefix(endpoint, "https://www.googleapis.com/upload/drive/v3/files"):
		return api.handleUploadLocked(method, endpoint, headers, body)
	default:
		return 500, []byte(`{"error":"unexpected endpoint"}`), nil
	}
}

func (api *fakeChatDriveAPI) handleDownloadLocked(endpoint string) (int, []byte, error) {
	id := pathBaseWithoutQuery(endpoint)
	file, ok := api.files[id]
	if !ok {
		return 404, []byte(`{"error":"missing"}`), nil
	}
	// Copy content so concurrent readers are not affected by later upserts.
	out := append([]byte(nil), file.Content...)
	return 200, out, nil
}

func (api *fakeChatDriveAPI) handleListLocked(endpoint string) (int, []byte, error) {
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

func (api *fakeChatDriveAPI) handleUploadLocked(method, endpoint string, headers map[string]string, body []byte) (int, []byte, error) {
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
	// BUG-312: without these overrides, saveProviderAccountState (called below,
	// and again internally by ListProviderAccounts/resolveAccountHome on every
	// sync/restore) writes straight through to the REAL machine-wide
	// provider-accounts.json instead of a sandboxed path -- confirmed live,
	// this clobbered a real pre-existing account entry's id on the developer's
	// own machine. Overriding only HOME is not enough on Windows:
	// getPossibleHomeDirs() (used by ListProviderAccounts' auto-discovery) also
	// reads USERPROFILE and APPDATA directly, so without these too, discovery
	// still finds and re-registers whatever real provider accounts happen to
	// exist on the host. Every other test that registers provider accounts
	// (provider_accounts_test.go, grok_process_test.go, grok_registry_test.go,
	// cross_account_resume_test.go, bug083_test.go) already isolates all of
	// these the same way.
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH", filepath.Join(home, "provider-accounts.json"))
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
	// Timestamps must stay inside sessionStoreMaxAge (90d) or loadFromDisk prunes
	// the row on "restart" — a fixed date turns every seed into a time bomb.
	now := time.Now().UTC()
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
		StartedAt:         now.Add(-time.Hour).Format(time.RFC3339Nano),
		UpdatedAt:         now.Format(time.RFC3339Nano),
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

// TestBuildChatSessionSyncManifestAcceptsFlowHubWithPlaceholderSession replaces
// the old TestBuildChatSessionSyncManifestRejectsNonChatRun (Task-190 / CP-36
// P-5): a flow-engine (workflow) run must now sync, including the common case
// where its hub has no real provider transcript at all yet -- its own provider
// turn is deliberately suppressed while the flow runs (BUG-250), so
// provider_session_id never advances past the synthetic "thread-<n>"
// placeholder assigned at spawn. A "chat" run reaching that same placeholder
// state is a genuine anomaly and must still fail (session_unavailable) --
// unaffected by this test.
func TestBuildChatSessionSyncManifestAcceptsFlowHubWithPlaceholderSession(t *testing.T) {
	svc, _, store, _, workspace, _ := newChatSyncService(t)
	loopState := AgentLoopState{Status: "running", Round: 1, RoundCap: 3, ActiveNode: "coder"}
	state := ProviderSessionState{
		RunID:             "run-flow-hub",
		ProjectID:         "project-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: "thread-0",
		ProviderAccountID: "acct-sync",
		WorkingDirectory:  workspace,
		Status:            RunStatusRunning,
		RunKind:           "workflow",
		AutoOrchestrate:   true,
		FlowCohortID:      "flow-auto-coder-round-0",
		LoopState:         loopState,
		ActiveFlowNodes:   []agentpack.FlowNode{{ID: "coder", Run: "delegate"}},
	}
	if err := store.UpsertProviderSession(context.Background(), state); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}
	manifest, body, apiErr := svc.BuildChatSessionSyncManifest(context.Background(), state.RunID)
	if apiErr != nil {
		t.Fatalf("BuildChatSessionSyncManifest() failed: %v", apiErr)
	}
	if body != nil {
		t.Fatalf("expected nil provider body for a placeholder-session flow hub, got %d bytes", len(body))
	}
	if manifest.ProviderFile.RelativePath != "" || manifest.ProviderFile.SHA256 != "" {
		t.Fatalf("expected empty ProviderFile, got %#v", manifest.ProviderFile)
	}
	if manifest.RunKind != "workflow" || !manifest.AutoOrchestrate || manifest.FlowCohortID != "flow-auto-coder-round-0" {
		t.Fatalf("expected flow fields to carry through, got %#v", manifest)
	}
	if manifest.LoopState == nil || *manifest.LoopState != loopState {
		t.Fatalf("LoopState = %#v, want %#v", manifest.LoopState, loopState)
	}
	if len(manifest.ActiveFlowNodes) != 1 || manifest.ActiveFlowNodes[0].ID != "coder" {
		t.Fatalf("ActiveFlowNodes = %#v", manifest.ActiveFlowNodes)
	}
}

// TestBuildChatSessionSyncManifestChatRunStillRejectsPlaceholderSession keeps
// the pre-Task-190 strict behavior for a "chat" run: it must never have a
// placeholder-session escape hatch, since that state is only ever legitimate
// for a flow-engine hub (BUG-250).
func TestBuildChatSessionSyncManifestChatRunStillRejectsPlaceholderSession(t *testing.T) {
	svc, _, store, _, workspace, _ := newChatSyncService(t)
	state := ProviderSessionState{
		RunID:             "run-chat-placeholder",
		ProjectID:         "project-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: "thread-0",
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

func TestListRemoteChatSessionsHidesChildAgentRecords(t *testing.T) {
	svc, _, store, drive, workspace, accountHome := newChatSyncService(t)
	parent := seedLocalChatRun(t, store, accountHome, workspace, "run-parent", []byte("parent-session"))
	child := seedLocalChatRun(t, store, accountHome, workspace, "run-child", []byte("child-session"))
	child.ParentRunID = parent.RunID
	child.AgentName = "reviewer"
	child.Role = "reviewer"
	child.AgentStatus = string(RunStatusCompleted)
	if err := store.UpsertProviderSession(context.Background(), child); err != nil {
		t.Fatalf("UpsertProviderSession child: %v", err)
	}

	if _, apiErr := svc.syncChatRunToDrive(context.Background(), parent.RunID, ChatSessionSyncRequest{}); apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}

	// Simulate the BUG-119 index shape that existed before parent_run_id was added.
	for id, file := range drive.files {
		if file.Name != "sessions.ndjson" {
			continue
		}
		records := parseChatSessionDriveIndex(file.Content)
		lines := make([]string, 0, len(records))
		for i := range records {
			records[i].ParentRunID = ""
			line, err := json.Marshal(records[i])
			if err != nil {
				t.Fatalf("marshal legacy index record: %v", err)
			}
			lines = append(lines, string(line))
		}
		file.Content = []byte(strings.Join(lines, "\n") + "\n")
		drive.files[id] = file
	}

	summaries, apiErr := svc.listRemoteChatSessions(context.Background(), "project-1")
	if apiErr != nil {
		t.Fatalf("listRemoteChatSessions() failed: %v", apiErr)
	}
	if len(summaries) != 1 || summaries[0].SourceRunID != parent.RunID {
		t.Fatalf("remote summaries = %#v, want only parent", summaries)
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
	// UpdatedAt must be relative: seedLocalChatRun stamps the Codex record with
	// time.Now(), so a hardcoded Claude date silently drifts behind it and the
	// UpdatedAt-desc ordering assertion flips (same class as ece72e4d). The
	// remote record must be OLDER than the fresh local one — the assertion
	// below expects the local codex record to lead.
	indexFile := api.files[indexFileID]
	indexFile.Content = mergeChatSessionDriveIndex(indexFile.Content, chatSessionDriveIndexRecord{
		RunID:           "run-claude",
		ProjectID:       "project-id-from-another-pc",
		ProviderKey:     string(ProviderKeyClaude),
		RunKind:         "chat",
		SourceMachineID: "mch_remote",
		SourceRunID:     "run-claude",
		UpdatedAt:       time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano),
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
	// listRemoteChatSessions sorts by UpdatedAt desc — the remote claude
	// record is older, so the fresh local codex record leads.
	if summaries[0].ProviderKey != ProviderKeyCodex || summaries[1].ProviderKey != ProviderKeyClaude {
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
	restored, apiErr := svc.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
		ProjectID:       "project-1",
		SourceMachineID: result.SourceMachineID,
		SourceRunID:     result.SourceRunID,
		Cwd:             workspace,
	})
	if apiErr != nil {
		t.Fatalf("restoreChatRunFromDrive() failed without provider account: %v", apiErr)
	}
	session, found, err := store.GetProviderSession(context.Background(), restored.RunID)
	if err != nil || !found {
		t.Fatalf("GetProviderSession() failed: found=%v err=%v", found, err)
	}
	if session.ProviderAccountID != "default" {
		t.Fatalf("ProviderAccountID = %q, want default", session.ProviderAccountID)
	}
	defaultHome, ok := defaultProviderSessionHome(ProviderKeyCodex)
	if !ok {
		t.Fatal("expected default Codex session home")
	}
	if _, found := LocateSessionFile(ProviderKeyCodex, defaultHome, session.ProviderSessionID, workspace); !found {
		t.Fatal("expected restored session in the default Codex data home")
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
	restored, apiErr := svc.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
		ProjectID:       "project-1",
		SourceMachineID: result.SourceMachineID,
		SourceRunID:     result.SourceRunID,
		Cwd:             workspace,
	})
	if apiErr != nil {
		t.Fatalf("restoreChatRunFromDrive() failed without provider auth: %v", apiErr)
	}
	if _, apiErr := svc.resumeRun(restored.RunID); apiErr != nil {
		t.Fatalf("resumeRun() should open restored chat read-only without provider auth: %v", apiErr)
	}
}

func TestDefaultProviderSessionHomeSupportsClaudeWithoutInstallation(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// Windows preferredUserHomeDir reads USERPROFILE first.
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")

	targetHome, ok := defaultProviderSessionHome(ProviderKeyClaude)
	if !ok {
		t.Fatal("expected default Claude session home")
	}
	if targetHome != home {
		t.Fatalf("default Claude session home = %q, want %q", targetHome, home)
	}

	sessionID := "claude-restored-session"
	relativePath := filepath.ToSlash(filepath.Join(".claude", "projects", "restored-project", sessionID+".jsonl"))
	body := []byte(`{"type":"user","message":{"role":"user","content":"restored prompt"}}` + "\n")
	restoredPath, err := RestoreSessionFile(ProviderKeyClaude, targetHome, relativePath, sessionID, t.TempDir(), body)
	if err != nil {
		t.Fatalf("RestoreSessionFile() failed without Claude installation: %v", err)
	}
	if got, err := os.ReadFile(restoredPath); err != nil || !bytes.Equal(got, body) {
		t.Fatalf("restored Claude session = %q, err=%v", got, err)
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

func TestRestoreChatRunFromDriveRemapsChildParentRunIDOnCollision(t *testing.T) {
	svc, _, store, _, workspace, accountHome := newChatSyncService(t)
	parent := seedLocalChatRun(t, store, accountHome, workspace, "run-collision", []byte("session-body"))
	child := seedLocalChatRun(t, store, accountHome, workspace, "child-1", []byte("child-session-body"))
	child.ParentRunID = parent.RunID
	child.AgentName = "coder"
	child.Role = "coder"
	child.AgentStatus = string(RunStatusCompleted)
	if err := store.UpsertProviderSession(context.Background(), child); err != nil {
		t.Fatalf("UpsertProviderSession child: %v", err)
	}
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

	summaries := svc.listAgentRunSummaries(restored.RunID)
	if len(summaries) != 1 {
		t.Fatalf("listAgentRunSummaries() = %d, want 1", len(summaries))
	}
	if summaries[0].ParentRunID != restored.RunID {
		t.Fatalf("child ParentRunID = %q, want %q", summaries[0].ParentRunID, restored.RunID)
	}
}

// TestSyncAndRestoreFlowRunRoundTripAppliesFlowStateAndNormalizesStatus is
// Task-190's core regression guard for CP-36 Scenario 6 (Drive Sync Cross-PC).
// It syncs a flow-engine hub run -- whose own provider turn is still the
// synthetic placeholder (BUG-250), so it has no transcript -- together with a
// genuinely "running" child, then restores both on a fresh store/service
// (Machine B). It asserts: (1) the sync/restore succeeds with no transcript for
// the hub, (2) flow runtime state (LoopState/ActiveFlowNodes/AutoOrchestrate/
// FlowCohortID) round-trips so the board can render round/children/status, and
// (3) both the hub's and the child's in-flight "running" status are normalized
// to "cancelled" on restore (BUG-251 parity) -- nothing is actually running
// them on the machine that just restored the snapshot.
func TestSyncAndRestoreFlowRunRoundTripAppliesFlowStateAndNormalizesStatus(t *testing.T) {
	svc, instance, sourceStore, _, workspace, accountHome := newChatSyncService(t)

	loopState := AgentLoopState{Status: "running", Round: 1, RoundCap: 3, ActiveNode: "reviewer_correctness"}
	hub := ProviderSessionState{
		RunID:             "run-flow-hub-roundtrip",
		ProjectID:         "project-1",
		ProviderKey:       ProviderKeyCodex,
		ProviderSessionID: "thread-0",
		ProviderAccountID: "acct-sync",
		WorkingDirectory:  workspace,
		Status:            RunStatusRunning,
		RunKind:           "workflow",
		AutoOrchestrate:   true,
		FlowCohortID:      "flow-auto-coder-round-0",
		LoopState:         loopState,
		ActiveFlowNodes:   []agentpack.FlowNode{{ID: "coder", Run: "delegate"}, {ID: "reviewer_correctness", Run: "delegate"}},
		ActiveFlowEdges:   []agentpack.FlowEdge{{From: "coder", To: "reviewer_correctness"}},
	}
	if err := sourceStore.UpsertProviderSession(context.Background(), hub); err != nil {
		t.Fatalf("UpsertProviderSession hub: %v", err)
	}

	child := seedLocalChatRun(t, sourceStore, accountHome, workspace, "run-flow-reviewer-roundtrip", []byte("reviewer-session"))
	child.ParentRunID = hub.RunID
	child.AgentName = "reviewer_correctness"
	child.Role = "reviewer"
	child.Status = RunStatusRunning
	child.AgentStatus = string(RunStatusRunning)
	if err := sourceStore.UpsertProviderSession(context.Background(), child); err != nil {
		t.Fatalf("UpsertProviderSession child: %v", err)
	}

	synced, apiErr := svc.syncChatRunToDrive(context.Background(), hub.RunID, ChatSessionSyncRequest{})
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
		t.Fatalf("restoreChatRunFromDrive() failed: %v", apiErr)
	}

	restoredHub, found, err := restoredStore.GetProviderSession(context.Background(), restored.RunID)
	if err != nil || !found {
		t.Fatalf("GetProviderSession(hub) failed: found=%v err=%v", found, err)
	}
	if restoredHub.RunKind != "workflow" {
		t.Fatalf("restored hub RunKind = %q, want workflow", restoredHub.RunKind)
	}
	if restoredHub.Status != RunStatusCancelled {
		t.Fatalf("restored hub Status = %q, want cancelled (BUG-251 parity)", restoredHub.Status)
	}
	if !restoredHub.AutoOrchestrate || restoredHub.FlowCohortID != "flow-auto-coder-round-0" {
		t.Fatalf("restored hub flow fields = %#v", restoredHub)
	}
	if restoredHub.LoopState != loopState {
		t.Fatalf("restored hub LoopState = %#v, want %#v", restoredHub.LoopState, loopState)
	}
	if len(restoredHub.ActiveFlowNodes) != 2 || len(restoredHub.ActiveFlowEdges) != 1 {
		t.Fatalf("restored hub flow topology = %#v", restoredHub)
	}

	sessions, err := restoredStore.ListAllProviderSessions(context.Background())
	if err != nil {
		t.Fatalf("ListAllProviderSessions: %v", err)
	}
	var restoredChild ProviderSessionState
	for _, session := range sessions {
		if session.ParentRunID == restored.RunID {
			restoredChild = session
			break
		}
	}
	if restoredChild.RunID == "" {
		t.Fatalf("restored sessions = %#v, want a persisted child of %q", sessions, restored.RunID)
	}
	if restoredChild.Status != RunStatusCancelled {
		t.Fatalf("restored child Status = %q, want cancelled (BUG-251 parity)", restoredChild.Status)
	}
	if restoredChild.AgentStatus != string(RunStatusCancelled) {
		t.Fatalf("restored child AgentStatus = %q, want cancelled (BUG-251 parity)", restoredChild.AgentStatus)
	}
}

func TestRestoreParentChatRestoresChildrenAndPersistsAgentTree(t *testing.T) {
	svc, instance, sourceStore, _, workspace, accountHome := newChatSyncService(t)
	parent := seedLocalChatRun(t, sourceStore, accountHome, workspace, "run-parent-tree", []byte("parent-session"))
	child := seedLocalChatRun(t, sourceStore, accountHome, workspace, "run-child-tree", []byte("child-session"))
	child.ParentRunID = parent.RunID
	child.AgentName = "coder"
	child.Role = "coder"
	child.AgentStatus = string(RunStatusCompleted)
	child.ModelName = "gpt-5-codex"
	if err := sourceStore.UpsertProviderSession(context.Background(), child); err != nil {
		t.Fatalf("UpsertProviderSession child: %v", err)
	}

	synced, apiErr := svc.syncChatRunToDrive(context.Background(), parent.RunID, ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}

	restoredStoreBase, err := NewLocalFileSessionStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore restored: %v", err)
	}
	restoredStore := &recordingChatSessionStore{localFileSessionStore: restoredStoreBase}
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
		t.Fatalf("restoreChatRunFromDrive() failed: %v", apiErr)
	}

	sessions, err := restoredStore.ListAllProviderSessions(context.Background())
	if err != nil {
		t.Fatalf("ListAllProviderSessions: %v", err)
	}
	var restoredChild ProviderSessionState
	for _, session := range sessions {
		if session.ParentRunID == restored.RunID {
			restoredChild = session
			break
		}
	}
	if restoredChild.RunID == "" {
		t.Fatalf("restored sessions = %#v, want persisted child of %q", sessions, restored.RunID)
	}
	if restoredChild.AgentName != "coder" || restoredChild.Role != "coder" || restoredChild.ModelName != "gpt-5-codex" {
		t.Fatalf("restored child metadata = %#v", restoredChild)
	}
	if len(restoredStore.upsertOrder) == 0 || restoredStore.upsertOrder[len(restoredStore.upsertOrder)-1] != restored.RunID {
		t.Fatalf("session publication order = %#v, want parent %q last", restoredStore.upsertOrder, restored.RunID)
	}
	if _, apiErr := restoredService.resumeRun(restoredChild.RunID); apiErr != nil {
		t.Fatalf("resumeRun(restored child) failed: %v", apiErr)
	}

	restartedService := NewInteractiveServiceWithStore(DefaultProviderRegistry(), nil, restoredStore)
	summaries := restartedService.listAgentRunSummaries(restored.RunID)
	if len(summaries) != 1 || summaries[0].RunID != restoredChild.RunID || summaries[0].ParentRunID != restored.RunID {
		t.Fatalf("agent summaries after restart = %#v", summaries)
	}
}

func TestRestoreParentChatDoesNotPublishMainHistoryWhenChildRestoreFails(t *testing.T) {
	svc, instance, sourceStore, drive, workspace, accountHome := newChatSyncService(t)
	parent := seedLocalChatRun(t, sourceStore, accountHome, workspace, "run-parent-failed-child", []byte("parent-session"))
	child := seedLocalChatRun(t, sourceStore, accountHome, workspace, "run-child-fails", []byte("child-session"))
	child.ParentRunID = parent.RunID
	child.AgentName = "reviewer"
	if err := sourceStore.UpsertProviderSession(context.Background(), child); err != nil {
		t.Fatalf("UpsertProviderSession child: %v", err)
	}
	synced, apiErr := svc.syncChatRunToDrive(context.Background(), parent.RunID, ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}
	childFileID := remoteProviderFileID(drive, "rollout-local-session-run-child-fails.jsonl")
	if childFileID == "" {
		t.Fatal("expected child provider file")
	}
	delete(drive.files, childFileID)

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
		t.Fatalf("restoreChatRunFromDrive() error = %#v, want child sync_remote_not_found", apiErr)
	}
	history, err := restoredStore.ListProviderSessionsByProject(context.Background(), "project-1")
	if err != nil {
		t.Fatalf("ListProviderSessionsByProject: %v", err)
	}
	if len(history) != 0 {
		t.Fatalf("main history = %#v, want empty until every child restores", history)
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
	// BUG-312: without these overrides, saveProviderAccountState (called below,
	// and again internally by ListProviderAccounts/resolveAccountHome on every
	// sync/restore) writes straight through to the REAL machine-wide
	// provider-accounts.json instead of a sandboxed path -- confirmed live,
	// this clobbered a real pre-existing account entry's id on the developer's
	// own machine. Overriding only HOME is not enough on Windows:
	// getPossibleHomeDirs() (used by ListProviderAccounts' auto-discovery) also
	// reads USERPROFILE and APPDATA directly, so without these too, discovery
	// still finds and re-registers whatever real provider accounts happen to
	// exist on the host. Every other test that registers provider accounts
	// (provider_accounts_test.go, grok_process_test.go, grok_registry_test.go,
	// cross_account_resume_test.go, bug083_test.go) already isolates all of
	// these the same way.
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	t.Setenv("FLOWPILOT_PROVIDER_ACCOUNTS_CONFIG_PATH", filepath.Join(home, "provider-accounts.json"))
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

// plantDriveRunManifest inserts a run folder + manifest.json under the chat
// sync root without going through syncChatRunToDrive (orphaned blob case).
func plantDriveRunManifest(t *testing.T, api *fakeChatDriveAPI, rootID string, manifest ChatSessionSyncManifest) {
	t.Helper()
	api.mu.Lock()
	defer api.mu.Unlock()
	ensure := func(parentID, name string) string {
		for _, f := range api.files {
			if f.ParentID == parentID && f.Name == name {
				return f.ID
			}
		}
		api.nextID++
		id := "plant-" + strconv.Itoa(api.nextID)
		api.files[id] = fakeChatDriveFile{
			ID:       id,
			Name:     name,
			ParentID: parentID,
			MimeType: googleDriveFolderMimeType,
		}
		return id
	}
	chatSessions := ensure(rootID, "chat-sessions")
	runs := ensure(chatSessions, "runs")
	machine := ensure(runs, safeChatSessionSegment(manifest.SourceMachineID))
	runFolder := ensure(machine, safeChatSessionSegment(manifest.SourceRunID))
	body, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal plant manifest: %v", err)
	}
	api.nextID++
	mid := "plant-manifest-" + strconv.Itoa(api.nextID)
	api.files[mid] = fakeChatDriveFile{
		ID:       mid,
		Name:     "manifest.json",
		ParentID: runFolder,
		MimeType: "application/json",
		Content:  body,
	}
}

func TestSyncChatRunToDriveConcurrentIndexMergesPreserveAllParentRows(t *testing.T) {
	svc, _, store, api, workspace, accountHome := newChatSyncService(t)
	const n = 8
	for i := 0; i < n; i++ {
		seedLocalChatRun(t, store, accountHome, workspace, "run-concurrent-"+strconv.Itoa(i), []byte("body-"+strconv.Itoa(i)))
	}

	var wg sync.WaitGroup
	errs := make(chan *apiErr, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, apiErr := svc.syncChatRunToDrive(context.Background(), "run-concurrent-"+strconv.Itoa(i), ChatSessionSyncRequest{}); apiErr != nil {
				errs <- apiErr
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for apiErr := range errs {
		t.Fatalf("syncChatRunToDrive concurrent failed: %s %s", apiErr.code, apiErr.msg)
	}

	api.mu.Lock()
	var index []byte
	for _, file := range api.files {
		if file.Name == "sessions.ndjson" {
			index = append([]byte(nil), file.Content...)
			break
		}
	}
	api.mu.Unlock()
	if len(index) == 0 {
		t.Fatal("expected sessions.ndjson after concurrent syncs")
	}
	records := parseChatSessionDriveIndex(index)
	if len(records) != n {
		t.Fatalf("index rows = %d, want %d (last-write-wins race?)", len(records), n)
	}
	seen := map[string]bool{}
	for _, r := range records {
		seen[r.SourceRunID] = true
	}
	for i := 0; i < n; i++ {
		id := "run-concurrent-" + strconv.Itoa(i)
		if !seen[id] {
			t.Fatalf("missing %s in index after concurrent sync", id)
		}
	}
}

func TestListRemoteChatSessionsRepairsIndexFromOrphanedRunManifests(t *testing.T) {
	svc, _, _, api, _, _ := newChatSyncService(t)
	// Index intentionally empty / missing; blobs exist under runs/ (Windows-style under-list).
	plantDriveRunManifest(t, api, "drive-root", ChatSessionSyncManifest{
		SchemaVersion:   1,
		SourceMachineID: "mch_c3ab_orphan",
		SourceRunID:     "run-orphan-1",
		ProjectID:       "project-1",
		ProviderKey:     ProviderKeyGrok,
		RunKind:         "chat",
		Status:          "completed",
		LastPrompt:      "turn trước fix gì vậy",
		SyncedAt:        "2026-07-22T07:16:54Z",
		UpdatedAt:       "2026-07-22T07:16:54Z",
	})
	plantDriveRunManifest(t, api, "drive-root", ChatSessionSyncManifest{
		SchemaVersion:   1,
		SourceMachineID: "mch_c3ab_orphan",
		SourceRunID:     "run-orphan-2",
		ProjectID:       "project-1",
		ProviderKey:     ProviderKeyCodex,
		RunKind:         "chat",
		Status:          "completed",
		LastPrompt:      "second orphaned chat",
		SyncedAt:        "2026-07-22T08:00:00Z",
		UpdatedAt:       "2026-07-22T08:00:00Z",
	})

	summaries, apiErr := svc.listRemoteChatSessions(context.Background(), "project-1")
	if apiErr != nil {
		t.Fatalf("listRemoteChatSessions: %v", apiErr)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 repaired remote rows, got %#v", summaries)
	}
	ids := map[string]bool{}
	for _, s := range summaries {
		ids[s.SourceRunID] = true
	}
	if !ids["run-orphan-1"] || !ids["run-orphan-2"] {
		t.Fatalf("repaired summaries missing orphans: %#v", summaries)
	}

	// Repair must write sessions.ndjson so subsequent list is index-backed.
	api.mu.Lock()
	var index []byte
	for _, file := range api.files {
		if file.Name == "sessions.ndjson" {
			index = append([]byte(nil), file.Content...)
			break
		}
	}
	api.mu.Unlock()
	if len(parseChatSessionDriveIndex(index)) != 2 {
		t.Fatalf("expected repaired index with 2 rows, got %q", string(index))
	}
}

func TestListRemoteChatSessionsHidesChildrenButKeepsSiblingParentsAfterRepair(t *testing.T) {
	svc, _, _, api, _, _ := newChatSyncService(t)
	plantDriveRunManifest(t, api, "drive-root", ChatSessionSyncManifest{
		SchemaVersion:   1,
		SourceMachineID: "mch_win",
		SourceRunID:     "run-parent-a",
		ProjectID:       "project-1",
		ProviderKey:     ProviderKeyGrok,
		RunKind:         "chat",
		LastPrompt:      "parent a",
		SyncedAt:        "2026-07-22T01:00:00Z",
		UpdatedAt:       "2026-07-22T01:00:00Z",
		ChildAgents:     []AgentRunSummary{{RunID: "run-child-a1", AgentName: "coder"}},
	})
	plantDriveRunManifest(t, api, "drive-root", ChatSessionSyncManifest{
		SchemaVersion:   1,
		SourceMachineID: "mch_win",
		SourceRunID:     "run-child-a1",
		ProjectID:       "project-1",
		ProviderKey:     ProviderKeyGrok,
		RunKind:         "chat",
		ParentRunID:     "run-parent-a",
		LastPrompt:      "child a1",
		SyncedAt:        "2026-07-22T01:01:00Z",
		UpdatedAt:       "2026-07-22T01:01:00Z",
	})
	plantDriveRunManifest(t, api, "drive-root", ChatSessionSyncManifest{
		SchemaVersion:   1,
		SourceMachineID: "mch_win",
		SourceRunID:     "run-parent-b",
		ProjectID:       "project-1",
		ProviderKey:     ProviderKeyCodex,
		RunKind:         "chat",
		LastPrompt:      "parent b",
		SyncedAt:        "2026-07-22T02:00:00Z",
		UpdatedAt:       "2026-07-22T02:00:00Z",
	})

	summaries, apiErr := svc.listRemoteChatSessions(context.Background(), "project-1")
	if apiErr != nil {
		t.Fatalf("listRemoteChatSessions: %v", apiErr)
	}
	if len(summaries) != 2 {
		t.Fatalf("want 2 top-level parents, got %#v", summaries)
	}
	for _, s := range summaries {
		if s.SourceRunID == "run-child-a1" {
			t.Fatalf("child must stay hidden from REMOTE CHATS: %#v", summaries)
		}
	}
	ids := map[string]bool{}
	for _, s := range summaries {
		ids[s.SourceRunID] = true
	}
	if !ids["run-parent-a"] || !ids["run-parent-b"] {
		t.Fatalf("sibling parents missing: %#v", summaries)
	}
}

func TestListRemoteChatSessionsReturnsAllTopLevelRowsAfterMultiRunSync(t *testing.T) {
	svc, _, store, _, workspace, accountHome := newChatSyncService(t)
	for _, id := range []string{"run-multi-1", "run-multi-2", "run-multi-3"} {
		seedLocalChatRun(t, store, accountHome, workspace, id, []byte("body-"+id))
		if _, apiErr := svc.syncChatRunToDrive(context.Background(), id, ChatSessionSyncRequest{}); apiErr != nil {
			t.Fatalf("sync %s: %v", id, apiErr)
		}
	}
	summaries, apiErr := svc.listRemoteChatSessions(context.Background(), "project-1")
	if apiErr != nil {
		t.Fatalf("listRemoteChatSessions: %v", apiErr)
	}
	if len(summaries) != 3 {
		t.Fatalf("expected 3 top-level remote rows after sequential multi-run sync, got %#v", summaries)
	}
}

// TestRestoreChatRunFromDriveGrokRecomputesPathForDifferentCwd is the
// end-to-end regression proof for BUG-312: restoreTargetPath previously had no
// case for ProviderKeyGrok at all, so every Grok restore failed with
// sync_integrity_failed ("remote provider session file path is invalid") --
// confirmed live against the running dev server before this fix. This test
// syncs a Grok chat from one cwd (Mac-shaped, matching this project's real
// Codex machine) and restores it under a DIFFERENT cwd (a fresh temp dir,
// standing in for "a different machine"), then verifies the restored session
// is actually discoverable via LocateSessionFile under the NEW cwd -- not just
// that restore returned no error.
func TestRestoreChatRunFromDriveGrokRecomputesPathForDifferentCwd(t *testing.T) {
	svc, instance, store, _, _, accountHome := newChatSyncService(t)
	grokHome := filepath.Join(filepath.Dir(accountHome), "grok-home")
	if err := instance.saveProviderAccountState(providerAccountState{
		Accounts: []ProviderAccount{{
			ID:          "acct-grok-sync",
			ProviderKey: "grok",
			DisplayName: "Grok Account 1",
			HomePath:    grokHome,
			SlotIndex:   1,
			IsActive:    true,
			AuthStatus:  "connected",
			CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		}},
	}); err != nil {
		t.Fatalf("saveProviderAccountState (grok): %v", err)
	}
	// ListProviderAccounts' sync pass re-validates every registered account's
	// AuthStatus against its home's auth.json on every call (accountAuthPaths /
	// hasValidProviderAuthFile) -- without a plausible one here, it silently
	// downgrades this account to "failed", which then makes ResolveProviderAccount
	// skip it and fall back to whatever the last-active OTHER provider's account
	// ID was, resolving the wrong target home entirely at restore time.
	if err := os.MkdirAll(grokHome, 0o755); err != nil {
		t.Fatalf("MkdirAll grokHome: %v", err)
	}
	if err := os.WriteFile(filepath.Join(grokHome, "auth.json"), []byte(`{"issuer::user-grok-sync":{"refresh_token":"rt","email":"grok-sync@example.com"}}`), 0o644); err != nil {
		t.Fatalf("write grok auth.json: %v", err)
	}

	const sourceCwd = "/Users/tiendat/Desktop/BE/gate-sandbox"
	const sessionID = "019f8722-c5fc-7f11-8d9e-4154bf38d338"
	const transcript = `{"type":"assistant","content":"hello from grok"}` + "\n"
	writeGrokSessionTree(t, grokHome, sourceCwd, sessionID, map[string]string{"chat_history.jsonl": transcript})

	state := ProviderSessionState{
		RunID:             "run-grok-restore",
		ProjectID:         "project-1",
		ProviderKey:       ProviderKeyGrok,
		ProviderSessionID: sessionID,
		ProviderAccountID: "acct-grok-sync",
		WorkingDirectory:  sourceCwd,
		Status:            RunStatusCompleted,
		LastPrompt:        "hi",
		LastMessage:       "hello from grok",
		StartedAt:         "2026-07-22T10:00:00Z",
		UpdatedAt:         "2026-07-22T10:05:00Z",
		RunKind:           "chat",
	}
	if err := store.UpsertProviderSession(context.Background(), state); err != nil {
		t.Fatalf("UpsertProviderSession: %v", err)
	}

	result, apiErr := svc.syncChatRunToDrive(context.Background(), "run-grok-restore", ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}

	targetCwd := filepath.Join(t.TempDir(), "restored-on-a-different-machine")
	if err := os.MkdirAll(targetCwd, 0o755); err != nil {
		t.Fatalf("MkdirAll target cwd: %v", err)
	}
	restored, apiErr := svc.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
		ProjectID:       "project-1",
		SourceMachineID: result.SourceMachineID,
		SourceRunID:     result.SourceRunID,
		Cwd:             targetCwd,
	})
	if apiErr != nil {
		t.Fatalf("BUG-312: restoreChatRunFromDrive() failed for Grok: %#v", apiErr)
	}
	if restored.RunID == "" {
		t.Fatal("expected a non-empty restored run id")
	}

	foundPath, found := LocateSessionFile(ProviderKeyGrok, grokHome, sessionID, targetCwd)
	if !found {
		t.Fatal("restored Grok session is not discoverable via LocateSessionFile under the new cwd")
	}
	body, err := os.ReadFile(filepath.Join(foundPath, "chat_history.jsonl"))
	if err != nil {
		t.Fatalf("read restored transcript: %v", err)
	}
	if string(body) != transcript {
		t.Fatalf("restored transcript = %q, want %q", body, transcript)
	}
}

// --- BUG-313: the sync manifest must carry the run's durable turn log, and
// restore must rebuild the local <runID>-turns.ndjson sidecar from it. The
// entire post-restart timeline reconstruction (seedTranscriptFromDisk /
// seedGrokTranscriptFromDisk / preferFlowHubTurnLogTranscript) is driven by
// that sidecar: raw user prompts (F-1), flow-hub prose (transcript_turn /
// assistant frames), per-turn provider session-id chains, and the empty-
// OccurredAt clustering that keeps agent cards beside their turns. Restoring
// a run without it reproduced the live defect: prompts gone, hub text wrong
// (cwd-wide session-dir fallback mixing other runs), agent cards dumped at
// the bottom. Live-confirmed on run-24345 (Gate-sandbox, 2026-07-23).

func bug313TurnLogEntries(prompt string) []turnLogLine {
	return []turnLogLine{
		{Kind: turnLogKindPrompt, TurnID: "turn-1", Prompt: prompt},
		{Kind: turnLogKindTranscriptTurn, TurnID: "turn-1", Prompt: prompt, Assistant: "hub synthesis answer"},
		{Kind: turnLogKindAssistant, TurnID: "turn-2", Assistant: "follow-up durable frame"},
	}
}

func appendBug313TurnLog(t *testing.T, store *localFileSessionStore, runID string, entries []turnLogLine) {
	t.Helper()
	for _, line := range entries {
		if err := store.AppendTurnLog(context.Background(), runID, line); err != nil {
			t.Fatalf("AppendTurnLog: %v", err)
		}
	}
}

func mustTurnLogJSON(t *testing.T, entries []turnLogLine) string {
	t.Helper()
	raw, err := json.Marshal(entries)
	if err != nil {
		t.Fatalf("marshal turn log: %v", err)
	}
	return string(raw)
}

// TestSyncChatRunToDriveManifestCarriesTurnLog asserts the uploaded
// manifest.json embeds the run's turn log verbatim. Deliberately inspects the
// raw JSON (not the Go struct) so this test compiles and FAILS on the pre-fix
// baseline, where the field does not exist.
func TestSyncChatRunToDriveManifestCarriesTurnLog(t *testing.T) {
	svc, _, store, api, workspace, accountHome := newChatSyncService(t)
	seedLocalChatRun(t, store, accountHome, workspace, "run-turnlog-sync", []byte("codex-session"))
	entries := bug313TurnLogEntries("what did the last turn fix?")
	appendBug313TurnLog(t, store, "run-turnlog-sync", entries)

	if _, apiErr := svc.syncChatRunToDrive(context.Background(), "run-turnlog-sync", ChatSessionSyncRequest{}); apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}
	manifestID := remoteManifestFileID(api)
	if manifestID == "" {
		t.Fatal("expected manifest upload")
	}
	var manifest map[string]any
	if err := json.Unmarshal(api.files[manifestID].Content, &manifest); err != nil {
		t.Fatalf("manifest json invalid: %v", err)
	}
	rawLog, ok := manifest["turnLog"].([]any)
	if !ok || len(rawLog) != len(entries) {
		t.Fatalf("BUG-313: manifest turnLog = %#v, want %d entries", manifest["turnLog"], len(entries))
	}
	first, _ := rawLog[0].(map[string]any)
	if first["kind"] != string(turnLogKindPrompt) || first["prompt"] != entries[0].Prompt {
		t.Fatalf("BUG-313: first turnLog entry = %#v, want raw prompt %q", first, entries[0].Prompt)
	}
}

// TestRestoreChatRunFromDriveRebuildsTurnLogSidecarAllProviders proves the
// sync->restore round trip rebuilds the turn-log sidecar identically for
// Codex, Claude, AND Grok (cross-provider parity rule) using the flow-hub
// shape (runKind=workflow, synthetic thread-* session, no provider file) --
// the exact class the live defect was reported on, and a shape whose timeline
// is rebuilt from the turn log ALONE.
func TestRestoreChatRunFromDriveRebuildsTurnLogSidecarAllProviders(t *testing.T) {
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
		provider  ProviderKey
		accountID string
		runID     string
	}{
		{ProviderKeyCodex, "acct-sync", "run-hub-codex"},
		{ProviderKeyClaude, "acct-claude", "run-hub-claude"},
		{ProviderKeyGrok, "acct-grok", "run-hub-grok"},
	}
	for _, tc := range cases {
		t.Run(string(tc.provider), func(t *testing.T) {
			state := ProviderSessionState{
				RunID:             tc.runID,
				ProjectID:         "project-1",
				WorkflowID:        "wf-" + tc.runID,
				ProviderKey:       tc.provider,
				ProviderSessionID: "thread-1",
				ProviderAccountID: tc.accountID,
				WorkingDirectory:  workspace,
				Status:            RunStatusCompleted,
				LastPrompt:        "fix bug 1+1 != 2",
				LastMessage:       "hub synthesis answer",
				StartedAt:         "2026-07-22T10:00:00Z",
				UpdatedAt:         "2026-07-22T10:05:00Z",
				RunKind:           "workflow",
			}
			if err := store.UpsertProviderSession(context.Background(), state); err != nil {
				t.Fatalf("UpsertProviderSession: %v", err)
			}
			entries := bug313TurnLogEntries("fix bug 1+1 != 2")
			appendBug313TurnLog(t, store, tc.runID, entries)

			result, apiErr := svc.syncChatRunToDrive(context.Background(), tc.runID, ChatSessionSyncRequest{})
			if apiErr != nil {
				t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
			}
			// Simulate the restoring machine: no local sidecar for this run.
			if err := store.DeleteTurnLog(context.Background(), tc.runID); err != nil {
				t.Fatalf("DeleteTurnLog: %v", err)
			}
			restored, apiErr := svc.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
				ProjectID:       "project-1",
				SourceMachineID: result.SourceMachineID,
				SourceRunID:     result.SourceRunID,
				Cwd:             workspace,
			})
			if apiErr != nil {
				t.Fatalf("restoreChatRunFromDrive() failed: %#v", apiErr)
			}
			got, err := store.ReadTurnLog(context.Background(), restored.RunID)
			if err != nil {
				t.Fatalf("ReadTurnLog: %v", err)
			}
			if mustTurnLogJSON(t, got) != mustTurnLogJSON(t, entries) {
				t.Fatalf("BUG-313 (%s): restored turn log = %s, want %s", tc.provider, mustTurnLogJSON(t, got), mustTurnLogJSON(t, entries))
			}
		})
	}
}

// TestRestoreChatRunFromDriveDoesNotDuplicateTurnLogSidecar guards the
// idempotency rule: restoring onto a machine that still has the run's original
// sidecar must not append a second copy of every line, and a re-restore after
// the sidecar is wiped rebuilds it exactly once.
func TestRestoreChatRunFromDriveDoesNotDuplicateTurnLogSidecar(t *testing.T) {
	svc, _, store, _, workspace, accountHome := newChatSyncService(t)
	seedLocalChatRun(t, store, accountHome, workspace, "run-turnlog-nodup", []byte("codex-session"))
	entries := bug313TurnLogEntries("original raw prompt")
	appendBug313TurnLog(t, store, "run-turnlog-nodup", entries)

	result, apiErr := svc.syncChatRunToDrive(context.Background(), "run-turnlog-nodup", ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive() failed: %v", apiErr)
	}
	// Restore while the original sidecar is still present -- must not duplicate.
	restored, apiErr := svc.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
		ProjectID:       "project-1",
		SourceMachineID: result.SourceMachineID,
		SourceRunID:     result.SourceRunID,
		Cwd:             workspace,
	})
	if apiErr != nil {
		t.Fatalf("restoreChatRunFromDrive() failed: %#v", apiErr)
	}
	got, err := store.ReadTurnLog(context.Background(), restored.RunID)
	if err != nil {
		t.Fatalf("ReadTurnLog: %v", err)
	}
	if len(got) != len(entries) {
		t.Fatalf("restore duplicated the existing sidecar: %d entries, want %d", len(got), len(entries))
	}
	// Wipe and restore again -- rebuilds exactly once.
	if err := store.DeleteTurnLog(context.Background(), restored.RunID); err != nil {
		t.Fatalf("DeleteTurnLog: %v", err)
	}
	restored2, apiErr := svc.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
		ProjectID:       "project-1",
		SourceMachineID: result.SourceMachineID,
		SourceRunID:     result.SourceRunID,
		Cwd:             workspace,
	})
	if apiErr != nil {
		t.Fatalf("second restoreChatRunFromDrive() failed: %#v", apiErr)
	}
	got2, err := store.ReadTurnLog(context.Background(), restored2.RunID)
	if err != nil {
		t.Fatalf("ReadTurnLog after rebuild: %v", err)
	}
	if mustTurnLogJSON(t, got2) != mustTurnLogJSON(t, entries) {
		t.Fatalf("rebuilt turn log = %s, want %s", mustTurnLogJSON(t, got2), mustTurnLogJSON(t, entries))
	}
}
