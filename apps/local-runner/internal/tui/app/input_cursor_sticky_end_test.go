package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/config"
)

// CA-486: backspace at sticky-end must not leave caret before the last rune.
func TestBackspaceAtEnd_KeepsStickyEnd(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:9")
	m.sessionLoading = false
	m.inputValue = "ab"
	m.inputCursor = -1
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	am := m2.(*AppModel)
	if am.inputValue != "a" {
		t.Fatalf("value=%q want a", am.inputValue)
	}
	if am.inputCursor != -1 {
		t.Fatalf("cursor=%d want sticky-end -1 (was %d)", am.inputCursor, am.inputCursor)
	}
	// Next type must append, not prepend.
	m3, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	if got := m3.(*AppModel).inputValue; got != "ac" {
		t.Fatalf("after type c: %q want ac", got)
	}
}

// Clear all (Esc) then type abc must not reverse to bca.
func TestEscClear_ThenTypeABC(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:9")
	m.sessionLoading = false
	m.inputValue = "old"
	m.inputCursor = 1 // mid-string before clear
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEscape})
	am := m2.(*AppModel)
	if am.inputValue != "" || am.inputCursor != -1 {
		t.Fatalf("after esc: value=%q cursor=%d", am.inputValue, am.inputCursor)
	}
	for _, r := range []rune{'a', 'b', 'c'} {
		m2, _ = am.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		am = m2.(*AppModel)
	}
	if am.inputValue != "abc" {
		t.Fatalf("got %q want abc", am.inputValue)
	}
}

// Backspace entire line from sticky-end then retype.
func TestBackspaceAll_ThenTypeABC(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:9")
	m.sessionLoading = false
	m.inputValue = "xy"
	m.inputCursor = -1
	for i := 0; i < 3; i++ {
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
		m = m2.(*AppModel)
	}
	if m.inputValue != "" {
		t.Fatalf("expected empty, got %q cursor=%d", m.inputValue, m.inputCursor)
	}
	for _, r := range []rune{'a', 'b', 'c'} {
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	if m.inputValue != "abc" {
		t.Fatalf("got %q want abc (cursor=%d)", m.inputValue, m.inputCursor)
	}
}

func TestInsertAtStickyEnd_AfterPartialBackspace(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:9")
	m.sessionLoading = false
	m.inputValue = "hello"
	m.inputCursor = -1
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	am := m2.(*AppModel)
	if am.inputValue != "hell" || am.inputCursor != -1 {
		t.Fatalf("value=%q cursor=%d", am.inputValue, am.inputCursor)
	}
	m3, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if got := m3.(*AppModel).inputValue; got != "hello" {
		t.Fatalf("got %q want hello", got)
	}
}
