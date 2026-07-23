package runner

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"flowpilot-runner/internal/agentpack"
)

const chatSessionManifestSchemaVersion = 1

// remoteChatSessionsManifestScanConcurrency bounds how many manifest fetches
// listRemoteChatSessions runs in parallel for the BUG-123 legacy-child-discovery
// scan, so a large project doesn't fire dozens of concurrent Drive API calls at
// once (rate-limit friendliness) while still turning an O(N) sequential scan
// into roughly O(N / concurrency) wall-clock time.
const remoteChatSessionsManifestScanConcurrency = 6

var chatSessionSafeSegmentPattern = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

type ChatSessionSyncManifest struct {
	SchemaVersion     int             `json:"schemaVersion"`
	SourceMachineID   string          `json:"sourceMachineId"`
	SourceRunID       string          `json:"sourceRunId"`
	ProjectID         string          `json:"projectId"`
	WorkflowID        string          `json:"workflowId,omitempty"`
	ProviderKey       ProviderKey     `json:"providerKey"`
	ProviderSessionID string          `json:"providerSessionId"`
	ProviderAccountID string          `json:"providerAccountId,omitempty"`
	RunKind           string          `json:"runKind"`
	Status            string          `json:"status,omitempty"`
	OriginalCwd       string          `json:"originalCwd,omitempty"`
	LastPrompt        string          `json:"lastPrompt,omitempty"`
	LastMessage       string          `json:"lastMessage,omitempty"`
	StartedAt         string          `json:"startedAt,omitempty"`
	UpdatedAt         string          `json:"updatedAt,omitempty"`
	SyncedAt          string          `json:"syncedAt"`
	ProviderFile      ChatSessionFile `json:"providerFile"`
	ParentRunID       string          `json:"parentRunId,omitempty"`
	AgentName         string          `json:"agentName,omitempty"`
	Role              string          `json:"role,omitempty"`
	DependsOn         []string        `json:"dependsOn,omitempty"`
	AgentStatus       string          `json:"agentStatus,omitempty"`
	ModelName         string          `json:"modelName,omitempty"`
	// ChildAgents records the agent tree at sync time so it survives a restore
	// round-trip (CP-19 / Task-082 acceptance check T-4). Omitted for runs with
	// no children.
	ChildAgents []AgentRunSummary `json:"childAgents,omitempty"`
	Extra       map[string]string `json:"extra,omitempty"`
	// Flow runtime state for a flow-engine (workflow) run's hub, mirroring the
	// fields ndjsonSessionRecord persists locally, so a Drive restore on another
	// machine reaches parity with a local restart-resume (Task-190 / CP-36 P-5,
	// Scenario 6). Empty/nil for a plain chat run.
	LoopState           *AgentLoopState      `json:"loopState,omitempty"`
	AutoOrchestrate     bool                 `json:"autoOrchestrate,omitempty"`
	FlowCohortID        string               `json:"flowCohortId,omitempty"`
	ActiveFlowEdges     []agentpack.FlowEdge `json:"activeFlowEdges,omitempty"`
	ActiveFlowNodes     []agentpack.FlowNode `json:"activeFlowNodes,omitempty"`
	PendingAgentContext []string             `json:"pendingAgentContext,omitempty"`
	// ChatSubMode/ChatFlowRef persist the Chat-Mode orchestration picker
	// selection (BUG-263) so a restored run keeps its Bug/Review-Loop intent.
	ChatSubMode string `json:"chatSubMode,omitempty"`
	ChatFlowRef string `json:"chatFlowRef,omitempty"`
	// TurnCount carries the hub's completed-turn count (BUG-315). The flow's
	// entry nodes are started only on a run's genuine first turn, gated on
	// turnCount==0 (startTurn) and re-resolved from workflowID only then
	// (resolveWorkflowFlowRef). The manifest carried every other flow-runtime
	// field but not this one, so a Drive-restored hub came back with turnCount=0
	// and its next plain follow-up re-spawned the entire flow (coder + reviewers
	// + synthesis) instead of reaching the hub. Restoring the real count keeps a
	// restored chat's continuation a plain hub turn -- and also fixes the other
	// turnCount-gated behaviors a restored run silently mis-ran (review-outcome
	// tool offering, mode-prefix). omitempty: absent == 0 == pre-fix behavior.
	TurnCount int `json:"turnCount,omitempty"`
	// TurnLog carries the run's durable turn-log sidecar verbatim (BUG-313):
	// raw user prompts, per-turn provider session-id chains, and durable
	// transcript_turn/assistant frames. Restore rewrites the local sidecar so a
	// restored run rebuilds its timeline exactly like a same-machine restart --
	// seedTranscriptFromDisk / seedGrokTranscriptFromDisk / the flow-hub
	// turn-log path all read this sidecar for prompts, hub prose, and
	// agent-card clustering. Provider-agnostic; omitempty keeps old manifests
	// valid (absent log == pre-fix behavior).
	TurnLog []turnLogLine `json:"turnLog,omitempty"`
}

// isSyncableRunKind reports whether a run's runKind can sync to / restore from
// Google Drive. "chat" is a normal chat run (and a spawned child agent run --
// createRun always sets ChatMode="normal_chat" for those, so they already carry
// runKind="chat"). "" / "workflow" are a flow-engine run's own hub (Task-190 /
// CP-36 P-5); its children are separately covered by the "chat" case above.
func isSyncableRunKind(runKind string) bool {
	return runKind == "chat" || runKind == "" || runKind == "workflow"
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
	ParentRunID     string `json:"parent_run_id,omitempty"`
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
	if !isSyncableRunKind(session.RunKind) {
		return ChatSessionSyncManifest{}, nil, newAPIErr(http.StatusConflict, "resume_unsupported", "this run kind cannot be synced in this version")
	}
	accountHome, ok := s.resolveAccountHome(session.ProviderKey, session.ProviderAccountID)
	if !ok {
		// Stale account ID recovery: scan same-provider registered homes for the exact
		// session file (mirrors the BUG-092 fallback in ensureResumeReady). This allows
		// sync to succeed even when provider-accounts.json was regenerated since the run
		// was created — without requiring the user to open the chat first.
		if recovered, _, found := s.locateSessionAcrossProviderAccounts(
			session.ProviderKey, "", session.ProviderSessionID, session.WorkingDirectory,
		); found {
			log.Printf("[chat-sync] stale account recovered run_id=%q stale_account_id=%q recovered_account_id=%q recovered_home=%q",
				runID, session.ProviderAccountID, recovered.ID, recovered.HomePath)
			session.ProviderAccountID = recovered.ID
			accountHome = recovered.HomePath
			ok = true
			// Repair the persisted stale account ID so future syncs avoid the scan.
			_ = s.updateLocalSessionSyncStatus(ctx, runID, func(st *ProviderSessionState) {
				st.ProviderAccountID = recovered.ID
			})
		}
	}
	if !ok {
		return ChatSessionSyncManifest{}, nil, newAPIErr(http.StatusConflict, "account_unavailable", "provider account home not found")
	}
	providerFile, body, sessionID, hasTranscript, transcriptErr := s.resolveChatSessionTranscript(session, accountHome)
	if transcriptErr != nil {
		return ChatSessionSyncManifest{}, nil, transcriptErr
	}
	if !hasTranscript && session.RunKind == "chat" {
		// BUG-311: a turn that was cancelled/interrupted before the provider
		// ever wrote a resumable session file will NEVER later gain one --
		// this is a permanent fact about this specific run, not a transient
		// failure. Persist it so the Navigator's unsynced-count stops
		// perpetually re-counting and silently re-attempting a sync that can
		// never succeed on this machine.
		_ = s.updateLocalSessionSyncStatus(ctx, runID, func(st *ProviderSessionState) {
			st.SyncStatus = "unsyncable"
		})
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
		SchemaVersion:       chatSessionManifestSchemaVersion,
		SourceMachineID:     identity.MachineID,
		SourceRunID:         session.RunID,
		ProjectID:           session.ProjectID,
		WorkflowID:          session.WorkflowID,
		ProviderKey:         session.ProviderKey,
		ProviderSessionID:   sessionID,
		ProviderAccountID:   session.ProviderAccountID,
		RunKind:             session.RunKind,
		Status:              string(session.Status),
		OriginalCwd:         session.WorkingDirectory,
		LastPrompt:          session.LastPrompt,
		LastMessage:         session.LastMessage,
		StartedAt:           session.StartedAt,
		UpdatedAt:           session.UpdatedAt,
		SyncedAt:            syncedAt,
		ParentRunID:         session.ParentRunID,
		AgentName:           session.AgentName,
		Role:                session.Role,
		DependsOn:           append([]string(nil), session.DependsOn...),
		AgentStatus:         session.AgentStatus,
		ModelName:           session.ModelName,
		ProviderFile:        providerFile,
		LoopState:           loopStatePtrIfSet(session.LoopState),
		AutoOrchestrate:     session.AutoOrchestrate,
		FlowCohortID:        session.FlowCohortID,
		ActiveFlowEdges:     append([]agentpack.FlowEdge(nil), session.ActiveFlowEdges...),
		ActiveFlowNodes:     append([]agentpack.FlowNode(nil), session.ActiveFlowNodes...),
		PendingAgentContext: append([]string(nil), session.PendingAgentContext...),
		ChatSubMode:         session.ChatSubMode,
		ChatFlowRef:         session.ChatFlowRef,
		TurnCount:           session.TurnCount, // BUG-315
	}
	if children := s.listAgentRunSummaries(runID); len(children) > 0 {
		manifest.ChildAgents = children
	}
	// BUG-313: carry the durable turn log with the manifest. Without it a
	// restored run has no raw prompts (flow hubs have no other prompt source --
	// CP-42 suppresses the hub's own provider turn), no durable hub
	// prose/transcript frames, and no per-turn session-id chain -- the exact
	// inputs the post-restart timeline reconstruction reads from the local
	// <runID>-turns.ndjson sidecar. Children get theirs automatically: the
	// child sync loop builds each child's manifest through this same function.
	if logger, ok := s.workflowStore.(TurnLogStore); ok {
		if entries, logErr := logger.ReadTurnLog(ctx, runID); logErr == nil && len(entries) > 0 {
			manifest.TurnLog = entries
		}
	}
	return manifest, body, nil
}

// resolveChatSessionTranscript locates and reads a run's own provider transcript
// file for the sync manifest. ok=false (with a nil error) means no real
// transcript exists for this run right now — for a "chat" run the caller turns
// that into the existing session_unavailable error; for a flow/workflow run a
// missing transcript is expected while the hub's own provider turn is still the
// synthetic "thread-<n>" placeholder (BUG-250 — CP-42 suppresses the hub's first
// turn), so the caller instead syncs flow state + child transcripts with an
// empty ProviderFile (Task-190).
func (s *InteractiveService) resolveChatSessionTranscript(session ProviderSessionState, accountHome string) (ChatSessionFile, []byte, string, bool, *apiErr) {
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
		return ChatSessionFile{}, nil, s.resumeSessionID(tempRun), false, nil
	}
	sessionID := s.resumeSessionID(tempRun)
	// A flow-engine hub's provider_session_id never advances past this synthetic
	// placeholder while the flow runs, so there is nothing real to locate yet.
	if strings.HasPrefix(sessionID, "thread-") {
		return ChatSessionFile{}, nil, sessionID, false, nil
	}
	sessionPath, found := LocateSessionFile(session.ProviderKey, accountHome, sessionID, session.WorkingDirectory)
	if !found {
		return ChatSessionFile{}, nil, sessionID, false, nil
	}
	// BUG-310: Grok's LocateSessionFile intentionally returns the session
	// DIRECTORY (it stores chat_history.jsonl plus sidecar files there), unlike
	// Codex/Claude which return a single rollout/transcript file directly.
	// os.ReadFile on that directory path fails (ERROR_INVALID_FUNCTION /
	// "Incorrect function" on Windows, EISDIR elsewhere) -- read the actual
	// transcript file inside it instead.
	readPath := sessionPath
	if session.ProviderKey == ProviderKeyGrok {
		readPath = filepath.Join(sessionPath, "chat_history.jsonl")
	}
	body, err := os.ReadFile(readPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ChatSessionFile{}, nil, sessionID, false, nil
		}
		return ChatSessionFile{}, nil, sessionID, false, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}
	relativePath, err := filepath.Rel(accountHome, readPath)
	if err != nil {
		return ChatSessionFile{}, nil, sessionID, false, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}
	relativePath = filepath.ToSlash(relativePath)
	if strings.HasPrefix(relativePath, "../") || relativePath == ".." {
		return ChatSessionFile{}, nil, sessionID, false, nil
	}
	return ChatSessionFile{
		RelativePath: relativePath,
		SizeBytes:    int64(len(body)),
		SHA256:       hashBytesSHA256(body),
	}, body, sessionID, true, nil
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

// chatSessionIndexLock returns the per-project mutex that serializes Drive
// sessions.ndjson read-merge-write for chat sync and remote-list reconcile.
func (s *InteractiveService) chatSessionIndexLock(projectID string) *sync.Mutex {
	key := strings.TrimSpace(projectID)
	if key == "" {
		key = "_"
	}
	s.chatSessionIndexMu.Lock()
	defer s.chatSessionIndexMu.Unlock()
	if s.chatSessionIndexLocks == nil {
		s.chatSessionIndexLocks = map[string]*sync.Mutex{}
	}
	if s.chatSessionIndexLocks[key] == nil {
		s.chatSessionIndexLocks[key] = &sync.Mutex{}
	}
	return s.chatSessionIndexLocks[key]
}

// mergeAndUpsertChatSessionDriveIndexLocked merges records into the project's
// Drive sessions.ndjson. Caller must hold chatSessionIndexLock(projectID).
func (s *InteractiveService) mergeAndUpsertChatSessionDriveIndexLocked(
	ctx context.Context,
	accessToken, rootFolderID string,
	records []chatSessionDriveIndexRecord,
) ([]byte, *apiErr) {
	indexFolderID, err := ensureGoogleDriveFolderPath(accessToken, rootFolderID, []string{"chat-sessions", "_index"})
	if err != nil {
		return nil, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}
	var existingIndex []byte
	if existing, findErr := findGoogleDriveFile(accessToken, indexFolderID, "sessions.ndjson"); findErr == nil && strings.TrimSpace(existing.ID) != "" {
		existingIndex, _ = downloadGoogleDriveFileByID(ctx, accessToken, existing.ID)
	}
	merged := existingIndex
	for i := range records {
		merged = mergeChatSessionDriveIndex(merged, records[i])
	}
	if bytes.Equal(merged, existingIndex) {
		return merged, nil
	}
	if _, err := upsertGoogleDriveFile(accessToken, indexFolderID, "sessions.ndjson", merged, "application/x-ndjson", nil); err != nil {
		return nil, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}
	return merged, nil
}

// discoverChatSessionDriveManifestRecords walks chat-sessions/runs/<machine>/<run>/manifest.json
// and builds index records. This repairs an under-written sessions.ndjson when
// blobs were uploaded but index rows were lost (last-write-wins race).
func discoverChatSessionDriveManifestRecords(ctx context.Context, accessToken, rootFolderID string) ([]chatSessionDriveIndexRecord, error) {
	runsFolder, err := findGoogleDriveFileByLogicalPath(accessToken, rootFolderID, "chat-sessions/runs")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	machineFolders, err := listGoogleDriveFolderChildren(accessToken, runsFolder.ID)
	if err != nil {
		return nil, err
	}

	type found struct {
		record chatSessionDriveIndexRecord
		ok     bool
	}
	// Collect run folders first so we can fan out manifest downloads.
	type runFolder struct {
		machineName string
		runName     string
		folderID    string
	}
	var runFolders []runFolder
	for _, machine := range machineFolders {
		if machine.MimeType != googleDriveFolderMimeType {
			continue
		}
		machineName := strings.TrimSpace(machine.Name)
		if machineName == "" {
			continue
		}
		children, listErr := listGoogleDriveFolderChildren(accessToken, machine.ID)
		if listErr != nil {
			return nil, listErr
		}
		for _, child := range children {
			if child.MimeType != googleDriveFolderMimeType {
				continue
			}
			runName := strings.TrimSpace(child.Name)
			if runName == "" {
				continue
			}
			runFolders = append(runFolders, runFolder{machineName: machineName, runName: runName, folderID: child.ID})
		}
	}

	foundRecords := make([]found, len(runFolders))
	var wg sync.WaitGroup
	sem := make(chan struct{}, remoteChatSessionsManifestScanConcurrency)
	for i, rf := range runFolders {
		wg.Add(1)
		go func(i int, rf runFolder) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			manifestFile, findErr := findGoogleDriveFile(accessToken, rf.folderID, "manifest.json")
			if findErr != nil {
				return
			}
			manifestBytes, downloadErr := downloadGoogleDriveFileByID(ctx, accessToken, manifestFile.ID)
			if downloadErr != nil {
				return
			}
			var manifest ChatSessionSyncManifest
			if json.Unmarshal(manifestBytes, &manifest) != nil {
				return
			}
			if strings.TrimSpace(manifest.SourceMachineID) == "" {
				manifest.SourceMachineID = rf.machineName
			}
			if strings.TrimSpace(manifest.SourceRunID) == "" {
				manifest.SourceRunID = rf.runName
			}
			if strings.TrimSpace(manifest.SourceMachineID) == "" || strings.TrimSpace(manifest.SourceRunID) == "" {
				return
			}
			foundRecords[i] = found{record: manifestToDriveIndexRecord(manifest), ok: true}
		}(i, rf)
	}
	wg.Wait()

	out := make([]chatSessionDriveIndexRecord, 0, len(foundRecords))
	for _, f := range foundRecords {
		if f.ok {
			out = append(out, f.record)
		}
	}
	return out, nil
}

// loadChatSessionDriveIndexRecords downloads sessions.ndjson, merges any run
// manifests missing from the index, and rewrites the index when repair is needed.
func (s *InteractiveService) loadChatSessionDriveIndexRecords(
	ctx context.Context,
	projectID, accessToken, rootFolderID string,
) ([]chatSessionDriveIndexRecord, *apiErr) {
	discovered, discoverErr := discoverChatSessionDriveManifestRecords(ctx, accessToken, rootFolderID)
	if discoverErr != nil {
		return nil, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", discoverErr.Error())
	}

	lock := s.chatSessionIndexLock(projectID)
	lock.Lock()
	defer lock.Unlock()

	// No run manifests: read index only (do not create empty Drive folders).
	if len(discovered) == 0 {
		indexFile, err := findGoogleDriveFileByLogicalPath(accessToken, rootFolderID, "chat-sessions/_index/sessions.ndjson")
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return []chatSessionDriveIndexRecord{}, nil
			}
			return nil, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
		}
		raw, err := downloadGoogleDriveFileByID(ctx, accessToken, indexFile.ID)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return []chatSessionDriveIndexRecord{}, nil
			}
			return nil, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
		}
		records := parseChatSessionDriveIndex(raw)
		log.Printf("[chat-sync] remote index project=%s index_rows=%d discovered_manifests=0", projectID, len(records))
		return records, nil
	}

	merged, apiErr := s.mergeAndUpsertChatSessionDriveIndexLocked(ctx, accessToken, rootFolderID, discovered)
	if apiErr != nil {
		return nil, apiErr
	}
	records := parseChatSessionDriveIndex(merged)
	log.Printf(
		"[chat-sync] remote index project=%s index_rows=%d discovered_manifests=%d",
		projectID,
		len(records),
		len(discovered),
	)
	return records, nil
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
		ParentRunID:     manifest.ParentRunID,
	}
}

// uploadChatSessionRunFiles uploads one run's provider transcript file and its manifest.json
// to the run's own Drive folder, stamping the resulting Drive object id back onto the
// manifest. Shared by the parent run and each child agent run so child sub-chats survive a
// cross-machine restore. (BUG-119)
func (s *InteractiveService) uploadChatSessionRunFiles(accessToken, rootFolderID string, manifest *ChatSessionSyncManifest, providerBytes []byte) *apiErr {
	// A flow-engine hub run may have no real transcript to upload while its own
	// provider turn is still suppressed (BUG-250 / Task-190) -- only its
	// manifest (flow state + child references) is uploaded in that case.
	if strings.TrimSpace(manifest.ProviderFile.RelativePath) != "" {
		runFolderID, err := ensureGoogleDriveFolderPath(accessToken, rootFolderID, []string{
			"chat-sessions", "runs", safeChatSessionSegment(manifest.SourceMachineID), safeChatSessionSegment(manifest.SourceRunID), chatSessionProviderFolder(manifest.ProviderKey),
		})
		if err != nil {
			return newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
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
			return newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
		}
		manifest.ProviderFile.DriveObjectID = uploadedFile.ID
	}

	manifestDirID, err := ensureGoogleDriveFolderPath(accessToken, rootFolderID, []string{
		"chat-sessions", "runs", safeChatSessionSegment(manifest.SourceMachineID), safeChatSessionSegment(manifest.SourceRunID),
	})
	if err != nil {
		return newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}
	if _, err := upsertGoogleDriveFile(accessToken, manifestDirID, "manifest.json", manifestBytes, "application/json", nil); err != nil {
		return newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}
	return nil
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

	// Upload the parent run, then each child agent run, so child sub-chats are openable on
	// another machine. Previously only the parent provider file was uploaded; a restore
	// elsewhere then found the agent tree (manifest.ChildAgents) but no child transcripts. (BUG-119)
	manifests := []ChatSessionSyncManifest{manifest}
	if upErr := s.uploadChatSessionRunFiles(accessToken, rootFolderID, &manifests[0], providerBytes); upErr != nil {
		return ChatSessionSyncResult{}, upErr
	}
	syncedChildRunIDs := []string{}
	for _, child := range manifest.ChildAgents {
		childRunID := strings.TrimSpace(child.RunID)
		if childRunID == "" || childRunID == runID {
			continue
		}
		childManifest, childBytes, childErr := s.BuildChatSessionSyncManifest(ctx, childRunID)
		if childErr != nil {
			// Best-effort: a child whose transcript is missing locally shouldn't fail the
			// whole sync — log and continue so the rest still upload.
			log.Printf("[chat-sync] skip child run_id=%q parent=%q code=%q msg=%q", childRunID, runID, childErr.code, childErr.msg)
			continue
		}
		if upErr := s.uploadChatSessionRunFiles(accessToken, rootFolderID, &childManifest, childBytes); upErr != nil {
			log.Printf("[chat-sync] child upload failed run_id=%q parent=%q code=%q msg=%q", childRunID, runID, upErr.code, upErr.msg)
			continue
		}
		manifests = append(manifests, childManifest)
		syncedChildRunIDs = append(syncedChildRunIDs, childRunID)
	}

	indexRecords := make([]chatSessionDriveIndexRecord, 0, len(manifests))
	for i := range manifests {
		indexRecords = append(indexRecords, manifestToDriveIndexRecord(manifests[i]))
	}
	// Serialize index read-merge-write so concurrent syncs cannot last-write-wins
	// and drop sibling parent rows (REMOTE CHATS under-list after multi-run sync).
	indexLock := s.chatSessionIndexLock(projectID)
	indexLock.Lock()
	_, indexErr := s.mergeAndUpsertChatSessionDriveIndexLocked(ctx, accessToken, rootFolderID, indexRecords)
	indexLock.Unlock()
	if indexErr != nil {
		return ChatSessionSyncResult{}, indexErr
	}

	// CP-51: upload per-project dispatch.ndjson alongside chat sessions so another
	// machine can restore durable turn state (best-effort — session sync still wins).
	if dispErr := s.syncDispatchLogToDrive(ctx, projectID, accessToken, rootFolderID); dispErr != nil {
		log.Printf("[chat-sync] dispatch upload failed project=%s: %v", projectID, dispErr)
	}

	if syncErr := s.updateLocalSessionSyncStatus(ctx, runID, func(state *ProviderSessionState) {
		state.SourceMachineID = manifest.SourceMachineID
		state.SourceRunID = manifest.SourceRunID
		state.SyncStatus = "synced"
		state.SyncUpdatedAt = manifest.SyncedAt
	}); syncErr != nil {
		return ChatSessionSyncResult{}, syncErr
	}
	// Mark each synced child as synced too (best-effort — index/parent already uploaded).
	for i, childRunID := range syncedChildRunIDs {
		childManifest := manifests[i+1]
		_ = s.updateLocalSessionSyncStatus(ctx, childRunID, func(state *ProviderSessionState) {
			state.SourceMachineID = childManifest.SourceMachineID
			state.SourceRunID = childManifest.SourceRunID
			state.SyncStatus = "synced"
			state.SyncUpdatedAt = childManifest.SyncedAt
		})
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
	// Load index and repair from run manifests when blobs exist but index rows
	// were lost (concurrent sync last-write-wins). Missing index with no runs
	// still yields an empty remote list.
	records, loadErr := s.loadChatSessionDriveIndexRecords(ctx, projectID, accessToken, rootFolderID)
	if loadErr != nil {
		return nil, loadErr
	}
	childKeys := make(map[string]struct{})
	for _, record := range records {
		if strings.TrimSpace(record.ParentRunID) != "" {
			childKeys[record.SourceMachineID+"\x00"+record.SourceRunID] = struct{}{}
		}
	}
	// BUG-123 backward compatibility: BUG-119 uploaded child manifests and index rows
	// before parent_run_id was added to the index. Read the manifests once to discover
	// those legacy child identities, then keep them out of the top-level remote list.
	//
	// Perf (found while investigating a multi-minute REMOTE CHATS load): each manifest
	// lives at a 5-segment logical path, and findGoogleDriveFileByLogicalPath resolves
	// every path segment as its own sequential Drive "list files" API call -- so one
	// manifest costs ~5-6 round trips, and this loop used to run that cost, one record
	// at a time, for every record in the index on every single load (no caching). A
	// project with N synced chats paid roughly 5N-6N sequential Drive API calls just to
	// list what's already synced. Two independent fixes, both safe because this loop
	// only ever accumulates into childKeys (order-independent, read-only per record):
	//  1. skip records that already carry a real parent_run_id -- they're already-known
	//     children (excluded above) and, since child-of-child nesting isn't supported
	//     (BUG-119), their own manifest can't discover any further exclusions.
	//  2. fetch the remaining candidates concurrently (bounded pool) instead of one at a
	//     time, so wall-clock cost is ~1 record's worth of round trips instead of N's.
	type discoveredChildren struct {
		machineID string
		runIDs    []string
	}
	discoveries := make([]discoveredChildren, len(records))
	var wg sync.WaitGroup
	sem := make(chan struct{}, remoteChatSessionsManifestScanConcurrency)
	for i, record := range records {
		if strings.TrimSpace(record.ParentRunID) != "" {
			continue
		}
		wg.Add(1)
		go func(i int, record chatSessionDriveIndexRecord) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			manifestFile, findErr := findGoogleDriveFileByLogicalPath(accessToken, rootFolderID, record.ManifestPath)
			if findErr != nil {
				return
			}
			manifestBytes, downloadErr := downloadGoogleDriveFileByID(ctx, accessToken, manifestFile.ID)
			if downloadErr != nil {
				return
			}
			var manifest ChatSessionSyncManifest
			if json.Unmarshal(manifestBytes, &manifest) != nil {
				return
			}
			var runIDs []string
			for _, child := range manifest.ChildAgents {
				if childRunID := strings.TrimSpace(child.RunID); childRunID != "" {
					runIDs = append(runIDs, childRunID)
				}
			}
			if len(runIDs) > 0 {
				discoveries[i] = discoveredChildren{machineID: manifest.SourceMachineID, runIDs: runIDs}
			}
		}(i, record)
	}
	wg.Wait()
	for _, d := range discoveries {
		for _, childRunID := range d.runIDs {
			childKeys[d.machineID+"\x00"+childRunID] = struct{}{}
		}
	}
	out := make([]RemoteChatSessionSummary, 0, len(records))
	for _, record := range records {
		if _, isChild := childKeys[record.SourceMachineID+"\x00"+record.SourceRunID]; isChild {
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
	return s.restoreChatRunTreeFromDrive(ctx, req, make(map[string]struct{}))
}

func (s *InteractiveService) restoreChatRunTreeFromDrive(ctx context.Context, req ChatSessionRestoreRequest, restoring map[string]struct{}) (ChatSessionRestoreResult, *apiErr) {
	restoreKey := strings.TrimSpace(req.SourceMachineID) + "\x00" + strings.TrimSpace(req.SourceRunID)
	if _, duplicate := restoring[restoreKey]; duplicate {
		return ChatSessionRestoreResult{}, newAPIErr(http.StatusConflict, "sync_integrity_failed", "remote chat session child graph contains a cycle")
	}
	restoring[restoreKey] = struct{}{}
	defer delete(restoring, restoreKey)

	projectID := strings.TrimSpace(req.ProjectID)
	if projectID == "" {
		return ChatSessionRestoreResult{}, newAPIErr(http.StatusBadRequest, "invalid_request", "projectId is required")
	}
	rootFolderID, accessToken, driveErr := s.ensureChatSessionDriveRoot(projectID)
	if driveErr != nil {
		return ChatSessionRestoreResult{}, driveErr
	}
	// CP-51: restore dispatch log before session/run resume so recovery sees durable states.
	if dispErr := s.restoreDispatchLogFromDrive(ctx, projectID, accessToken, rootFolderID); dispErr != nil {
		log.Printf("[chat-sync] dispatch restore failed project=%s: %v", projectID, dispErr)
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
	hasProviderFile := strings.TrimSpace(manifest.ProviderFile.SHA256) != "" && strings.TrimSpace(manifest.ProviderFile.RelativePath) != ""
	if manifest.SchemaVersion != chatSessionManifestSchemaVersion ||
		manifest.SourceMachineID != req.SourceMachineID ||
		manifest.SourceRunID != req.SourceRunID ||
		!isSyncableRunKind(manifest.RunKind) ||
		// A "chat" manifest must always carry a real transcript; a flow/workflow
		// hub manifest may legitimately have none while its own provider turn is
		// still suppressed (BUG-250 parity — Task-190).
		(manifest.RunKind == "chat" && !hasProviderFile) {
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
	activeAccountID := s.activeAccountForProvider(manifest.ProviderKey)
	targetHome, ok := s.resolveAccountHome(manifest.ProviderKey, activeAccountID)
	if !ok {
		targetHome, ok = defaultProviderSessionHome(manifest.ProviderKey)
		if !ok {
			return ChatSessionRestoreResult{}, newAPIErr(http.StatusConflict, "account_unavailable", "provider session storage home not found")
		}
		activeAccountID = "default"
	}

	// A flow-engine hub manifest may carry no provider transcript at all (its own
	// turn is suppressed while the flow runs — BUG-250); restore it read-only,
	// with only flow state + children, instead of failing (Task-190).
	localAhead := false
	if hasProviderFile {
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
		// Codex rollout files are append-only JSONL, so a hash mismatch can simply mean
		// one side has more turns than the other. Accept prefix-compatible extensions of
		// the same session (mirrors updateCodexDestinationIfSameSessionExtends); only
		// reject genuinely divergent content. Other providers keep strict equality —
		// their append semantics are not confirmed, so any mismatch stays a conflict. (BUG-091)
		if existing, readErr := os.ReadFile(targetPath); readErr == nil {
			codexExtend := manifest.ProviderKey == ProviderKeyCodex
			switch {
			case hashBytesSHA256(existing) == manifest.ProviderFile.SHA256:
				// identical — nothing to write
			case codexExtend && bytes.HasPrefix(providerBytes, existing):
				// remote is a newer prefix-compatible extension of local — overwrite
				if err := os.WriteFile(targetPath, providerBytes, 0o644); err != nil {
					return ChatSessionRestoreResult{}, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
				}
			case codexExtend && bytes.HasPrefix(existing, providerBytes):
				// local already extends remote — keep the newer local file untouched
				localAhead = true
			default:
				return ChatSessionRestoreResult{}, newAPIErr(http.StatusConflict, "session_file_conflict", "a different local session file already exists for this chat")
			}
		} else if !errors.Is(readErr, os.ErrNotExist) {
			return ChatSessionRestoreResult{}, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", readErr.Error())
		} else if _, err := RestoreSessionFile(manifest.ProviderKey, targetHome, manifest.ProviderFile.RelativePath, manifest.ProviderSessionID, cwd, providerBytes); err != nil {
			return ChatSessionRestoreResult{}, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
		}
	}

	localRunID := s.resolveRestoredRunID(ctx, manifest.SourceMachineID, manifest.SourceRunID)
	session := ProviderSessionState{
		RunID:             localRunID,
		ProjectID:         projectID,
		WorkflowID:        manifest.WorkflowID,
		ProviderSessionID: manifest.ProviderSessionID,
		ProviderKey:       manifest.ProviderKey,
		ProviderAccountID: activeAccountID,
		WorkingDirectory:  cwd,
		// A synced status is a live snapshot from the source machine at sync
		// time; once restored elsewhere nothing is actually running it, so an
		// in-flight status must be normalized the same way a restart-rebuilt
		// run already is (BUG-251 parity — Task-190).
		Status:              normalizeResumedStatus(RunStatus(firstNonEmpty(manifest.Status, string(RunStatusCompleted)))),
		LastPrompt:          manifest.LastPrompt,
		LastMessage:         manifest.LastMessage,
		StartedAt:           manifest.StartedAt,
		UpdatedAt:           manifest.UpdatedAt,
		RunKind:             manifest.RunKind,
		SourceMachineID:     manifest.SourceMachineID,
		SourceRunID:         manifest.SourceRunID,
		RestoredFrom:        "google_drive",
		SyncStatus:          "restored",
		SyncUpdatedAt:       time.Now().UTC().Format(time.RFC3339Nano),
		ParentRunID:         manifest.ParentRunID,
		AgentName:           manifest.AgentName,
		Role:                manifest.Role,
		DependsOn:           append([]string(nil), manifest.DependsOn...),
		AgentStatus:         string(normalizeResumedStatus(RunStatus(manifest.AgentStatus))),
		ModelName:           manifest.ModelName,
		LoopState:           loopStateFromPtr(manifest.LoopState),
		AutoOrchestrate:     manifest.AutoOrchestrate,
		FlowCohortID:        manifest.FlowCohortID,
		ActiveFlowEdges:     append([]agentpack.FlowEdge(nil), manifest.ActiveFlowEdges...),
		ActiveFlowNodes:     append([]agentpack.FlowNode(nil), manifest.ActiveFlowNodes...),
		PendingAgentContext: append([]string(nil), manifest.PendingAgentContext...),
		ChatSubMode:         manifest.ChatSubMode,
		ChatFlowRef:         manifest.ChatFlowRef,
		TurnCount:           manifest.TurnCount, // BUG-315
	}
	if localAhead {
		// The local rollout file is ahead of the restored snapshot, so the older
		// remote manifest must not downgrade the local conversation metadata. (BUG-091)
		if reader, ok := s.workflowStore.(SessionHistoryReader); ok {
			if local, found, _ := reader.GetProviderSession(ctx, localRunID); found {
				if strings.TrimSpace(local.LastPrompt) != "" {
					session.LastPrompt = local.LastPrompt
				}
				if strings.TrimSpace(local.LastMessage) != "" {
					session.LastMessage = local.LastMessage
				}
				if strings.TrimSpace(string(local.Status)) != "" {
					session.Status = local.Status
				}
				if strings.TrimSpace(local.UpdatedAt) != "" {
					session.UpdatedAt = local.UpdatedAt
				}
			}
		}
	}
	// Restore every child transcript referenced by the parent manifest and persist its
	// relationship metadata. Loading summaries alone made the panel look correct only
	// until restart and left child chats unopened on the restored machine. (BUG-123)
	var remapped []AgentRunSummary
	if len(manifest.ChildAgents) > 0 {
		remapped = make([]AgentRunSummary, len(manifest.ChildAgents))
		runIDMap := map[string]string{manifest.SourceRunID: localRunID}
		for i, child := range manifest.ChildAgents {
			remapped[i] = child
			// A synced child's status is a live snapshot from the source machine;
			// once restored elsewhere nothing is actually running it, so an
			// in-flight status must be normalized the same way the disk-fallback
			// branch of listAgentRunSummaries already does (BUG-251 parity —
			// Task-190) — otherwise a reviewer synced mid-turn shows a
			// permanently stale "running" badge on the restored machine.
			remapped[i].Status = normalizeResumedStatus(child.Status)
			remapped[i].AgentStatus = string(normalizeResumedStatus(RunStatus(child.AgentStatus)))
			remapped[i].DependsOn = append([]string(nil), child.DependsOn...)
			childRunID := strings.TrimSpace(child.RunID)
			if childRunID == "" {
				continue
			}
			childResult, childErr := s.restoreChatRunTreeFromDrive(ctx, ChatSessionRestoreRequest{
				ProjectID:       projectID,
				SourceMachineID: manifest.SourceMachineID,
				SourceRunID:     childRunID,
				Cwd:             cwd,
			}, restoring)
			if childErr != nil {
				log.Printf("[chat-sync] child restore failed run_id=%q parent=%q code=%q msg=%q", childRunID, manifest.SourceRunID, childErr.code, childErr.msg)
				return ChatSessionRestoreResult{}, childErr
			}
			runIDMap[childRunID] = childResult.RunID
			remapped[i].RunID = childResult.RunID
		}
		for i := range remapped {
			remapped[i].ParentRunID = localRunID
			for j, dependency := range remapped[i].DependsOn {
				if localDependency, ok := runIDMap[dependency]; ok {
					remapped[i].DependsOn[j] = localDependency
				}
			}
			childLocalRunID := runIDMap[manifest.ChildAgents[i].RunID]
			if childLocalRunID == "" {
				continue
			}
			if metadataErr := s.updateLocalSessionSyncStatus(ctx, childLocalRunID, func(childSession *ProviderSessionState) {
				childSession.ParentRunID = localRunID
				childSession.AgentName = remapped[i].AgentName
				childSession.Role = remapped[i].Role
				childSession.DependsOn = append([]string(nil), remapped[i].DependsOn...)
				childSession.AgentStatus = remapped[i].AgentStatus
				childSession.ModelName = remapped[i].ModelName
			}); metadataErr != nil {
				return ChatSessionRestoreResult{}, metadataErr
			}
		}
	}
	// BUG-313: rebuild the per-run turn-log sidecar from the manifest so the
	// restored run reopens with the same timeline a same-machine restart would
	// build (raw prompts, flow-hub prose, agent-card clustering, per-turn
	// session-id chains). Skipped when the local sidecar already has entries --
	// on the original machine the local log IS the source of truth, and a
	// repeated restore must not append a second copy of every line.
	if len(manifest.TurnLog) > 0 {
		if logger, ok := s.workflowStore.(TurnLogStore); ok {
			if existing, readErr := logger.ReadTurnLog(ctx, localRunID); readErr == nil && len(existing) == 0 {
				for _, line := range manifest.TurnLog {
					if appendErr := logger.AppendTurnLog(ctx, localRunID, line); appendErr != nil {
						return ChatSessionRestoreResult{}, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", appendErr.Error())
					}
				}
			}
		}
	}
	// Publish the parent to main history only after all child files and relationship
	// metadata are durable. History polling can run while restore is in progress, so
	// persisting the parent earlier exposed an incomplete tree in the UI. (BUG-123)
	if err := s.persistProviderSession(session); err != nil {
		return ChatSessionRestoreResult{}, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
	}
	if len(remapped) > 0 {
		s.agentOrchestrator.setHistoricalChildren(localRunID, remapped)
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
