package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestActiveSlashLine_WordBoundary(t *testing.T) {
	line, start, ok := activeSlashLine("/model", 6)
	if !ok || line != "/model" || start != 0 {
		t.Fatalf("start-of-input: line=%q start=%d ok=%v", line, start, ok)
	}
	line, start, ok = activeSlashLine("draft /mo", 9)
	if !ok || line != "/mo" || start != 6 {
		t.Fatalf("after-space: line=%q start=%d ok=%v", line, start, ok)
	}
	line, start, ok = activeSlashLine("  /image ", 9)
	if !ok || line != "/image " || start != 2 {
		t.Fatalf("leading-space: line=%q start=%d ok=%v", line, start, ok)
	}
	if _, _, ok := activeSlashLine("hello/", 6); ok {
		t.Fatal("glued slash after letters is not a command")
	}
	if _, _, ok := activeSlashLine("https://x", 9); ok {
		t.Fatal("URL slash is not a command")
	}
	if _, _, ok := activeSlashLine("hello", 5); ok {
		t.Fatal("no slash")
	}
	// Caret before the slash → no active fragment (fallback is whole input).
	if _, _, ok := activeSlashLine("draft /mo", 3); ok {
		t.Fatal("caret in draft prefix must not activate slash")
	}
	line, start, ok = activeSlashLine("line1\n/help", 11)
	if !ok || line != "/help" || start != 6 {
		t.Fatalf("after-newline: line=%q start=%d ok=%v", line, start, ok)
	}
}

func TestSlashAfterDraft_TypesOpensCommandList(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m.inputValue = "leftover draft"
	m.inputCursor = -1
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	am := m2.(*AppModel)
	if am.inputValue != "leftover draft /" {
		t.Fatalf("input=%q want leftover draft /", am.inputValue)
	}
	items := am.collectSuggestions()
	if len(items) == 0 {
		t.Fatal("expected command list after typing / on leftover draft")
	}
	found := false
	for _, it := range items {
		if it.value == "/image" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing /image in list: %+v", items)
	}
}

func TestSlashAfterDraft_TabReplacesWithCommand(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m.inputValue = "draft /pro"
	m.inputCursor = -1
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	if got := m2.(*AppModel).inputValue; got != "/provider " {
		t.Fatalf("inputValue=%q want /provider ", got)
	}
}

func TestSlashAfterDraft_EnterRunsHighlightedCommand(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m.inputValue = "draft /"
	m.inputCursor = -1
	items := m.collectSuggestions()
	yoloIdx := -1
	for i, it := range items {
		if it.value == "/yolo" {
			yoloIdx = i
			break
		}
	}
	if yoloIdx < 0 {
		t.Fatal("/yolo not in mid-draft suggestions")
	}
	m.suggIdx = yoloIdx
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	am := m2.(*AppModel)
	if am.inputValue != "" {
		t.Fatalf("input should clear after Enter, got %q", am.inputValue)
	}
	if !strings.Contains(am.View(), "YOLO mode:") {
		t.Fatalf("Enter should run /yolo from mid-draft:\n%s", am.View())
	}
}

func TestSlashAfterDraft_ImagePicker(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:9")
	m.sessionLoading = false
	m.inputValue = "notes /image "
	m.inputCursor = -1
	subs := m.collectSuggestions()
	if len(subs) < 4 {
		t.Fatalf("image subs=%d", len(subs))
	}
	m.appendPendingAttachment(samplePendingAtt("z", "shot.png"))
	m.inputValue = "notes /image open "
	m.inputCursor = -1
	opens := m.collectSuggestions()
	if len(opens) != 1 || opens[0].kind != "image-open" {
		t.Fatalf("opens=%+v", opens)
	}
}

func TestSlashAfterDraft_FlowPicker(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m.flowBuiltins = []client.BuiltinFlowOption{
		{FlowRef: "pack/review-loop", Label: "review-loop"},
	}
	m.inputValue = "hello /flow "
	m.inputCursor = -1
	got := m.collectSuggestions()
	if len(got) == 0 || got[0].kind != "flow" {
		t.Fatalf("flow picker: %+v", got)
	}
}

func TestSlashAtStartUnchanged(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m.inputValue = ""
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	am := m2.(*AppModel)
	if am.inputValue != "/" {
		t.Fatalf("bare / became %q", am.inputValue)
	}
	if len(am.collectSuggestions()) == 0 {
		t.Fatal("bare / must still show commands")
	}
}

func TestSlashURLDoesNotOpenList(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m.inputValue = "https://example.com/path"
	m.inputCursor = -1
	if items := m.collectSuggestions(); len(items) != 0 {
		t.Fatalf("URL must not open command list: %+v", items)
	}
}

func TestSlashAfterDraft_WhileLoading(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.sessionLoading = true
	m.inputValue = "draft"
	m.inputCursor = -1
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	am := m2.(*AppModel)
	if am.inputValue != "draft /" {
		t.Fatalf("loading slash: %q", am.inputValue)
	}
	if len(am.collectSuggestions()) == 0 {
		t.Fatal("command list should open while session is loading")
	}
}

func TestPasswordSlashNoAutospacing(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.authPhase = AuthPassword
	m.inputValue = "ab"
	m.inputCursor = -1
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if got := m2.(*AppModel).inputValue; got != "ab/" {
		t.Fatalf("password / rewritten: %q", got)
	}
}

func TestSlashSuggestLine_FallbackWhenCaretBeforeSlash(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m.inputValue = "/model"
	m.inputCursor = 0
	// Caret on the '/', no active fragment — fallback to full input.
	if got := m.slashSuggestLine(); got != "/model" {
		t.Fatalf("slashSuggestLine=%q", got)
	}
	items := m.collectSuggestions()
	if len(items) == 0 {
		t.Fatal("Home+caret on /model must still list /model")
	}
}
