package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// inputAttachChipPlain is the pending-image chip in the prompt row.
// Empty when nothing is attached so the box does not look like it has an image.
// Click still works on "[N img]" (hitAttachChrome); attach via Alt+V / /image paste.
func (m *AppModel) inputAttachChipPlain() string {
	n := len(m.pendingAttach)
	if n <= 0 {
		return ""
	}
	return fmt.Sprintf("[%d img] ", n)
}

// inputCaretIndex is the rune offset where the caret sits.
// inputCursor < 0 means "stick to end" so tests that only set inputValue
// still append, matching the previous always-at-end behavior.
func (m *AppModel) inputCaretIndex() int {
	n := len([]rune(m.inputValue))
	if m.inputCursor < 0 || m.inputCursor > n {
		return n
	}
	return m.inputCursor
}

func (m *AppModel) moveInputCursor(delta int) {
	n := len([]rune(m.inputValue))
	cur := m.inputCaretIndex() + delta
	if cur < 0 {
		cur = 0
	}
	if cur > n {
		cur = n
	}
	if cur == n {
		m.inputCursor = -1
		return
	}
	m.inputCursor = cur
}

func (m *AppModel) insertInputAtCursor(s string) {
	if s == "" {
		return
	}
	runes := []rune(m.inputValue)
	cur := m.inputCaretIndex()
	extra := []rune(s)
	out := make([]rune, 0, len(runes)+len(extra))
	out = append(out, runes[:cur]...)
	out = append(out, extra...)
	out = append(out, runes[cur:]...)
	m.inputValue = string(out)
	m.moveInputCursor(len(extra))
}

func (m *AppModel) deleteInputBeforeCursor() {
	runes := []rune(m.inputValue)
	cur := m.inputCaretIndex()
	if cur == 0 || len(runes) == 0 {
		return
	}
	out := append([]rune{}, runes[:cur-1]...)
	out = append(out, runes[cur:]...)
	m.inputValue = string(out)
	m.moveInputCursor(-1)
}

// windowRunesAround keeps caret visible when a single input line is wider
// than the box (same tail-window idea as before, but centered on the caret).
func windowRunesAround(runes []rune, caret, width int) (shown []rune, caretIn int) {
	if width < 1 {
		width = 1
	}
	if len(runes) <= width {
		return runes, caret
	}
	start := caret - width + 1
	if start < 0 {
		start = 0
	}
	end := start + width
	if end > len(runes) {
		end = len(runes)
		start = end - width
		if start < 0 {
			start = 0
		}
	}
	return runes[start:end], caret - start
}

// tryPlaceInputCursor maps a mouse click (cell coords) onto the chat prompt
// caret. Works for Windows and macOS under tea.WithMouseCellMotion — both
// deliver left press as MouseActionPress (or Windows Type=MouseLeft).
// Returns true when the click landed on the input body (not chips/borders).
func (m *AppModel) tryPlaceInputCursor(x, y int) bool {
	c := m.tuiChrome()
	if c.inputH <= 0 || y < c.inputY || y >= c.inputY+c.inputH {
		return false
	}
	// Leave interactive chrome alone.
	if hitAttachChrome(c, x, y) {
		return false
	}
	if hitApprovalChrome(c, x, y) != "" {
		return false
	}
	if hitQuestionChrome(m, c, x, y) != "" {
		return false
	}

	w := m.width
	if w < 1 {
		w = 1
	}
	innerW := w - 2
	if innerW < 1 {
		innerW = 1
	}
	attach := m.inputAttachChipPlain()
	if attach != "" && lipgloss.Width(attach)+8 > innerW {
		attach = ""
	}
	attachW := lipgloss.Width(attach)
	// Match renderInputLine: reserve leading space + caret cell.
	fixedWidth := 1 + attachW + 1
	availWidth := innerW - fixedWidth
	if availWidth < 1 {
		availWidth = 1
	}

	body := m.inputValue
	bodyLines := strings.Split(body, "\n")
	const maxVis = 6
	hiddenLines := 0
	if len(bodyLines) > maxVis {
		hiddenLines = len(bodyLines) - maxVis
		bodyLines = bodyLines[len(bodyLines)-maxVis:]
	}
	off := 0
	if hiddenLines > 0 {
		all := strings.Split(m.inputValue, "\n")
		for i := 0; i < hiddenLines && i < len(all); i++ {
			off += len([]rune(all[i])) + 1
		}
	}

	innerLead := 0
	if m.approval != nil {
		innerLead++
	}
	if m.question != nil {
		innerLead++
	}

	rel := y - c.inputY
	// frameInput: row 0 = top border, last = bottom border, middle = inner lines.
	if rel <= 0 {
		return false
	}
	if rel >= c.inputH-1 {
		// Bottom border → stick caret to end.
		m.inputCursor = -1
		return true
	}
	innerIdx := rel - 1
	if innerIdx < innerLead {
		return false
	}
	bodyIdx := innerIdx - innerLead
	if bodyIdx < 0 || bodyIdx >= len(bodyLines) {
		return false
	}

	// frame line: v + " " + (attach? + text) …  → text starts at x=2 (+ attach on first body row).
	textStartX := 2
	if bodyIdx == 0 {
		textStartX += attachW
	}
	col := x - textStartX
	if col < 0 {
		col = 0
	}

	lineRunes := []rune(bodyLines[bodyIdx])
	lineStart := off
	for i := 0; i < bodyIdx; i++ {
		lineStart += len([]rune(bodyLines[i])) + 1
	}
	lineEnd := lineStart + len(lineRunes)

	windowStart := 0
	if len(lineRunes) > availWidth {
		caretAt := m.inputCaretIndex()
		colForWin := caretAt - lineStart
		onLine := caretAt >= lineStart && (caretAt < lineEnd || (caretAt == lineEnd && bodyIdx == len(bodyLines)-1))
		if !onLine {
			colForWin = len(lineRunes)
		}
		if colForWin < 0 {
			colForWin = 0
		}
		if colForWin > len(lineRunes) {
			colForWin = len(lineRunes)
		}
		_, caretIn := windowRunesAround(lineRunes, colForWin, availWidth)
		windowStart = colForWin - caretIn
		if windowStart < 0 {
			windowStart = 0
		}
	}
	if col > availWidth {
		col = availWidth
	}
	// Click past the last visible rune → end of this line (or of window).
	maxCol := len(lineRunes) - windowStart
	if maxCol < 0 {
		maxCol = 0
	}
	if maxCol > availWidth {
		maxCol = availWidth
	}
	if col > maxCol {
		col = maxCol
	}

	abs := lineStart + windowStart + col
	n := len([]rune(m.inputValue))
	if abs >= n {
		m.inputCursor = -1
	} else {
		m.inputCursor = abs
	}
	return true
}
