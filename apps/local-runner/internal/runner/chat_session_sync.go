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
	"strconv"
	"strings"
	"sync"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// chatSessionManifestSchemaVersion stays 1: the RUN-level manifest keeps its
// format version and gains BUG-476 leg identity as additive omitempty fields
// (older readers ignore unknown JSON fields). The versioned artifact is the
// chat-level envelope (chat.json, chatSyncManifestSchemaVersion=2) — matching
// the pre-existing contract "v1 is run-level, v2 is chat-level"
// (chat_sync_manifest.go). A v1 manifest without chatId restores explicitly
// as a single-leg chat.
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
	// BUG-476 (CP-59/SD-26): the sync unit is the LOGICAL CHAT, not one run.
	// These fields carry the durable provider-leg identity (CP-63 leg model)
	// so a manifest describes WHICH leg of WHICH chat it is and where it sits
	// in the switch chain. Empty/zero on a pre-fix (v1) manifest — restore
	// treats that as an explicit single-leg chat.
	ChatID          string `json:"chatId,omitempty"`
	LegSeq          int    `json:"legSeq,omitempty"`
	LegState        string `json:"legState,omitempty"`
	LegClosedReason string `json:"legClosedReason,omitempty"`
	SwitchFromRunID string `json:"switchFromRunId,omitempty"`
	// TurnLog carries the run's durable turn-log sidecar verbatim (BUG-313):
	// raw user prompts, per-turn provider session-id chains, and durable
	// transcript_turn/assistant frames. Restore rewrites the local sidecar so a
	// restored run rebuilds its timeline exactly like a same-machine restart --
	// seedTranscriptFromDisk / seedGrokTranscriptFromDisk / the flow-hub
	// turn-log path all read this sidecar for prompts, hub prose, and
	// agent-card clustering. Provider-agnostic; omitempty keeps old manifests
	// valid (absent log == pre-fix behavior).
	TurnLog []turnLogLine `json:"turnLog,omitempty"`
	// ProviderFiles carries every OTHER regular file in a Grok ACP session
	// directory besides ProviderFile (chat_history.jsonl) -- BUG-316: Grok's own
	// session/load call reads the whole directory (events.jsonl, updates.jsonl,
	// prompt_context.json, system_prompt.txt, summary.json, signals.json,
	// resources_state.json, rewind_points.jsonl, announcement_state.json), so
	// restoring only chat_history.jsonl leaves it unable to find its own state --
	// the NEXT turn after a restore fails FS_NOT_FOUND ("Path not found."),
	// confirmed live even with the correct account/cwd. Always empty for
	// Codex/Claude (single-file resume, no directory-wide provider state).
	ProviderFiles []ChatSessionFile `json:"providerFiles,omitempty"`
	// grokSidecarBodies carries the actual bytes for ProviderFiles, keyed by
	// RelativePath. Deliberately unexported: encoding/json silently skips
	// unexported fields, so this never reaches manifest.json -- it only exists
	// to carry bytes from BuildChatSessionSyncManifest to
	// uploadChatSessionRunFiles without widening either function's public
	// signature (both have pre-existing test call sites on the current arity).
	grokSidecarBodies map[string][]byte
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
	ChatID          string      `json:"chatId,omitempty"`
	LegSeq          int         `json:"legSeq,omitempty"`
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
	// BUG-476: chat-leg identity so remote listing can group legs of one
	// logical chat into a single row (absent on pre-v2 rows == single-leg).
	ChatID string `json:"chat_id,omitempty"`
	LegSeq int    `json:"leg_seq,omitempty"`
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

// chatSessionChatManifestPath is the chat-level (BUG-476) commit marker: the
// ordered list of provider legs for one logical chat. It is uploaded LAST —
// only after every leg's own manifest + provider file landed — so a partial
// upload never advertises a complete restorable chat.
func chatSessionChatManifestPath(machineID, chatID string) string {
	return filepath.ToSlash(filepath.Join(
		"chat-sessions",
		"chats",
		safeChatSessionSegment(machineID),
		safeChatSessionSegment(chatID),
		"chat.json",
	))
}

// chatSessionChatManifest is the v2 chat-level envelope. Restore prefers it
// (ordered legs, explicit completeness) and falls back to index rows sharing
// the same chat_id when it is absent (partial upload / interrupted sync).
type chatSessionChatManifest struct {
	SchemaVersion   int                  `json:"schemaVersion"`
	ChatID          string               `json:"chatId"`
	SourceMachineID string               `json:"sourceMachineId"`
	Legs            []chatSessionChatLeg `json:"legs"`
	SyncedAt        string               `json:"syncedAt"`
}

type chatSessionChatLeg struct {
	SourceRunID  string `json:"sourceRunId"`
	LegSeq       int    `json:"legSeq"`
	LegState     string `json:"legState,omitempty"`
	ProviderKey  string `json:"providerKey"`
	ManifestPath string `json:"manifestPath"`
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
	// BUG-483: the merge must be lossless. Lines we cannot interpret —
	// malformed JSON, or valid JSON rows without source identity — are
	// preserved verbatim at the tail of the file instead of being silently
	// dropped (dropping them destroys remote rows other devices wrote).
	var preserved []string
	scanner := bufio.NewScanner(strings.NewReader(string(existing)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record chatSessionDriveIndexRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			preserved = append(preserved, line)
			continue
		}
		if strings.TrimSpace(record.SourceMachineID) == "" || strings.TrimSpace(record.SourceRunID) == "" {
			preserved = append(preserved, line)
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

	lines := make([]string, 0, len(keys)+len(preserved))
	for _, key := range keys {
		line, err := json.Marshal(merged[key])
		if err != nil {
			continue
		}
		lines = append(lines, string(line))
	}
	lines = append(lines, preserved...)
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
		// BUG-476: carry the durable provider-leg identity so the manifest
		// describes which leg of which logical chat it is.
		ChatID:          session.ChatID,
		LegSeq:          session.LegSeq,
		LegState:        session.LegState,
		LegClosedReason: session.LegClosedReason,
		SwitchFromRunID: session.SwitchFromRunID,
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
	// BUG-316: Grok's session/load needs the whole session directory, not just
	// chat_history.jsonl -- carry every other regular file alongside it so a
	// restored chat's next turn can actually resume instead of FS_NOT_FOUND.
	// Best-effort: a read error here degrades to "sync without sidecars" (old
	// behavior) rather than failing the sync outright.
	if session.ProviderKey == ProviderKeyGrok && isGrokRealSessionID(sessionID) {
		if sidecarFiles, sidecarBodies, sidecarErr := resolveGrokSessionSidecarFiles(accountHome, session.WorkingDirectory, sessionID); sidecarErr == nil {
			manifest.ProviderFiles = sidecarFiles
			manifest.grokSidecarBodies = sidecarBodies
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

// resolveGrokSessionSidecarFiles lists every regular file in a Grok ACP
// session directory besides chat_history.jsonl (already carried by
// resolveChatSessionTranscript, above) and *.lock markers (zero-byte runtime
// locks Grok recreates itself -- carrying a stale one risks the restored
// machine seeing an already-held lock). BUG-316: Grok's session/load reads the
// WHOLE directory (events.jsonl, updates.jsonl, prompt_context.json,
// system_prompt.txt, summary.json, signals.json, resources_state.json,
// rewind_points.jsonl, announcement_state.json) to resume -- a restored
// directory missing these fails FS_NOT_FOUND ("Path not found.") on the very
// next turn after restore, confirmed live even with the correct account/cwd
// and a byte-correct chat_history.jsonl. Returns (nil, nil, nil) when the
// session directory itself doesn't exist (nothing to carry, not an error).
func resolveGrokSessionSidecarFiles(accountHome, cwd, sessionID string) ([]ChatSessionFile, map[string][]byte, error) {
	dir := grokSessionDirPath(accountHome, cwd, sessionID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	var files []ChatSessionFile
	bodies := make(map[string][]byte)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "chat_history.jsonl" || strings.HasSuffix(name, ".lock") {
			continue
		}
		fullPath := filepath.Join(dir, name)
		body, readErr := os.ReadFile(fullPath)
		if readErr != nil {
			// Best-effort: a sidecar mid-write by a live Grok process should not
			// fail the whole sync -- it just won't be as complete this round.
			continue
		}
		relativePath, relErr := filepath.Rel(accountHome, fullPath)
		if relErr != nil {
			continue
		}
		relativePath = filepath.ToSlash(relativePath)
		if strings.HasPrefix(relativePath, "../") || relativePath == ".." {
			continue
		}
		files = append(files, ChatSessionFile{
			RelativePath: relativePath,
			SizeBytes:    int64(len(body)),
			SHA256:       hashBytesSHA256(body),
		})
		bodies[relativePath] = body
	}
	return files, bodies, nil
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
	// BUG-483: fail closed on any authority-read failure. Only a confirmed
	// "file absent" outcome may use an empty baseline — a lookup error or a
	// download error must abort BEFORE the upsert, or a transient read
	// failure overwrites the remote index with just our rows.
	indexFile, findErr := findGoogleDriveFile(accessToken, indexFolderID, "sessions.ndjson")
	if findErr != nil {
		return nil, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", findErr.Error())
	}
	var existingIndex []byte
	if strings.TrimSpace(indexFile.ID) != "" {
		body, dlErr := downloadGoogleDriveFileByID(ctx, accessToken, indexFile.ID)
		if dlErr != nil {
			return nil, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", dlErr.Error())
		}
		existingIndex = body
	}
	merged := existingIndex
	for i := range records {
		merged = mergeChatSessionDriveIndex(merged, records[i])
	}
	if bytes.Equal(merged, existingIndex) {
		return merged, nil
	}
	// BUG-483 optimistic concurrency: a second device may have written the
	// index between our read and this upsert. Re-read once right before
	// overwriting; if the baseline changed, merge our rows onto the FRESH
	// content instead of last-write-wins clobbering. A failed re-read also
	// aborts — overwriting on an unreadable authority is exactly the bug.
	latestFile, reFindErr := findGoogleDriveFile(accessToken, indexFolderID, "sessions.ndjson")
	if reFindErr != nil {
		return nil, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", reFindErr.Error())
	}
	var latestIndex []byte
	if strings.TrimSpace(latestFile.ID) != "" {
		body, dlErr := downloadGoogleDriveFileByID(ctx, accessToken, latestFile.ID)
		if dlErr != nil {
			return nil, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", dlErr.Error())
		}
		latestIndex = body
	}
	if !bytes.Equal(latestIndex, existingIndex) {
		merged = latestIndex
		for i := range records {
			merged = mergeChatSessionDriveIndex(merged, records[i])
		}
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

// readChatSessionDriveIndex downloads chat-sessions/_index/sessions.ndjson
// only. Missing index is an empty list, not an error (CA-555 fast path).
func readChatSessionDriveIndex(ctx context.Context, accessToken, rootFolderID string) ([]chatSessionDriveIndexRecord, *apiErr) {
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
	return parseChatSessionDriveIndex(raw), nil
}

func chatSessionIndexHasParentRunID(records []chatSessionDriveIndexRecord) bool {
	for _, record := range records {
		if strings.TrimSpace(record.ParentRunID) != "" {
			return true
		}
	}
	return false
}

// scheduleChatSessionIndexRepair walks runs/ and merges missing rows in the
// background. Used when the index already has rows so list can return immediately
// (CA-555) while still eventually repairing CA-404 under-written indexes.
func (s *InteractiveService) scheduleChatSessionIndexRepair(projectID, accessToken, rootFolderID string) {
	key := strings.TrimSpace(projectID)
	if key == "" {
		key = "_"
	}
	s.chatSessionIndexMu.Lock()
	if s.chatSessionIndexRepairing == nil {
		s.chatSessionIndexRepairing = map[string]struct{}{}
	}
	if _, busy := s.chatSessionIndexRepairing[key]; busy {
		s.chatSessionIndexMu.Unlock()
		return
	}
	s.chatSessionIndexRepairing[key] = struct{}{}
	s.chatSessionIndexMu.Unlock()

	go func() {
		defer func() {
			s.chatSessionIndexMu.Lock()
			delete(s.chatSessionIndexRepairing, key)
			s.chatSessionIndexMu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		discovered, err := discoverChatSessionDriveManifestRecords(ctx, accessToken, rootFolderID)
		if err != nil || len(discovered) == 0 {
			return
		}
		lock := s.chatSessionIndexLock(projectID)
		lock.Lock()
		defer lock.Unlock()
		_, _ = s.mergeAndUpsertChatSessionDriveIndexLocked(ctx, accessToken, rootFolderID, discovered)
	}()
}

// loadChatSessionDriveIndexRecords reads sessions.ndjson first (CA-555). A
// non-empty index is returned immediately; discover+repair runs in the
// background. An empty/missing index still discovers runs/ synchronously so
// CA-404 orphan-manifest repair stays on the request path.
func (s *InteractiveService) loadChatSessionDriveIndexRecords(
	ctx context.Context,
	projectID, accessToken, rootFolderID string,
) ([]chatSessionDriveIndexRecord, *apiErr) {
	records, apiErr := readChatSessionDriveIndex(ctx, accessToken, rootFolderID)
	if apiErr != nil {
		return nil, apiErr
	}
	if len(records) > 0 {
		log.Printf("[chat-sync] remote index project=%s index_rows=%d discovered_manifests=deferred", projectID, len(records))
		s.scheduleChatSessionIndexRepair(projectID, accessToken, rootFolderID)
		return records, nil
	}

	discovered, discoverErr := discoverChatSessionDriveManifestRecords(ctx, accessToken, rootFolderID)
	if discoverErr != nil {
		return nil, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", discoverErr.Error())
	}
	if len(discovered) == 0 {
		log.Printf("[chat-sync] remote index project=%s index_rows=0 discovered_manifests=0", projectID)
		return []chatSessionDriveIndexRecord{}, nil
	}

	lock := s.chatSessionIndexLock(projectID)
	lock.Lock()
	defer lock.Unlock()

	merged, apiErr := s.mergeAndUpsertChatSessionDriveIndexLocked(ctx, accessToken, rootFolderID, discovered)
	if apiErr != nil {
		return nil, apiErr
	}
	records = parseChatSessionDriveIndex(merged)
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
		ChatID:          manifest.ChatID,
		LegSeq:          manifest.LegSeq,
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

		// BUG-316: upload every sidecar file (events.jsonl, updates.jsonl,
		// system_prompt.txt, ...) into the same per-run provider folder so a
		// restore can rebuild the whole Grok session directory, not just
		// chat_history.jsonl. Only ever non-empty for Grok (see
		// resolveGrokSessionSidecarFiles).
		for i := range manifest.ProviderFiles {
			sidecarBytes, ok := manifest.grokSidecarBodies[manifest.ProviderFiles[i].RelativePath]
			if !ok {
				continue
			}
			sidecarFileName := filepath.Base(manifest.ProviderFiles[i].RelativePath)
			uploadedSidecar, sidecarErr := upsertGoogleDriveFile(
				accessToken,
				runFolderID,
				sidecarFileName,
				sidecarBytes,
				"application/octet-stream",
				googleDriveAppProperties(map[string]string{"relativePath": manifest.ProviderFiles[i].RelativePath}),
			)
			if sidecarErr != nil {
				return newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", sidecarErr.Error())
			}
			manifest.ProviderFiles[i].DriveObjectID = uploadedSidecar.ID
		}
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

// uploadChildAgentManifests uploads one parent's child-agent manifests best-
// effort (extracted from syncChatRunToDrive for BUG-476 — sibling legs reuse
// the same loop so every leg's own subtree is uploaded too). Returns the
// uploaded manifests and the child runIDs that actually landed.
func (s *InteractiveService) uploadChildAgentManifests(ctx context.Context, accessToken, rootFolderID, parentRunID string, children []AgentRunSummary) ([]ChatSessionSyncManifest, []string) {
	var uploaded []ChatSessionSyncManifest
	var syncedChildRunIDs []string
	for _, child := range children {
		childRunID := strings.TrimSpace(child.RunID)
		if childRunID == "" || childRunID == parentRunID {
			continue
		}
		childManifest, childBytes, childErr := s.BuildChatSessionSyncManifest(ctx, childRunID)
		if childErr != nil {
			// Best-effort: a child whose transcript is missing locally shouldn't fail the
			// whole sync — log and continue so the rest still upload.
			log.Printf("[chat-sync] skip child run_id=%q parent=%q code=%q msg=%q", childRunID, parentRunID, childErr.code, childErr.msg)
			continue
		}
		if upErr := s.uploadChatSessionRunFiles(accessToken, rootFolderID, &childManifest, childBytes); upErr != nil {
			log.Printf("[chat-sync] child upload failed run_id=%q parent=%q code=%q msg=%q", childRunID, parentRunID, upErr.code, upErr.msg)
			continue
		}
		uploaded = append(uploaded, childManifest)
		syncedChildRunIDs = append(syncedChildRunIDs, childRunID)
	}
	return uploaded, syncedChildRunIDs
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
	childManifests, syncedChildRunIDs := s.uploadChildAgentManifests(ctx, accessToken, rootFolderID, runID, manifest.ChildAgents)
	manifests = append(manifests, childManifests...)

	// BUG-476: the sync unit is the logical chat. When this run is a leg of a
	// multi-provider chat, upload EVERY sibling leg (and each leg's child
	// agents) before publishing the chat-level manifest — the chat is only
	// advertised complete once every leg landed.
	chatLegs := []ChatSessionSyncManifest{manifest}
	syncedLegRunIDs := []string{}
	chatComplete := true
	if chatID := strings.TrimSpace(manifest.ChatID); chatID != "" {
		if reader, ok := s.workflowStore.(ChatSessionReader); ok {
			legs, legErr := reader.ListProviderSessionsByChat(ctx, chatID)
			if legErr != nil {
				log.Printf("[chat-sync] leg listing failed chat=%q: %v", chatID, legErr)
				chatComplete = false
			}
			for _, leg := range legs {
				if leg.RunID == runID {
					continue
				}
				legManifest, legBytes, buildErr := s.BuildChatSessionSyncManifest(ctx, leg.RunID)
				if buildErr != nil {
					log.Printf("[chat-sync] skip leg run_id=%q chat=%q code=%q msg=%q", leg.RunID, chatID, buildErr.code, buildErr.msg)
					chatComplete = false
					continue
				}
				if upErr := s.uploadChatSessionRunFiles(accessToken, rootFolderID, &legManifest, legBytes); upErr != nil {
					log.Printf("[chat-sync] leg upload failed run_id=%q chat=%q code=%q msg=%q", leg.RunID, chatID, upErr.code, upErr.msg)
					chatComplete = false
					continue
				}
				manifests = append(manifests, legManifest)
				chatLegs = append(chatLegs, legManifest)
				syncedLegRunIDs = append(syncedLegRunIDs, leg.RunID)
				legChildManifests, legChildSynced := s.uploadChildAgentManifests(ctx, accessToken, rootFolderID, leg.RunID, legManifest.ChildAgents)
				manifests = append(manifests, legChildManifests...)
				syncedChildRunIDs = append(syncedChildRunIDs, legChildSynced...)
			}
		}
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

	// BUG-476: publish the chat-level manifest LAST, after every leg's own
	// manifest/provider file and the index rows landed. A partial upload never
	// writes this commit marker, so an interrupted sync is discoverable only
	// as per-leg index rows — never as a complete restorable chat.
	if chatID := strings.TrimSpace(manifest.ChatID); chatID != "" && chatComplete {
		sort.Slice(chatLegs, func(i, j int) bool { return chatLegs[i].LegSeq < chatLegs[j].LegSeq })
		chatDoc := chatSessionChatManifest{
			SchemaVersion:   chatSyncManifestSchemaVersion,
			ChatID:          chatID,
			SourceMachineID: manifest.SourceMachineID,
			SyncedAt:        manifest.SyncedAt,
		}
		for _, leg := range chatLegs {
			chatDoc.Legs = append(chatDoc.Legs, chatSessionChatLeg{
				SourceRunID:  leg.SourceRunID,
				LegSeq:       leg.LegSeq,
				LegState:     leg.LegState,
				ProviderKey:  string(leg.ProviderKey),
				ManifestPath: chatSessionManifestPath(leg.SourceMachineID, leg.SourceRunID),
			})
		}
		if chatBytes, err := json.Marshal(chatDoc); err == nil {
			if chatDirID, dirErr := ensureGoogleDriveFolderPath(accessToken, rootFolderID, []string{
				"chat-sessions", "chats", safeChatSessionSegment(manifest.SourceMachineID), safeChatSessionSegment(chatID),
			}); dirErr != nil {
				log.Printf("[chat-sync] chat manifest folder failed chat=%q: %v", chatID, dirErr)
			} else if _, upErr := upsertGoogleDriveFile(accessToken, chatDirID, "chat.json", chatBytes, "application/json", nil); upErr != nil {
				log.Printf("[chat-sync] chat manifest upload failed chat=%q: %v", chatID, upErr)
			}
		}
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
	// Mark each synced leg/child as synced too (best-effort — index/parent
	// already uploaded). manifests now interleaves the primary run, sibling
	// legs, and every subtree's children, so resolve each runID's own manifest
	// rather than indexing by position.
	manifestBySourceRunID := make(map[string]ChatSessionSyncManifest, len(manifests))
	for i := range manifests {
		manifestBySourceRunID[manifests[i].SourceRunID] = manifests[i]
	}
	for _, syncedRunID := range append(syncedLegRunIDs, syncedChildRunIDs...) {
		syncedManifest, ok := manifestBySourceRunID[syncedRunID]
		if !ok {
			continue
		}
		_ = s.updateLocalSessionSyncStatus(ctx, syncedRunID, func(state *ProviderSessionState) {
			state.SourceMachineID = syncedManifest.SourceMachineID
			state.SourceRunID = syncedManifest.SourceRunID
			state.SyncStatus = "synced"
			state.SyncUpdatedAt = syncedManifest.SyncedAt
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
	// BUG-123 backward compatibility: BUG-119 uploaded child manifests and index
	// rows before parent_run_id was added. Only walk parent manifests when the
	// whole index is legacy (no ParentRunID on any row). A modern index already
	// marks children, so this scan would re-download every parent manifest on
	// every list (CA-555 timeout: ~56 chats × 5 Drive hops).
	if !chatSessionIndexHasParentRunID(records) {
		type discoveredChildren struct {
			machineID string
			runIDs    []string
		}
		discoveries := make([]discoveredChildren, len(records))
		var wg sync.WaitGroup
		sem := make(chan struct{}, remoteChatSessionsManifestScanConcurrency)
		for i, record := range records {
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
			ChatID:          record.ChatID,
			LegSeq:          record.LegSeq,
		})
	}
	// BUG-476: legs of one logical chat list as ONE row — the newest leg
	// (highest legSeq) represents the chat; restoring it pulls every leg.
	// Pre-v2 rows carry no chatId and keep their own rows (single-leg chats).
	grouped := make([]RemoteChatSessionSummary, 0, len(out))
	chatRowIdx := make(map[string]int, len(out))
	for _, row := range out {
		if strings.TrimSpace(row.ChatID) == "" {
			grouped = append(grouped, row)
			continue
		}
		key := row.SourceMachineID + "\x00" + row.ChatID
		if idx, seen := chatRowIdx[key]; seen {
			if row.LegSeq > grouped[idx].LegSeq {
				grouped[idx] = row
			}
			continue
		}
		chatRowIdx[key] = len(grouped)
		grouped = append(grouped, row)
	}
	out = grouped
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
				// BUG-320: the store only supports single-ID lookups here (no
				// ListAllProviderSessions), so collision-check each derived
				// candidate one at a time instead of returning the first one
				// unchecked.
				return firstFreeRestoredRunID(sourceMachineID, sourceRunID, func(candidate string) bool {
					_, found, _ := reader.GetProviderSession(ctx, candidate)
					return found
				})
			}
		}
		return sourceRunID
	}
	sessions, err := indexReader.ListAllProviderSessions(ctx)
	if err != nil {
		return sourceRunID
	}
	takenIDs := make(map[string]struct{}, len(sessions))
	for _, session := range sessions {
		if session.SourceMachineID == sourceMachineID && session.SourceRunID == sourceRunID && strings.TrimSpace(session.RunID) != "" {
			return session.RunID
		}
		if id := strings.TrimSpace(session.RunID); id != "" {
			takenIDs[id] = struct{}{}
		}
	}
	if _, taken := takenIDs[sourceRunID]; !taken {
		return sourceRunID
	}
	// BUG-320: resolveRestoredRunID previously returned the first derived
	// "sync-<machine>-<id>" candidate unchecked. If that candidate was ALSO
	// already taken (e.g. two source machines whose 8-char shortMachineID
	// prefixes collide, or a repeat restore racing a fresh local run that
	// happens to reuse the same derived id), UpsertProviderSession has no
	// create-only guard -- it would silently overwrite the unrelated
	// existing record. Loop with an incrementing suffix until a genuinely
	// free id is found.
	return firstFreeRestoredRunID(sourceMachineID, sourceRunID, func(candidate string) bool {
		_, taken := takenIDs[candidate]
		return taken
	})
}

// firstFreeRestoredRunID returns the first "sync-<machine>-<sourceRunID>"
// candidate that isTaken reports as free, appending an incrementing numeric
// suffix on repeated collisions (see BUG-320 note at the call sites above).
func firstFreeRestoredRunID(sourceMachineID, sourceRunID string, isTaken func(candidate string) bool) string {
	base := "sync-" + shortMachineID(sourceMachineID) + "-" + sourceRunID
	if !isTaken(base) {
		return base
	}
	for i := 2; ; i++ {
		candidate := base + "-" + strconv.Itoa(i)
		if !isTaken(candidate) {
			return candidate
		}
	}
}

func (s *InteractiveService) restoreChatRunFromDrive(ctx context.Context, req ChatSessionRestoreRequest) (ChatSessionRestoreResult, *apiErr) {
	return s.restoreChatRunTreeFromDrive(ctx, req, make(map[string]struct{}), make(map[string]string), true)
}

// remoteChatSiblingRunIDs enumerates a logical chat's other legs on Drive
// (BUG-476): prefer the chat-level commit marker (chat.json — ordered, only
// present when every leg uploaded), and fall back to index rows sharing the
// same chat_id when the marker is absent (partial/interrupted upload still
// leaves per-leg index rows worth restoring). Returned runIDs are ordered by
// legSeq so a switch chain remaps predecessor-first.
func (s *InteractiveService) remoteChatSiblingRunIDs(accessToken, rootFolderID string, indexRecords []chatSessionDriveIndexRecord, manifest ChatSessionSyncManifest) ([]string, error) {
	chatID := strings.TrimSpace(manifest.ChatID)
	if chatID == "" {
		return nil, nil
	}
	// Preferred: the commit marker written only after ALL legs uploaded.
	if chatFile, findErr := findGoogleDriveFileByLogicalPath(accessToken, rootFolderID, chatSessionChatManifestPath(manifest.SourceMachineID, chatID)); findErr == nil {
		if chatBytes, dlErr := downloadGoogleDriveFileByID(context.Background(), accessToken, chatFile.ID); dlErr == nil {
			var doc chatSessionChatManifest
			if json.Unmarshal(chatBytes, &doc) == nil && doc.ChatID == chatID && len(doc.Legs) > 0 {
				legs := append([]chatSessionChatLeg(nil), doc.Legs...)
				sort.Slice(legs, func(i, j int) bool { return legs[i].LegSeq < legs[j].LegSeq })
				out := make([]string, 0, len(legs))
				for _, leg := range legs {
					if id := strings.TrimSpace(leg.SourceRunID); id != "" {
						out = append(out, id)
					}
				}
				return out, nil
			}
		} else {
			return nil, dlErr
		}
	} else if !errors.Is(findErr, os.ErrNotExist) {
		return nil, findErr
	}
	// Fallback: per-leg index rows carrying the same chat_id (partial upload —
	// the commit marker is absent by design). Order by leg_seq; rows without
	// a seq sort last but stay restorable.
	var rows []chatSessionDriveIndexRecord
	for _, record := range indexRecords {
		if record.SourceMachineID == manifest.SourceMachineID && record.ChatID == chatID {
			rows = append(rows, record)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].LegSeq == rows[j].LegSeq {
			return rows[i].SourceRunID < rows[j].SourceRunID
		}
		return rows[i].LegSeq < rows[j].LegSeq
	})
	out := make([]string, 0, len(rows))
	for _, record := range rows {
		out = append(out, record.SourceRunID)
	}
	return out, nil
}

// chatSessionRemoteRunKnown reports whether a run has ANY remote trace on
// Drive for this project -- either an index row or (CA-404: index rows can be
// lost to a concurrent-sync last-write-wins race while the manifest blob
// itself survives) a manifest blob still present at its expected path.
// BUG-319/BUG-320: used only to tell a CHILD that was CONFIRMED never synced
// at all (no trace anywhere -- safe to tombstone instead of hard-failing the
// restore) apart from a child that WAS synced but is now broken in some other
// way (missing provider file, integrity mismatch, ...), which must still
// hard-fail the whole restore. BUG-320: a transient Drive lookup failure
// (network blip, API error) is NOT the same fact as "confirmed absent" --
// treating it as "never synced" would silently tombstone a child that may
// still have real data, the exact thing the CA-404 guard exists to prevent.
// Only a manifest-path lookup that resolves to os.ErrNotExist counts as
// confirmed-absent; any other error is returned so the caller hard-fails.
func chatSessionRemoteRunKnown(accessToken, rootFolderID string, indexRecords []chatSessionDriveIndexRecord, machineID, runID string) (bool, error) {
	for _, record := range indexRecords {
		if record.SourceMachineID == machineID && record.SourceRunID == runID {
			return true, nil
		}
	}
	_, err := findGoogleDriveFileByLogicalPath(accessToken, rootFolderID, chatSessionManifestPath(machineID, runID))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

// terminalizeTombstoneStatus normalizes a never-synced child's last-known
// status (captured in the parent's manifest.ChildAgents at sync time) into a
// tombstone-safe terminal state. normalizeResumedStatus deliberately keeps
// waiting_approval/waiting_question so a real restart can rehydrate the
// actionable gate card (rehydratePendingGatesLocked) -- but a BUG-320
// tombstone has no synced approval/question record to rehydrate from, so any
// non-terminal status here must collapse to Cancelled instead of leaving a
// permanently "waiting" card nothing will ever resolve.
func terminalizeTombstoneStatus(status RunStatus) RunStatus {
	switch normalizeResumedStatus(status) {
	case RunStatusCompleted, RunStatusFailed, RunStatusCancelled:
		return normalizeResumedStatus(status)
	default:
		return RunStatusCancelled
	}
}

// legMap maps source runID → collision-resolved local runID across the whole
// restore (requested leg + every sibling leg), so SwitchFromRunID chains and
// any cross-leg references stamp LOCAL ids. expandLegs is true only for the
// top-level restore — sibling recursion restores each leg's own subtree
// without re-expanding the chat (the restoring map also guards cycles).
func (s *InteractiveService) restoreChatRunTreeFromDrive(ctx context.Context, req ChatSessionRestoreRequest, restoring map[string]struct{}, legMap map[string]string, expandLegs bool) (ChatSessionRestoreResult, *apiErr) {
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
	indexRecords := parseChatSessionDriveIndex(indexBytes)
	var match *chatSessionDriveIndexRecord
	for _, record := range indexRecords {
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

		// BUG-316: restore every sidecar file into the SAME directory as the
		// primary file above -- Grok's own session/load call reads the whole
		// directory, not just chat_history.jsonl (see
		// resolveGrokSessionSidecarFiles). Mirrors the primary file's own
		// strictness (hard-fail on missing/integrity-mismatch/conflicting-local)
		// rather than best-effort: a silently-skipped sidecar reproduces this
		// exact bug on the very next turn. Reuses targetPath's own directory
		// rather than restoreTargetPath's per-provider prefix validation
		// (already applied once, above, for the primary file) since every
		// sidecar lives alongside it by construction. Always empty for
		// Codex/Claude manifests, so this loop is a no-op for them.
		sidecarDir := filepath.Dir(targetPath)
		for _, sidecarFile := range manifest.ProviderFiles {
			sidecarObjectID := strings.TrimSpace(sidecarFile.DriveObjectID)
			if sidecarObjectID == "" {
				file, findErr := findGoogleDriveFileByLogicalPath(accessToken, rootFolderID, chatSessionProviderLogicalPath(manifest.SourceMachineID, manifest.SourceRunID, manifest.ProviderKey, sidecarFile.RelativePath))
				if findErr != nil {
					return ChatSessionRestoreResult{}, newAPIErr(http.StatusNotFound, "sync_remote_not_found", "remote Grok session sidecar file was not found")
				}
				sidecarObjectID = file.ID
			}
			sidecarBytes, dlErr := downloadGoogleDriveFileByID(ctx, accessToken, sidecarObjectID)
			if dlErr != nil {
				return ChatSessionRestoreResult{}, newAPIErr(http.StatusNotFound, "sync_remote_not_found", "remote Grok session sidecar file was not found")
			}
			if int64(len(sidecarBytes)) != sidecarFile.SizeBytes || hashBytesSHA256(sidecarBytes) != sidecarFile.SHA256 {
				return ChatSessionRestoreResult{}, newAPIErr(http.StatusConflict, "sync_integrity_failed", "remote Grok session sidecar file failed integrity validation")
			}
			sidecarTargetPath := filepath.Join(sidecarDir, filepath.Base(sidecarFile.RelativePath))
			if existing, readErr := os.ReadFile(sidecarTargetPath); readErr == nil {
				if hashBytesSHA256(existing) != sidecarFile.SHA256 {
					return ChatSessionRestoreResult{}, newAPIErr(http.StatusConflict, "session_file_conflict", "a different local Grok session sidecar file already exists for this chat")
				}
			} else if !errors.Is(readErr, os.ErrNotExist) {
				return ChatSessionRestoreResult{}, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", readErr.Error())
			} else {
				if err := os.MkdirAll(sidecarDir, 0o755); err != nil {
					return ChatSessionRestoreResult{}, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
				}
				if err := os.WriteFile(sidecarTargetPath, sidecarBytes, 0o644); err != nil {
					return ChatSessionRestoreResult{}, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
				}
			}
		}
	}

	localRunID := s.resolveRestoredRunID(ctx, manifest.SourceMachineID, manifest.SourceRunID)
	legMap[manifest.SourceRunID] = localRunID

	// BUG-476: restore the whole logical chat — sibling legs in legSeq order
	// so switch chains remap predecessor-first into legMap before this leg's
	// own session is stamped below. A sibling restore failure hard-fails like
	// a missing child: silently dropping a leg recreates the exact data loss
	// this change fixes.
	if expandLegs {
		siblings, sibErr := s.remoteChatSiblingRunIDs(accessToken, rootFolderID, indexRecords, manifest)
		if sibErr != nil {
			return ChatSessionRestoreResult{}, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", sibErr.Error())
		}
		for _, sibRunID := range siblings {
			if sibRunID == manifest.SourceRunID {
				continue
			}
			if _, sibRestoreErr := s.restoreChatRunTreeFromDrive(ctx, ChatSessionRestoreRequest{
				ProjectID:       projectID,
				SourceMachineID: manifest.SourceMachineID,
				SourceRunID:     sibRunID,
				Cwd:             cwd,
			}, restoring, legMap, false); sibRestoreErr != nil {
				return ChatSessionRestoreResult{}, sibRestoreErr
			}
		}
	}

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
		// BUG-476: restore the leg's logical-chat identity so the restored chat
		// groups correctly via ListProviderSessionsByChat and keeps its history.
		ChatID:          manifest.ChatID,
		LegSeq:          manifest.LegSeq,
		LegState:        manifest.LegState,
		LegClosedReason: manifest.LegClosedReason,
	}
	// Switch lineage stamps the LOCAL id of the predecessor leg (restored
	// earlier in legSeq order); an unmapped source id is kept verbatim so the
	// chain stays diagnostically traceable rather than silently dropped.
	if src := strings.TrimSpace(manifest.SwitchFromRunID); src != "" {
		if mapped, ok := legMap[src]; ok {
			session.SwitchFromRunID = mapped
		} else {
			session.SwitchFromRunID = src
		}
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
				// BUG-322 (CP-51 C6): fields added after BUG-091 that also track live
				// flow progress, not just conversation metadata -- a stale remote
				// manifest must not regress these either, or a re-restore of an
				// already-locally-progressed chat can resurrect BUG-315's own
				// symptom (flow re-runs from scratch) by rolling turnCount/loop
				// state back down. Only fields that mutate over a run's lifetime are
				// preserved; static per-run identity (ProjectID, AgentName, ModelName,
				// DependsOn, ChatFlowRef, ...) is left as the manifest's, since it does
				// not change after the run starts and should already agree.
				if local.TurnCount > session.TurnCount {
					session.TurnCount = local.TurnCount
				}
				if local.LoopState.Round > session.LoopState.Round {
					session.LoopState = local.LoopState
				}
				if strings.TrimSpace(local.AgentStatus) != "" {
					session.AgentStatus = local.AgentStatus
				}
				if len(local.ActiveFlowNodes) > 0 {
					session.ActiveFlowNodes = append([]agentpack.FlowNode(nil), local.ActiveFlowNodes...)
				}
				if len(local.ActiveFlowEdges) > 0 {
					session.ActiveFlowEdges = append([]agentpack.FlowEdge(nil), local.ActiveFlowEdges...)
				}
				if len(local.PendingAgentContext) > 0 {
					session.PendingAgentContext = append([]string(nil), local.PendingAgentContext...)
				}
				if strings.TrimSpace(local.FlowCohortID) != "" {
					session.FlowCohortID = local.FlowCohortID
				}
			}
		}
	}
	// Restore every child transcript referenced by the parent manifest and persist its
	// relationship metadata. Loading summaries alone made the panel look correct only
	// until restart and left child chats unopened on the restored machine. (BUG-123)
	var remapped []AgentRunSummary
	// BUG-320: staged tombstones for children confirmed never-synced, keyed by
	// their (already collision-resolved) local RunID. Persisted once, AFTER
	// every child in this parent's ChildAgents list has been validated/restored
	// and DependsOn remapping has finished, immediately before the parent
	// itself -- never inside the loop. Persisting eagerly would (a) leave an
	// orphan tombstone on disk if a LATER sibling child hard-fails and this
	// whole restore call returns an error, and (b) get double-upserted by the
	// metadata-patch pass below, which patches already-restored real children
	// in place rather than writing a fresh record.
	tombstones := map[string]*ProviderSessionState{}
	if len(manifest.ChildAgents) > 0 {
		runIDMap := map[string]string{manifest.SourceRunID: localRunID}
		for _, child := range manifest.ChildAgents {
			entry := child
			// A synced child's status is a live snapshot from the source machine;
			// once restored elsewhere nothing is actually running it, so an
			// in-flight status must be normalized the same way the disk-fallback
			// branch of listAgentRunSummaries already does (BUG-251 parity —
			// Task-190) — otherwise a reviewer synced mid-turn shows a
			// permanently stale "running" badge on the restored machine.
			entry.Status = normalizeResumedStatus(child.Status)
			entry.AgentStatus = string(normalizeResumedStatus(RunStatus(child.AgentStatus)))
			entry.DependsOn = append([]string(nil), child.DependsOn...)
			childRunID := strings.TrimSpace(child.RunID)
			if childRunID == "" {
				continue
			}
			// BUG-319/BUG-320: a child whose own turn was interrupted before it
			// ever wrote a real transcript is never uploaded during sync-up
			// (syncChatRunToDrive's own best-effort "skip child" branch, above) --
			// it has no index row and no manifest blob anywhere on Drive. That's
			// not a data problem with the REST of this tree (the hub and every
			// other child may be fully restorable), so this child gets a
			// metadata-only TOMBSTONE record instead of a real transcript restore
			// -- rather than failing the whole request (mirrors sync-up's own
			// per-child leniency) or vanishing from the tree with no trace at all
			// (BUG-320: resumedFlowStepRows' evidence-walk needs SOME persisted
			// session for this child to mark its flow step FAILED/CANCELED
			// instead of defaulting every evidence-less node to DONE). A child
			// that WAS synced (has an index row, or per CA-404 at least a
			// surviving manifest blob) but fails to restore for any OTHER reason
			// (missing provider file, integrity mismatch, ...) still hard-fails
			// below, unchanged -- restoring it must never silently drop real data
			// just because it's now broken.
			known, knownErr := chatSessionRemoteRunKnown(accessToken, rootFolderID, indexRecords, manifest.SourceMachineID, childRunID)
			if knownErr != nil {
				// BUG-320: a transient Drive lookup failure (network blip, API
				// error) is not the same fact as "confirmed absent". Treating it
				// as never-synced could tombstone a child that still has real,
				// recoverable data -- exactly what the CA-404 guard exists to
				// prevent. Hard-fail instead of guessing.
				return ChatSessionRestoreResult{}, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", knownErr.Error())
			}
			if !known {
				log.Printf("[chat-sync] tombstone child restore (never synced) run_id=%q parent=%q", childRunID, manifest.SourceRunID)
				tombstoneID := s.resolveRestoredRunID(ctx, manifest.SourceMachineID, childRunID)
				now := time.Now().UTC().Format(time.RFC3339Nano)
				tombstones[tombstoneID] = &ProviderSessionState{
					RunID:           tombstoneID,
					ProjectID:       projectID,
					ProviderKey:     ProviderKey(child.ProviderKey),
					RunKind:         "chat",
					Status:          terminalizeTombstoneStatus(child.Status),
					AgentStatus:     string(terminalizeTombstoneStatus(RunStatus(child.AgentStatus))),
					AgentName:       child.AgentName,
					Role:            child.Role,
					Label:           child.Label,
					ModelName:       child.ModelName,
					StartedAt:       child.CreatedAt,
					UpdatedAt:       now,
					SourceMachineID: manifest.SourceMachineID,
					SourceRunID:     childRunID,
					RestoredFrom:    "google_drive",
					// BUG-311 semantics: a transcript-less tombstone can never be
					// synced FROM this machine either -- it has no real session
					// data here any more than it did on the source.
					SyncStatus:    "unsyncable",
					SyncUpdatedAt: now,
				}
				runIDMap[childRunID] = tombstoneID
				entry.RunID = tombstoneID
				entry.Status = terminalizeTombstoneStatus(child.Status)
				entry.AgentStatus = string(terminalizeTombstoneStatus(RunStatus(child.AgentStatus)))
				remapped = append(remapped, entry)
				continue
			}
			childResult, childErr := s.restoreChatRunTreeFromDrive(ctx, ChatSessionRestoreRequest{
				ProjectID:       projectID,
				SourceMachineID: manifest.SourceMachineID,
				SourceRunID:     childRunID,
				Cwd:             cwd,
			}, restoring, legMap, false)
			if childErr != nil {
				log.Printf("[chat-sync] child restore failed run_id=%q parent=%q code=%q msg=%q", childRunID, manifest.SourceRunID, childErr.code, childErr.msg)
				return ChatSessionRestoreResult{}, childErr
			}
			runIDMap[childRunID] = childResult.RunID
			entry.RunID = childResult.RunID
			remapped = append(remapped, entry)
		}
		for i := range remapped {
			remapped[i].ParentRunID = localRunID
			for j, dependency := range remapped[i].DependsOn {
				if localDependency, ok := runIDMap[dependency]; ok {
					remapped[i].DependsOn[j] = localDependency
				}
			}
			if tombstone, isTombstone := tombstones[remapped[i].RunID]; isTombstone {
				// Tombstones are staged, fully-built records (not yet persisted) --
				// fold in the final parent id + remapped dependencies directly
				// rather than patching a real persisted record via
				// updateLocalSessionSyncStatus.
				tombstone.ParentRunID = localRunID
				tombstone.DependsOn = append([]string(nil), remapped[i].DependsOn...)
				continue
			}
			if metadataErr := s.updateLocalSessionSyncStatus(ctx, remapped[i].RunID, func(childSession *ProviderSessionState) {
				childSession.ParentRunID = localRunID
				childSession.AgentName = remapped[i].AgentName
				childSession.Role = remapped[i].Role
				childSession.Label = remapped[i].Label
				childSession.DependsOn = append([]string(nil), remapped[i].DependsOn...)
				childSession.AgentStatus = remapped[i].AgentStatus
				childSession.ModelName = remapped[i].ModelName
			}); metadataErr != nil {
				return ChatSessionRestoreResult{}, metadataErr
			}
		}
		// BUG-320: persist every staged tombstone now that every child has been
		// validated/restored (so a hard-failing sibling never leaves one behind)
		// and DependsOn has its final remapped values -- still strictly before
		// the parent's own persist below.
		for _, tombstone := range tombstones {
			if err := s.persistProviderSession(*tombstone); err != nil {
				return ChatSessionRestoreResult{}, newAPIErr(http.StatusBadGateway, "workflow_state_unavailable", err.Error())
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
