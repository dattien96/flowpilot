package app

import (
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// TestHistoryPickerRefreshesStaleCache is the BUG-355 F1 regression: the
// /history|/open|/resume picker renders from m.chatList, and the first fetch
// of a session sticks for the whole session (runs started afterwards never
// appear — live: run-547025/run-548341 missing while the server listed 23
// runs). While the picker is open, a keypress must dispatch a silent
// background refresh; the stale cache keeps showing until the fresh
// ChatListMsg merges. Same pattern as the BUG-351 /flow picker fix.
func TestHistoryPickerRefreshesStaleCache(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.sessionLoading = false
	// Stale session-start snapshot: only the Sep-4 run, no Sep-5 runs yet.
	m.chatList = []client.RunHistoryItem{{RunID: "run-540927", Status: "cancelled"}}
	m.inputValue = "/history "
	m.inputCursor = -1

	// A keypress with the picker open dispatches exactly one refresh.
	if cmd := m.cmdMaybePrefetchHistory(); cmd == nil {
		t.Fatal("expected a silent background refresh while the picker is open")
	}
	if !m.chatListInflight {
		t.Fatal("expected chatListInflight while the refresh is running")
	}
	if cmd := m.cmdMaybePrefetchHistory(); cmd != nil {
		t.Fatal("in-flight refresh must not double-fire per keypress")
	}

	// Fresh list arrives (runs started after the first fetch): cache
	// converges, flag clears.
	fresh := []client.RunHistoryItem{
		{RunID: "run-548341", Status: "completed"},
		{RunID: "run-547025", Status: "completed"},
		{RunID: "run-540927", Status: "cancelled"},
	}
	m2, _ := m.Update(ChatListMsg{Items: fresh, Silent: true})
	am := m2.(*AppModel)
	if am.chatListInflight {
		t.Fatal("expected chatListInflight cleared after ChatListMsg")
	}
	if len(am.chatList) != 3 {
		t.Fatalf("chatList=%d, want converged 3", len(am.chatList))
	}
	if am.chatList[0].RunID != "run-548341" {
		t.Fatalf("newest run must lead after refresh: %+v", am.chatList)
	}
}

// TestChatListMsgErrorKeepsGoodCache pins the BUG-355 F1 companion rule: a
// failed background refresh must not wipe the last good list, and the next
// keypress may retry (no interval penalty on error).
func TestChatListMsgErrorKeepsGoodCache(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.sessionLoading = false
	m.chatList = []client.RunHistoryItem{{RunID: "run-540927", Status: "cancelled"}}
	m.chatListInflight = true
	m.inputValue = "/history "
	m.inputCursor = -1

	m2, _ := m.Update(ChatListMsg{Err: "boom", Silent: true})
	am := m2.(*AppModel)
	if am.chatListInflight {
		t.Fatal("expected chatListInflight cleared even on error")
	}
	if len(am.chatList) != 1 || am.chatList[0].RunID != "run-540927" {
		t.Fatalf("good cache must survive a failed refresh: %+v", am.chatList)
	}
	if cmd := am.cmdMaybePrefetchHistory(); cmd == nil {
		t.Fatal("error must not start the interval penalty — retry allowed")
	}
}

// TestHistoryPickerRefreshIntervalBound keeps per-keypress refreshes cheap
// while the picker stays open: a second refresh right after a completed one
// waits out chatPickerRefreshInterval instead of firing per keystroke.
func TestHistoryPickerRefreshIntervalBound(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.sessionLoading = false
	m.chatList = []client.RunHistoryItem{{RunID: "run-540927", Status: "cancelled"}}
	m.inputValue = "/open "
	m.inputCursor = -1

	m2, _ := m.Update(ChatListMsg{Items: m.chatList, Silent: true})
	am := m2.(*AppModel)
	// Fresh response just landed: no immediate second fetch.
	if cmd := am.cmdMaybePrefetchHistory(); cmd != nil {
		t.Fatal("refresh must wait out the interval instead of firing per keystroke")
	}
}
