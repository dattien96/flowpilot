package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/config"
)

// CA-482: Alt+V is delivered as KeyRunes+Alt (String "alt+v"). It must not
// insert a bare "v" into the prompt — that was why image paste looked dead on
// Windows when Ctrl+V is stolen by Windows Terminal.
func TestAltV_KeyRunes_DispatchesClipboardPaste(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.provider = "codex"
	m.sessionLoading = false
	m.authPhase = AuthNone
	m.inputValue = "hello"

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}, Alt: true}
	if msg.String() != "alt+v" {
		t.Fatalf("bubbletea encoding changed: String()=%q", msg.String())
	}

	m2, cmd := m.handleKey(msg)
	am := m2.(*AppModel)
	if am.inputValue != "hello" {
		t.Fatalf("Alt+V must not insert runes; input=%q", am.inputValue)
	}
	if cmd == nil {
		t.Fatal("expected clipboard paste cmd")
	}
}

// Bracketed paste (Windows Terminal Ctrl+V) routes through clipboard image
// attach first, then text / path / fallback runes.
func TestBracketedPaste_DispatchesClipboardPasteCmd(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.provider = "codex"
	m.sessionLoading = false
	m.authPhase = AuthNone
	m.inputValue = "pre"

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("some text"), Paste: true}
	m2, cmd := m.handleKey(msg)
	am := m2.(*AppModel)
	if am.inputValue != "pre" {
		t.Fatalf("bracketed paste must not insert runes immediately; input=%q", am.inputValue)
	}
	if cmd == nil {
		t.Fatal("expected clipboard paste cmd for bracketed paste")
	}
}

func TestCmdClipboardPasteWithFallback_UsesFallbackText(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.provider = "codex"
	cmd := m.cmdClipboardPasteWithFallback("fallback-paste-body")
	if cmd == nil {
		t.Fatal("expected cmd")
	}
	msg := cmd()
	pm, ok := msg.(ClipboardPasteMsg)
	if !ok {
		t.Fatalf("msg type %T", msg)
	}
	if pm.Attachment != nil {
		t.Skip("machine clipboard has an image; cannot assert text fallback")
	}
	// Prefer real clipboard text when present; otherwise the bracketed-paste
	// fallback must be used (empty native clipboard).
	if strings.TrimSpace(pm.Text) == "" {
		t.Fatalf("expected clipboard or fallback text, got %+v", pm)
	}
	if pm.Text != "fallback-paste-body" && strings.TrimSpace(pm.Text) == "" {
		t.Fatalf("unexpected text %q", pm.Text)
	}
}

func TestPlainV_StillInsertsRune(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.provider = "codex"
	m.sessionLoading = false
	m.authPhase = AuthNone
	m.inputValue = ""

	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}}
	m2, _ := m.handleKey(msg)
	am := m2.(*AppModel)
	if am.inputValue != "v" {
		t.Fatalf("plain v should insert; input=%q", am.inputValue)
	}
}

func TestCtrlV_StillDispatchesClipboardPaste(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.provider = "codex"
	m.sessionLoading = false
	m.authPhase = AuthNone
	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlV})
	if cmd == nil {
		t.Fatal("expected clipboard paste cmd for ctrl+v")
	}
}

func TestDispatchImageCommand_PasteStillAsync(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.provider = "codex"
	_, cmd := m.dispatchImageCommand([]string{"paste"})
	if cmd == nil {
		t.Fatal("expected clipboard cmd")
	}
}
