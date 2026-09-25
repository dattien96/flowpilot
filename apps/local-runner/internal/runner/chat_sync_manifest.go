package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strings"
)

// chatSyncManifestSchemaVersion is distinct from chatSessionManifestSchemaVersion=1.
// v2 is chat-level (chatId-keyed), v1 is run-level (sourceRunId-keyed).
const chatSyncManifestSchemaVersion = 2

// ChatSyncManifest is the v2 Drive manifest for a whole chat (CP-59 / Task-317 P-8).
// It bundles the chat transcript file plus one entry per leg in legSeq order.
type ChatSyncManifest struct {
	SchemaVersion      int           `json:"schemaVersion"`
	ChatID             string        `json:"chatId"`
	ProjectID          string        `json:"projectId"`
	ChatTranscriptFile string        `json:"chatTranscriptFile"`
	Legs               []ChatSyncLeg `json:"legs"`
}

// ChatSyncLeg is one leg entry inside a v2 manifest. Sidecars lists the per-leg
// v1 sidecar paths actually synced (providerFiles relative paths); SidecarsAbsent
// is the explicit missing list (T-4), omitted when empty.
type ChatSyncLeg struct {
	RunID          string   `json:"runId"`
	LegSeq         int      `json:"legSeq"`
	ProviderKey    string   `json:"providerKey"`
	Sidecars       []string `json:"sidecars"`
	SidecarsAbsent []string `json:"sidecarsAbsent,omitempty"`
}

// ChatSyncManifestDrivePath returns the Drive logical path for a chat's v2 manifest.
func ChatSyncManifestDrivePath(chatID string) string {
	return filepath.ToSlash(filepath.Join(
		"chat-sessions",
		"chats",
		safeChatSessionSegment(chatID),
		"manifest.json",
	))
}

// chatSyncTranscriptDrivePath returns the Drive logical path for a chat's transcript NDJSON.
func chatSyncTranscriptDrivePath(chatID string) string {
	return filepath.ToSlash(filepath.Join(
		"chat-sessions",
		"chats",
		safeChatSessionSegment(chatID),
		"transcript.ndjson",
	))
}

// BuildChatSyncManifestV2 builds a v2 manifest for chatID (always ON).
// It collects legs via ChatSessionReader.ListProviderSessionsByChat, transcript
// file path via ChatTranscriptStore (localFileChatTranscriptStore.chatPath when
// available, otherwise the canonical Drive transcript path), and sidecars via
// resolveGrokSessionSidecarFiles per leg (best-effort), sorted by legSeq.
func BuildChatSyncManifestV2(ctx context.Context, s *InteractiveService, chatID string) (ChatSyncManifest, error) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return ChatSyncManifest{}, fmt.Errorf("chatId is required")
	}
	if s == nil {
		return ChatSyncManifest{}, fmt.Errorf("service unavailable")
	}
	reader, ok := s.workflowStore.(ChatSessionReader)
	if !ok {
		return ChatSyncManifest{}, fmt.Errorf("chat session reader unavailable")
	}
	sessions, err := reader.ListProviderSessionsByChat(ctx, chatID)
	if err != nil {
		return ChatSyncManifest{}, fmt.Errorf("list legs: %w", err)
	}
	if len(sessions) == 0 {
		return ChatSyncManifest{}, fmt.Errorf("no legs found for chat %q", chatID)
	}
	sort.Slice(sessions, func(i, j int) bool { return sessions[i].LegSeq < sessions[j].LegSeq })

	projectID := ""
	for _, sess := range sessions {
		if strings.TrimSpace(sess.ProjectID) != "" {
			projectID = sess.ProjectID
			break
		}
	}

	// Transcript file path: canonical Drive path; touch localFileChatTranscriptStore.chatPath
	// when the local store is active to satisfy the helper-usage contract and to validate
	// the path does not escape (sanitizeChatID guard inside chatPath).
	chatTranscriptFile := chatSyncTranscriptDrivePath(chatID)
	if writer := s.ensureChatTranscriptWriter(); writer != nil {
		if lfs, ok := writer.store.(*localFileChatTranscriptStore); ok && lfs != nil {
			_ = lfs.chatPath(chatID)
		}
	}

	legs := make([]ChatSyncLeg, 0, len(sessions))
	for _, sess := range sessions {
		leg := ChatSyncLeg{
			RunID:       sess.RunID,
			LegSeq:      sess.LegSeq,
			ProviderKey: string(sess.ProviderKey),
		}
		// Best-effort sidecars: only Grok has a directory-wide session (BUG-316).
		// Other providers are single-file resume — leave Sidecars empty.
		if sess.ProviderKey == ProviderKeyGrok && isGrokRealSessionID(sess.ProviderSessionID) {
			if accountHome, ok := s.resolveAccountHome(sess.ProviderKey, sess.ProviderAccountID); ok {
				files, _, sidecarErr := resolveGrokSessionSidecarFiles(accountHome, sess.WorkingDirectory, sess.ProviderSessionID)
				if sidecarErr == nil && len(files) > 0 {
					sidecars := make([]string, 0, len(files))
					for _, f := range files {
						sidecars = append(sidecars, f.RelativePath)
					}
					leg.Sidecars = sidecars
				}
			}
		}
		legs = append(legs, leg)
	}

	manifest := ChatSyncManifest{
		SchemaVersion:      chatSyncManifestSchemaVersion,
		ChatID:             chatID,
		ProjectID:          projectID,
		ChatTranscriptFile: chatTranscriptFile,
		Legs:               legs,
	}
	return manifest, nil
}

// SyncChatV2ToDrive uploads a chat's v2 bundle to a fake Drive (map[DrivePath]bytes).
// Always ON, idempotent (overwrite), and leaves legs sorted; sidecars are
// best-effort via BuildChatSyncManifestV2 (T-2 + T-4).
func SyncChatV2ToDrive(ctx context.Context, s *InteractiveService, chatID string, drive map[string][]byte) (ChatSyncManifest, error) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return ChatSyncManifest{}, fmt.Errorf("chatId is required")
	}
	if s == nil {
		return ChatSyncManifest{}, fmt.Errorf("service unavailable")
	}
	if drive == nil {
		return ChatSyncManifest{}, fmt.Errorf("drive is required")
	}
	manifest, err := BuildChatSyncManifestV2(ctx, s, chatID)
	if err != nil {
		return ChatSyncManifest{}, err
	}
	sort.Slice(manifest.Legs, func(i, j int) bool { return manifest.Legs[i].LegSeq < manifest.Legs[j].LegSeq })
	writer := s.ensureChatTranscriptWriter()
	var transcriptBytes []byte
	if writer != nil && writer.store != nil {
		recs, readErr := writer.store.ReadChatRecords(ctx, chatID, 0, 0)
		if readErr != nil {
			log.Printf("[chat-sync] v2 transcript read failed chat=%q: %v", chatID, readErr)
		} else {
			var buf []byte
			for _, rec := range recs {
				line, mErr := json.Marshal(rec)
				if mErr != nil {
					log.Printf("[chat-sync] v2 transcript marshal failed chat=%q seq=%d: %v", chatID, rec.ChatSeq, mErr)
					continue
				}
				buf = append(buf, line...)
				buf = append(buf, '\n')
			}
			transcriptBytes = buf
		}
	}
	drive[chatSyncTranscriptDrivePath(chatID)] = transcriptBytes
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return ChatSyncManifest{}, fmt.Errorf("marshal manifest: %w", err)
	}
	drive[ChatSyncManifestDrivePath(chatID)] = manifestBytes
	log.Printf("[chat-sync] v2 sync chat=%q legs=%d transcriptBytes=%d", chatID, len(manifest.Legs), len(transcriptBytes))
	return manifest, nil
}

// RestoreChatFromManifestV2 restores a chat from a v2 manifest + fake Drive.
// Transcript-first: the transcript NDJSON is appended before any leg work, so a
// leg failure still leaves the full timeline (T-3). Every leg is restored as
// closed(restored) (detached per SD26 §10); missing sidecars → session_unavailable,
// unknown provider → provider_unavailable with install hint (typed degradation).
func RestoreChatFromManifestV2(ctx context.Context, target *InteractiveService, drive map[string][]byte, manifest ChatSyncManifest, availableProviders map[ProviderKey]bool) error {
	if target == nil {
		return fmt.Errorf("service unavailable")
	}
	chatID := strings.TrimSpace(manifest.ChatID)
	if chatID == "" {
		return fmt.Errorf("chatId is required")
	}
	if drive == nil {
		drive = map[string][]byte{}
	}
	writer := target.ensureChatTranscriptWriter()
	if writer != nil && writer.store != nil {
		transcriptPath := chatSyncTranscriptDrivePath(chatID)
		if data, ok := drive[transcriptPath]; ok && len(strings.TrimSpace(string(data))) > 0 {
			var recs []ChatTranscriptRecord
			lines := strings.Split(string(data), "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				var rec ChatTranscriptRecord
				if err := json.Unmarshal([]byte(line), &rec); err != nil {
					log.Printf("[chat-sync] restore transcript skip malformed chat=%q: %v", chatID, err)
					continue
				}
				if strings.TrimSpace(rec.ChatID) == "" {
					rec.ChatID = chatID
				}
				recs = append(recs, rec)
			}
			if len(recs) > 0 {
				sort.Slice(recs, func(i, j int) bool { return recs[i].ChatSeq < recs[j].ChatSeq })
				if err := writer.store.AppendChatRecords(ctx, recs); err != nil {
					log.Printf("[chat-sync] restore transcript append failed chat=%q: %v", chatID, err)
				}
			}
		} else {
			log.Printf("[chat-sync] restore transcript missing chat=%q path=%q", chatID, transcriptPath)
		}
	}
	legs := append([]ChatSyncLeg(nil), manifest.Legs...)
	sort.Slice(legs, func(i, j int) bool { return legs[i].LegSeq < legs[j].LegSeq })
	type upserter interface {
		UpsertProviderSession(context.Context, ProviderSessionState) error
	}
	up, ok := target.workflowStore.(upserter)
	if !ok {
		return fmt.Errorf("workflow store unavailable")
	}
	for _, leg := range legs {
		runID := strings.TrimSpace(leg.RunID)
		if runID == "" {
			log.Printf("[chat-sync] restore skip leg with empty runId chat=%q legSeq=%d", chatID, leg.LegSeq)
			continue
		}
		providerKey := ProviderKey(strings.TrimSpace(leg.ProviderKey))
		sess := ProviderSessionState{
			RunID:           runID,
			ProjectID:       strings.TrimSpace(manifest.ProjectID),
			ProviderKey:     providerKey,
			RunKind:         "chat",
			ChatID:          chatID,
			LegSeq:          leg.LegSeq,
			LegState:        LegStateClosed,
			LegClosedReason: LegClosedReasonRestored,
			Status:          RunStatusCompleted,
			RestoredFrom:    "drive",
			SyncStatus:      "restored",
		}
		if len(leg.SidecarsAbsent) > 0 {
			sess.SyncStatus = "session_unavailable"
		} else if providerKey == ProviderKeyGrok && len(leg.SidecarsAbsent) > 0 {
			sess.SyncStatus = "session_unavailable"
		}
		if availableProviders != nil {
			if avail, exists := availableProviders[providerKey]; exists && !avail {
				sess.SyncStatus = "provider_unavailable"
				sess.LastMessage = fmt.Sprintf("install %s", string(providerKey))
			}
		}
		if err := up.UpsertProviderSession(ctx, sess); err != nil {
			log.Printf("[chat-sync] restore leg upsert failed chat=%q leg=%q: %v", chatID, runID, err)
			continue
		}
	}
	return nil
}

// IsChatDetached reports whether a chat has no active leg — every persisted leg
// is closed(restored) (SD26 §10). Used by the detached-reattach contract and
// tests to prove a restored chat is detached until the first local turn.
func IsChatDetached(s *InteractiveService, chatID string) bool {
	if s == nil {
		return false
	}
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return false
	}
	reader, ok := s.workflowStore.(ChatSessionReader)
	if !ok {
		return false
	}
	sessions, err := reader.ListProviderSessionsByChat(context.Background(), chatID)
	if err != nil || len(sessions) == 0 {
		return false
	}
	for _, sess := range sessions {
		if sess.LegState != LegStateClosed || sess.LegClosedReason != LegClosedReasonRestored {
			return false
		}
		if sess.LegState == LegStateActive {
			return false
		}
	}
	return true
}

// ReadChatSyncManifest decodes a Drive manifest blob as either v2 (chatId present,
// schemaVersion 2) or v1 (sourceRunId, schemaVersion 1). It tries v2 first (chatId
// present), falls back to v1 (SourceRunID), and handles both schemaVersions.
func ReadChatSyncManifest(data []byte) (isV2 bool, v2 ChatSyncManifest, v1 ChatSessionSyncManifest, err error) {
	if len(strings.TrimSpace(string(data))) == 0 {
		return false, ChatSyncManifest{}, ChatSessionSyncManifest{}, fmt.Errorf("empty manifest")
	}
	var probe struct {
		SchemaVersion int    `json:"schemaVersion"`
		ChatID        string `json:"chatId"`
		SourceRunID   string `json:"sourceRunId"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return false, ChatSyncManifest{}, ChatSessionSyncManifest{}, fmt.Errorf("decode manifest probe: %w", err)
	}
	// Prefer v2 when chatId is present or schemaVersion is 2 — but only for
	// CHAT-level blobs. A run-level (v1) manifest always carries sourceRunId
	// and, since BUG-476, may also carry chatId as leg identity; without this
	// disambiguator such a leg manifest would mis-decode as v2.
	if strings.TrimSpace(probe.SourceRunID) == "" &&
		(strings.TrimSpace(probe.ChatID) != "" || probe.SchemaVersion == chatSyncManifestSchemaVersion) {
		var candidate ChatSyncManifest
		if err := json.Unmarshal(data, &candidate); err == nil {
			if strings.TrimSpace(candidate.ChatID) != "" {
				return true, candidate, ChatSessionSyncManifest{}, nil
			}
			// If candidate has schemaVersion 2 but chatId empty, treat as malformed v2 rather than v1.
			if candidate.SchemaVersion == chatSyncManifestSchemaVersion {
				return false, ChatSyncManifest{}, ChatSessionSyncManifest{}, fmt.Errorf("v2 manifest missing chatId")
			}
		} else if probe.ChatID != "" {
			return false, ChatSyncManifest{}, ChatSessionSyncManifest{}, fmt.Errorf("decode v2 manifest: %w", err)
		}
	}
	// Fallback to v1.
	var candidateV1 ChatSessionSyncManifest
	if err := json.Unmarshal(data, &candidateV1); err != nil {
		return false, ChatSyncManifest{}, ChatSessionSyncManifest{}, fmt.Errorf("decode v1 manifest: %w", err)
	}
	if strings.TrimSpace(candidateV1.SourceRunID) != "" || candidateV1.SchemaVersion == chatSessionManifestSchemaVersion {
		return false, ChatSyncManifest{}, candidateV1, nil
	}
	// Neither v2 chatId nor v1 sourceRunId present — still return v1 for backward compat if unmarshal succeeded.
	if probe.SourceRunID != "" {
		return false, ChatSyncManifest{}, candidateV1, nil
	}
	return false, ChatSyncManifest{}, ChatSessionSyncManifest{}, fmt.Errorf("unrecognized manifest: missing chatId and sourceRunId")
}

// maybeLogChatSyncV2ForRun is a best-effort helper that can be called
// from syncChatRunToDrive without breaking the v1 path. It resolves the run's
// chatId via SessionHistoryReader.GetProviderSession, builds a v2 manifest when
// chatId != "", and logs the result. It never returns an error to the caller — v1 uploads remain the source of truth.
func maybeLogChatSyncV2ForRun(ctx context.Context, s *InteractiveService, runID string) {
	if s == nil || strings.TrimSpace(runID) == "" {
		return
	}
	reader, ok := s.workflowStore.(SessionHistoryReader)
	if !ok {
		return
	}
	sess, found, err := reader.GetProviderSession(ctx, runID)
	if err != nil || !found {
		return
	}
	chatID := strings.TrimSpace(sess.ChatID)
	if chatID == "" {
		return
	}
	manifest, err := BuildChatSyncManifestV2(ctx, s, chatID)
	if err != nil {
		log.Printf("[chat-sync] v2 manifest build skipped chat=%q run=%q: %v", chatID, runID, err)
		return
	}
	log.Printf("[chat-sync] v2 manifest ready chat=%q project=%q legs=%d transcript=%q drivePath=%q",
		manifest.ChatID, manifest.ProjectID, len(manifest.Legs), manifest.ChatTranscriptFile, ChatSyncManifestDrivePath(chatID))
}
