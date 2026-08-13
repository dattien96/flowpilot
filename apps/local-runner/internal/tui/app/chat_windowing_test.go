package app

import (
	"fmt"
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/config"
)

func makePromptHistory(n int) []ChatMessage {
	var msgs []ChatMessage
	for i := 1; i <= n; i++ {
		msgs = append(msgs,
			ChatMessage{Role: "user", Content: fmt.Sprintf("prompt-%d", i)},
			ChatMessage{Role: "assistant", Content: fmt.Sprintf("answer-%d", i)},
		)
	}
	return msgs
}

func TestSliceMessagesFromPrompt_ShowsNewestGroups(t *testing.T) {
	msgs := makePromptHistory(10)
	got := sliceMessagesFromPrompt(msgs, chatPromptPageSize)
	if countUserPrompts(got) != chatPromptPageSize {
		t.Fatalf("visible prompts = %d, want %d", countUserPrompts(got), chatPromptPageSize)
	}
	if !strings.Contains(got[0].Content, "prompt-5") {
		t.Fatalf("first visible = %q, want prompt-5 group", got[0].Content)
	}
	if strings.Contains(got[0].Content, "prompt-4") {
		t.Fatal("must not include prompt-4 group")
	}
}

func TestEnsureAndExpandVisiblePromptCount(t *testing.T) {
	if got := ensureVisiblePromptCount(0, 10, chatPromptPageSize); got != chatPromptPageSize {
		t.Fatalf("init = %d, want %d", got, chatPromptPageSize)
	}
	if got := expandVisiblePromptCount(chatPromptPageSize, 14, chatPromptPageSize); got != 12 {
		t.Fatalf("expand = %d, want 12", got)
	}
	if got := expandVisiblePromptCount(12, 10, chatPromptPageSize); got != 10 {
		t.Fatalf("expand capped = %d, want 10", got)
	}
}

func TestChatWindow_ShowsNewestSixPromptGroups(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.messages = makePromptHistory(10)
	m.syncVisiblePromptCount()

	start := m.windowStartIndex()
	if got := countUserPrompts(m.messages[start:]); got != chatPromptPageSize {
		t.Fatalf("window prompts = %d, want %d", got, chatPromptPageSize)
	}
	if !strings.Contains(m.messages[start].Content, "prompt-5") {
		t.Fatalf("window starts at %q, want prompt-5", m.messages[start].Content)
	}

	rows := m.chatRows()
	if len(rows) == 0 {
		t.Fatal("expected chat rows")
	}
	if !rows[0].LoadEarlier {
		t.Fatal("expected load-earlier row at top")
	}
	if !strings.Contains(stripANSI(rows[0].Text), "Load earlier prompts (4)") {
		t.Fatalf("load-earlier label = %q", stripANSI(rows[0].Text))
	}
}

func TestChatWindow_LoadEarlierRevealsPreviousPage(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.messages = makePromptHistory(14)
	m.syncVisiblePromptCount()

	m.loadEarlierPrompts()
	start := m.windowStartIndex()
	if got := countUserPrompts(m.messages[start:]); got != 12 {
		t.Fatalf("after load earlier prompts = %d, want 12", got)
	}
	if !strings.Contains(m.messages[start].Content, "prompt-3") {
		t.Fatalf("window should now include prompt-3, start=%q", m.messages[start].Content)
	}
}

func TestChatWindow_PendingGateStaysVisible(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	base := makePromptHistory(10)
	// Pending approval on prompt 2 — would be paged out with the default 6-group window.
	var msgs []ChatMessage
	msgs = append(msgs, base[0], base[1], base[2], base[3])
	msgs = append(msgs, ChatMessage{
		Role: "system", Content: "Approve write test.txt", FormatHint: "approval",
	})
	msgs = append(msgs, base[4:]...)
	m.messages = msgs
	m.approval = &ApprovalState{ID: "ap-1", RunID: "run-1"}
	m.syncVisiblePromptCount()

	start := m.windowStartIndex()
	if !strings.Contains(m.messages[start].Content, "prompt-2") {
		t.Fatalf("pending approval must keep prompt-2 group visible, start=%q", m.messages[start].Content)
	}
	if strings.Contains(m.messages[start].Content, "prompt-1") {
		t.Fatal("prompt-1 should stay hidden when only prompt-2 group is pulled in")
	}
}

func TestChatWindow_NewResetsWindow(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.messages = makePromptHistory(10)
	m.visiblePromptCount = 10
	m2, _ := m.handleSlashCommand("/new")
	am := m2.(*AppModel)
	if am.visiblePromptCount != 0 {
		t.Fatalf("visiblePromptCount = %d, want 0 after /new", am.visiblePromptCount)
	}
	am.messages = makePromptHistory(10)
	am.syncVisiblePromptCount()
	if am.visiblePromptCount != chatPromptPageSize {
		t.Fatalf("after reload visiblePromptCount = %d, want %d", am.visiblePromptCount, chatPromptPageSize)
	}
}

func TestChatRows_WindowedCacheInvalidatesOnLoadEarlier(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.messages = makePromptHistory(10)
	m.syncVisiblePromptCount()
	firstSig := m.rowCacheSig
	_ = m.chatRows()
	if m.rowCacheSig == 0 {
		t.Fatal("expected row cache signature")
	}
	m.loadEarlierPrompts()
	_ = m.chatRows()
	if m.rowCacheSig == firstSig && firstSig != 0 {
		t.Fatal("load earlier must rebuild chat rows")
	}
}
