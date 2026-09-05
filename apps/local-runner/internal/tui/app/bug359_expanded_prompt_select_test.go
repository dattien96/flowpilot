package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/config"
)

// BUG-359: a press-down on an already-EXPANDED user prompt box toggled
// collapse immediately, so starting a drag-select to copy long prompt text
// collapsed the box first — long prompts were uncopyable. The toggle must
// fire on same-cell release (real click) only; press→release over different
// cells is a drag-select that copies and never toggles.

func bug359LongPromptModel(t *testing.T) (*AppModel, string) {
	t.Helper()
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.asciiMode = true
	content := strings.TrimSpace(strings.Repeat("copyable prompt word ", 30))
	m.addMessage("user", content, "")
	_ = m.View()
	return m, content
}

func bug359PressExpand(t *testing.T, m *AppModel, content string) *AppModel {
	t.Helper()
	c := m.tuiChrome()
	p1, _ := m.handleMouse(tea.MouseMsg{
		X: 4, Y: c.panelH, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
	})
	am := p1.(*AppModel)
	if !am.userPromptExpanded(content) {
		t.Fatal("press on collapsed prompt box must expand (existing behavior)")
	}
	return am
}

// Press on the expanded box must not collapse (the reported bug).
func TestExpandedPrompt_PressDoesNotCollapse(t *testing.T) {
	m, content := bug359LongPromptModel(t)
	am := bug359PressExpand(t, m, content)

	p2, _ := am.handleMouse(tea.MouseMsg{
		X: 6, Y: am.tuiChrome().panelH + 1, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
	})
	am2 := p2.(*AppModel)
	if !am2.userPromptExpanded(content) {
		t.Fatal("press-down on expanded prompt must not collapse (breaks drag-select copy)")
	}
	if am2.mouseDrag.expandKey == "" {
		t.Fatal("press on expanded prompt must arm expandKey drag")
	}
}

// Same-cell release on the expanded box is a real click → collapses.
func TestExpandedPrompt_SameCellReleaseCollapses(t *testing.T) {
	m, content := bug359LongPromptModel(t)
	am := bug359PressExpand(t, m, content)
	c := am.tuiChrome()

	p2, _ := am.handleMouse(tea.MouseMsg{
		X: 6, Y: c.panelH + 1, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
	})
	am2 := p2.(*AppModel)
	p3, _ := am2.handleMouse(tea.MouseMsg{
		X: 6, Y: c.panelH + 1, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft,
	})
	if p3.(*AppModel).userPromptExpanded(content) {
		t.Fatal("same-cell press+release on expanded prompt must collapse (click still works)")
	}
}

// Press→release over different cells (wheel-only, no motion) copies the
// selection and NEVER toggles.
func TestExpandedPrompt_DragCopiesWithoutCollapse(t *testing.T) {
	m, content := bug359LongPromptModel(t)
	am := bug359PressExpand(t, m, content)
	c := am.tuiChrome()

	p2, _ := am.handleMouse(tea.MouseMsg{
		X: 4, Y: c.panelH, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
	})
	am2 := p2.(*AppModel)
	p3, cmd := am2.handleMouse(tea.MouseMsg{
		X: 40, Y: c.panelH + 2, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft,
	})
	am3 := p3.(*AppModel)
	if !am3.userPromptExpanded(content) {
		t.Fatal("drag-select over expanded prompt must not collapse")
	}
	if cmd == nil {
		t.Fatal("drag release must issue the selection copy cmd")
	}
	if am3.mouseSel.empty() {
		t.Fatal("selection must stay armed after auto-copy")
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
