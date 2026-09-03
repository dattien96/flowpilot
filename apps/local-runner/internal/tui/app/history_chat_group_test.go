package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
)

// grouping mirror of Desktop chatHistory.ts: one chat = one row.

func chatItem(over client.RunHistoryItem) client.RunHistoryItem {
	base := client.RunHistoryItem{
		RunID:     "run-x",
		ProjectID: "p",
		Status:    "completed",
		RunKind:   "chat",
		UpdatedAt: "2026-08-31T00:00:00Z",
	}
	if over.RunID != "" {
		base.RunID = over.RunID
	}
	if over.ChatID != "" {
		base.ChatID = over.ChatID
	}
	if over.LegSeq != 0 {
		base.LegSeq = over.LegSeq
	}
	if over.ProviderKey != "" {
		base.ProviderKey = over.ProviderKey
	}
	if over.LastPrompt != "" {
		base.LastPrompt = over.LastPrompt
	}
	if over.UpdatedAt != "" {
		base.UpdatedAt = over.UpdatedAt
	}
	if over.RunKind != "" {
		base.RunKind = over.RunKind
	}
	if over.Status != "" {
		base.Status = over.Status
	}
	if over.SyncStatus != "" {
		base.SyncStatus = over.SyncStatus
	}
	if over.WorkflowID != "" {
		base.WorkflowID = over.WorkflowID
	}
	return base
}

func TestGroupRunsByChatId_CollapsesLegsUnderOneChat(t *testing.T) {
	items := []client.RunHistoryItem{
		chatItem(client.RunHistoryItem{RunID: "run-1", ChatID: "cht_a", LegSeq: 0, ProviderKey: "codex", UpdatedAt: "2026-08-31T00:00:00Z"}),
		chatItem(client.RunHistoryItem{RunID: "run-2", ChatID: "cht_a", LegSeq: 1, ProviderKey: "grok", UpdatedAt: "2026-08-31T00:01:00Z"}),
		chatItem(client.RunHistoryItem{RunID: "run-3", ChatID: "cht_a", LegSeq: 2, ProviderKey: "opencode", UpdatedAt: "2026-08-31T00:02:00Z"}),
		chatItem(client.RunHistoryItem{RunID: "run-wf", ProviderKey: "codex", RunKind: "workflow"}),
	}
	rows := groupRunsByChatId(items)
	if len(rows) != 2 {
		t.Fatalf("rows=%d want 2 (one grouped chat + one workflow)", len(rows))
	}
	// First row is the chat, head is latest leg (legSeq 2)
	if rows[0].item.RunID != "run-3" {
		t.Fatalf("head RunID=%q want run-3 (latest leg)", rows[0].item.RunID)
	}
	if rows[0].group == nil || len(rows[0].group.legs) != 3 {
		t.Fatalf("group legs=%v want 3", rows[0].group)
	}
	// Untagged passes through 1:1
	if rows[1].group != nil {
		t.Fatalf("workflow row should not be grouped")
	}
	if rows[1].item.RunID != "run-wf" {
		t.Fatalf("second row RunID=%q", rows[1].item.RunID)
	}
}

func TestGroupRunsByChatId_LegacyUntaggedStayOneToOne(t *testing.T) {
	items := []client.RunHistoryItem{
		chatItem(client.RunHistoryItem{RunID: "run-aaa", LastPrompt: "fix login"}),
		chatItem(client.RunHistoryItem{RunID: "run-bbb", LastPrompt: "ship android"}),
	}
	rows := groupRunsByChatId(items)
	if len(rows) != 2 {
		t.Fatalf("legacy rows=%d want 2", len(rows))
	}
	if rows[0].group != nil || rows[1].group != nil {
		t.Fatalf("legacy rows must not have group")
	}
}

func TestGroupRunsByChatId_NewestFirstInputStaysGrouped(t *testing.T) {
	// Runner sorts by UpdatedAt desc (newest first). Group must still collapse.
	items := []client.RunHistoryItem{
		chatItem(client.RunHistoryItem{RunID: "run-2", ChatID: "cht_a", LegSeq: 1, UpdatedAt: "2026-08-31T00:01:00Z"}),
		chatItem(client.RunHistoryItem{RunID: "run-1", ChatID: "cht_a", LegSeq: 0, UpdatedAt: "2026-08-31T00:00:00Z"}),
	}
	rows := groupRunsByChatId(items)
	if len(rows) != 1 {
		t.Fatalf("rows=%d want 1", len(rows))
	}
	if rows[0].item.RunID != "run-2" {
		t.Fatalf("head=%q want run-2 (newest first input)", rows[0].item.RunID)
	}
	if len(rows[0].group.legs) != 2 {
		t.Fatalf("legs=%d want 2", len(rows[0].group.legs))
	}
}

func TestFilterHistorySuggestions_GroupsChatsAndShowsLegs(t *testing.T) {
	items := []client.RunHistoryItem{
		chatItem(client.RunHistoryItem{RunID: "run-1", ChatID: "cht_a", LegSeq: 0, LastPrompt: "hello A", UpdatedAt: "2026-08-31T00:00:00Z"}),
		chatItem(client.RunHistoryItem{RunID: "run-2", ChatID: "cht_a", LegSeq: 1, LastPrompt: "hello grok", UpdatedAt: "2026-08-31T00:01:00Z"}),
		chatItem(client.RunHistoryItem{RunID: "run-b", LastPrompt: "other chat", UpdatedAt: "2026-08-31T00:02:00Z"}),
	}
	sugg := filterHistorySuggestions("/history ", items)
	if len(sugg) != 2 {
		t.Fatalf("sugg=%d want 2 (one grouped + one solo), got %+v", len(sugg), sugg)
	}
	// First suggestion is the grouped chat's head (run-2), detail shows 2 legs
	if sugg[0].value != "run-2" {
		t.Fatalf("first sugg value=%q want run-2 (head)", sugg[0].value)
	}
	if !strings.Contains(sugg[0].detail, "2 legs") {
		t.Fatalf("detail must show legs count, got %q", sugg[0].detail)
	}
	// Filtering by old leg runId still surfaces the chat
	filt := filterHistorySuggestions("/history run-1", items)
	if len(filt) != 1 || filt[0].value != "run-2" {
		t.Fatalf("filter by old leg should surface grouped head, got %+v", filt)
	}
	// Filtering by chatId
	filt2 := filterHistorySuggestions("/history cht_a", items)
	if len(filt2) != 1 || filt2[0].value != "run-2" {
		t.Fatalf("filter by chatId should surface grouped head, got %+v", filt2)
	}
}

func TestFormatChatList_GroupsDumpAndShowsLegs(t *testing.T) {
	items := []client.RunHistoryItem{
		chatItem(client.RunHistoryItem{RunID: "run-1", ChatID: "cht_a", LegSeq: 0, LastPrompt: "fix login", UpdatedAt: "2026-08-31T00:00:00Z"}),
		chatItem(client.RunHistoryItem{RunID: "run-2", ChatID: "cht_a", LegSeq: 1, LastPrompt: "fix login continued", UpdatedAt: "2026-08-31T00:01:00Z"}),
		chatItem(client.RunHistoryItem{RunID: "run-3", ChatID: "cht_a", LegSeq: 2, LastPrompt: "fix login v3", UpdatedAt: "2026-08-31T00:02:00Z"}),
	}
	dump := formatChatList(items)
	// Dump should have one row, not three
	lines := strings.Split(dump, "\n")
	// Count lines that start with "   1" and "   2" etc — the numbered rows
	numRows := 0
	for _, l := range lines {
		if strings.HasPrefix(l, "   1") || strings.HasPrefix(l, "   2") || strings.HasPrefix(l, "   3") {
			numRows++
		}
	}
	if numRows != 1 {
		t.Fatalf("dump should have 1 grouped row, got %d lines:\n%s", numRows, dump)
	}
	if !strings.Contains(dump, "3 legs") {
		t.Fatalf("dump missing legs count:\n%s", dump)
	}
	if !strings.Contains(dump, "cht_a") {
		t.Fatalf("dump should show chatId for grouped chat:\n%s", dump)
	}
	// Legacy: untagged items stay 1:1
	legacy := []client.RunHistoryItem{
		chatItem(client.RunHistoryItem{RunID: "run-x", LastPrompt: "a"}),
		chatItem(client.RunHistoryItem{RunID: "run-y", LastPrompt: "b"}),
	}
	dump2 := formatChatList(legacy)
	if strings.Count(dump2, "  id run-") != 2 {
		t.Fatalf("legacy dump should have 2 rows:\n%s", dump2)
	}
}

func TestResolveChatOpenTarget_UsesGroupedIndexAndChatId(t *testing.T) {
	items := []client.RunHistoryItem{
		chatItem(client.RunHistoryItem{RunID: "run-1", ChatID: "cht_a", LegSeq: 0}),
		chatItem(client.RunHistoryItem{RunID: "run-2", ChatID: "cht_a", LegSeq: 1}),
		chatItem(client.RunHistoryItem{RunID: "run-b", LastPrompt: "solo"}),
	}
	// Numeric index 1 should be the grouped chat head (run-2), not run-1
	id, err := resolveChatOpenTarget([]string{"1"}, items)
	if err != nil || id != "run-2" {
		t.Fatalf("index 1 id=%q err=%v want run-2", id, err)
	}
	id, err = resolveChatOpenTarget([]string{"2"}, items)
	if err != nil || id != "run-b" {
		t.Fatalf("index 2 id=%q err=%v want run-b", id, err)
	}
	// ChatId resolves to head
	id, err = resolveChatOpenTarget([]string{"cht_a"}, items)
	if err != nil || id != "run-2" {
		t.Fatalf("chatId id=%q err=%v want run-2", id, err)
	}
	// Old leg runId resolves to head
	id, err = resolveChatOpenTarget([]string{"run-1"}, items)
	if err != nil || id != "run-2" {
		t.Fatalf("old leg runId should resolve to head run-2, got %q err=%v", id, err)
	}
	// Unknown runId passes through (allows manual /open run-xyz not in list)
	id, err = resolveChatOpenTarget([]string{"run-unknown"}, items)
	if err != nil || id != "run-unknown" {
		t.Fatalf("unknown should pass through, got %q err=%v", id, err)
	}
}

func TestGroupRunsByChatId_MixedWorkflowAndChat(t *testing.T) {
	items := []client.RunHistoryItem{
		chatItem(client.RunHistoryItem{RunID: "run-wf-1", ProviderKey: "codex", RunKind: "workflow", WorkflowID: "wf1", UpdatedAt: "2026-08-31T00:03:00Z"}),
		chatItem(client.RunHistoryItem{RunID: "run-1", ChatID: "cht_x", LegSeq: 0, UpdatedAt: "2026-08-31T00:01:00Z"}),
		chatItem(client.RunHistoryItem{RunID: "run-2", ChatID: "cht_x", LegSeq: 1, UpdatedAt: "2026-08-31T00:02:00Z"}),
	}
	rows := groupRunsByChatId(items)
	if len(rows) != 2 {
		t.Fatalf("rows=%d want 2 (workflow + grouped chat)", len(rows))
	}
	// Workflow first (newest updatedAt) stays separate
	if rows[0].item.RunID != "run-wf-1" || rows[0].group != nil {
		t.Fatalf("first row should be workflow ungrouped, got %+v", rows[0])
	}
	if rows[1].item.RunID != "run-2" || rows[1].group == nil || len(rows[1].group.legs) != 2 {
		t.Fatalf("second row should be grouped chat head run-2, got %+v", rows[1])
	}
}
