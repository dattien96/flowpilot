package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-554 — the /restore Tab picker lists Drive-backed chats one at a time
// (Desktop Navigator restore parity). The remote index must prefetch for a
// restore command even when the local chatList is cached, and the picker shows
// a loading/empty placeholder while it is in flight.

func TestCmdMaybePrefetchHistory_RestorePrefetchesRemoteEvenWhenChatListCached(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.inputValue = "/restore "
	m.chatList = []client.RunHistoryItem{{RunID: "run-a", Status: "completed", RunKind: "chat"}}
	if m.cmdMaybePrefetchHistory() == nil {
		t.Fatal("/restore  with cached chatList must still prefetch the Drive index")
	}
}

func TestCmdMaybePrefetchHistory_RestoreSkipsWhenRemoteCached(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.inputValue = "/restore "
	m.chatList = []client.RunHistoryItem{{RunID: "run-a", Status: "completed", RunKind: "chat"}}
	m.remoteChatList = []client.RemoteChatSessionSummary{remoteItem("m1", "r1", "hi", "completed")}
	if m.cmdMaybePrefetchHistory() != nil {
		t.Fatal("/restore  with cached remote index must not re-fetch")
	}
}

func TestCollectSuggestions_RestoreLoadingPlaceholder(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.inputValue = "/restore "
	items := m.collectSuggestions()
	if len(items) != 1 || items[0].kind != "restore" || items[0].slash != "/restore" {
		t.Fatalf("loading placeholder wrong: %+v", items)
	}
	if !strings.Contains(items[0].detail, "loading") {
		t.Fatalf("loading placeholder detail: %q", items[0].detail)
	}
}

func TestCollectSuggestions_RestoreEmptyState(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.inputValue = "/restore "
	m.remoteChatList = []client.RemoteChatSessionSummary{}
	items := m.collectSuggestions()
	if len(items) != 1 || items[0].kind != "restore" {
		t.Fatalf("empty-state placeholder wrong: %+v", items)
	}
	if !strings.Contains(items[0].detail, "no Drive-backed") {
		t.Fatalf("empty-state detail: %q", items[0].detail)
	}
}

func TestCollectSuggestions_RestoreListsAllRowAndChats(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.inputValue = "/restore "
	m.remoteChatList = []client.RemoteChatSessionSummary{
		remoteItem("m1", "r1", "fix login bug", "completed"),
		remoteItem("m2", "r2", "ship android", "completed"),
	}
	items := m.collectSuggestions()
	if len(items) != 3 {
		t.Fatalf("restore picker rows=%d want 3: %+v", len(items), items)
	}
	if items[0].value != "all" || items[0].kind != "restore" || items[0].slash != "/restore" {
		t.Fatalf("all row wrong: %+v", items[0])
	}
	if items[1].value != "m1:r1" || items[2].value != "m2:r2" {
		t.Fatalf("restore values wrong: %+v", items)
	}
}

func TestFilterRestoreSuggestions_DetailCarriesIndexKindStatusDate(t *testing.T) {
	remote := []client.RemoteChatSessionSummary{
		remoteItem("m1", "r1", "fix login bug", "completed"),
	}
	sugg := filterRestoreSuggestions("/restore ", remote)
	if len(sugg) != 2 {
		t.Fatalf("rows=%d want 2 (all + one chat): %+v", len(sugg), sugg)
	}
	d := sugg[1].detail
	if !strings.HasPrefix(d, "#1 · ") {
		t.Fatalf("detail must start with #N: %q", d)
	}
	if !strings.Contains(d, "completed") || !strings.Contains(d, "fix login bug") {
		t.Fatalf("detail missing kind/status/title: %q", d)
	}
}

func TestRenderSuggestions_HeaderLabelsRestore(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	items := []suggestItem{{value: "all", detail: "Restore all 1 Drive-backed chats", kind: "restore", slash: "/restore"}}
	view := m.renderSuggestions(items)
	if !strings.Contains(view, "restore:") {
		t.Fatalf("picker header must say restore::\n%s", view)
	}
}

func TestRenderSuggestions_HeaderLabelsSync(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	items := []suggestItem{{value: "all", detail: "Sync all 1 syncable chats to Drive", kind: "sync", slash: "/sync"}}
	view := m.renderSuggestions(items)
	if !strings.Contains(view, "sync:") {
		t.Fatalf("picker header must say sync::\n%s", view)
	}
}
