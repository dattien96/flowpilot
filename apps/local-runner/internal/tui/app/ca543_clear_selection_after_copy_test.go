package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/config"
)

// CA-543 — a successful drag-selection copy must clear the drag highlight so the
// UI returns to normal (read) mode instead of staying in "Copied" state. Copy
// failures keep the selection so Ctrl+C remains a working fallback, and
// non-selection copy kinds never touch the drag highlight.

// TestCopiedMsg_SelectionSuccessClearsHighlight: a successful selection copy clears
// the armed drag highlight and still shows the "Copied selection." toast.
func TestCopiedMsg_SelectionSuccessClearsHighlight(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.mouseSel = mouseSelect{armed: true, x0: 1, y0: 2, x1: 30, y1: 2}
			m2, _ := m.Update(CopiedMsg{Kind: "selection"})
			am := m2.(*AppModel)
			if !am.mouseSel.empty() {
				t.Fatalf("%s: successful selection copy must clear the highlight", pk)
			}
			if !strings.Contains(am.flashToast, "Copied selection") {
				t.Fatalf("%s: toast=%q", pk, am.flashToast)
			}
		})
	}
}

// TestCopiedMsg_EmptyKindDefaultsSelectionAndClears: a CopiedMsg with empty kind
// means a selection copy and must also clear the highlight.
func TestCopiedMsg_EmptyKindDefaultsSelectionAndClears(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.mouseSel = mouseSelect{armed: true, x0: 0, y0: 0, x1: 10, y1: 0}
			m2, _ := m.Update(CopiedMsg{})
			am := m2.(*AppModel)
			if !am.mouseSel.empty() {
				t.Fatalf("%s: empty-kind copy must be treated as selection and clear highlight", pk)
			}
		})
	}
}

// TestCopiedMsg_SelectionFailureKeepsHighlight: a failed selection copy must keep
// the drag highlight (Ctrl+C fallback) and surface the failure in the chat.
func TestCopiedMsg_SelectionFailureKeepsHighlight(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.mouseSel = mouseSelect{armed: true, x0: 0, y0: 0, x1: 10, y1: 0}
			m2, _ := m.Update(CopiedMsg{Kind: "selection", Err: "clipboard blocked"})
			am := m2.(*AppModel)
			if am.mouseSel.empty() {
				t.Fatalf("%s: failed copy must keep the highlight", pk)
			}
		})
	}
}

// TestCopiedMsg_NonSelectionKindUntouched: copying code/answer text never clears a
// drag highlight.
func TestCopiedMsg_NonSelectionKindUntouched(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.mouseSel = mouseSelect{armed: true, x0: 0, y0: 0, x1: 5, y1: 0}
	m2, _ := m.Update(CopiedMsg{Kind: "code"})
	am := m2.(*AppModel)
	if am.mouseSel.empty() {
		t.Fatal("non-selection copy must not clear the drag highlight")
	}
	if !strings.Contains(am.flashToast, "Copied code") {
		t.Fatalf("toast=%q", am.flashToast)
	}
}

// TestDragRelease_SelectionClearsAfterCopiedMsg drives the full end-to-end drag
// path (CA-515 auto-copy on release): the highlight stays armed right after
// release (Ctrl+C fallback, CA-480) and clears once the CopiedMsg is handled.
func TestDragRelease_SelectionClearsAfterCopiedMsg(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.asciiMode = true
	m.addMessage("assistant", "drag copy clear line", "")
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
	if am.mouseSel.empty() {
		t.Fatal("selection must stay armed right after release (Ctrl+C fallback)")
	}
	if cmd == nil {
		t.Fatal("drag release must issue a copy cmd")
	}
	msg := cmd()
	m4, _ := am.Update(msg)
	toasted := m4.(*AppModel)
	if !toasted.mouseSel.empty() {
		t.Fatal("handling the CopiedMsg must clear the drag highlight (CA-543)")
	}
	if !strings.Contains(toasted.flashToast, "Copied selection") {
		t.Fatalf("toast=%q", toasted.flashToast)
	}
}
