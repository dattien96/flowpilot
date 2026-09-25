package runner

// BUG-476 (CP-59/SD-26): the Drive sync unit is the LOGICAL CHAT, not one run.
// A multi-provider chat is an ordered set of legs; the manifest must carry
// leg identity, syncing any leg must upload every sibling leg plus a
// chat-level manifest written last, remote listing groups legs as one chat,
// and restore recreates the whole chat with collision-safe ID remapping.

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"
)

func seedLocalChatLeg(t *testing.T, store *localFileSessionStore, accountHome, workspace, runID, chatID string, legSeq int, switchFrom string, fileBody []byte) ProviderSessionState {
	t.Helper()
	state := seedLocalChatRun(t, store, accountHome, workspace, runID, fileBody)
	state.ChatID = chatID
	state.LegSeq = legSeq
	state.SwitchFromRunID = switchFrom
	state.LegState = LegStateActive
	if err := store.UpsertProviderSession(context.Background(), state); err != nil {
		t.Fatalf("UpsertProviderSession leg: %v", err)
	}
	return state
}

func countDriveManifests(api *fakeChatDriveAPI) int {
	n := 0
	for _, f := range api.files {
		if f.Name == "manifest.json" {
			n++
		}
	}
	return n
}

func driveFileByName(api *fakeChatDriveAPI, name string) (fakeChatDriveFile, bool) {
	for _, f := range api.files {
		if f.Name == name {
			return f, true
		}
	}
	return fakeChatDriveFile{}, false
}

// The per-run manifest must carry the chat-leg identity the durable session
// already holds — without it a restore can never rebuild one logical chat.
func TestBUG476_ManifestCarriesChatLegIdentity(t *testing.T) {
	svc, _, store, _, workspace, accountHome := newChatSyncService(t)
	seedLocalChatLeg(t, store, accountHome, workspace, "run-leg1", "chat-476", 1, "", []byte(`{"msg":"l1"}`))
	seedLocalChatLeg(t, store, accountHome, workspace, "run-leg2", "chat-476", 2, "run-leg1", []byte(`{"msg":"l2"}`))

	manifest, _, apiErr := svc.BuildChatSessionSyncManifest(context.Background(), "run-leg2")
	if apiErr != nil {
		t.Fatalf("BuildChatSessionSyncManifest: %v", apiErr)
	}
	if manifest.ChatID != "chat-476" || manifest.LegSeq != 2 || manifest.SwitchFromRunID != "run-leg1" {
		t.Fatalf("manifest dropped chat-leg identity: %+v", manifest)
	}
}

// Syncing ONE leg must upload EVERY sibling leg manifest plus a chat-level
// manifest written after the legs (so a partial upload never advertises a
// complete restorable chat).
func TestBUG476_SyncUploadsAllChatLegsAndChatManifest(t *testing.T) {
	svc, _, store, api, workspace, accountHome := newChatSyncService(t)
	seedLocalChatLeg(t, store, accountHome, workspace, "run-leg1", "chat-476", 1, "", []byte(`{"msg":"l1"}`))
	seedLocalChatLeg(t, store, accountHome, workspace, "run-leg2", "chat-476", 2, "run-leg1", []byte(`{"msg":"l2"}`))
	seedLocalChatLeg(t, store, accountHome, workspace, "run-leg3", "chat-476", 3, "run-leg2", []byte(`{"msg":"l3"}`))

	res, apiErr := svc.syncChatRunToDrive(context.Background(), "run-leg2", ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive: %v", apiErr)
	}
	if res.SyncStatus != "synced" {
		t.Fatalf("syncStatus=%q", res.SyncStatus)
	}
	if got := countDriveManifests(api); got != 3 {
		t.Fatalf("expected 3 leg manifests uploaded, got %d", got)
	}
	chatFile, ok := driveFileByName(api, "chat.json")
	if !ok {
		t.Fatal("chat-level manifest (chat.json) was not uploaded")
	}
	var chatDoc struct {
		ChatID string `json:"chatId"`
		Legs   []struct {
			SourceRunID string `json:"sourceRunId"`
			LegSeq      int    `json:"legSeq"`
		} `json:"legs"`
	}
	if err := json.Unmarshal(chatFile.Content, &chatDoc); err != nil {
		t.Fatalf("chat.json not valid JSON: %v", err)
	}
	if chatDoc.ChatID != "chat-476" || len(chatDoc.Legs) != 3 {
		t.Fatalf("chat.json legs = %+v", chatDoc)
	}
	for i, leg := range chatDoc.Legs {
		if leg.LegSeq != i+1 {
			t.Fatalf("chat.json legs out of order: %+v", chatDoc.Legs)
		}
	}
	// Index must carry chat identity so remote listing can group legs.
	idxFile, ok := driveFileByName(api, "sessions.ndjson")
	if !ok {
		t.Fatal("drive index missing")
	}
	// Scope to this machine's rows — a stale background index-repair from a
	// previous test can merge its own discovered rows through the shared
	// httpRequestFn hook (harness leak, not a product path).
	var records []chatSessionDriveIndexRecord
	for _, r := range parseChatSessionDriveIndex(idxFile.Content) {
		if r.SourceMachineID == res.SourceMachineID {
			records = append(records, r)
		}
	}
	if len(records) != 3 {
		t.Fatalf("index records for machine = %d, want 3 leg rows", len(records))
	}
	for _, r := range records {
		if r.ChatID != "chat-476" || r.LegSeq == 0 {
			t.Fatalf("index record missing chat identity: %+v", r)
		}
	}
}

// Remote listing groups legs of one chat into ONE row — the newest leg
// represents the chat (status/updatedAt/provider).
func TestBUG476_RemoteListGroupsLegsAsOneChat(t *testing.T) {
	svc, _, store, _, workspace, accountHome := newChatSyncService(t)
	seedLocalChatLeg(t, store, accountHome, workspace, "run-leg1", "chat-476", 1, "", []byte(`{"msg":"l1"}`))
	seedLocalChatLeg(t, store, accountHome, workspace, "run-leg2", "chat-476", 2, "run-leg1", []byte(`{"msg":"l2"}`))
	seedLocalChatLeg(t, store, accountHome, workspace, "run-leg3", "chat-476", 3, "run-leg2", []byte(`{"msg":"l3"}`))
	if _, apiErr := svc.syncChatRunToDrive(context.Background(), "run-leg2", ChatSessionSyncRequest{}); apiErr != nil {
		t.Fatalf("syncChatRunToDrive: %v", apiErr)
	}

	list, apiErr := svc.listRemoteChatSessions(context.Background(), "project-1")
	if apiErr != nil {
		t.Fatalf("listRemoteChatSessions: %v", apiErr)
	}
	// Scoped to this chatID — see the harness-leak note in the restore test.
	var chatRows []RemoteChatSessionSummary
	for _, row := range list {
		if row.ChatID == "chat-476" {
			chatRows = append(chatRows, row)
		}
	}
	if len(chatRows) != 1 {
		t.Fatalf("chat legs must list as ONE remote chat, got %d rows", len(chatRows))
	}
	if chatRows[0].SourceRunID != "run-leg3" {
		t.Fatalf("chat row should be the newest leg (run-leg3), got %+v", chatRows[0])
	}
}

// Full round-trip: restore the chat on a second machine — every leg is
// restored, ordered, ChatID preserved, and SwitchFromRunID remapped to the
// predecessor's LOCAL run id.
func TestBUG476_RestoreRecreatesWholeChatWithRemap(t *testing.T) {
	svcA, _, storeA, api, workspaceA, accountHomeA := newChatSyncService(t)
	seedLocalChatLeg(t, storeA, accountHomeA, workspaceA, "run-leg1", "chat-476", 1, "", []byte(`{"msg":"l1"}`))
	seedLocalChatLeg(t, storeA, accountHomeA, workspaceA, "run-leg2", "chat-476", 2, "run-leg1", []byte(`{"msg":"l2"}`))
	seedLocalChatLeg(t, storeA, accountHomeA, workspaceA, "run-leg3", "chat-476", 3, "run-leg2", []byte(`{"msg":"l3"}`))
	resA, apiErr := svcA.syncChatRunToDrive(context.Background(), "run-leg3", ChatSessionSyncRequest{})
	if apiErr != nil {
		t.Fatalf("syncChatRunToDrive: %v", apiErr)
	}
	machineA := resA.SourceMachineID

	// Machine B: same fake Drive backend, fresh workspace/store/account.
	svcB, _, storeB, _, _, _ := newChatSyncService(t)
	// Re-point B's Drive calls at A's fake backend (one shared remote).
	httpRequestFn = api.handle

	result, apiErr := svcB.restoreChatRunFromDrive(context.Background(), ChatSessionRestoreRequest{
		ProjectID:       "project-1",
		SourceMachineID: machineA,
		SourceRunID:     "run-leg3",
	})
	if apiErr != nil {
		t.Fatalf("restoreChatRunFromDrive: %v", apiErr)
	}
	if result.RestoreStatus != "restored" {
		t.Fatalf("restoreStatus=%q", result.RestoreStatus)
	}

	legs, err := storeB.ListProviderSessionsByChat(context.Background(), "chat-476")
	if err != nil {
		t.Fatalf("ListProviderSessionsByChat: %v", err)
	}
	if len(legs) != 3 {
		t.Fatalf("restored chat must contain all 3 legs, got %d", len(legs))
	}
	sort.Slice(legs, func(i, j int) bool { return legs[i].LegSeq < legs[j].LegSeq })
	bySource := map[string]ProviderSessionState{}
	for _, leg := range legs {
		bySource[leg.SourceRunID] = leg
	}
	for i, wantSource := range []string{"run-leg1", "run-leg2", "run-leg3"} {
		leg, ok := bySource[wantSource]
		if !ok {
			t.Fatalf("leg %s missing after restore", wantSource)
		}
		if leg.ChatID != "chat-476" || leg.LegSeq != i+1 {
			t.Fatalf("leg %s identity wrong: %+v", wantSource, leg)
		}
	}
	// Switch lineage remapped to LOCAL run ids.
	if bySource["run-leg2"].SwitchFromRunID != bySource["run-leg1"].RunID {
		t.Fatalf("leg2 SwitchFromRunID=%q want local id of leg1 %q",
			bySource["run-leg2"].SwitchFromRunID, bySource["run-leg1"].RunID)
	}
	if bySource["run-leg3"].SwitchFromRunID != bySource["run-leg2"].RunID {
		t.Fatalf("leg3 SwitchFromRunID=%q want local id of leg2 %q",
			bySource["run-leg3"].SwitchFromRunID, bySource["run-leg2"].RunID)
	}
	// Each leg's provider file must be restored on machine B.
	for _, leg := range legs {
		if strings.TrimSpace(leg.ProviderSessionID) == "" {
			t.Fatalf("leg %+v lost provider session id", leg)
		}
	}
	// B's remote listing shows exactly ONE row for the chat. Scope the
	// assertion to this chatID: a stale background index-repair goroutine from
	// a previous test in the same package can still be merging ITS discovered
	// records through the shared httpRequestFn hook (test-harness leak, not a
	// product path — real usage has one Drive backend).
	httpRequestFn = api.handle
	list, apiErr := svcB.listRemoteChatSessions(context.Background(), "project-1")
	if apiErr != nil {
		t.Fatalf("listRemoteChatSessions: %v", apiErr)
	}
	var chatRows []RemoteChatSessionSummary
	for _, row := range list {
		if row.ChatID == "chat-476" {
			chatRows = append(chatRows, row)
		}
	}
	if len(chatRows) != 1 {
		for i, row := range list {
			t.Logf("row %d: %+v", i, row)
		}
		t.Fatalf("chat-476 must group as one remote chat, got %d rows", len(chatRows))
	}
	if chatRows[0].SourceRunID != "run-leg3" {
		t.Fatalf("chat row should be the newest leg (run-leg3), got %+v", chatRows[0])
	}
}
