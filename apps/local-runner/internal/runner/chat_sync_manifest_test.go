package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// failingUpsertStore is a test wrapper that injects a failure for one RunID
// to prove transcript-first semantics: transcript is already persisted even
// when a leg upsert fails.
type failingUpsertStore struct {
	*fakeWorkflowStore
	failRunID string
}

func (f *failingUpsertStore) UpsertProviderSession(ctx context.Context, sess ProviderSessionState) error {
	if sess.RunID == f.failRunID {
		return fmt.Errorf("injected failure for %q", sess.RunID)
	}
	return f.fakeWorkflowStore.UpsertProviderSession(ctx, sess)
}

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

func TestSyncChatV2UploadsTranscriptAndLegs(t *testing.T) {
	t.Setenv("FLOWPILOT_CHAT_SSOT", "1")
	dir := t.TempDir()
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", dir)
	fws := newFakeWorkflowStore()
	ctx := context.Background()
	for i, pk := range []ProviderKey{ProviderKeyCodex, ProviderKeyOpencode} {
		sess := ProviderSessionState{
			RunID:       fmt.Sprintf("run-%d", i+1),
			ProjectID:   "proj-1",
			ProviderKey: pk,
			RunKind:     "chat",
			ChatID:      "cht_demo",
			LegSeq:      i,
			LegState:    LegStateActive,
			Status:      RunStatusIdle,
		}
		if i == 0 {
			sess.LegState = LegStateClosed
			sess.LegClosedReason = LegClosedReasonProviderSwitch
		}
		if err := fws.UpsertProviderSession(ctx, sess); err != nil {
			t.Fatal(err)
		}
	}
	svc := &InteractiveService{
		workflowStore: fws,
		runs:          map[string]*interactiveRun{},
	}
	writer := svc.ensureChatTranscriptWriter()
	if writer == nil {
		t.Fatal("writer nil")
	}
	recs := []ChatTranscriptRecord{
		{ChatID: "cht_demo", Type: EventTypeChatTurnStarted, Payload: json.RawMessage(`{"prompt":"hello"}`)},
		{ChatID: "cht_demo", Type: EventTypeChatMessageCompleted, Payload: json.RawMessage(`{"text":"hi"}`)},
	}
	if err := writer.append(ctx, recs...); err != nil {
		t.Fatal(err)
	}
	drive := map[string][]byte{}
	manifest, err := SyncChatV2ToDrive(ctx, svc, "cht_demo", drive)
	if err != nil {
		t.Fatalf("SyncChatV2ToDrive: %v", err)
	}
	if len(manifest.Legs) != 2 {
		t.Fatalf("legs = %d want 2", len(manifest.Legs))
	}
	if manifest.ChatID != "cht_demo" {
		t.Fatalf("ChatID = %q want cht_demo", manifest.ChatID)
	}
	tp := chatSyncTranscriptDrivePath("cht_demo")
	mp := ChatSyncManifestDrivePath("cht_demo")
	if _, ok := drive[tp]; !ok {
		t.Fatalf("missing transcript path %q", tp)
	}
	if _, ok := drive[mp]; !ok {
		t.Fatalf("missing manifest path %q", mp)
	}
	data := drive[tp]
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("transcript lines = %d want 2: %q", len(lines), string(data))
	}
	for _, line := range lines {
		var rec ChatTranscriptRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("unmarshal transcript line: %v", err)
		}
		if rec.ChatID != "cht_demo" {
			t.Fatalf("rec ChatID = %q want cht_demo", rec.ChatID)
		}
		if rec.ChatSeq == 0 {
			t.Fatalf("rec ChatSeq 0")
		}
	}
	isV2, v2, _, err := ReadChatSyncManifest(drive[mp])
	if err != nil || !isV2 {
		t.Fatalf("manifest decode isV2=%v err=%v", isV2, err)
	}
	if v2.ChatID != "cht_demo" || len(v2.Legs) != 2 {
		t.Fatalf("manifest mismatch: %+v", v2)
	}
	if v2.Legs[0].LegSeq != 0 || v2.Legs[1].LegSeq != 1 {
		t.Fatalf("legs not sorted: %+v", v2.Legs)
	}
}

func TestSyncChatV2IdempotentReupload(t *testing.T) {
	t.Setenv("FLOWPILOT_CHAT_SSOT", "1")
	dir := t.TempDir()
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", dir)
	fws := newFakeWorkflowStore()
	ctx := context.Background()
	sess := ProviderSessionState{RunID: "run-1", ProjectID: "proj-1", ProviderKey: ProviderKeyCodex, RunKind: "chat", ChatID: "cht_idem", LegSeq: 0, LegState: LegStateActive, Status: RunStatusIdle}
	if err := fws.UpsertProviderSession(ctx, sess); err != nil {
		t.Fatal(err)
	}
	svc := &InteractiveService{workflowStore: fws, runs: map[string]*interactiveRun{}}
	writer := svc.ensureChatTranscriptWriter()
	if err := writer.append(ctx, ChatTranscriptRecord{ChatID: "cht_idem", Type: EventTypeChatTurnStarted, Payload: json.RawMessage(`{"prompt":"a"}`)}); err != nil {
		t.Fatal(err)
	}
	drive := map[string][]byte{}
	m1, err := SyncChatV2ToDrive(ctx, svc, "cht_idem", drive)
	if err != nil {
		t.Fatal(err)
	}
	b1 := append([]byte(nil), drive[ChatSyncManifestDrivePath("cht_idem")]...)
	t1 := append([]byte(nil), drive[chatSyncTranscriptDrivePath("cht_idem")]...)
	m2, err := SyncChatV2ToDrive(ctx, svc, "cht_idem", drive)
	if err != nil {
		t.Fatal(err)
	}
	b2 := drive[ChatSyncManifestDrivePath("cht_idem")]
	t2 := drive[chatSyncTranscriptDrivePath("cht_idem")]
	if string(b1) != string(b2) {
		t.Fatalf("manifest not idempotent: %q vs %q", string(b1), string(b2))
	}
	if string(t1) != string(t2) {
		t.Fatalf("transcript not idempotent")
	}
	if len(m1.Legs) != len(m2.Legs) {
		t.Fatalf("legs count changed %d vs %d", len(m1.Legs), len(m2.Legs))
	}
	sess2 := ProviderSessionState{RunID: "run-2", ProjectID: "proj-1", ProviderKey: ProviderKeyOpencode, RunKind: "chat", ChatID: "cht_idem", LegSeq: 1, LegState: LegStateActive, Status: RunStatusIdle}
	if err := fws.UpsertProviderSession(ctx, sess2); err != nil {
		t.Fatal(err)
	}
	if err := writer.append(ctx, ChatTranscriptRecord{ChatID: "cht_idem", Type: EventTypeChatMessageCompleted, Payload: json.RawMessage(`{"text":"b"}`)}); err != nil {
		t.Fatal(err)
	}
	m3, err := SyncChatV2ToDrive(ctx, svc, "cht_idem", drive)
	if err != nil {
		t.Fatal(err)
	}
	if len(m3.Legs) != 2 {
		t.Fatalf("after switch legs = %d want 2", len(m3.Legs))
	}
	if m3.Legs[0].RunID != "run-1" || m3.Legs[1].RunID != "run-2" {
		t.Fatalf("legs order wrong %+v", m3.Legs)
	}
	lines := strings.Split(strings.TrimSpace(string(drive[chatSyncTranscriptDrivePath("cht_idem")])), "\n")
	if len(lines) != 2 {
		t.Fatalf("transcript after switch lines = %d want 2", len(lines))
	}
}

func TestRestoreChatTranscriptFirstEvenIfLegFails(t *testing.T) {
	t.Setenv("FLOWPILOT_CHAT_SSOT", "1")
	srcDir := t.TempDir()
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", srcDir)
	fws := newFakeWorkflowStore()
	ctx := context.Background()
	for i, pk := range []ProviderKey{ProviderKeyCodex, ProviderKeyClaude} {
		sess := ProviderSessionState{RunID: fmt.Sprintf("run-%d", i+1), ProjectID: "proj-1", ProviderKey: pk, RunKind: "chat", ChatID: "cht_fail", LegSeq: i, LegState: LegStateActive, Status: RunStatusIdle}
		if err := fws.UpsertProviderSession(ctx, sess); err != nil {
			t.Fatal(err)
		}
	}
	srcSvc := &InteractiveService{workflowStore: fws, runs: map[string]*interactiveRun{}}
	writer := srcSvc.ensureChatTranscriptWriter()
	if err := writer.append(ctx, ChatTranscriptRecord{ChatID: "cht_fail", Type: EventTypeChatTurnStarted, Payload: json.RawMessage(`{"prompt":"hello"}`)}, ChatTranscriptRecord{ChatID: "cht_fail", Type: EventTypeChatMessageCompleted, Payload: json.RawMessage(`{"text":"world"}`)}); err != nil {
		t.Fatal(err)
	}
	drive := map[string][]byte{}
	manifest, err := SyncChatV2ToDrive(ctx, srcSvc, "cht_fail", drive)
	if err != nil {
		t.Fatal(err)
	}
	tgtDir := t.TempDir()
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", tgtDir)
	baseFWS := newFakeWorkflowStore()
	failing := &failingUpsertStore{fakeWorkflowStore: baseFWS, failRunID: "run-2"}
	tgtSvc := &InteractiveService{workflowStore: failing, runs: map[string]*interactiveRun{}}
	if err := RestoreChatFromManifestV2(ctx, tgtSvc, drive, manifest, nil); err != nil {
		t.Fatalf("restore err: %v", err)
	}
	tgtWriter := tgtSvc.ensureChatTranscriptWriter()
	recs, err := tgtWriter.store.ReadChatRecords(ctx, "cht_fail", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("transcript records after restore = %d want 2 (transcript-first)", len(recs))
	}
	sessions, err := failing.ListProviderSessionsByChat(ctx, "cht_fail")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions after partial fail = %d want 1 (run-1 succeeded)", len(sessions))
	}
	if sessions[0].RunID != "run-1" {
		t.Fatalf("remaining session RunID = %q want run-1", sessions[0].RunID)
	}
}

func TestRestoreChatDetachedNoActiveLeg(t *testing.T) {
	t.Setenv("FLOWPILOT_CHAT_SSOT", "1")
	srcDir := t.TempDir()
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", srcDir)
	fws := newFakeWorkflowStore()
	ctx := context.Background()
	for i := 0; i < 2; i++ {
		sess := ProviderSessionState{RunID: fmt.Sprintf("run-%d", i+1), ProjectID: "proj-1", ProviderKey: ProviderKeyCodex, RunKind: "chat", ChatID: "cht_detached", LegSeq: i, LegState: LegStateActive, Status: RunStatusRunning}
		if err := fws.UpsertProviderSession(ctx, sess); err != nil {
			t.Fatal(err)
		}
	}
	src := &InteractiveService{workflowStore: fws, runs: map[string]*interactiveRun{}}
	if err := src.ensureChatTranscriptWriter().append(ctx, ChatTranscriptRecord{ChatID: "cht_detached", Type: EventTypeChatTurnStarted, Payload: json.RawMessage(`{"prompt":"hi"}`)}); err != nil {
		t.Fatal(err)
	}
	drive := map[string][]byte{}
	manifest, err := SyncChatV2ToDrive(ctx, src, "cht_detached", drive)
	if err != nil {
		t.Fatal(err)
	}
	tgtDir := t.TempDir()
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", tgtDir)
	tgtFWS := newFakeWorkflowStore()
	tgt := &InteractiveService{workflowStore: tgtFWS, runs: map[string]*interactiveRun{}}
	if err := RestoreChatFromManifestV2(ctx, tgt, drive, manifest, nil); err != nil {
		t.Fatal(err)
	}
	sessions, err := tgtFWS.ListProviderSessionsByChat(ctx, "cht_detached")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions = %d want 2", len(sessions))
	}
	for _, sess := range sessions {
		if sess.LegState != LegStateClosed || sess.LegClosedReason != LegClosedReasonRestored {
			t.Fatalf("leg not detached closed(restored): %+v", sess)
		}
		if sess.Status != RunStatusCompleted {
			t.Fatalf("status not completed: %+v", sess)
		}
		if sess.LegState == LegStateActive {
			t.Fatalf("found active leg: %+v", sess)
		}
	}
	if !IsChatDetached(tgt, "cht_detached") {
		t.Fatalf("IsChatDetached false want true")
	}
	recs, err := tgt.ensureChatTranscriptWriter().store.ReadChatRecords(ctx, "cht_detached", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 {
		t.Fatalf("timeline records = %d want 1", len(recs))
	}
}

func TestRestoreChatDegradation(t *testing.T) {
	t.Setenv("FLOWPILOT_CHAT_SSOT", "1")
	dir := t.TempDir()
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", dir)
	fws := newFakeWorkflowStore()
	ctx := context.Background()
	manifest := ChatSyncManifest{
		SchemaVersion:      chatSyncManifestSchemaVersion,
		ChatID:             "cht_deg",
		ProjectID:          "proj-1",
		ChatTranscriptFile: chatSyncTranscriptDrivePath("cht_deg"),
		Legs: []ChatSyncLeg{
			{RunID: "run-1", LegSeq: 0, ProviderKey: string(ProviderKeyCodex), Sidecars: []string{"a.jsonl"}},
			{RunID: "run-2", LegSeq: 1, ProviderKey: string(ProviderKeyGrok), SidecarsAbsent: []string{"events.jsonl"}},
		},
	}
	drive := map[string][]byte{}
	rec := ChatTranscriptRecord{ChatID: "cht_deg", ChatSeq: 1, LegRunID: "run-1", Type: EventTypeChatMessageCompleted, Payload: json.RawMessage(`{"text":"hello"}`)}
	line, _ := json.Marshal(rec)
	drive[chatSyncTranscriptDrivePath("cht_deg")] = append(line, '\n')
	mb, _ := json.Marshal(manifest)
	drive[ChatSyncManifestDrivePath("cht_deg")] = mb
	tgt := &InteractiveService{workflowStore: fws, runs: map[string]*interactiveRun{}}
	if err := RestoreChatFromManifestV2(ctx, tgt, drive, manifest, nil); err != nil {
		t.Fatal(err)
	}
	sessions, err := fws.ListProviderSessionsByChat(ctx, "cht_deg")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions = %d want 2", len(sessions))
	}
	var degraded *ProviderSessionState
	for i := range sessions {
		if sessions[i].RunID == "run-2" {
			degraded = &sessions[i]
			break
		}
	}
	if degraded == nil {
		t.Fatal("run-2 not found")
	}
	if degraded.SyncStatus != "session_unavailable" {
		t.Fatalf("degraded SyncStatus = %q want session_unavailable", degraded.SyncStatus)
	}
	if degraded.LegState != LegStateClosed || degraded.LegClosedReason != LegClosedReasonRestored {
		t.Fatalf("degraded leg not detached: %+v", degraded)
	}
	for _, s := range sessions {
		if s.RunID == "run-1" && s.SyncStatus == "session_unavailable" {
			t.Fatalf("run-1 incorrectly degraded")
		}
	}
	if !IsChatDetached(tgt, "cht_deg") {
		t.Fatalf("detached false")
	}
	recs, err := tgt.ensureChatTranscriptWriter().store.ReadChatRecords(ctx, "cht_deg", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 {
		t.Fatalf("transcript after degraded restore = %d want 1", len(recs))
	}
}

func TestRestoreChatProviderUnavailable(t *testing.T) {
	t.Setenv("FLOWPILOT_CHAT_SSOT", "1")
	dir := t.TempDir()
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", dir)
	fws := newFakeWorkflowStore()
	ctx := context.Background()
	manifest := ChatSyncManifest{
		SchemaVersion:      chatSyncManifestSchemaVersion,
		ChatID:             "cht_prov",
		ProjectID:          "proj-1",
		ChatTranscriptFile: chatSyncTranscriptDrivePath("cht_prov"),
		Legs: []ChatSyncLeg{
			{RunID: "run-1", LegSeq: 0, ProviderKey: string(ProviderKeyCodex)},
			{RunID: "run-2", LegSeq: 1, ProviderKey: string(ProviderKeyGrok)},
		},
	}
	drive := map[string][]byte{}
	rec := ChatTranscriptRecord{ChatID: "cht_prov", ChatSeq: 1, Type: EventTypeChatMessageCompleted, Payload: json.RawMessage(`{"text":"hi"}`)}
	line, _ := json.Marshal(rec)
	drive[chatSyncTranscriptDrivePath("cht_prov")] = append(line, '\n')
	tgt := &InteractiveService{workflowStore: fws, runs: map[string]*interactiveRun{}}
	available := map[ProviderKey]bool{
		ProviderKeyCodex:    true,
		ProviderKeyGrok:     false,
		ProviderKeyClaude:   true,
		ProviderKeyOpencode: true,
	}
	if err := RestoreChatFromManifestV2(ctx, tgt, drive, manifest, available); err != nil {
		t.Fatal(err)
	}
	sessions, err := fws.ListProviderSessionsByChat(ctx, "cht_prov")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("sessions = %d want 2", len(sessions))
	}
	var grokSess *ProviderSessionState
	for i := range sessions {
		if sessions[i].ProviderKey == ProviderKeyGrok {
			grokSess = &sessions[i]
			break
		}
	}
	if grokSess == nil {
		t.Fatal("grok session not found")
	}
	if grokSess.SyncStatus != "provider_unavailable" {
		t.Fatalf("SyncStatus = %q want provider_unavailable", grokSess.SyncStatus)
	}
	if !strings.Contains(strings.ToLower(grokSess.LastMessage), "install") || !strings.Contains(strings.ToLower(grokSess.LastMessage), "grok") {
		t.Fatalf("provider hint missing in LastMessage %q", grokSess.LastMessage)
	}
	for _, s := range sessions {
		if s.ProviderKey == ProviderKeyCodex && s.SyncStatus == "provider_unavailable" {
			t.Fatalf("codex incorrectly unavailable")
		}
	}
	if !IsChatDetached(tgt, "cht_prov") {
		t.Fatalf("detached false")
	}
}

func TestRestoreChatIdempotentAndLegOrder(t *testing.T) {
	t.Setenv("FLOWPILOT_CHAT_SSOT", "1")
	dir := t.TempDir()
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", dir)
	fws := newFakeWorkflowStore()
	ctx := context.Background()
	manifest := ChatSyncManifest{
		SchemaVersion:      chatSyncManifestSchemaVersion,
		ChatID:             "cht_order",
		ProjectID:          "proj-1",
		ChatTranscriptFile: chatSyncTranscriptDrivePath("cht_order"),
		Legs: []ChatSyncLeg{
			{RunID: "run-3", LegSeq: 2, ProviderKey: string(ProviderKeyOpencode)},
			{RunID: "run-1", LegSeq: 0, ProviderKey: string(ProviderKeyCodex)},
			{RunID: "run-2", LegSeq: 1, ProviderKey: string(ProviderKeyGrok)},
		},
	}
	drive := map[string][]byte{}
	var buf []byte
	for i := 1; i <= 3; i++ {
		rec := ChatTranscriptRecord{ChatID: "cht_order", ChatSeq: int64(i), LegRunID: fmt.Sprintf("run-%d", i), Type: EventTypeChatMessageCompleted, Payload: json.RawMessage(fmt.Sprintf(`{"text":"msg%d"}`, i))}
		line, _ := json.Marshal(rec)
		buf = append(buf, line...)
		buf = append(buf, '\n')
	}
	drive[chatSyncTranscriptDrivePath("cht_order")] = buf
	tgt := &InteractiveService{workflowStore: fws, runs: map[string]*interactiveRun{}}
	if err := RestoreChatFromManifestV2(ctx, tgt, drive, manifest, nil); err != nil {
		t.Fatal(err)
	}
	if err := RestoreChatFromManifestV2(ctx, tgt, drive, manifest, nil); err != nil {
		t.Fatal(err)
	}
	recs, err := tgt.ensureChatTranscriptWriter().store.ReadChatRecords(ctx, "cht_order", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 {
		t.Fatalf("records after double restore = %d want 3 idempotent", len(recs))
	}
	sessions, err := fws.ListProviderSessionsByChat(ctx, "cht_order")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 3 {
		t.Fatalf("sessions = %d want 3", len(sessions))
	}
	for i, sess := range sessions {
		if sess.LegSeq != i {
			t.Fatalf("session %d LegSeq = %d want %d", i, sess.LegSeq, i)
		}
		expectedRunID := fmt.Sprintf("run-%d", i+1)
		if sess.RunID != expectedRunID {
			t.Fatalf("session order RunID = %q want %q", sess.RunID, expectedRunID)
		}
	}
}

func TestRestoreChatV1Untouched(t *testing.T) {
	v1 := ChatSessionSyncManifest{
		SchemaVersion:   chatSessionManifestSchemaVersion,
		SourceMachineID: "mch_abc",
		SourceRunID:     "run-1",
		ProjectID:       "proj-1",
		ProviderKey:     ProviderKeyCodex,
		ProviderFile:    ChatSessionFile{RelativePath: "codex/ses_abc.jsonl", SizeBytes: 10, SHA256: "abc"},
		RunKind:         "chat",
	}
	data, err := json.Marshal(v1)
	if err != nil {
		t.Fatal(err)
	}
	isV2, _, gotV1, err := ReadChatSyncManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	if isV2 {
		t.Fatal("v1 decoded as v2")
	}
	if gotV1.SourceRunID != "run-1" {
		t.Fatalf("v1 mismatch: %+v", gotV1)
	}
	// Flag removed: always ON — even with env 0 these must NOT be disabled.
	t.Setenv("FLOWPILOT_CHAT_SSOT", "0")
	t.Setenv("FLOWPILOT_CHAT_STORE_DIR", t.TempDir())
	fws := newFakeWorkflowStore()
	svc := &InteractiveService{workflowStore: fws, runs: map[string]*interactiveRun{}}
	_, err = SyncChatV2ToDrive(context.Background(), svc, "cht_x", map[string][]byte{})
	if err != nil && (strings.Contains(err.Error(), "SSOT") || strings.Contains(err.Error(), "disabled")) {
		t.Fatalf("must not be disabled when flag removed, got %v", err)
	}
	err = RestoreChatFromManifestV2(context.Background(), svc, map[string][]byte{}, ChatSyncManifest{ChatID: "cht_x", SchemaVersion: 2}, nil)
	if err != nil && (strings.Contains(err.Error(), "SSOT") || strings.Contains(err.Error(), "disabled")) {
		t.Fatalf("restore must not be disabled when flag removed, got %v", err)
	}
}
