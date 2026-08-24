package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/config"
)

func newInputModel() *AppModel {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:9")
	m.sessionLoading = false
	m.width, m.height = 100, 30
	m.asciiMode = true
	return m
}

// CA-624: Alt+V pasted long/multi-line block collapses to [Pasted N chars].
// One Backspace must delete the whole token, not just one rune.
func TestCA624_BackspaceDeletesPastedToken(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := newInputModel()
			m.provider = pk
			full := strings.Join([]string{"L1", "L2", "L3", "L4", "L5", "L6", "L7"}, "\n")
			m2, _ := m.Update(ClipboardPasteMsg{Text: full})
			am := m2.(*AppModel)
			if len(am.pasteSegments) == 0 {
				t.Fatal("expected pasteSegments after long paste")
			}
			token := am.pasteSegments[0].token
			if !strings.Contains(am.inputValue, token) {
				t.Fatalf("input must contain token %q got %q", token, am.inputValue)
			}
			// Caret at sticky-end, one Backspace should clear token
			m3, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
			am3 := m3.(*AppModel)
			if am3.inputValue != "" {
				t.Fatalf("one Backspace must delete whole token, got %q (segments %v)", am3.inputValue, am3.pasteSegments)
			}
			if len(am3.pasteSegments) != 0 {
				t.Fatalf("segments must be empty after delete, got %v", am3.pasteSegments)
			}
		})
	}
}

func TestCA624_EscClearsPastedToken(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := newInputModel()
			m.provider = pk
			full := "This is a long collapsed paste that exceeds threshold for token"
			m2, _ := m.Update(ClipboardPasteMsg{Text: full})
			am := m2.(*AppModel)
			if am.inputValue == "" || len(am.pasteSegments) == 0 {
				t.Fatal("expected token")
			}
			// Esc should clear input and segments
			m3, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyEscape})
			am3 := m3.(*AppModel)
			if am3.inputValue != "" || len(am3.pasteSegments) != 0 {
				t.Fatalf("Esc must clear token: input=%q segs=%v", am3.inputValue, am3.pasteSegments)
			}
			// Also KeyEsc alias
			m4, _ := m2.(*AppModel).handleKey(tea.KeyMsg{Type: tea.KeyEsc})
			if m4.(*AppModel).inputValue != "" {
				t.Fatalf("KeyEsc must also clear, got %q", m4.(*AppModel).inputValue)
			}
		})
	}
}

func TestCA624_TokenWithTrailingText_BackspaceOnlyRemovesToken(t *testing.T) {
	m := newInputModel()
	tokenText := strings.Repeat("A", 20) // collapses
	m2, _ := m.Update(ClipboardPasteMsg{Text: tokenText})
	am := m2.(*AppModel)
	token := am.pasteSegments[0].token
	// Append trailing plain text after token
	am.inputValue = token + " hello"
	am.inputCursor = len([]rune(token)) // caret right after token, before trailing
	am.pasteSegments = []pasteSegment{{token: token, text: tokenText}}
	m3, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	am3 := m3.(*AppModel)
	if am3.inputValue != " hello" {
		t.Fatalf("Backspace after token should leave trailing text, got %q", am3.inputValue)
	}
	if len(am3.pasteSegments) != 0 {
		t.Fatalf("segment should be removed, got %v", am3.pasteSegments)
	}
}

func TestCA624_TypedShort_NotAffected(t *testing.T) {
	m := newInputModel()
	m.inputValue = "abc"
	m.inputCursor = -1
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	if got := m2.(*AppModel).inputValue; got != "ab" {
		t.Fatalf("short typed backspace must remove one char, got %q", got)
	}
}

func TestCA624_DeleteForwardRemovesToken(t *testing.T) {
	m := newInputModel()
	tokenText := strings.Repeat("B", 20)
	m2, _ := m.Update(ClipboardPasteMsg{Text: tokenText})
	am := m2.(*AppModel)
	token := am.pasteSegments[0].token
	// Place caret before token
	am.inputValue = token + "x"
	am.inputCursor = 0
	// Delete forward should remove token
	m3, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyDelete})
	am3 := m3.(*AppModel)
	if strings.Contains(am3.inputValue, token) {
		t.Fatalf("Delete forward should remove token, got %q", am3.inputValue)
	}
	if len(am3.pasteSegments) != 0 {
		t.Fatalf("segments must empty after forward delete, got %v", am3.pasteSegments)
	}
}

func TestCA624_EscWithSelectionAndInputClearsBoth(t *testing.T) {
	m := newInputModel()
	m.inputValue = "[Pasted 20 chars]"
	m.pasteSegments = []pasteSegment{{token: "[Pasted 20 chars]", text: strings.Repeat("Z", 20)}}
	m.mouseSel = mouseSelect{armed: true, x0: 0, y0: 10, x1: 5, y1: 10}
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEscape})
	am := m2.(*AppModel)
	// First Esc clears selection only (legacy contract: TestEscape_ClearsSelectionBeforePrompt)
	if !am.mouseSel.empty() {
		t.Fatal("first Esc must clear selection")
	}
	if am.inputValue == "" {
		t.Fatalf("first Esc should not also wipe prompt, got empty")
	}
	// Second Esc clears the pasted token
	m3, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyEscape})
	am3 := m3.(*AppModel)
	if am3.inputValue != "" || len(am3.pasteSegments) != 0 {
		t.Fatalf("second Esc must clear token: input=%q segs=%v", am3.inputValue, am3.pasteSegments)
	}
}
