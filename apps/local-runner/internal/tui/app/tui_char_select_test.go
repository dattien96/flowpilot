package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/config"
)

func TestHighlightVisualColumns_PartialLine(t *testing.T) {
	// Cells 2..5 of "abcdefgh" → "cdef" (styling may be a no-op in dumb color profile).
	got := highlightVisualColumns("abcdefgh", 2, 5)
	if stripANSI(got) != "abcdefgh" {
		t.Fatalf("full line text should remain, got %q", stripANSI(got))
	}
	mid := extractVisualColumns("abcdefgh", 2, 5)
	if mid != "cdef" {
		t.Fatalf("extract=%q want cdef", mid)
	}
	// applyMouseSelection should keep unselected siblings unselected as full-line paint.
	lines := applyMouseSelection([]string{"abcdefgh", "zzzzzzzz"}, mouseSelect{armed: true, x0: 2, y0: 0, x1: 5, y1: 0}, 0)
	if stripANSI(lines[0]) != "abcdefgh" {
		t.Fatalf("line0 plain=%q", stripANSI(lines[0]))
	}
	if stripANSI(lines[1]) != "zzzzzzzz" {
		t.Fatalf("line1 must be untouched: %q", stripANSI(lines[1]))
	}
	if extractVisualColumns(stripANSI(lines[0]), 2, 5) != "cdef" {
		t.Fatal("selected span should still be cdef")
	}
}

func TestExtractVisualColumns_SingleChar(t *testing.T) {
	if got := extractVisualColumns("hello", 1, 1); got != "e" {
		t.Fatalf("got %q want e", got)
	}
}

func TestColsOnLine_MultiLineRanges(t *testing.T) {
	sel := mouseSelect{armed: true, x0: 5, y0: 2, x1: 10, y1: 4}
	x0, x1, ok := sel.colsOnLine(2)
	if !ok || x0 != 5 || x1 < 1000 {
		t.Fatalf("first line: x0=%d x1=%d ok=%v", x0, x1, ok)
	}
	x0, x1, ok = sel.colsOnLine(3)
	if !ok || x0 != 0 || x1 < 1000 {
		t.Fatalf("middle line: x0=%d x1=%d ok=%v", x0, x1, ok)
	}
	x0, x1, ok = sel.colsOnLine(4)
	if !ok || x0 != 0 || x1 != 10 {
		t.Fatalf("last line: x0=%d x1=%d ok=%v", x0, x1, ok)
	}
	if _, _, ok := sel.colsOnLine(1); ok {
		t.Fatal("row above selection must be out")
	}
}

func TestApplyMouseSelection_DoesNotStyleWholeUnselectedLine(t *testing.T) {
	lines := []string{"abcdefghij"}
	sel := mouseSelect{armed: true, x0: 2, y0: 0, x1: 4, y1: 0}
	out := applyMouseSelection(lines, sel, 0)
	if len(out) != 1 {
		t.Fatalf("len=%d", len(out))
	}
	// Full line still readable
	if stripANSI(out[0]) != "abcdefghij" {
		t.Fatalf("plain=%q", stripANSI(out[0]))
	}
	// Selection extract matches chars only
	if got := extractVisualColumns(stripANSI(out[0]), 2, 4); got != "cde" {
		t.Fatalf("span=%q", got)
	}
}

func TestSelectionPlainText_CharRangeOnly(t *testing.T) {
	m := New(configChatForSelectTest(), "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.asciiMode = true
	// Use a single plain system line so strip content is predictable enough.
	m.addMessage("system", "ABCDEFGHIJ", "")
	_ = m.View()
	c := m.tuiChrome()
	// Find a viewport row that contains ABCDEFGHIJ.
	lines := m.renderMessages()
	m.clampViewport(len(lines), c.messagesHeight)
	vis := sliceViewport(lines, c.messagesHeight, m.viewport.offset)
	row := -1
	colA := -1
	for i, line := range vis {
		plain := stripANSI(line)
		if j := strings.Index(plain, "ABCDEFGHIJ"); j >= 0 {
			row = i
			colA = j
			break
		}
	}
	if row < 0 {
		t.Fatalf("could not find ABCDEFGHIJ in viewport:\n%s", strings.Join(vis, "\n"))
	}
	// Select only "CDE" (indices colA+2 .. colA+4)
	y := c.panelH + row
	m.mouseSel = mouseSelect{armed: true, x0: colA + 2, y0: y, x1: colA + 4, y1: y}
	got := m.selectionPlainText()
	if got != "CDE" {
		t.Fatalf("selectionPlainText=%q want CDE (row=%d colA=%d)", got, row, colA)
	}
}

func configChatForSelectTest() config.ChatConfig {
	return config.ChatConfig{Provider: "codex"}
}
