package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/config"
)

func TestPlainDrag_SelectsTranscriptWithoutShift(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m2, _ := m.handleMouse(tea.MouseMsg{
		X: 4, Y: 6, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
	})
	am := m2.(*AppModel)
	if !am.mouseSel.empty() {
		t.Fatal("press alone must not arm a highlight (that is still a click)")
	}
	m3, _ := am.handleMouse(tea.MouseMsg{
		X: 12, Y: 9, Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft,
	})
	am = m3.(*AppModel)
	if am.mouseSel.empty() || am.mouseSel.x1 != 12 || am.mouseSel.y1 != 9 {
		t.Fatalf("plain drag should select: %+v", am.mouseSel)
	}
	m4, _ := am.handleMouse(tea.MouseMsg{
		X: 14, Y: 9, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft,
	})
	am = m4.(*AppModel)
	if am.mouseSel.empty() || am.mouseSel.x1 != 12 {
		t.Fatalf("release after drag must keep selection: %+v", am.mouseSel)
	}
}

func TestInputArrows_InsertInMiddle(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m.inputValue = "abcd"
	m.inputCursor = -1
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyLeft})
	am := m2.(*AppModel)
	m3, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyLeft})
	am = m3.(*AppModel)
	m4, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	am = m4.(*AppModel)
	if am.inputValue != "abXcd" {
		t.Fatalf("input=%q want abXcd", am.inputValue)
	}
}

func TestInputArrows_BackspaceDeletesAtCursor(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m.inputValue = "abcd"
	m.inputCursor = 2
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	if got := m2.(*AppModel).inputValue; got != "acd" {
		t.Fatalf("input=%q want acd", got)
	}
}

func TestInputCaretDefaultsToEndWhenUnset(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m.inputValue = "hello"
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeySpace})
	if got := m2.(*AppModel).inputValue; got != "hello " {
		t.Fatalf("unset cursor must still append: %q", got)
	}
}

func TestInputHomeEnd(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m.inputValue = "abcd"
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyHome})
	am := m2.(*AppModel)
	m3, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Z'}})
	if got := m3.(*AppModel).inputValue; got != "Zabcd" {
		t.Fatalf("home insert=%q", got)
	}
}

func TestInputRender_PlacesCaretInMiddle(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.asciiMode = true
	m.inputValue = "abcd"
	m.inputCursor = 2
	m.cursorOn = true
	got := stripANSI(m.renderInputLine())
	if !strings.Contains(got, "ab_cd") && !strings.Contains(got, "ab▌cd") {
		t.Fatalf("caret should sit between ab and cd:\n%s", got)
	}
}
