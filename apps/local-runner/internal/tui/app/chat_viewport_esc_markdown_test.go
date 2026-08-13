package app

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestRenderMarkdown_ShowsRawSourceMVP(t *testing.T) {
	src := "# Heading\n\nUse `code` and **bold** and *italic*.\n\n- item one\n\n```\nfn()\n```\n\nSee [docs](https://example.com)."
	joined := stripANSI(strings.Join(renderMarkdown(src, 40), "\n"))
	for _, want := range []string{"Heading", "bold", "item one", "fn()", "docs"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("styled markdown missing %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "# Heading") || strings.Contains(joined, "**bold**") {
		t.Fatalf("raw markers leaked:\n%s", joined)
	}
}

func TestRenderMarkdown_WrapKeepsMarkers(t *testing.T) {
	src := "**" + strings.TrimSpace(strings.Repeat("word ", 20)) + "**"
	joined := stripANSI(strings.Join(renderMarkdown(src, 16), "\n"))
	if strings.Contains(joined, "**") {
		t.Fatalf("wrapped bold still shows markers:\n%s", joined)
	}
	if !strings.Contains(joined, "word") {
		t.Fatalf("expected wrapped text:\n%s", joined)
	}
}

func TestSliceViewport_ScrollsFromBottom(t *testing.T) {
	lines := []string{"a", "b", "c", "d", "e"}
	got := sliceViewport(lines, 3, 0)
	if strings.Join(got, "") != "cde" {
		t.Fatalf("follow live: %v", got)
	}
	got = sliceViewport(lines, 3, 2)
	if strings.Join(got, "") != "abc" {
		t.Fatalf("scrolled up: %v", got)
	}
	got = sliceViewport(lines, 3, 99)
	if strings.Join(got, "") != "abc" {
		t.Fatalf("clamped: %v", got)
	}
}

func TestView_RespectsViewportOffset(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 16
	m.asciiMode = true
	m.sessionLoading = false
	m.sessionPanel.Collapsed = true
	for i := 0; i < 40; i++ {
		m.addMessage("assistant", fmt.Sprintf("line-%02d unique", i), "")
	}
	bottom := m.View()
	if !strings.Contains(bottom, "line-39") {
		t.Fatalf("live view should pin to the latest line:\n%s", bottom)
	}
	m.viewport.offset = 1000
	top := m.View()
	if !strings.Contains(top, "line-00") {
		t.Fatalf("scrolled view should show the oldest line:\n%s", top)
	}
	if strings.Contains(top, "line-39") {
		t.Fatal("scrolled view must not stay pinned to the bottom")
	}
}

func TestHandleKey_PageUpScrollsTranscript(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 16
	m.sessionLoading = false
	for i := 0; i < 40; i++ {
		m.addMessage("assistant", fmt.Sprintf("row-%02d", i), "")
	}
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyPgUp})
	am := m2.(*AppModel)
	if am.viewport.offset <= 0 {
		t.Fatal("PgUp should scroll the transcript up")
	}
}

func TestHandleMouse_WheelScrollsTranscript(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 16
	m.sessionLoading = false
	m.handleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelUp})
	if m.viewport.offset != 3 {
		t.Fatalf("wheel up offset=%d", m.viewport.offset)
	}
	m.handleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelDown})
	if m.viewport.offset != 0 {
		t.Fatalf("wheel down offset=%d", m.viewport.offset)
	}
}

func TestEscape_ClearsPromptWithoutQuit(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m.inputValue = "draft question"
	m2, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEscape})
	am := m2.(*AppModel)
	if am.quitting {
		t.Fatal("Esc must not kill the TUI")
	}
	if am.inputValue != "" {
		t.Fatalf("Esc should clear the prompt, got %q", am.inputValue)
	}
	if cmd != nil {
		if _, ok := cmd().(QuitMsg); ok {
			t.Fatal("Esc must not emit QuitMsg")
		}
	}
}

func TestEscape_EmptyPromptDoesNotQuit(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m.inputValue = ""
	m2, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEscape})
	am := m2.(*AppModel)
	if am.quitting {
		t.Fatal("Esc with an empty prompt must not kill the TUI")
	}
	if cmd != nil {
		if _, ok := cmd().(QuitMsg); ok {
			t.Fatal("Esc must not emit QuitMsg")
		}
	}
}

func TestModifiedEnterInsertsNewline(t *testing.T) {
	prev := enterModifierHeld
	enterModifierHeld = func() (bool, bool) { return true, false }
	t.Cleanup(func() { enterModifierHeld = prev })

	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m.inputValue = "hello"
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if got := m2.(*AppModel).inputValue; got != "hello\n" {
		t.Fatalf("ctrl/shift+enter should insert a newline, got %q", got)
	}
}

func TestPlainEnterStillSendsWhenModifiersOff(t *testing.T) {
	prev := enterModifierHeld
	enterModifierHeld = defaultEnterModifiers
	t.Cleanup(func() { enterModifierHeld = prev })

	m := New(config.ChatConfig{Provider: "codex", ProjectPath: t.TempDir()}, "http://127.0.0.1:9")
	m.sessionLoading = false
	m.sessionDefaultsLoaded = true
	m.provider = "codex"
	m.inputValue = "hello"
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	am := m2.(*AppModel)
	if am.inputValue != "" {
		t.Fatalf("plain Enter should send, input still %q", am.inputValue)
	}
	found := false
	for _, msg := range am.messages {
		if msg.Role == "user" && msg.Content == "hello" {
			found = true
		}
	}
	if !found {
		t.Fatal("plain Enter should send the draft")
	}
}

func TestPromptNewlineKey_LiteralNewlineRune(t *testing.T) {
	if !isPromptNewlineKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'\n'}}) {
		t.Fatal("a lone newline rune should insert a newline")
	}
}

func TestRenderMarkdown_DesktopRawBlocks(t *testing.T) {
	src := "## Section\n\nA **bold** paragraph.\n\n1. first\n2. second\n\n| Col | Val |\n| --- | --- |\n| a | b |\n\n---\n\n> quoted\n"
	joined := stripANSI(strings.Join(renderMarkdown(src, 60), "\n"))
	if strings.Contains(joined, "## Section") || strings.Contains(joined, "**bold**") {
		t.Fatalf("raw markdown leaked:\n%s", joined)
	}
	if !strings.Contains(joined, "Section") || !strings.Contains(joined, "first") || !strings.Contains(joined, "quoted") {
		t.Fatalf("missing styled text:\n%s", joined)
	}
	if !strings.Contains(joined, "Col") || !strings.Contains(joined, "a") {
		t.Fatalf("missing table cells:\n%s", joined)
	}
}

func TestRenderMessages_AssistantShowsRawMarkdownBox(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.width = 80
	m.addMessage("assistant", "# Title\n\nUse **bold** and `code`.\n\n```\nfn()\n```", "")
	joined := stripANSI(strings.Join(m.renderMessages(), "\n"))
	if strings.Contains(joined, "markdown") {
		t.Fatalf("assistant reply must not use a markdown-labeled box:\n%s", joined)
	}
	if !strings.Contains(joined, "Title") || !strings.Contains(joined, "bold") || !strings.Contains(joined, "fn()") {
		t.Fatalf("expected styled markdown:\n%s", joined)
	}
	if strings.Contains(joined, "**bold**") || strings.Contains(joined, "# Title") {
		t.Fatalf("raw markers leaked:\n%s", joined)
	}
}

func TestRenderMarkdown_CodeFenceStaysRaw(t *testing.T) {
	src := "Intro\n\n```go\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n```\n"
	joined := stripANSI(strings.Join(renderMarkdown(src, 40), "\n"))
	if strings.Contains(joined, "```") {
		t.Fatalf("fence ticks leaked:\n%s", joined)
	}
	if !strings.Contains(joined, "func main()") {
		t.Fatalf("expected fence body:\n%s", joined)
	}
}

func TestView_ChatSeparatedFromStatusByRule(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.asciiMode = true
	m.sessionLoading = false
	m.addMessage("user", "hello", "")
	m.addMessage("assistant", "world", "")
	view := m.View()
	if !strings.Contains(view, strings.Repeat("-", 80)) {
		t.Fatal("expected a horizontal rule between chat and status")
	}
	plain := stripANSI(view)
	if !strings.Contains(plain, "|") {
		t.Fatal("expected stroke boxes around chat bubbles")
	}
}

func TestStatusLine_PutsContextUsageOnOwnRow(t *testing.T) {
	win := int64(128000)
	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.6"}, "http://127.0.0.1:4317")
	m.width = 72
	m.asciiMode = true
	m.reasoningEffort = "high"
	m.accountLabel = "trashname899@gmail.com"
	m.lastTokens = &client.TokenUsageSnapshot{
		ModelContextWindow: &win,
		Total:              &client.TokenUsageBreakdown{TotalTokens: 12000, InputTokens: 9000},
		Last:               &client.TokenUsageBreakdown{TotalTokens: 800, InputTokens: 500, OutputTokens: 300},
	}
	got := m.renderStatusLine()
	lines := strings.Split(got, "\n")
	if len(lines) < 3 {
		t.Fatalf("want usage on its own row, got %d lines:\n%s", len(lines), got)
	}
	usage := lines[len(lines)-1]
	if !strings.Contains(usage, "ctx") || !strings.Contains(usage, "128.0k") || !strings.Contains(usage, "left") {
		t.Fatalf("usage row missing window context: %q", usage)
	}
	if !strings.Contains(usage, "in:500") || !strings.Contains(usage, "out:300") || !strings.Contains(usage, "last 800") {
		t.Fatalf("usage row missing token counts: %q", usage)
	}
	if strings.Contains(lines[0], "ctx ") {
		t.Fatal("context must not sit on the truncated chip row")
	}
}

func TestFormatContextLimits_FallbackWindowWithoutUsage(t *testing.T) {
	got := formatContextLimits(nil, 128000)
	if got != "ctx 128.0k window" {
		t.Fatalf("got %q", got)
	}
}
