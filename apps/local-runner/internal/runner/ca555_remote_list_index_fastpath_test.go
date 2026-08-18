package runner

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"
)

func plantDriveIndex(t *testing.T, api *fakeChatDriveAPI, rootID string, records []chatSessionDriveIndexRecord) {
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
		id := "plant-idx-" + strconv.Itoa(api.nextID)
		api.files[id] = fakeChatDriveFile{
			ID:       id,
			Name:     name,
			ParentID: parentID,
			MimeType: googleDriveFolderMimeType,
		}
		return id
	}
	chatSessions := ensure(rootID, "chat-sessions")
	indexFolder := ensure(chatSessions, "_index")
	lines := make([]string, 0, len(records))
	for _, rec := range records {
		line, err := json.Marshal(rec)
		if err != nil {
			t.Fatalf("marshal index record: %v", err)
		}
		lines = append(lines, string(line))
	}
	api.nextID++
	id := "plant-sessions-" + strconv.Itoa(api.nextID)
	api.files[id] = fakeChatDriveFile{
		ID:       id,
		Name:     "sessions.ndjson",
		ParentID: indexFolder,
		MimeType: "application/x-ndjson",
		Content:  []byte(strings.Join(lines, "\n") + "\n"),
	}
}

func ca555IndexRecord(provider ProviderKey, machineID, runID, parentRunID, updatedAt string) chatSessionDriveIndexRecord {
	return chatSessionDriveIndexRecord{
		RunID:           runID,
		ProjectID:       "project-1",
		ProviderKey:     string(provider),
		Status:          "completed",
		RunKind:         "chat",
		SourceMachineID: machineID,
		SourceRunID:     runID,
		LastPrompt:      runID + " prompt",
		UpdatedAt:       updatedAt,
		SyncedAt:        updatedAt,
		ManifestPath:    chatSessionManifestPath(machineID, runID),
		ParentRunID:     parentRunID,
	}
}

func TestCA555ListRemoteUsesIndexWithoutDiscoveringOrphans(t *testing.T) {
	svc, _, _, api, _, _ := newChatSyncService(t)
	plantDriveIndex(t, api, "drive-root", []chatSessionDriveIndexRecord{
		ca555IndexRecord(ProviderKeyClaude, "mch_a", "run-claude", "", "2026-08-18T01:00:00Z"),
		ca555IndexRecord(ProviderKeyCodex, "mch_a", "run-codex", "", "2026-08-18T02:00:00Z"),
		ca555IndexRecord(ProviderKeyGrok, "mch_b", "run-grok", "", "2026-08-18T03:00:00Z"),
	})
	plantDriveRunManifest(t, api, "drive-root", ChatSessionSyncManifest{
		SchemaVersion:   1,
		SourceMachineID: "mch_orphan",
		SourceRunID:     "run-orphan-not-in-index",
		ProjectID:       "project-1",
		ProviderKey:     ProviderKeyClaude,
		RunKind:         "chat",
		Status:          "completed",
		LastPrompt:      "should not appear on this list",
		SyncedAt:        "2026-08-18T04:00:00Z",
		UpdatedAt:       "2026-08-18T04:00:00Z",
	})

	summaries, apiErr := svc.listRemoteChatSessions(context.Background(), "project-1")
	if apiErr != nil {
		t.Fatalf("listRemoteChatSessions: %v", apiErr)
	}
	if len(summaries) != 3 {
		t.Fatalf("want 3 index rows on the request path, got %#v", summaries)
	}
	ids := map[string]ProviderKey{}
	for _, s := range summaries {
		ids[s.SourceRunID] = s.ProviderKey
	}
	if ids["run-claude"] != ProviderKeyClaude || ids["run-codex"] != ProviderKeyCodex || ids["run-grok"] != ProviderKeyGrok {
		t.Fatalf("mixed-provider index rows missing: %#v", summaries)
	}
	if _, leaked := ids["run-orphan-not-in-index"]; leaked {
		t.Fatalf("orphan manifest must not appear on the index-first list: %#v", summaries)
	}
}

func TestCA555EmptyIndexStillRepairsMixedProviderOrphans(t *testing.T) {
	svc, _, _, api, _, _ := newChatSyncService(t)
	plantDriveRunManifest(t, api, "drive-root", ChatSessionSyncManifest{
		SchemaVersion:   1,
		SourceMachineID: "mch_empty",
		SourceRunID:     "run-claude-orphan",
		ProjectID:       "project-1",
		ProviderKey:     ProviderKeyClaude,
		RunKind:         "chat",
		Status:          "completed",
		LastPrompt:      "claude orphan",
		SyncedAt:        "2026-08-18T01:00:00Z",
		UpdatedAt:       "2026-08-18T01:00:00Z",
	})
	plantDriveRunManifest(t, api, "drive-root", ChatSessionSyncManifest{
		SchemaVersion:   1,
		SourceMachineID: "mch_empty",
		SourceRunID:     "run-codex-orphan",
		ProjectID:       "project-1",
		ProviderKey:     ProviderKeyCodex,
		RunKind:         "chat",
		Status:          "completed",
		LastPrompt:      "codex orphan",
		SyncedAt:        "2026-08-18T02:00:00Z",
		UpdatedAt:       "2026-08-18T02:00:00Z",
	})
	plantDriveRunManifest(t, api, "drive-root", ChatSessionSyncManifest{
		SchemaVersion:   1,
		SourceMachineID: "mch_empty",
		SourceRunID:     "run-grok-orphan",
		ProjectID:       "project-1",
		ProviderKey:     ProviderKeyGrok,
		RunKind:         "chat",
		Status:          "completed",
		LastPrompt:      "grok orphan",
		SyncedAt:        "2026-08-18T03:00:00Z",
		UpdatedAt:       "2026-08-18T03:00:00Z",
	})

	summaries, apiErr := svc.listRemoteChatSessions(context.Background(), "project-1")
	if apiErr != nil {
		t.Fatalf("listRemoteChatSessions: %v", apiErr)
	}
	if len(summaries) != 3 {
		t.Fatalf("CA-404 empty-index repair must return 3 mixed-provider orphans, got %#v", summaries)
	}
	ids := map[string]ProviderKey{}
	for _, s := range summaries {
		ids[s.SourceRunID] = s.ProviderKey
	}
	if ids["run-claude-orphan"] != ProviderKeyClaude || ids["run-codex-orphan"] != ProviderKeyCodex || ids["run-grok-orphan"] != ProviderKeyGrok {
		t.Fatalf("repaired mixed-provider orphans missing: %#v", summaries)
	}
}

func TestCA555ModernIndexHidesChildrenWithoutManifestScan(t *testing.T) {
	svc, _, _, api, _, _ := newChatSyncService(t)
	plantDriveIndex(t, api, "drive-root", []chatSessionDriveIndexRecord{
		ca555IndexRecord(ProviderKeyClaude, "mch_mod", "run-parent", "", "2026-08-18T01:00:00Z"),
		ca555IndexRecord(ProviderKeyClaude, "mch_mod", "run-child", "run-parent", "2026-08-18T01:01:00Z"),
		ca555IndexRecord(ProviderKeyCodex, "mch_mod", "run-sibling", "", "2026-08-18T02:00:00Z"),
		ca555IndexRecord(ProviderKeyGrok, "mch_mod", "run-hub", "", "2026-08-18T03:00:00Z"),
	})

	summaries, apiErr := svc.listRemoteChatSessions(context.Background(), "project-1")
	if apiErr != nil {
		t.Fatalf("listRemoteChatSessions: %v", apiErr)
	}
	if len(summaries) != 3 {
		t.Fatalf("want 3 top-level parents on modern index, got %#v", summaries)
	}
	for _, s := range summaries {
		if s.SourceRunID == "run-child" {
			t.Fatalf("modern-index child leaked: %#v", summaries)
		}
	}
}

func TestCA555LegacyIndexStillHidesChildrenViaManifestScan(t *testing.T) {
	svc, _, _, api, _, _ := newChatSyncService(t)
	plantDriveIndex(t, api, "drive-root", []chatSessionDriveIndexRecord{
		ca555IndexRecord(ProviderKeyGrok, "mch_leg", "run-parent", "", "2026-08-18T01:00:00Z"),
		ca555IndexRecord(ProviderKeyGrok, "mch_leg", "run-child", "", "2026-08-18T01:01:00Z"),
	})
	plantDriveRunManifest(t, api, "drive-root", ChatSessionSyncManifest{
		SchemaVersion:   1,
		SourceMachineID: "mch_leg",
		SourceRunID:     "run-parent",
		ProjectID:       "project-1",
		ProviderKey:     ProviderKeyGrok,
		RunKind:         "chat",
		LastPrompt:      "parent",
		SyncedAt:        "2026-08-18T01:00:00Z",
		UpdatedAt:       "2026-08-18T01:00:00Z",
		ChildAgents:     []AgentRunSummary{{RunID: "run-child", AgentName: "coder"}},
	})
	plantDriveRunManifest(t, api, "drive-root", ChatSessionSyncManifest{
		SchemaVersion:   1,
		SourceMachineID: "mch_leg",
		SourceRunID:     "run-child",
		ProjectID:       "project-1",
		ProviderKey:     ProviderKeyGrok,
		RunKind:         "chat",
		ParentRunID:     "run-parent",
		LastPrompt:      "child",
		SyncedAt:        "2026-08-18T01:01:00Z",
		UpdatedAt:       "2026-08-18T01:01:00Z",
	})

	summaries, apiErr := svc.listRemoteChatSessions(context.Background(), "project-1")
	if apiErr != nil {
		t.Fatalf("listRemoteChatSessions: %v", apiErr)
	}
	if len(summaries) != 1 || summaries[0].SourceRunID != "run-parent" {
		t.Fatalf("legacy BUG-123 scan must hide child, got %#v", summaries)
	}
}

func TestCA555BackgroundRepairMergesOrphanAfterIndexHit(t *testing.T) {
	svc, _, _, api, _, _ := newChatSyncService(t)
	plantDriveIndex(t, api, "drive-root", []chatSessionDriveIndexRecord{
		ca555IndexRecord(ProviderKeyCodex, "mch_bg", "run-indexed", "", "2026-08-18T01:00:00Z"),
	})
	plantDriveRunManifest(t, api, "drive-root", ChatSessionSyncManifest{
		SchemaVersion:   1,
		SourceMachineID: "mch_bg",
		SourceRunID:     "run-indexed",
		ProjectID:       "project-1",
		ProviderKey:     ProviderKeyCodex,
		RunKind:         "chat",
		LastPrompt:      "indexed",
		SyncedAt:        "2026-08-18T01:00:00Z",
		UpdatedAt:       "2026-08-18T01:00:00Z",
	})
	plantDriveRunManifest(t, api, "drive-root", ChatSessionSyncManifest{
		SchemaVersion:   1,
		SourceMachineID: "mch_bg",
		SourceRunID:     "run-bg-orphan",
		ProjectID:       "project-1",
		ProviderKey:     ProviderKeyClaude,
		RunKind:         "chat",
		LastPrompt:      "background orphan",
		SyncedAt:        "2026-08-18T02:00:00Z",
		UpdatedAt:       "2026-08-18T02:00:00Z",
	})

	first, apiErr := svc.listRemoteChatSessions(context.Background(), "project-1")
	if apiErr != nil {
		t.Fatalf("listRemoteChatSessions: %v", apiErr)
	}
	if len(first) != 1 || first[0].SourceRunID != "run-indexed" {
		t.Fatalf("first list must be index-only, got %#v", first)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		api.mu.Lock()
		var index []byte
		for _, file := range api.files {
			if file.Name == "sessions.ndjson" {
				index = append([]byte(nil), file.Content...)
				break
			}
		}
		api.mu.Unlock()
		ids := map[string]bool{}
		for _, rec := range parseChatSessionDriveIndex(index) {
			ids[rec.SourceRunID] = true
		}
		if ids["run-indexed"] && ids["run-bg-orphan"] {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("background repair did not merge orphan into sessions.ndjson")
}

func TestCA555ChatSessionIndexHasParentRunID(t *testing.T) {
	if chatSessionIndexHasParentRunID(nil) {
		t.Fatal("empty index is legacy")
	}
	if chatSessionIndexHasParentRunID([]chatSessionDriveIndexRecord{
		{SourceRunID: "a"},
		{SourceRunID: "b"},
	}) {
		t.Fatal("no ParentRunID is legacy")
	}
	if !chatSessionIndexHasParentRunID([]chatSessionDriveIndexRecord{
		{SourceRunID: "a"},
		{SourceRunID: "b", ParentRunID: "a"},
	}) {
		t.Fatal("any ParentRunID is modern")
	}
}
