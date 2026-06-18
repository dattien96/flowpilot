package runner

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const chatSessionManifestSchemaVersion = 1

var chatSessionSafeSegmentPattern = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

type ChatSessionSyncManifest struct {
	SchemaVersion     int               `json:"schemaVersion"`
	SourceMachineID   string            `json:"sourceMachineId"`
	SourceRunID       string            `json:"sourceRunId"`
	ProjectID         string            `json:"projectId"`
	WorkflowID        string            `json:"workflowId,omitempty"`
	ProviderKey       ProviderKey       `json:"providerKey"`
	ProviderSessionID string            `json:"providerSessionId"`
	ProviderAccountID string            `json:"providerAccountId,omitempty"`
	RunKind           string            `json:"runKind"`
	Status            string            `json:"status,omitempty"`
	OriginalCwd       string            `json:"originalCwd,omitempty"`
	LastPrompt        string            `json:"lastPrompt,omitempty"`
	LastMessage       string            `json:"lastMessage,omitempty"`
	StartedAt         string            `json:"startedAt,omitempty"`
	UpdatedAt         string            `json:"updatedAt,omitempty"`
	SyncedAt          string            `json:"syncedAt"`
	ProviderFile      ChatSessionFile   `json:"providerFile"`
	Extra             map[string]string `json:"extra,omitempty"`
}

type ChatSessionFile struct {
	RelativePath  string `json:"relativePath"`
	DriveObjectID string `json:"driveObjectId,omitempty"`
	SizeBytes     int64  `json:"sizeBytes"`
	SHA256        string `json:"sha256"`
}

type ChatSessionSyncRequest struct {
	GoogleDriveProjectID string `json:"googleDriveProjectId,omitempty"`
	GoogleDriveFolderID  string `json:"googleDriveFolderId,omitempty"`
}

type ChatSessionSyncResult struct {
	RunID           string `json:"runId"`
	SourceMachineID string `json:"sourceMachineId"`
	SourceRunID     string `json:"sourceRunId"`
	SyncStatus      string `json:"syncStatus"`
	SyncedAt        string `json:"syncedAt"`
	RemotePath      string `json:"remotePath"`
}

type ChatSessionRestoreRequest struct {
	ProjectID       string `json:"projectId"`
	SourceMachineID string `json:"sourceMachineId"`
	SourceRunID     string `json:"sourceRunId"`
	Cwd             string `json:"cwd,omitempty"`
}

type ChatSessionRestoreResult struct {
	RunID           string      `json:"runId"`
	SourceMachineID string      `json:"sourceMachineId"`
	SourceRunID     string      `json:"sourceRunId"`
	ProviderKey     ProviderKey `json:"providerKey"`
	RestoreStatus   string      `json:"restoreStatus"`
}

type RemoteChatSessionSummary struct {
	RunID           string      `json:"runId"`
	ProjectID       string      `json:"projectId"`
	WorkflowID      string      `json:"workflowId,omitempty"`
	ProviderKey     ProviderKey `json:"providerKey"`
	Status          string      `json:"status,omitempty"`
	RunKind         string      `json:"runKind,omitempty"`
	SourceMachineID string      `json:"sourceMachineId"`
	SourceRunID     string      `json:"sourceRunId"`
	LastPrompt      string      `json:"lastPrompt,omitempty"`
	LastMessage     string      `json:"lastMessage,omitempty"`
	StartedAt       string      `json:"startedAt,omitempty"`
	UpdatedAt       string      `json:"updatedAt,omitempty"`
	SyncedAt        string      `json:"syncedAt,omitempty"`
}

type chatSessionDriveIndexRecord struct {
	RunID           string `json:"run_id,omitempty"`
	ProjectID       string `json:"project_id"`
	WorkflowID      string `json:"workflow_id,omitempty"`
	ProviderKey     string `json:"provider_key"`
	Status          string `json:"status,omitempty"`
	RunKind         string `json:"run_kind,omitempty"`
	SourceMachineID string `json:"source_machine_id"`
	SourceRunID     string `json:"source_run_id"`
	LastPrompt      string `json:"last_prompt,omitempty"`
	LastMessage     string `json:"last_message,omitempty"`
	StartedAt       string `json:"started_at,omitempty"`
	UpdatedAt       string `json:"updated_at,omitempty"`
	SyncedAt        string `json:"synced_at"`
	ManifestPath    string `json:"manifest_path"`
}

func (s *InteractiveService) chatSessionStoreDir() string {
	if s.runner == nil || strings.TrimSpace(s.runner.workspace) == "" {
		return ""
	}
	return filepath.Join(s.runner.workspace, ".flowpilot", "chats")
}

func shortMachineID(machineID string) string {
	trimmed := strings.TrimPrefix(strings.TrimSpace(machineID), "mch_")
	if len(trimmed) > 8 {
		return trimmed[:8]
	}
	if trimmed == "" {
		return "unknown"
	}
	return trimmed
}

func safeChatSessionSegment(raw string) string {
	clean := chatSessionSafeSegmentPattern.ReplaceAllString(strings.TrimSpace(raw), "_")
	clean = strings.Trim(clean, "._-")
	if clean == "" {
		return "unknown"
	}
	return clean
}

func chatSessionProviderFolder(providerKey ProviderKey) string {
	return safeChatSessionSegment(string(providerKey))
}

func chatSessionManifestPath(machineID, runID string) string {
	return filepath.ToSlash(filepath.Join(
		"chat-sessions",
		"runs",
		safeChatSessionSegment(machineID),
		safeChatSessionSegment(runID),
		"manifest.json",
	))
}

func chatSessionProviderLogicalPath(machineID, runID string, providerKey ProviderKey, relativePath string) string {
	return filepath.ToSlash(filepath.Join(
		"chat-sessions",
		"runs",
		safeChatSessionSegment(machineID),
		safeChatSessionSegment(runID),
		chatSessionProviderFolder(providerKey),
		filepath.Base(relativePath),
	))
}

func mergeChatSessionDriveIndex(existing []byte, replacement chatSessionDriveIndexRecord) []byte {
	merged := make(map[string]chatSessionDriveIndexRecord)
	scanner := bufio.NewScanner(strings.NewReader(string(existing)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record chatSessionDriveIndexRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		if strings.TrimSpace(record.SourceMachineID) == "" || strings.TrimSpace(record.SourceRunID) == "" {
			continue
		}
		merged[record.SourceMachineID+"::"+record.SourceRunID] = record
	}
	merged[replacement.SourceMachineID+"::"+replacement.SourceRunID] = replacement

	keys := make([]string, 0, len(merged))
	for key := range merged {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	lines := make([]string, 0, len(keys))
	for _, key := range keys {
		line, err := json.Marshal(merged[key])
		if err != nil {
			continue
		}
		lines = append(lines, string(line))
	}
	if len(lines) == 0 {
		return nil
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

func parseChatSessionDriveIndex(raw []byte) []chatSessionDriveIndexRecord {
	records := make([]chatSessionDriveIndexRecord, 0)
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record chatSessionDriveIndexRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		if strings.TrimSpace(record.SourceMachineID) == "" || strings.TrimSpace(record.SourceRunID) == "" {
			continue
		}
		records = append(records, record)
	}
	return records
}

func hashBytesSHA256(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func (s *InteractiveService) BuildChatSessionSyncManifest(ctx context.Context, runID string) (ChatSessionSyncManifest, []byte, *apiErr) {
	reader, ok := s.workflowStore.(SessionHistoryReader)
	if !ok {
		return ChatSessionSyncManifest{}, nil, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", "session history is unavailable")
	}
	session, found, err := reader.GetProviderSession(ctx, runID)
	if err != nil {
		return ChatSessionSyncManifest{}, nil, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}
	if !found {
		return ChatSessionSyncManifest{}, nil, newAPIErr(http.StatusNotFound, "run_not_found", "workflow run not found")
	}
	if session.RunKind != "chat" {
		return ChatSessionSyncManifest{}, nil, newAPIErr(http.StatusConflict, "resume_unsupported", "only chat runs can be resumed in this version")
	}
	accountHome, ok := s.resolveAccountHome(session.ProviderKey, session.ProviderAccountID)
	if !ok {
		return ChatSessionSyncManifest{}, nil, newAPIErr(http.StatusConflict, "account_unavailable", "provider account home not found")
	}
	tempRun := &interactiveRun{
		id:                    session.RunID,
		projectID:             session.ProjectID,
		workflowID:            session.WorkflowID,
		providerKey:           session.ProviderKey,
		providerSessionID:     session.ProviderSessionID,
		realProviderSessionID: session.ProviderSessionID,
		providerAccountID:     session.ProviderAccountID,
		workspaceCwd:          session.WorkingDirectory,
		runKind:               session.RunKind,
	}
	if !s.ensureProviderResumeHandle(tempRun, accountHome) {
		return ChatSessionSyncManifest{}, nil, newAPIErr(http.StatusConflict, "session_unavailable", "session data not found on this machine")
	}
	sessionID := s.resumeSessionID(tempRun)
	sessionPath, found := LocateSessionFile(session.ProviderKey, accountHome, sessionID, session.WorkingDirectory)
	if !found {
		return ChatSessionSyncManifest{}, nil, newAPIErr(http.StatusConflict, "session_unavailable", "session data not found on this machine")
	}
	body, err := os.ReadFile(sessionPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ChatSessionSyncManifest{}, nil, newAPIErr(http.StatusConflict, "session_unavailable", "session data not found on this machine")
		}
		return ChatSessionSyncManifest{}, nil, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}
	relativePath, err := filepath.Rel(accountHome, sessionPath)
	if err != nil {
		return ChatSessionSyncManifest{}, nil, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}
	relativePath = filepath.ToSlash(relativePath)
	if strings.HasPrefix(relativePath, "../") || relativePath == ".." {
		return ChatSessionSyncManifest{}, nil, newAPIErr(http.StatusConflict, "session_unavailable", "session data not found on this machine")
	}
	storeDir := s.chatSessionStoreDir()
	if strings.TrimSpace(storeDir) == "" {
		return ChatSessionSyncManifest{}, nil, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", "runner workspace is unavailable")
	}
	identity, err := loadOrCreateMachineIdentity(storeDir)
	if err != nil {
		return ChatSessionSyncManifest{}, nil, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}
	syncedAt := time.Now().UTC().Format(time.RFC3339Nano)
	manifest := ChatSessionSyncManifest{
		SchemaVersion:     chatSessionManifestSchemaVersion,
		SourceMachineID:   identity.MachineID,
		SourceRunID:       session.RunID,
		ProjectID:         session.ProjectID,
		WorkflowID:        session.WorkflowID,
		ProviderKey:       session.ProviderKey,
		ProviderSessionID: sessionID,
		ProviderAccountID: session.ProviderAccountID,
		RunKind:           session.RunKind,
		Status:            string(session.Status),
		OriginalCwd:       session.WorkingDirectory,
		LastPrompt:        session.LastPrompt,
		LastMessage:       session.LastMessage,
		StartedAt:         session.StartedAt,
		UpdatedAt:         session.UpdatedAt,
		SyncedAt:          syncedAt,
		ProviderFile: ChatSessionFile{
			RelativePath: relativePath,
			SizeBytes:    int64(len(body)),
			SHA256:       hashBytesSHA256(body),
		},
	}
	return manifest, body, nil
}

func (s *InteractiveService) ensureChatSessionDriveRoot(projectID string) (string, string, *apiErr) {
	if s.runner == nil {
		return "", "", newAPIErr(http.StatusConflict, "google_drive_not_connected", "google drive is not connected for this project")
	}
	status, err := s.runner.GetGoogleDriveChatSyncConnectionStatus(projectID, "")
	if err != nil {
		return "", "", newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}
	rootFolderID := strings.TrimSpace(status.Connection.FolderID)
	if rootFolderID == "" || status.Connection.Status != "connected" {
		return "", "", newAPIErr(http.StatusConflict, "google_drive_not_connected", "google drive is not connected for this project")
	}
	accountID := strings.TrimSpace(status.Connection.AccountID)
	if accountID == "" {
		accountID = normalizeGoogleDriveStoredAccountID(status.Connection.AccountEmail)
	}
	var creds googleDriveCredential
	if accountID != "" {
		creds, err = s.runner.loadGoogleDriveCredentialByAccount(accountID)
	} else {
		creds, err = s.runner.loadGoogleDriveCredentialByProject(projectID)
	}
	if err != nil {
		return "", "", newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}
	accessToken, err := s.runner.refreshGoogleDriveAccessToken(creds.RefreshToken)
	if err != nil {
		return "", "", newAPIErr(http.StatusConflict, "google_drive_not_connected", err.Error())
	}
	return rootFolderID, accessToken, nil
}

func (s *InteractiveService) updateLocalSessionSyncStatus(ctx context.Context, runID string, apply func(*ProviderSessionState)) *apiErr {
	reader, ok := s.workflowStore.(SessionHistoryReader)
	if !ok {
		return nil
	}
	session, found, err := reader.GetProviderSession(ctx, runID)
	if err != nil || !found {
		return nil
	}
	apply(&session)
	if err := s.persistProviderSession(session); err != nil {
		return newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}
	return nil
}

func manifestToDriveIndexRecord(manifest ChatSessionSyncManifest) chatSessionDriveIndexRecord {
	return chatSessionDriveIndexRecord{
		RunID:           manifest.SourceRunID,
		ProjectID:       manifest.ProjectID,
		WorkflowID:      manifest.WorkflowID,
		ProviderKey:     string(manifest.ProviderKey),
		Status:          manifest.Status,
		RunKind:         manifest.RunKind,
		SourceMachineID: manifest.SourceMachineID,
		SourceRunID:     manifest.SourceRunID,
		LastPrompt:      manifest.LastPrompt,
		LastMessage:     manifest.LastMessage,
		StartedAt:       manifest.StartedAt,
		UpdatedAt:       manifest.UpdatedAt,
		SyncedAt:        manifest.SyncedAt,
		ManifestPath:    chatSessionManifestPath(manifest.SourceMachineID, manifest.SourceRunID),
	}
}

func (s *InteractiveService) syncChatRunToDrive(ctx context.Context, runID string, req ChatSessionSyncRequest) (ChatSessionSyncResult, *apiErr) {
	manifest, providerBytes, apiErr := s.BuildChatSessionSyncManifest(ctx, runID)
	if apiErr != nil {
		return ChatSessionSyncResult{}, apiErr
	}
	projectID := strings.TrimSpace(req.GoogleDriveProjectID)
	if projectID == "" {
		projectID = manifest.ProjectID
	}
	if strings.TrimSpace(req.GoogleDriveFolderID) != "" {
		return ChatSessionSyncResult{}, newAPIErr(http.StatusBadRequest, "invalid_request", "googleDriveFolderId override is not supported for chat sync; select the project chat sync folder instead")
	}
	rootFolderID, accessToken, driveErr := s.ensureChatSessionDriveRoot(projectID)
	if driveErr != nil {
		return ChatSessionSyncResult{}, driveErr
	}

	runFolderID, err := ensureGoogleDriveFolderPath(accessToken, rootFolderID, []string{
		"chat-sessions", "runs", safeChatSessionSegment(manifest.SourceMachineID), safeChatSessionSegment(manifest.SourceRunID), chatSessionProviderFolder(manifest.ProviderKey),
	})
	if err != nil {
		return ChatSessionSyncResult{}, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}
	providerFileName := filepath.Base(manifest.ProviderFile.RelativePath)
	uploadedFile, err := upsertGoogleDriveFile(
		accessToken,
		runFolderID,
		providerFileName,
		providerBytes,
		"application/octet-stream",
		googleDriveAppProperties(map[string]string{"relativePath": manifest.ProviderFile.RelativePath}),
	)
	if err != nil {
		return ChatSessionSyncResult{}, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}
	manifest.ProviderFile.DriveObjectID = uploadedFile.ID

	manifestDirID, err := ensureGoogleDriveFolderPath(accessToken, rootFolderID, []string{
		"chat-sessions", "runs", safeChatSessionSegment(manifest.SourceMachineID), safeChatSessionSegment(manifest.SourceRunID),
	})
	if err != nil {
		return ChatSessionSyncResult{}, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return ChatSessionSyncResult{}, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}
	if _, err := upsertGoogleDriveFile(accessToken, manifestDirID, "manifest.json", manifestBytes, "application/json", nil); err != nil {
		return ChatSessionSyncResult{}, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}

	indexFolderID, err := ensureGoogleDriveFolderPath(accessToken, rootFolderID, []string{"chat-sessions", "_index"})
	if err != nil {
		return ChatSessionSyncResult{}, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}
	var existingIndex []byte
	if existing, findErr := findGoogleDriveFile(accessToken, indexFolderID, "sessions.ndjson"); findErr == nil && strings.TrimSpace(existing.ID) != "" {
		existingIndex, _ = downloadGoogleDriveFileByID(ctx, accessToken, existing.ID)
	}
	merged := mergeChatSessionDriveIndex(existingIndex, manifestToDriveIndexRecord(manifest))
	if _, err := upsertGoogleDriveFile(accessToken, indexFolderID, "sessions.ndjson", merged, "application/x-ndjson", nil); err != nil {
		return ChatSessionSyncResult{}, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}

	if syncErr := s.updateLocalSessionSyncStatus(ctx, runID, func(state *ProviderSessionState) {
		state.SourceMachineID = manifest.SourceMachineID
		state.SourceRunID = manifest.SourceRunID
		state.SyncStatus = "synced"
		state.SyncUpdatedAt = manifest.SyncedAt
	}); syncErr != nil {
		return ChatSessionSyncResult{}, syncErr
	}

	return ChatSessionSyncResult{
		RunID:           runID,
		SourceMachineID: manifest.SourceMachineID,
		SourceRunID:     manifest.SourceRunID,
		SyncStatus:      "synced",
		SyncedAt:        manifest.SyncedAt,
		RemotePath:      chatSessionManifestPath(manifest.SourceMachineID, manifest.SourceRunID),
	}, nil
}

func (s *InteractiveService) listRemoteChatSessions(ctx context.Context, projectID string) ([]RemoteChatSessionSummary, *apiErr) {
	rootFolderID, accessToken, driveErr := s.ensureChatSessionDriveRoot(projectID)
	if driveErr != nil {
		return nil, driveErr
	}
	indexFile, err := findGoogleDriveFileByLogicalPath(accessToken, rootFolderID, "chat-sessions/_index/sessions.ndjson")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []RemoteChatSessionSummary{}, nil
		}
		return nil, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}
	raw, err := downloadGoogleDriveFileByID(ctx, accessToken, indexFile.ID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []RemoteChatSessionSummary{}, nil
		}
		return nil, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}
	records := parseChatSessionDriveIndex(raw)
	out := make([]RemoteChatSessionSummary, 0, len(records))
	for _, record := range records {
		if record.ProjectID != projectID {
			continue
		}
		out = append(out, RemoteChatSessionSummary{
			RunID:           record.RunID,
			ProjectID:       record.ProjectID,
			WorkflowID:      record.WorkflowID,
			ProviderKey:     ProviderKey(record.ProviderKey),
			Status:          record.Status,
			RunKind:         record.RunKind,
			SourceMachineID: record.SourceMachineID,
			SourceRunID:     record.SourceRunID,
			LastPrompt:      record.LastPrompt,
			LastMessage:     record.LastMessage,
			StartedAt:       record.StartedAt,
			UpdatedAt:       record.UpdatedAt,
			SyncedAt:        record.SyncedAt,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt > out[j].UpdatedAt })
	return out, nil
}

func (s *InteractiveService) resolveRestoredRunID(ctx context.Context, sourceMachineID, sourceRunID string) string {
	indexReader, ok := s.workflowStore.(SessionIndexReader)
	if !ok {
		if reader, ok := s.workflowStore.(SessionHistoryReader); ok {
			if existing, found, _ := reader.GetProviderSession(ctx, sourceRunID); found {
				if existing.SourceMachineID == sourceMachineID && existing.SourceRunID == sourceRunID {
					return existing.RunID
				}
				return "sync-" + shortMachineID(sourceMachineID) + "-" + sourceRunID
			}
		}
		return sourceRunID
	}
	sessions, err := indexReader.ListAllProviderSessions(ctx)
	if err != nil {
		return sourceRunID
	}
	runIDTaken := false
	for _, session := range sessions {
		if session.SourceMachineID == sourceMachineID && session.SourceRunID == sourceRunID && strings.TrimSpace(session.RunID) != "" {
			return session.RunID
		}
		if session.RunID == sourceRunID {
			runIDTaken = true
		}
	}
	if !runIDTaken {
		return sourceRunID
	}
	return "sync-" + shortMachineID(sourceMachineID) + "-" + sourceRunID
}

func (s *InteractiveService) restoreChatRunFromDrive(ctx context.Context, req ChatSessionRestoreRequest) (ChatSessionRestoreResult, *apiErr) {
	projectID := strings.TrimSpace(req.ProjectID)
	if projectID == "" {
		return ChatSessionRestoreResult{}, newAPIErr(http.StatusBadRequest, "invalid_request", "projectId is required")
	}
	rootFolderID, accessToken, driveErr := s.ensureChatSessionDriveRoot(projectID)
	if driveErr != nil {
		return ChatSessionRestoreResult{}, driveErr
	}
	indexFile, err := findGoogleDriveFileByLogicalPath(accessToken, rootFolderID, "chat-sessions/_index/sessions.ndjson")
	if err != nil {
		return ChatSessionRestoreResult{}, newAPIErr(http.StatusNotFound, "sync_remote_not_found", "remote chat session was not found")
	}
	indexBytes, err := downloadGoogleDriveFileByID(ctx, accessToken, indexFile.ID)
	if err != nil {
		return ChatSessionRestoreResult{}, newAPIErr(http.StatusNotFound, "sync_remote_not_found", "remote chat session was not found")
	}
	var match *chatSessionDriveIndexRecord
	for _, record := range parseChatSessionDriveIndex(indexBytes) {
		if record.SourceMachineID == req.SourceMachineID && record.SourceRunID == req.SourceRunID {
			rec := record
			match = &rec
			break
		}
	}
	if match == nil {
		return ChatSessionRestoreResult{}, newAPIErr(http.StatusNotFound, "sync_remote_not_found", "remote chat session was not found")
	}
	manifestFile, err := findGoogleDriveFileByLogicalPath(accessToken, rootFolderID, match.ManifestPath)
	if err != nil {
		return ChatSessionRestoreResult{}, newAPIErr(http.StatusNotFound, "sync_remote_not_found", "remote chat session was not found")
	}
	manifestBytes, err := downloadGoogleDriveFileByID(ctx, accessToken, manifestFile.ID)
	if err != nil {
		return ChatSessionRestoreResult{}, newAPIErr(http.StatusNotFound, "sync_remote_not_found", "remote chat session was not found")
	}
	var manifest ChatSessionSyncManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return ChatSessionRestoreResult{}, newAPIErr(http.StatusConflict, "sync_integrity_failed", "remote chat session manifest is invalid")
	}
	if manifest.SchemaVersion != chatSessionManifestSchemaVersion ||
		manifest.SourceMachineID != req.SourceMachineID ||
		manifest.SourceRunID != req.SourceRunID ||
		manifest.RunKind != "chat" ||
		strings.TrimSpace(manifest.ProviderFile.SHA256) == "" ||
		strings.TrimSpace(manifest.ProviderFile.RelativePath) == "" {
		return ChatSessionRestoreResult{}, newAPIErr(http.StatusConflict, "sync_integrity_failed", "remote chat session manifest is invalid")
	}

	cwd := strings.TrimSpace(req.Cwd)
	if cwd == "" {
		if strings.TrimSpace(manifest.OriginalCwd) != "" {
			if info, statErr := os.Stat(manifest.OriginalCwd); statErr == nil && info.IsDir() {
				cwd = manifest.OriginalCwd
			}
		}
	}
	if cwd == "" {
		return ChatSessionRestoreResult{}, newAPIErr(http.StatusConflict, "cwd_remap_required", "select a local project path before restoring this chat")
	}
	targetHome, ok := s.resolveAccountHome(manifest.ProviderKey, s.ActiveAccount())
	if !ok {
		return ChatSessionRestoreResult{}, newAPIErr(http.StatusConflict, "account_unavailable", "active account home not found")
	}
	if !HasLocalAuthAtPath(string(manifest.ProviderKey), targetHome) {
		return ChatSessionRestoreResult{}, newAPIErr(http.StatusConflict, "account_not_signed_in", "can't open — the active account isn't signed in")
	}

	objectID := strings.TrimSpace(manifest.ProviderFile.DriveObjectID)
	if objectID == "" {
		file, findErr := findGoogleDriveFileByLogicalPath(accessToken, rootFolderID, chatSessionProviderLogicalPath(manifest.SourceMachineID, manifest.SourceRunID, manifest.ProviderKey, manifest.ProviderFile.RelativePath))
		if findErr != nil {
			return ChatSessionRestoreResult{}, newAPIErr(http.StatusNotFound, "sync_remote_not_found", "remote provider session file was not found")
		}
		objectID = file.ID
	}
	providerBytes, err := downloadGoogleDriveFileByID(ctx, accessToken, objectID)
	if err != nil {
		return ChatSessionRestoreResult{}, newAPIErr(http.StatusNotFound, "sync_remote_not_found", "remote provider session file was not found")
	}
	if int64(len(providerBytes)) != manifest.ProviderFile.SizeBytes || hashBytesSHA256(providerBytes) != manifest.ProviderFile.SHA256 {
		return ChatSessionRestoreResult{}, newAPIErr(http.StatusConflict, "sync_integrity_failed", "remote provider session file failed integrity validation")
	}
	targetPath, err := restoreTargetPath(manifest.ProviderKey, targetHome, manifest.ProviderFile.RelativePath, manifest.ProviderSessionID, cwd)
	if err != nil {
		return ChatSessionRestoreResult{}, newAPIErr(http.StatusConflict, "sync_integrity_failed", "remote provider session file path is invalid")
	}
	if existing, readErr := os.ReadFile(targetPath); readErr == nil {
		if hashBytesSHA256(existing) != manifest.ProviderFile.SHA256 {
			return ChatSessionRestoreResult{}, newAPIErr(http.StatusConflict, "session_file_conflict", "a different local session file already exists for this chat")
		}
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return ChatSessionRestoreResult{}, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", readErr.Error())
	} else if _, err := RestoreSessionFile(manifest.ProviderKey, targetHome, manifest.ProviderFile.RelativePath, manifest.ProviderSessionID, cwd, providerBytes); err != nil {
		return ChatSessionRestoreResult{}, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}

	localRunID := s.resolveRestoredRunID(ctx, manifest.SourceMachineID, manifest.SourceRunID)
	session := ProviderSessionState{
		RunID:             localRunID,
		ProjectID:         projectID,
		WorkflowID:        manifest.WorkflowID,
		ProviderSessionID: manifest.ProviderSessionID,
		ProviderKey:       manifest.ProviderKey,
		ProviderAccountID: s.ActiveAccount(),
		WorkingDirectory:  cwd,
		Status:            RunStatus(firstNonEmpty(manifest.Status, string(RunStatusCompleted))),
		LastPrompt:        manifest.LastPrompt,
		LastMessage:       manifest.LastMessage,
		StartedAt:         manifest.StartedAt,
		UpdatedAt:         manifest.UpdatedAt,
		RunKind:           manifest.RunKind,
		SourceMachineID:   manifest.SourceMachineID,
		SourceRunID:       manifest.SourceRunID,
		RestoredFrom:      "google_drive",
		SyncStatus:        "restored",
		SyncUpdatedAt:     time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := s.persistProviderSession(session); err != nil {
		return ChatSessionRestoreResult{}, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}
	return ChatSessionRestoreResult{
		RunID:           localRunID,
		SourceMachineID: manifest.SourceMachineID,
		SourceRunID:     manifest.SourceRunID,
		ProviderKey:     manifest.ProviderKey,
		RestoreStatus:   "restored",
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
