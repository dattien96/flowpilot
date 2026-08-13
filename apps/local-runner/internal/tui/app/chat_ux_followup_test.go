package app

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestMouseClick_WindowsTypeLeftClearsSelection(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.mouseSel = mouseSelect{armed: true, x0: 0, y0: 2, x1: 10, y1: 4}
	m2, _ := m.Update(tea.MouseMsg{X: 40, Y: 20, Type: tea.MouseLeft})
	if !m2.(*AppModel).mouseSel.empty() {
		t.Fatal("Windows Type=MouseLeft click must clear highlight")
	}
}

func TestMouseClick_ClearsSelectionThenPulsesTracking(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.mouseSel = mouseSelect{armed: true, x0: 1, y0: 1, x1: 8, y1: 3}
	_, cmd := m.Update(clickLeft(50, 18))
	if cmd == nil {
		t.Fatal("click-to-clear should pulse mouse tracking so the terminal drops native selection")
	}
}

func TestFormatHistoryChangedAt_PrefersUpdatedAt(t *testing.T) {
	updated := time.Date(2026, 8, 13, 10, 15, 0, 0, time.UTC).Format(time.RFC3339)
	started := time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC).Format(time.RFC3339)
	got := formatHistoryChangedAt(client.RunHistoryItem{UpdatedAt: updated, StartedAt: started})
	want := formatQuotaResetAt(updated)
	if got != want || got == "" {
		t.Fatalf("got %q want %q", got, want)
	}
	startedOnly := formatHistoryChangedAt(client.RunHistoryItem{StartedAt: started})
	if startedOnly != formatQuotaResetAt(started) {
		t.Fatalf("started fallback=%q", startedOnly)
	}
}

func TestFormatChatList_ShowsLastChangedTime(t *testing.T) {
	updated := time.Date(2026, 8, 13, 9, 30, 0, 0, time.UTC).Format(time.RFC3339)
	dump := formatChatList([]client.RunHistoryItem{{
		RunID:      "run-time",
		LastPrompt: "fix login",
		Status:     "completed",
		RunKind:    "chat",
		UpdatedAt:  updated,
	}})
	when := formatQuotaResetAt(updated)
	if !strings.Contains(dump, when) || !strings.Contains(dump, "fix login") {
		t.Fatalf("dump missing last-changed column:\n%s", dump)
	}
}

func TestFilterHistorySuggestions_IncludesLastChanged(t *testing.T) {
	updated := time.Date(2026, 8, 13, 11, 0, 0, 0, time.UTC).Format(time.RFC3339)
	items := []client.RunHistoryItem{{
		RunID:      "run-aaa",
		LastPrompt: "fix login bug",
		Status:     "completed",
		RunKind:    "chat",
		UpdatedAt:  updated,
	}}
	got := filterHistorySuggestions("/history ", items)
	if len(got) != 1 {
		t.Fatalf("got=%+v", got)
	}
	when := formatQuotaResetAt(updated)
	if !strings.Contains(got[0].detail, when) {
		t.Fatalf("picker missing last-changed: %q", got[0].detail)
	}
}

func TestView_PadsBlankLineAndRuleBeforeStatus(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.asciiMode = true
	m.sessionLoading = false
	m.addMessage("user", "hello", "")
	m.addMessage("assistant", "world", "")
	view := m.View()
	rule := strings.Repeat("-", 80)
	plain := stripANSI(view)
	idx := strings.Index(plain, "YOLO")
	if idx < 0 {
		t.Fatalf("missing status:\n%s", plain)
	}
	before := plain[:idx]
	if !strings.Contains(before, rule) {
		t.Fatal("expected a rule above the status line")
	}
	if !strings.Contains(plain, "\n"+rule+"\n") {
		t.Fatalf("rule must have padding around it, got:\n%s", plain)
	}
}

func TestRenderMessages_UserQueryHasNoYouPrefix(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width = 40
	m.messages = []ChatMessage{
		{Role: "user", Content: "hello right"},
		{Role: "assistant", Content: "hello left"},
	}
	joined := stripANSI(strings.Join(m.renderMessages(), "\n"))
	if strings.Contains(joined, "You: hello right") {
		t.Fatalf("ask text should not repeat You: prefix:\n%s", joined)
	}
	if !strings.Contains(joined, "hello right") {
		t.Fatalf("missing ask text:\n%s", joined)
	}
}

func TestRenderMessages_AssistantMarkdownBox(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width = 40
	m.asciiMode = true
	m.messages = []ChatMessage{
		{Role: "user", Content: "hello right"},
		{Role: "assistant", Content: "# hello left"},
	}
	joined := stripANSI(strings.Join(m.renderMessages(), "\n"))
	if !strings.Contains(joined, "markdown") {
		t.Fatalf("expected a markdown-labeled box:\n%s", joined)
	}
	if !strings.Contains(joined, "# hello left") {
		t.Fatalf("expected raw markdown:\n%s", joined)
	}
}

func TestRenderMarkdown_ShowsRawSource(t *testing.T) {
	src := "# Title\n\nUse `code` and **bold**.\n\n```go\nfn()\n```\n"
	joined := strings.Join(renderMarkdown(src, 80), "\n")
	if !strings.Contains(joined, "# Title") || !strings.Contains(joined, "**bold**") || !strings.Contains(joined, "```go") {
		t.Fatalf("MVP should show raw markdown:\n%s", joined)
	}
}
