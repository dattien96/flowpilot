package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

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

// BUG-345 follow-up (operator: copy works but no live bôi-đen while dragging —
// "kéo tới đâu" không thấy): ?1002h (drag-only motion) must be part of the
// wheel/drag ANSI so the selection paints as the cursor moves; hover ?1003h
// must stay OFF (BUG-328 wedge class).
func TestWheelMouseANSI_DragMotionOnHoverOff(t *testing.T) {
	if !strings.Contains(wheelMouseOnANSI, "\x1b[?1000h") {
		t.Fatalf("wheel ANSI must keep button/wheel tracking 1000h, got %q", wheelMouseOnANSI)
	}
	if !strings.Contains(wheelMouseOnANSI, "\x1b[?1002h") {
		t.Fatalf("wheel ANSI must enable drag motion 1002h for live highlight, got %q", wheelMouseOnANSI)
	}
	if !strings.Contains(wheelMouseOnANSI, "\x1b[?1003l") {
		t.Fatalf("wheel ANSI must disable hover 1003, got %q", wheelMouseOnANSI)
	}
	if strings.Contains(wheelMouseOnANSI, "\x1b[?1003h") {
		t.Fatalf("wheel ANSI must never enable hover 1003h (BUG-328), got %q", wheelMouseOnANSI)
	}
}

// With ?1002h, motion events arrive while the button is held: the selection
// must update live (before release) and the View paints reverse-video bôi-đen
// on the spanned row.
func TestWheelOnly_DragMotionPaintsLiveHighlight(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.asciiMode = true
	m.addMessage("assistant", "drag live highlight row", "")
	_ = m.View()
	c := m.tuiChrome()

	p1, _ := m.handleMouse(tea.MouseMsg{
		X: 4, Y: c.panelH, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
	})
	p2, _ := p1.(*AppModel).handleMouse(tea.MouseMsg{
		X: 30, Y: c.panelH, Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft,
	})
	am := p2.(*AppModel)
	if am.mouseSel.empty() || am.mouseSel.x1 != 30 || am.mouseSel.y1 != c.panelH {
		t.Fatalf("motion must update selection live, got %+v", am.mouseSel)
	}
	painted := applyMouseSelection([]string{"drag live highlight row"}, am.mouseSel, c.panelH)
	if !strings.Contains(painted[0], "\x1b[7m") {
		t.Fatalf("live drag must paint reverse-video highlight, got %q", painted[0])
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