package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/config"
)

// CA-526 — click hit-test and drag-select must share the exact wrap width the
// transcript was painted at (chatWidth). Before the fix, View() temporarily
// narrowed m.width to contentWidth() for the F2 sidebar, then restored it, so
// clickTargetAt / selection ran at a different line wrap than the painted rows:
// the tool-group summary was hit a row above its visible position, drag
// highlight drifted from the cursor, and a top-of-history drag clamped to the
// bottom. This suite pins the alignment + the viewport freeze.

// sidebarToolGroupModel builds a wide (120) model with an expanded F2 panel so
// the right sidebar engages, plus a wrapped assistant message and a 3-tool run.
func sidebarToolGroupModel(pk string) *AppModel {
	m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
	m.width, m.height = 120, 30
	m.asciiMode = true
	m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
	enableSidebarForTest(m)
	m.addMessage("user", "fix the bug", "")
	m.addMessage("assistant", strings.Repeat("context line ", 25), "")
	m.addMessage("tool", "→ read_file", "tool")
	m.addMessage("tool", "→ grep", "tool")
	m.addMessage("tool", "→ write", "tool")
	m.addMessage("assistant", "done", "")
	return m
}

// viewLineIndex returns the screen Y (index in the View() output) of the first
// line containing want, or -1.
func viewLineIndex(m *AppModel, want string) int {
	for i, line := range strings.Split(stripANSI(m.View()), "\n") {
		if strings.Contains(line, want) {
			return i
		}
	}
	return -1
}

// TestClickToolGroup_AlignsWithWideSidebar: with the F2 sidebar active, a click
// on the visually-rendered summary row must hit the tool-group target (before
// CA-526 it landed a row off because the hit-test wrapped at full terminal
// width while the paint wrapped at contentWidth).
func TestClickToolGroup_AlignsWithWideSidebar(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := sidebarToolGroupModel(pk)
			if !m.useRightSidebar() {
				t.Fatalf("%s: test precondition — right sidebar must engage", pk)
			}
			key := toolGroupKey([]ChatMessage{
				{Role: "tool", Content: "→ read_file"},
				{Role: "tool", Content: "→ grep"},
				{Role: "tool", Content: "→ write"},
			})
			y := viewLineIndex(m, "tool calls")
			if y < 0 {
				t.Fatalf("%s: summary row not found in view", pk)
			}
			if got := m.clickTargetAt(5, y); got != "tool-group:"+key {
				t.Fatalf("%s: click at the painted summary row got %q, want %q (y=%d)", pk, got, "tool-group:"+key, y)
			}
			// Full path: the click toggles the group open.
			m2, _ := m.Update(clickLeft(5, y))
			if view := stripANSI(m2.(*AppModel).View()); !strings.Contains(view, "→ read_file") {
				t.Fatalf("%s: aligned click must expand the group:\n%s", pk, view)
			}
		})
	}
}

// TestClickToolGroup_NarrowNoSidebar: without the sidebar the aligned click also
// resolves (CA-525 regression guard at width 80).
func TestClickToolGroup_NarrowNoSidebar(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := toolGroupModel(pk)
			m.addMessage("user", "u", "")
			m.addMessage("assistant", "a", "")
			m.addMessage("tool", "→ read_file", "tool")
			m.addMessage("tool", "→ grep", "tool")
			m.addMessage("tool", "→ write", "tool")
			key := toolGroupKey([]ChatMessage{
				{Role: "tool", Content: "→ read_file"},
				{Role: "tool", Content: "→ grep"},
				{Role: "tool", Content: "→ write"},
			})
			y := viewLineIndex(m, "tool calls")
			if y < 0 {
				t.Fatalf("%s: summary row not found in view", pk)
			}
			if got := m.clickTargetAt(5, y); got != "tool-group:"+key {
				t.Fatalf("%s: got %q, want %q (y=%d)", pk, got, "tool-group:"+key, y)
			}
		})
	}
}

// TestViewportFrozenWhileSelecting: when the user is mid-drag at the top of a
// scrolled transcript and the line count shrinks (e.g. collapsing a tool group),
// the model offset must not be clamped to the bottom until the selection ends.
func TestViewportFrozenWhileSelecting(t *testing.T) {
	m := toolGroupModel("codex")
	m.addMessage("user", strings.Repeat("long line ", 120), "")
	m.addMessage("tool", "→ read_file", "tool")
	m.addMessage("tool", "→ grep", "tool")
	m.addMessage("tool", "→ write", "tool")
	key := toolGroupKey([]ChatMessage{
		{Role: "tool", Content: "→ read_file"},
		{Role: "tool", Content: "→ grep"},
		{Role: "tool", Content: "→ write"},
	})
	m.toggleToolGroup(key) // expand: 3 tool lines visible
	_ = m.View()
	c := m.tuiChrome()
	total := len(m.renderMessages())
	m.clampViewport(total, c.messagesHeight)
	maxOff := total - c.messagesHeight
	if maxOff < 1 {
		t.Skip("transcript must be scrollable for this test")
	}
	m.viewport.offset = maxOff // top of history
	// Arm a selection (drag highlight).
	m.mouseSel = mouseSelect{armed: true, x0: 5, y0: c.panelH, x1: 9, y1: c.panelH}
	before := m.viewport.offset

	// Collapsing the group shrinks the transcript; while selecting, the model
	// offset must stay put instead of being clamped toward the bottom.
	m.toggleToolGroup(key)
	_ = m.View()
	if m.viewport.offset != before {
		t.Fatalf("viewport offset changed mid-drag: before=%d after=%d", before, m.viewport.offset)
	}

	// Once the selection clears, the next render clamps to the new (smaller) top.
	m.mouseSel = mouseSelect{}
	_ = m.View()
	want := len(m.renderMessages()) - c.messagesHeight
	if want < 0 {
		want = 0
	}
	if m.viewport.offset != want {
		t.Fatalf("after selection cleared offset=%d, want clamped %d", m.viewport.offset, want)
	}
}

// TestDragSelectionTracksRenderedRows: pressing and releasing a drag on the same
// painted assistant row selects that row's text (no row drift from a width
// mismatch between paint and selection).
func TestDragSelectionTracksRenderedRows(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := sidebarToolGroupModel(pk)
			y := viewLineIndex(m, "context line")
			if y < 0 {
				t.Fatalf("%s: assistant wrapped row not found in view", pk)
			}
			_ = m.View()
			p1, _ := m.handleMouse(tea.MouseMsg{
				X: 2, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
			})
			p2, _ := p1.(*AppModel).handleMouse(tea.MouseMsg{
				X: 30, Y: y, Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft,
			})
			am := p2.(*AppModel)
			if am.mouseSel.empty() || am.mouseSel.y0 != y || am.mouseSel.y1 != y {
				t.Fatalf("%s: drag must select the same painted row y=%d, got %+v", pk, y, am.mouseSel)
			}
			// Selection text comes from the rendered (chatWidth-wrapped) rows.
			text := am.selectionPlainText()
			if strings.TrimSpace(text) == "" || !strings.Contains(text, "context line") {
				t.Fatalf("%s: selection text must match the painted row:\n%q", pk, text)
			}
		})
	}
}
