package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
)

// G1 (CA-548): /history rows now surface Drive chat-session sync status
// (Desktop Navigator syncStatus parity). Empty badge keeps local-first rows
// visually unchanged; old assertions on title/time still hold.

func TestFormatSyncBadge_Statuses(t *testing.T) {
	cases := []struct {
		status string
		want   string
	}{
		{"synced", "synced"},
		{"failed", "failed"},
		{"unsyncable", "unsyncable"},
		{"syncing", "syncing"},
		{"", ""},
		{"local_only", ""},
	}
	for _, tc := range cases {
		got := formatSyncBadge(client.RunHistoryItem{SyncStatus: tc.status})
		if got != tc.want {
			t.Errorf("status=%q got=%q want=%q", tc.status, got, tc.want)
		}
	}
}

func TestFormatChatList_ShowsSyncBadgeAndKeepsTitle(t *testing.T) {
	items := []client.RunHistoryItem{
		{RunID: "run-a", LastPrompt: "fix login", Status: "completed", RunKind: "chat", SyncStatus: "synced"},
		{RunID: "run-b", LastPrompt: "ship android", Status: "completed", RunKind: "chat", SyncStatus: "failed"},
		{RunID: "run-c", LastPrompt: "local only", Status: "completed", RunKind: "chat"},
	}
	dump := formatChatList(items)
	if !strings.Contains(dump, "fix login") || !strings.Contains(dump, "ship android") {
		t.Fatalf("titles must stay:\n%s", dump)
	}
	if !strings.Contains(dump, "(synced)") {
		t.Fatalf("missing synced badge:\n%s", dump)
	}
	if !strings.Contains(dump, "(failed)") {
		t.Fatalf("missing failed badge:\n%s", dump)
	}
	if strings.Contains(dump, "local only)") || strings.Contains(dump, "  ()") {
		t.Fatalf("empty badge must not render:\n%s", dump)
	}
}

func TestFilterHistorySuggestions_DetailCarriesBadgeAndHaystackFilters(t *testing.T) {
	items := []client.RunHistoryItem{
		{RunID: "run-aaa", LastPrompt: "fix login bug", Status: "completed", RunKind: "chat", SyncStatus: "synced"},
		{RunID: "run-bbb", LastPrompt: "ship android", Status: "completed", RunKind: "chat", SyncStatus: "failed"},
	}
	all := filterHistorySuggestions("/history ", items)
	if len(all) != 2 {
		t.Fatalf("all=%+v", all)
	}
	if !strings.Contains(all[0].detail, "synced") {
		t.Fatalf("picker detail missing synced: %q", all[0].detail)
	}
	failed := filterHistorySuggestions("/history fail", items)
	if len(failed) != 1 || failed[0].value != "run-bbb" {
		t.Fatalf("filter by badge failed: %+v", failed)
	}
	// value + slash contract unchanged (old picker tests rely on it).
	if all[0].value != "run-aaa" || all[0].slash != "/history" {
		t.Fatalf("value=%q slash=%q", all[0].value, all[0].slash)
	}
}
