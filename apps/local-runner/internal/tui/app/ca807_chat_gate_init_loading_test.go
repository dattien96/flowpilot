package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// The chat gate holds ready/send — never the session banner (sessionLoading
// stays owned by the catalog gate, CA-514, pinned by old tests).

func ca807DefaultsWithProject(t *testing.T) *AppModel {
	t.Helper()
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.width, m.height = 120, 40
	m2, _ := m.Update(SessionDefaultsMsg{
		Provider: "codex", Model: "m",
		Projects: []client.Project{{ID: "p1", Name: "proj"}},
		Project:  &client.Project{ID: "p1", Name: "proj"},
	})
	return m2.(*AppModel)
}

func TestCA807_DefaultsWithProjectHoldsReadyUntilChats(t *testing.T) {
	am := ca807DefaultsWithProject(t)
	if am.sessionLoading {
		t.Fatal("banner contract stays catalog-owned: sessionLoading must clear")
	}
	if !am.chatWaitPending {
		t.Fatal("startup chat wait must arm")
	}
	if am.statusMsg != "loading chats…" {
		t.Fatalf("statusMsg=%q want loading chats…", am.statusMsg)
	}
	m2, _ := am.Update(ChatListMsg{Items: []client.RunHistoryItem{{RunID: "run-1", Status: "completed", RunKind: "chat"}}, Silent: true})
	am2 := m2.(*AppModel)
	if am2.chatWaitPending {
		t.Fatal("chat wait must disarm once chats arrive")
	}
	if am2.statusMsg != "ready" {
		t.Fatalf("statusMsg=%q want ready", am2.statusMsg)
	}
	if len(am2.chatList) != 1 {
		t.Fatalf("chatList=%d want 1", len(am2.chatList))
	}
}

func TestCA807_StartupChatErrorPassesDegradedLoud(t *testing.T) {
	am := ca807DefaultsWithProject(t)
	m2, _ := am.Update(ChatListMsg{Err: "boom", Silent: true})
	am2 := m2.(*AppModel)
	if am2.chatWaitPending {
		t.Fatal("failed startup fetch must not hold the chat wait forever")
	}
	if len(am2.chatList) != 0 {
		t.Fatal("failed fetch must not populate the list")
	}
	joined := ""
	for _, msg := range am2.messages {
		joined += msg.Content
	}
	if !strings.Contains(joined, "Chat list failed: boom") {
		t.Fatalf("startup chat error must surface loudly, got:\n%s", joined)
	}
	if am2.statusMsg != "ready" {
		t.Fatalf("statusMsg=%q want ready after degraded pass", am2.statusMsg)
	}
}

func TestCA807_SendBlockedUntilChatsSettle(t *testing.T) {
	am := ca807DefaultsWithProject(t)
	blocked, _ := am.processInput("hello while chats load")
	b := blocked.(*AppModel)
	if len(b.messages) == 0 || !strings.Contains(b.messages[len(b.messages)-1].Content, "Still loading chats") {
		t.Fatalf("non-slash send must stay blocked until chats settle: %+v", b.messages)
	}
}

func TestCA807_DefaultsWithoutProjectKeepsOldBehavior(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m2, _ := m.Update(SessionDefaultsMsg{
		Provider: "codex", Model: "m",
		Projects: []client.Project{{ID: "p1", Name: "proj"}},
	})
	am := m2.(*AppModel)
	if am.sessionLoading {
		t.Fatal("no bound project: init-loading must pass as before (ca633)")
	}
	if am.chatWaitPending {
		t.Fatal("no bound project: no chat wait to arm")
	}
	if am.statusMsg == "loading chats…" {
		t.Fatal("no bound project: must not show the chat hold")
	}
}

func TestCA807_FirstOpenPrefetchDedupsInflight(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.sessionLoading = false
	m.inputValue = "/open "
	m.inputCursor = -1
	if cmd := m.cmdMaybePrefetchHistory(); cmd == nil {
		t.Fatal("first /open must prefetch chats")
	}
	if !m.chatListInflight {
		t.Fatal("first prefetch must arm the in-flight flag")
	}
	if cmd := m.cmdMaybePrefetchHistory(); cmd != nil {
		t.Fatal("in-flight first fetch must not double-fire per keypress")
	}
}

func TestCA807_ChatBudgetPassesDegraded(t *testing.T) {
	am := ca807DefaultsWithProject(t)
	m2, _ := am.Update(chatLoadTimeoutMsg{})
	am2 := m2.(*AppModel)
	if am2.chatWaitPending {
		t.Fatal("budget expiry must disarm the chat wait")
	}
	if am2.statusMsg != "ready" {
		t.Fatalf("statusMsg=%q want ready after budget pass", am2.statusMsg)
	}
	joined := ""
	for _, msg := range am2.messages {
		joined += msg.Content
	}
	if !strings.Contains(joined, "taking too long") {
		t.Fatalf("budget pass must say why, got:\n%s", joined)
	}
}
