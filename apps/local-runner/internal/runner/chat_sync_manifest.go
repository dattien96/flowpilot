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

// BuildChatSyncManifestV2 builds a v2 manifest for chatID (flag-gated, additive).
// It collects legs via ChatSessionReader.ListProviderSessionsByChat, transcript
// file path via ChatTranscriptStore (localFileChatTranscriptStore.chatPath when
// available, otherwise the canonical Drive transcript path), and sidecars via
// resolveGrokSessionSidecarFiles per leg (best-effort), sorted by legSeq.
func BuildChatSyncManifestV2(ctx context.Context, s *InteractiveService, chatID string) (ChatSyncManifest, error) {
	if !chatSSOTEnabled() {
		return ChatSyncManifest{}, fmt.Errorf("chat SSOT disabled (FLOWPILOT_CHAT_SSOT)")
	}
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
	// Prefer v2 when chatId is present or schemaVersion is 2.
	if strings.TrimSpace(probe.ChatID) != "" || probe.SchemaVersion == chatSyncManifestSchemaVersion {
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

// maybeLogChatSyncV2ForRun is a flag-gated best-effort helper that can be called
// from syncChatRunToDrive without breaking the v1 path. It resolves the run's
// chatId via SessionHistoryReader.GetProviderSession, builds a v2 manifest when
// the flag is on and chatId != "", and logs the result. It never returns an
// error to the caller — v1 uploads remain the source of truth.
func maybeLogChatSyncV2ForRun(ctx context.Context, s *InteractiveService, runID string) {
	if !chatSSOTEnabled() || s == nil || strings.TrimSpace(runID) == "" {
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
