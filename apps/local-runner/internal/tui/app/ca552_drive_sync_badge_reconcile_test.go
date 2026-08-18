package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-552 — the (synced) badge survives a restart and never re-targets an
// already-synced chat: reconcile the history rows against the confirmed Drive
// index (remoteChatList) when the runner's local syncStatus is stale/empty, and
// carry known local markers across a refetch that races the async store write.

func remoteRow(machine, sourceRun string) client.RemoteChatSessionSummary {
	return client.RemoteChatSessionSummary{
		SourceMachineID: machine,
		SourceRunID:     sourceRun,
		Status:          "completed",
	}
}

func TestSyncBadgeWithRemote_ReconcilesWhenLocalEmpty(t *testing.T) {
	it := client.RunHistoryItem{RunID: "run-107774", Status: "completed", RunKind: "chat"}
	remote := []client.RemoteChatSessionSummary{remoteRow("mch_xyz", "run-107774")}
	if got := syncBadgeWithRemote(it, remote); got != "synced" {
		t.Fatalf("reconcile got=%q want synced", got)
	}
}

func TestSyncBadgeWithRemote_PairMatch(t *testing.T) {
	it := client.RunHistoryItem{RunID: "r1", SourceMachineID: "mch_a", SourceRunID: "r1"}
	remote := []client.RemoteChatSessionSummary{remoteRow("mch_a", "r1")}
	if got := syncBadgeWithRemote(it, remote); got != "synced" {
		t.Fatalf("pair reconcile got=%q", got)
	}
}

func TestSyncBadgeWithRemote_LocalStatusWins(t *testing.T) {
	it := client.RunHistoryItem{RunID: "r1", SyncStatus: "failed"}
	remote := []client.RemoteChatSessionSummary{remoteRow("mch_a", "r1")}
	if got := syncBadgeWithRemote(it, remote); got != "failed" {
		t.Fatalf("failed must win over remote reconcile: got=%q", got)
	}
	it2 := client.RunHistoryItem{RunID: "r2", SyncStatus: "unsyncable"}
	if got := syncBadgeWithRemote(it2, remote); got != "unsyncable" {
		t.Fatalf("unsyncable must win: got=%q", got)
	}
}

func TestSyncBadgeWithRemote_NoRemoteNoBadge(t *testing.T) {
	it := client.RunHistoryItem{RunID: "r1"}
	if got := syncBadgeWithRemote(it, nil); got != "" {
		t.Fatalf("nil remote got=%q", got)
	}
	if got := syncBadgeWithRemote(it, []client.RemoteChatSessionSummary{remoteRow("mch_a", "other")}); got != "" {
		t.Fatalf("no-match got=%q", got)
	}
}

func TestFormatChatListWithRemote_ReconciledBadge(t *testing.T) {
	items := []client.RunHistoryItem{
		{RunID: "run-a", LastPrompt: "fix login", Status: "completed", RunKind: "chat"},
	}
	dump := formatChatListWithRemote(items, []client.RemoteChatSessionSummary{remoteRow("mch_a", "run-a")})
	if !strings.Contains(dump, "(synced)") {
		t.Fatalf("dump must show reconciled badge:\n%s", dump)
	}
}

func TestFilterHistorySuggestionsWithRemote_BadgeBeforeTitle(t *testing.T) {
	items := []client.RunHistoryItem{
		{RunID: "run-a", LastPrompt: "fix login bug on windows", Status: "completed", RunKind: "chat"},
	}
	sugg := filterHistorySuggestionsWithRemote("/history ", items, []client.RemoteChatSessionSummary{remoteRow("mch_a", "run-a")})
	if len(sugg) != 1 {
		t.Fatalf("suggestions=%+v", sugg)
	}
	idx := strings.Index(sugg[0].detail, "synced")
	titleIdx := strings.Index(sugg[0].detail, "fix login")
	if idx < 0 || titleIdx < 0 || idx > titleIdx {
		t.Fatalf("badge must precede the title: %q", sugg[0].detail)
	}
}

func TestFilterSyncSuggestionsWithRemote_ReconciledBadge(t *testing.T) {
	syncable := []client.RunHistoryItem{
		{RunID: "run-a", LastPrompt: "fix", Status: "completed", RunKind: "chat"},
	}
	sugg := filterSyncSuggestionsWithRemote("/sync ", syncable, []client.RemoteChatSessionSummary{remoteRow("mch_a", "run-a")})
	found := false
	for _, s := range sugg {
		if s.value == "run-a" && strings.Contains(s.detail, "synced") {
			found = true
		}
	}
	if !found {
		t.Fatalf("sync picker must show reconciled badge: %+v", sugg)
	}
}

func TestMergeChatListSyncStatus_PreservesLocalOnEmptyFresh(t *testing.T) {
	cached := []client.RunHistoryItem{{RunID: "r1", SyncStatus: "synced"}}
	fresh := []client.RunHistoryItem{{RunID: "r1"}}
	merged := mergeChatListSyncStatus(cached, fresh)
	if merged[0].SyncStatus != "synced" {
		t.Fatalf("local synced marker lost on fresh empty: %+v", merged)
	}
}

func TestMergeChatListSyncStatus_FreshStatusWins(t *testing.T) {
	cached := []client.RunHistoryItem{{RunID: "r1", SyncStatus: "synced"}}
	fresh := []client.RunHistoryItem{{RunID: "r1", SyncStatus: "failed"}}
	merged := mergeChatListSyncStatus(cached, fresh)
	if merged[0].SyncStatus != "failed" {
		t.Fatalf("fresh status must win: %+v", merged)
	}
}

func TestMergeChatListSyncStatus_NeverMutatesFreshRowsWithoutMarker(t *testing.T) {
	cached := []client.RunHistoryItem{{RunID: "r1", SyncStatus: "synced"}}
	fresh := []client.RunHistoryItem{{RunID: "r2"}}
	merged := mergeChatListSyncStatus(cached, fresh)
	if merged[0].SyncStatus != "" {
		t.Fatalf("unrelated fresh row picked up a marker: %+v", merged)
	}
}

func TestChatListMsg_SilentRefetchKeepsSyncedMarker(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.chatList = []client.RunHistoryItem{{RunID: "r1", SyncStatus: "synced"}}
	m2, _ := m.Update(ChatListMsg{Items: []client.RunHistoryItem{{RunID: "r1"}}, Silent: true})
	if got := m2.(*AppModel).chatList[0].SyncStatus; got != "synced" {
		t.Fatalf("silent refetch clobbered synced marker: got=%q", got)
	}
}

func TestChatListMsg_LoudDumpReconcilesRemote(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.remoteChatList = []client.RemoteChatSessionSummary{remoteRow("mch_a", "run-a")}
	m2, _ := m.Update(ChatListMsg{Items: []client.RunHistoryItem{{RunID: "run-a", LastPrompt: "hi", Status: "completed", RunKind: "chat"}}})
	if !strings.Contains(m2.(*AppModel).View(), "(synced)") {
		t.Fatal("loud dump must reconcile against the cached remote index")
	}
}
