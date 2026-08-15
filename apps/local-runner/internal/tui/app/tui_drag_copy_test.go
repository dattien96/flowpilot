package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/config"
)

func TestDragRelease_CopiesSelectionAndShowsToast(t *testing.T) {
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
	p2, _ := p1.(*AppModel).handleMouse(tea.MouseMsg{
		X: 40, Y: c.panelH, Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft,
	})
	p3, cmd := p2.(*AppModel).handleMouse(tea.MouseMsg{
		X: 40, Y: c.panelH, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft,
	})
	am := p3.(*AppModel)

	if cmd == nil {
		t.Fatal("drag release must issue a copy cmd")
	}
	// Selection stays armed so Ctrl+C still works as a fallback (CA-480 keeps
	// the highlight after release); the next press clears it.
	if am.mouseSel.empty() {
		t.Fatal("selection must stay armed after auto-copy (Ctrl+C fallback)")
	}
	// Driving the copy cmd yields CopiedMsg; the handler toasts "Copied selection."
	msg := cmd()
	if _, ok := msg.(QuitMsg); ok {
		t.Fatal("auto-copy must not emit QuitMsg")
	}
	cm, ok := msg.(CopiedMsg)
	if !ok {
		t.Fatalf("expected CopiedMsg, got %T", msg)
	}
	if cm.Err == "" && cm.Kind != "selection" {
		t.Fatalf("kind=%q want selection", cm.Kind)
	}
	m4, _ := am.Update(msg)
	toasted := m4.(*AppModel)
	if !strings.Contains(toasted.flashToast, "Copied selection") {
		t.Fatalf("expected copy toast, got %q", toasted.flashToast)
	}
}

func TestShiftDragRelease_CopiesSelection(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.asciiMode = true
	m.addMessage("assistant", "shift drag copy line", "")
	_ = m.View()
	c := m.tuiChrome()

	p1, _ := m.handleMouse(tea.MouseMsg{
		X: 2, Y: c.panelH, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, Shift: true,
	})
	p2, _ := p1.(*AppModel).handleMouse(tea.MouseMsg{
		X: 30, Y: c.panelH, Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft, Shift: true,
	})
	p3, cmd := p2.(*AppModel).handleMouse(tea.MouseMsg{
		X: 32, Y: c.panelH, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft, Shift: true,
	})
	am := p3.(*AppModel)

	if cmd == nil {
		t.Fatal("shift-drag release must issue a copy cmd")
	}
	if am.mouseSel.empty() {
		t.Fatal("selection must stay armed after shift auto-copy")
	}
	msg := cmd()
	if cm, ok := msg.(CopiedMsg); ok {
		if cm.Err == "" && cm.Kind != "selection" {
			t.Fatalf("kind=%q want selection", cm.Kind)
		}
	} else if _, ok := msg.(QuitMsg); ok {
		t.Fatal("shift auto-copy must not emit QuitMsg")
	} else {
		t.Fatalf("expected CopiedMsg, got %T", msg)
	}
}

func TestClickRelease_NoAutoCopyWithoutDrag(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.asciiMode = true
	m.addMessage("assistant", "click line", "")
	_ = m.View()
	c := m.tuiChrome()

	p1, _ := m.handleMouse(tea.MouseMsg{
		X: 4, Y: c.panelH, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
	})
	p2, cmd := p1.(*AppModel).handleMouse(tea.MouseMsg{
		X: 4, Y: c.panelH, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft,
	})
	am := p2.(*AppModel)

	if !am.mouseSel.empty() {
		t.Fatal("click release without drag must clear leftover highlight")
	}
	if cmd != nil {
		if msg := cmd(); isQuitOrCopied(msg) {
			t.Fatalf("click release must only pulse mouse mode, got %T", msg)
		}
	}
}

func isQuitOrCopied(msg tea.Msg) bool {
	switch msg.(type) {
	case QuitMsg, CopiedMsg:
		return true
	}
	return false
}