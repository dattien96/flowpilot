package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/config"
)

// BUG-345: wheel-only mouse (?1000h) delivers press and release but no motion
// events. A press→release over different cells must still be a drag-select
// that auto-copies — previously only MouseActionMotion armed the selection, so
// the copy affordance silently died with the scroll fix.
func TestWheelOnly_PressReleaseDifferentCellCopiesWithoutMotion(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.asciiMode = true
	m.addMessage("assistant", "drag copy target line", "")
	_ = m.View()
	c := m.tuiChrome()

	p1, _ := m.handleMouse(tea.MouseMsg{
		X: 4, Y: c.panelH, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
	})
	am := p1.(*AppModel)
	if am.mouseDrag.down == false {
		t.Fatal("press over transcript must arm a drag")
	}
	// No motion event in between — wheel-only 1000h mode.
	p2, cmd := am.handleMouse(tea.MouseMsg{
		X: 40, Y: c.panelH, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft,
	})
	am2 := p2.(*AppModel)
	if cmd == nil {
		t.Fatal("press→release over different cells must issue a copy cmd without motion")
	}
	if am2.mouseSel.empty() {
		t.Fatal("selection must stay armed after auto-copy (Ctrl+C fallback)")
	}
	if am2.mouseSel.x1 != 40 || am2.mouseSel.y1 != c.panelH {
		t.Fatalf("selection must span press→release cells, got %+v", am2.mouseSel)
	}
	msg := cmd()
	cm, ok := msg.(CopiedMsg)
	if !ok {
		t.Fatalf("expected CopiedMsg, got %T", msg)
	}
	if cm.Err == "" && cm.Kind != "selection" {
		t.Fatalf("kind=%q want selection", cm.Kind)
	}
}

func TestWheelOnly_SameCellClickDoesNotCopy(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.asciiMode = true
	m.addMessage("assistant", "click target line", "")
	_ = m.View()
	c := m.tuiChrome()

	p1, _ := m.handleMouse(tea.MouseMsg{
		X: 5, Y: c.panelH, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
	})
	p2, cmd := p1.(*AppModel).handleMouse(tea.MouseMsg{
		X: 5, Y: c.panelH, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft,
	})
	am := p2.(*AppModel)
	if !am.mouseSel.empty() {
		t.Fatalf("zero-area click must not arm a selection, got %+v", am.mouseSel)
	}
	if cmd != nil {
		if msg := cmd(); msg != nil {
			if _, isCopy := msg.(CopiedMsg); isCopy {
				t.Fatal("zero-area click must not copy")
			}
		}
	}
}

// Shift-click on a line must keep copying the whole line even without motion.
func TestWheelOnly_ShiftReleaseWithoutMotionCopiesLine(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.asciiMode = true
	m.addMessage("assistant", "shift click copy line", "")
	_ = m.View()
	c := m.tuiChrome()

	p1, _ := m.handleMouse(tea.MouseMsg{
		X: 3, Y: c.panelH, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, Shift: true,
	})
	p2, cmd := p1.(*AppModel).handleMouse(tea.MouseMsg{
		X: 3, Y: c.panelH, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft, Shift: true,
	})
	am := p2.(*AppModel)
	if cmd == nil {
		t.Fatal("shift+click release must issue a copy cmd")
	}
	if am.mouseSel.empty() {
		t.Fatal("shift selection must stay armed after auto-copy")
	}
	msg := cmd()
	if _, ok := msg.(CopiedMsg); !ok {
		t.Fatalf("expected CopiedMsg, got %T", msg)
	}
	if !strings.Contains(am.selectionPlainText(), "shift click copy line") {
		t.Fatalf("shift+click must copy the whole clicked line, got %q", am.selectionPlainText())
	}
}