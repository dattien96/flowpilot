package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// G2 (CA-549): /sync push-to-Drive parity (Desktop Navigator "Sync all").
// Batch runs one HTTP per Update, never freezes the composer, and a single
// failure must not abort the remaining queue.

func syncableItem(runID, prompt, kind, status string) client.RunHistoryItem {
	return client.RunHistoryItem{
		RunID:      runID,
		LastPrompt: prompt,
		Status:     status,
		RunKind:    kind,
	}
}

func TestIsSyncableRun_KindAndStatus(t *testing.T) {
	base := syncableItem("r1", "fix login", "chat", "completed")
	cases := []struct {
		name string
		it   client.RunHistoryItem
		want bool
	}{
		{"chat kind", base, true},
		{"empty kind", syncableItem("r2", "hi", "", "completed"), true},
		{"workflow kind", syncableItem("r3", "wf", "workflow", "completed"), true},
		{"agent run not syncable", client.RunHistoryItem{RunID: "r4", LastPrompt: "you are the coder sub-agent. fix", RunKind: "chat"}, false},
		{"parent run id not syncable", client.RunHistoryItem{RunID: "r5", LastPrompt: "fix", RunKind: "chat", ParentRunID: "r1"}, false},
		{"synced not syncable", client.RunHistoryItem{RunID: "r6", LastPrompt: "fix", RunKind: "chat", SyncStatus: "synced"}, false},
		{"unsyncable not syncable", client.RunHistoryItem{RunID: "r7", LastPrompt: "fix", RunKind: "chat", SyncStatus: "unsyncable"}, false},
		{"unavailable not syncable", client.RunHistoryItem{RunID: "r8", LastPrompt: "fix", RunKind: "chat", UnavailableReason: "gone"}, false},
		{"exec kind not syncable", client.RunHistoryItem{RunID: "r9", LastPrompt: "ls", RunKind: "exec"}, false},
		{"already remote not syncable", client.RunHistoryItem{RunID: "r10", LastPrompt: "fix", RunKind: "chat", SourceMachineID: "m1", SourceRunID: "r1"}, false},
	}
	for _, tc := range cases {
		remote := []client.RemoteChatSessionSummary{}
		if tc.it.SourceMachineID != "" {
			remote = []client.RemoteChatSessionSummary{{SourceMachineID: tc.it.SourceMachineID, SourceRunID: tc.it.SourceRunID}}
		}
		if got := isSyncableRun(tc.it, remote); got != tc.want {
			t.Errorf("%s: isSyncableRun got=%v want=%v", tc.name, got, tc.want)
		}
	}
}

func TestIsSyncableRun_RemoteIndexOverridesStaleStatus(t *testing.T) {
	// A restored run keeps source ids; the remote index is the truth for them.
	// A "synced" status alone already blocks re-sync even when the ids moved.
	it := client.RunHistoryItem{RunID: "r1", LastPrompt: "fix", RunKind: "chat", SyncStatus: "failed"}
	if !isSyncableRun(it, nil) {
		t.Fatal("failed row with no source ids must stay syncable")
	}
	it.SyncStatus = "synced"
	if isSyncableRun(it, []client.RemoteChatSessionSummary{{SourceMachineID: "m1", SourceRunID: "r9"}}) {
		t.Fatal("synced status must block re-sync regardless of the remote index")
	}
	it.SyncStatus = "failed"
	it.SourceMachineID = "m1"
	it.SourceRunID = "r9" // different run on Drive
	if !isSyncableRun(it, []client.RemoteChatSessionSummary{{SourceMachineID: "m1", SourceRunID: "r1"}}) {
		t.Fatal("source ids not in remote index must stay syncable")
	}
	if isSyncableRun(it, []client.RemoteChatSessionSummary{{SourceMachineID: "m1", SourceRunID: "r9"}}) {
		t.Fatal("source ids in remote index must not be re-synced")
	}
}

func TestFilterSyncSuggestions_AllRowPlusFiltering(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.chatList = []client.RunHistoryItem{
		syncableItem("r1", "fix login bug", "chat", "completed"),
		syncableItem("r2", "ship android", "chat", "completed"),
		syncableItem("r3", "already in drive", "chat", "completed"),
	}
	m.chatList[2].SyncStatus = "synced"
	syncable := m.syncableChats()
	if len(syncable) != 2 {
		t.Fatalf("syncableChats=%+v", syncable)
	}

	all := filterSyncSuggestions("/sync ", syncable)
	if len(all) != 3 || all[0].value != "all" || all[0].kind != "sync" || all[0].slash != "/sync" {
		t.Fatalf("all row missing or miswired: %+v", all)
	}
	q := filterSyncSuggestions("/sync ship", syncable)
	// The bulk "all" row stays visible while filtering (Desktop "Sync all").
	if len(q) != 2 || q[0].value != "all" || q[1].value != "r2" {
		t.Fatalf("query filter wrong: %+v", q)
	}
	if filterSyncSuggestions("/hist", syncable) != nil {
		t.Fatal("non-/sync input must not open the sync picker")
	}
	if filterSyncSuggestions("/sync ", nil) != nil {
		t.Fatal("no syncable chats must return no rows")
	}
}

func TestSuggestionAcceptValue_SyncWritesSlashPlusRunID(t *testing.T) {
	if got := suggestionAcceptValue(suggestItem{kind: "sync", value: "r1", slash: "/sync"}); got != "/sync r1" {
		t.Fatalf("got %q", got)
	}
	if got := suggestionAcceptValue(suggestItem{kind: "sync", value: "all", slash: "/sync"}); got != "/sync all" {
		t.Fatalf("got %q", got)
	}
}

func TestRunSyncDispatch_CurrentChatTargetsOpenRun(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.project = &client.Project{ID: "p1", Name: "proj"}
	m.runHandle = &client.RunHandle{RunID: "run-42"}
	_, cmd := m.runSyncDispatch(nil)
	if cmd == nil {
		t.Fatal("expected a sync cmd for the open run")
	}
	if m.driveSync == nil || m.driveSync.total != 1 || m.driveSync.projectID != "p1" {
		t.Fatalf("driveSync=%+v", m.driveSync)
	}
}

func TestRunSyncDispatch_SyncAllBatchQueue(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.project = &client.Project{ID: "p1", Name: "proj"}
	m.chatList = []client.RunHistoryItem{
		syncableItem("r1", "fix login bug", "chat", "completed"),
		syncableItem("r2", "ship android", "chat", "completed"),
		syncableItem("r3", "already in drive", "chat", "completed"),
	}
	m.chatList[2].SyncStatus = "synced"
	_, cmd := m.runSyncDispatch([]string{"all"})
	if cmd == nil {
		t.Fatal("expected a sync cmd for /sync all")
	}
	if m.driveSync == nil || m.driveSync.total != 2 || len(m.driveSync.queue) != 1 {
		t.Fatalf("driveSync=%+v", m.driveSync)
	}
}

func TestRunSyncDispatch_NoProjectIsHarmless(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	am, cmd := m.runSyncDispatch(nil)
	if cmd != nil || am == nil {
		t.Fatalf("no-project /sync must not panic or start a batch: cmd=%v", cmd)
	}
}

func TestRunSyncDispatch_UnknownTargetErrorsWithoutBatch(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.project = &client.Project{ID: "p1", Name: "proj"}
	m.chatList = []client.RunHistoryItem{syncableItem("r1", "fix login bug", "chat", "completed")}
	am, cmd := m.runSyncDispatch([]string{"999"})
	if cmd != nil || am == nil {
		t.Fatalf("bad target must not start a batch: cmd=%v", cmd)
	}
}

func TestDriveSyncBatch_ProgressAndFailureSurvive(t *testing.T) {
	// The Update state machine: one failure must not abort the rest, and the
	// badge writes back to the cached chatList row.
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.project = &client.Project{ID: "p1", Name: "proj"}
	m.chatList = []client.RunHistoryItem{syncableItem("r1", "fix", "chat", "completed"), syncableItem("r2", "ship", "chat", "completed")}
	m.driveSync = &driveSyncState{queue: []string{"r2"}, projectID: "p1", total: 2, done: 0, failed: 0}

	m2, cmd := m.Update(DriveSyncBatchMsg{RunID: "r1", Err: &client.APIError{Status: 409, Code: "google_drive_not_connected", Message: "not connected"}})
	am := m2.(*AppModel)
	if cmd == nil {
		t.Fatal("batch must continue after one failure")
	}
	if am.driveSync == nil || am.driveSync.done != 1 || am.driveSync.failed != 1 || len(am.driveSync.queue) != 0 {
		t.Fatalf("driveSync=%+v", am.driveSync)
	}
	if am.chatList[0].SyncStatus != "failed" {
		t.Fatalf("badge not written back: %+v", am.chatList[0])
	}

	am2, cmd2 := am.Update(DriveSyncBatchMsg{RunID: "r2", Result: &client.ChatSessionSyncResult{SyncStatus: "synced"}})
	am = am2.(*AppModel)
	if cmd2 != nil {
		t.Fatal("batch must finish after the last item")
	}
	if am.driveSync != nil {
		t.Fatal("driveSync must reset to nil when finished")
	}
	if am.chatList[1].SyncStatus != "synced" {
		t.Fatalf("synced badge missing: %+v", am.chatList[1])
	}
}

func TestDriveSyncBatch_SummaryMessage(t *testing.T) {
	if got := formatDriveSyncSummary(&driveSyncState{total: 3, done: 3, failed: 0}); !strings.Contains(got, "Synced 3/3") {
		t.Fatalf("all-ok summary=%q", got)
	}
	if got := formatDriveSyncSummary(&driveSyncState{total: 3, done: 3, failed: 1}); !strings.Contains(got, "Synced 2/3") || !strings.Contains(got, "1 failed") {
		t.Fatalf("partial-fail summary=%q", got)
	}
}

func TestFormatDriveSyncErr_Hints(t *testing.T) {
	if got := formatDriveSyncErr("r1", &client.APIError{Status: 409, Code: "google_drive_not_connected", Message: "not connected"}); !strings.Contains(got, "/settings") {
		t.Fatalf("drive hint missing: %q", got)
	}
	if got := formatDriveSyncErr("r1", &client.APIError{Status: 422, Code: "unsyncable", Message: "no session"}); !strings.Contains(got, "never be synced") {
		t.Fatalf("unsyncable hint missing: %q", got)
	}
}

func TestCmdMaybePrefetchHistory_TriggersOnSync(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.inputValue = "/sync "
	if m.cmdMaybePrefetchHistory() == nil {
		t.Fatal("/sync  must prefetch the chat list")
	}
	m.chatList = []client.RunHistoryItem{syncableItem("r1", "fix", "chat", "completed")}
	if m.cmdMaybePrefetchHistory() != nil {
		t.Fatal("already-cached list must not re-fetch")
	}
}
