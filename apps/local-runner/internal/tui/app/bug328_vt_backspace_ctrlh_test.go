package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/config"
)

func TestBug328_CtrlHRemapsToBackspace(t *testing.T) {
	got := remapVTControlKeys(tea.KeyMsg{Type: tea.KeyCtrlH})
	if got.Type != tea.KeyBackspace {
		t.Fatalf("Windows VT Backspace (ctrl+h) must remap to KeyBackspace, got %v", got.Type)
	}
}

func TestBug328_CtrlHDeletesComposerChar(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.authPhase = AuthNone
	m.sessionDefaultsLoaded = true
	m.inputValue = "ab"
	m.setInputCaret(2)

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlH})
	am := got.(*AppModel)
	if am.inputValue != "a" {
		t.Fatalf("ctrl+h must delete like Backspace, got %q", am.inputValue)
	}
}

func TestBug328_TuiMsgFilterRemapsCtrlH(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	out := tuiMsgFilter(m, tea.KeyMsg{Type: tea.KeyCtrlH})
	km, ok := out.(tea.KeyMsg)
	if !ok {
		t.Fatalf("filter must keep KeyMsg, got %T", out)
	}
	if km.Type != tea.KeyBackspace {
		t.Fatalf("filter must remap ctrl+h to backspace, got %v", km.Type)
	}
}
