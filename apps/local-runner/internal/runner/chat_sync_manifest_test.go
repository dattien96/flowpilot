package runner

import (
	"context"
	"encoding/json"
	"testing"
)

func TestChatSyncManifestV2Builder(t *testing.T) {
	t.Setenv("FLOWPILOT_CHAT_SSOT", "1")
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", t.TempDir())
	fws := newFakeWorkflowStore()
	ctx := context.Background()
	// 3 legs for cht_x: opencode, grok, codex
	for i, pk := range []ProviderKey{ProviderKeyOpencode, ProviderKeyGrok, ProviderKeyCodex} {
		sess := ProviderSessionState{
			RunID:       "run-" + string(rune('1'+i)),
			ProjectID:   "proj-1",
			ProviderKey: pk,
			RunKind:     "chat",
			ChatID:      "cht_x",
			LegSeq:      i,
			LegState:    LegStateActive,
			Status:      RunStatusIdle,
		}
		if i == 0 {
			sess.LegState = LegStateClosed
			sess.LegClosedReason = LegClosedReasonProviderSwitch
		}
		if pk == ProviderKeyGrok {
			// Grok session needs a real session ID for sidecar listing (best-effort, but we don't have a home dir, so sidecars will be empty)
			sess.ProviderSessionID = "grok-ses-abc123"
			sess.ProviderAccountID = "acct-grok-1"
			sess.WorkingDirectory = "/tmp"
		}
		if err := fws.UpsertProviderSession(ctx, sess); err != nil {
			t.Fatal(err)
		}
	}
	svc := &InteractiveService{
		workflowStore: fws,
		runs:          map[string]*interactiveRun{},
	}
	manifest, err := BuildChatSyncManifestV2(ctx, svc, "cht_x")
	if err != nil {
		t.Fatalf("BuildChatSyncManifestV2: %v", err)
	}
	if manifest.SchemaVersion != chatSyncManifestSchemaVersion {
		t.Fatalf("SchemaVersion = %d, want %d", manifest.SchemaVersion, chatSyncManifestSchemaVersion)
	}
	if manifest.ChatID != "cht_x" {
		t.Fatalf("ChatID = %q, want cht_x", manifest.ChatID)
	}
	if manifest.ChatTranscriptFile != "chat-sessions/chats/cht_x/transcript.ndjson" {
		t.Fatalf("ChatTranscriptFile = %q", manifest.ChatTranscriptFile)
	}
	if len(manifest.Legs) != 3 {
		t.Fatalf("Legs = %d, want 3", len(manifest.Legs))
	}
	// legs sorted by legSeq
	for i, leg := range manifest.Legs {
		if leg.LegSeq != i {
			t.Fatalf("leg %d LegSeq = %d", i, leg.LegSeq)
		}
		if leg.RunID != "run-"+string(rune('1'+i)) {
			t.Fatalf("leg %d RunID = %q", i, leg.RunID)
		}
	}
	// Check provider keys
	if manifest.Legs[0].ProviderKey != string(ProviderKeyOpencode) || manifest.Legs[1].ProviderKey != string(ProviderKeyGrok) {
		t.Fatalf("provider keys wrong: %+v", manifest.Legs)
	}
	// JSON round-trip
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	isV2, v2, _, err := ReadChatSyncManifest(data)
	if err != nil || !isV2 {
		t.Fatalf("ReadChatSyncManifest v2 round-trip isV2=%v err=%v", isV2, err)
	}
	if v2.ChatID != "cht_x" || len(v2.Legs) != 3 {
		t.Fatalf("round-trip v2 mismatch: %+v", v2)
	}
	// Drive path helper
	if got := ChatSyncManifestDrivePath("cht_x"); got != "chat-sessions/chats/cht_x/manifest.json" {
		t.Fatalf("DrivePath = %q", got)
	}
}

func TestChatManifestReaderAcceptsV1(t *testing.T) {
	// Craft v1 payload
	v1 := ChatSessionSyncManifest{
		SchemaVersion:   chatSessionManifestSchemaVersion,
		SourceMachineID: "mch_abc",
		SourceRunID:     "run-1",
		ProjectID:       "proj-1",
		ProviderKey:     ProviderKeyCodex,
		ProviderFile:    ChatSessionFile{RelativePath: "codex/ses_abc.jsonl", SizeBytes: 10, SHA256: "abc"},
		RunKind:         "chat",
	}
	v1Bytes, err := json.Marshal(v1)
	if err != nil {
		t.Fatal(err)
	}
	isV2, _, gotV1, err := ReadChatSyncManifest(v1Bytes)
	if err != nil {
		t.Fatalf("v1 read err: %v", err)
	}
	if isV2 {
		t.Fatal("v1 manifest decoded as v2")
	}
	if gotV1.SourceRunID != "run-1" || gotV1.ProviderKey != ProviderKeyCodex {
		t.Fatalf("v1 mismatch: %+v", gotV1)
	}
	// Craft v2 payload
	v2 := ChatSyncManifest{
		SchemaVersion:      chatSyncManifestSchemaVersion,
		ChatID:             "cht_y",
		ProjectID:          "proj-1",
		ChatTranscriptFile: "chat-sessions/chats/cht_y/transcript.ndjson",
		Legs: []ChatSyncLeg{
			{RunID: "run-1", LegSeq: 0, ProviderKey: string(ProviderKeyCodex), Sidecars: []string{"a.jsonl"}},
			{RunID: "run-2", LegSeq: 1, ProviderKey: string(ProviderKeyGrok)},
		},
	}
	v2Bytes, _ := json.Marshal(v2)
	isV2, gotV2, _, err := ReadChatSyncManifest(v2Bytes)
	if err != nil || !isV2 {
		t.Fatalf("v2 read isV2=%v err=%v", isV2, err)
	}
	if gotV2.ChatID != "cht_y" || len(gotV2.Legs) != 2 {
		t.Fatalf("v2 mismatch: %+v", gotV2)
	}
	// Malformed
	if _, _, _, err := ReadChatSyncManifest([]byte("{bad")); err == nil {
		t.Fatal("expected error for malformed json")
	}
	// Empty
	if _, _, _, err := ReadChatSyncManifest([]byte("")); err == nil {
		t.Fatal("expected error for empty manifest")
	}
}
