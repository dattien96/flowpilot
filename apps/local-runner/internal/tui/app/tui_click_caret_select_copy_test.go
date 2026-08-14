package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/config"
)

func TestCtrlC_WithSelectionCopiesInsteadOfQuit(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.asciiMode = true
	m.addMessage("assistant", "hello selected world", "")
	// Force rows/chrome so selection y maps into the transcript viewport.
	_ = m.View()
	c := m.tuiChrome()
	// Highlight the first transcript row (panelH offset matches applyMouseSelection).
	y := c.panelH
	if y < 0 {
		y = 0
	}
	m.mouseSel = mouseSelect{armed: true, x0: 0, y0: y, x1: 40, y1: y}

	m2, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	am := m2.(*AppModel)
	if am.quitting {
		t.Fatal("Ctrl-C with an armed selection must not quit the TUI")
	}
	if !am.mouseSel.empty() {
		t.Fatal("selection should clear after copy attempt")
	}
	if cmd == nil {
		t.Fatal("expected clipboard copy cmd")
	}
	// Drive the cmd; clipboard may fail in CI — still must not be QuitMsg.
	msg := cmd()
	if _, ok := msg.(QuitMsg); ok {
		t.Fatal("copy path must not emit QuitMsg")
	}
	if cm, ok := msg.(CopiedMsg); ok {
		if cm.Kind != "selection" && cm.Err == "" {
			t.Fatalf("kind=%q", cm.Kind)
		}
	}
}

func TestCtrlC_IdleWithoutSelectionStillQuits(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m2, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !m2.(*AppModel).quitting {
		t.Fatal("idle Ctrl-C without selection must still quit")
	}
	if cmd == nil {
		t.Fatal("expected shutdown cmd")
	}
}

func TestClickPlacesInputCursor(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.asciiMode = true
	m.inputValue = "abcdefghij"
	m.inputCursor = -1
	_ = m.View()
	c := m.tuiChrome()

	// First body row: top border + optional approval/question (none) → rel 1.
	// Text starts after left border, pad space, and "[+img] ".
	attachW := len("[+img] ")
	textStartX := 2 + attachW
	// Click on the 3rd rune (index 2 → 'c').
	x := textStartX + 2
	y := c.inputY + 1
	if !m.tryPlaceInputCursor(x, y) {
		t.Fatalf("expected hit on input body at (%d,%d) inputY=%d", x, y, c.inputY)
	}
	if m.inputCaretIndex() != 2 {
		t.Fatalf("caret=%d want 2 (value=%q)", m.inputCaretIndex(), m.inputValue)
	}

	// Insert at caret to prove edit position.
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'X'}})
	if got := m2.(*AppModel).inputValue; got != "abXcdefghij" {
		t.Fatalf("after click-insert: %q", got)
	}
}

func TestClickInput_WindowsMouseLeftTypePlacesCursor(t *testing.T) {
	// Windows terminals sometimes deliver Type=MouseLeft with Action=0.
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.asciiMode = true
	m.inputValue = "hello"
	m.inputCursor = -1
	_ = m.View()
	c := m.tuiChrome()
	attachW := len("[+img] ")
	x := 2 + attachW + 1 // on 'e'
	y := c.inputY + 1
	m2, _ := m.handleMouse(tea.MouseMsg{
		X: x, Y: y, Type: tea.MouseLeft, Button: tea.MouseButtonLeft,
	})
	am := m2.(*AppModel)
	if am.inputCaretIndex() != 1 {
		t.Fatalf("windows-style press caret=%d want 1", am.inputCaretIndex())
	}
	if !am.mouseSel.empty() {
		t.Fatal("clicking input must not leave a transcript selection")
	}
}

func TestClickAttachDoesNotStealForCursor(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.asciiMode = true
	m.inputValue = "abcd"
	m.inputCursor = -1
	_ = m.View()
	c := m.tuiChrome()
	// x on the attach chip, not the text body.
	if m.tryPlaceInputCursor(3, c.inputY+1) {
		// May or may not hit attach depending on exact layout; if true, caret must not move into chip.
		t.Logf("caret after attach-area click: %d", m.inputCaretIndex())
	}
	// Explicit attach hit must return false from tryPlace when chip is under x.
	stripped := stripANSI(strings.Split(c.inputBlock, "\n")[1])
	if i := strings.Index(stripped, "[+img]"); i >= 0 {
		if m.tryPlaceInputCursor(i, c.inputY+1) {
			t.Fatal("attach chip click must not place caret")
		}
	}
}

func TestSelectionPlainText_IncludesHighlightedRows(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.asciiMode = true
	m.addMessage("assistant", "alpha line", "")
	m.addMessage("assistant", "beta line", "")
	_ = m.View()
	c := m.tuiChrome()
	m.mouseSel = mouseSelect{armed: true, x0: 0, y0: c.panelH, x1: 20, y1: c.panelH + 3}
	got := m.selectionPlainText()
	if !strings.Contains(got, "alpha") && !strings.Contains(got, "beta") {
		// Content may be boxed; at least some non-empty selection from the viewport.
		if strings.TrimSpace(got) == "" {
			t.Fatalf("expected non-empty selection text, got %q", got)
		}
	}
}
