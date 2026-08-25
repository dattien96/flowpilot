package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// inputAttachChipPlain is the pending-image chip in the prompt row.
// Empty when nothing is attached so the box does not look like it has an image.
// Click "[N img]" opens the manage panel (detach); attach via Alt+V / /image paste.
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

// setInputCaret places the caret at abs (rune index). abs < 0 or abs >= len → sticky end.
func (m *AppModel) setInputCaret(abs int) {
	n := len([]rune(m.inputValue))
	if abs < 0 || abs >= n {
		m.inputCursor = -1
		return
	}
	m.inputCursor = abs
}

// clearInputValue empties the prompt and resets the caret to sticky-end so the
// next keystroke appends (Esc clear / Enter send must not leave a mid-string index).
func (m *AppModel) clearInputValue() {
	m.inputValue = ""
	m.inputCursor = -1
	m.suggIdx = 0
	m.pasteSegments = nil
	m.resetPasteBurst()
}

func (m *AppModel) insertInputAtCursor(s string) {
	s = strings.ReplaceAll(s, "\x00", "")
	if s == "" {
		return
	}
	runes := []rune(m.inputValue)
	cur := m.inputCaretIndex()
	// Typing '/' after leftover draft text starts command mode (word-boundary
	// slash). Skip during auth so emails/passwords are not rewritten. Bulk
	// paste of a path is a longer string, not a lone "/".
	if m.authPhase == AuthNone && s == "/" && cur > 0 && !isSlashBoundary(runes[cur-1]) && runes[cur-1] != '/' {
		s = " /"
	}
	extra := []rune(s)
	out := make([]rune, 0, len(runes)+len(extra))
	out = append(out, runes[:cur]...)
	out = append(out, extra...)
	out = append(out, runes[cur:]...)
	m.inputValue = string(out)
	// Do not call moveInputCursor(len): with sticky-end (inputCursor==-1),
	// inputCaretIndex() already reflects the *new* length after assignment,
	// so +len would overshoot or leave a stale mid-index after backspace.
	m.setInputCaret(cur + len(extra))
}

func (m *AppModel) deleteInputBeforeCursor() {
	runes := []rune(m.inputValue)
	cur := m.inputCaretIndex()
	if cur == 0 || len(runes) == 0 {
		return
	}
	// CA-624: paste tokens "[Pasted N chars]" must delete as a whole.
	// If caret is right after a collapsed token, remove the entire token
	// and its pasteSegment so one Backspace clears the paste.
	if len(m.pasteSegments) > 0 {
		for i, seg := range m.pasteSegments {
			tokenRunes := []rune(seg.token)
			tlen := len(tokenRunes)
			if tlen == 0 || cur < tlen {
				continue
			}
			start := cur - tlen
			if start < 0 || start+tlen > len(runes) {
				continue
			}
			match := true
			for k := 0; k < tlen; k++ {
				if runes[start+k] != tokenRunes[k] {
					match = false
					break
				}
			}
			if match {
				out := append(append([]rune{}, runes[:start]...), runes[cur:]...)
				m.inputValue = string(out)
				// Remove the matched segment
				m.pasteSegments = append(m.pasteSegments[:i], m.pasteSegments[i+1:]...)
				if m.inputCursor < 0 {
					// sticky-end stays sticky
					m.inputCursor = -1
				} else {
					m.setInputCaret(start)
				}
				// Pasting may have left burst state armed; clear it
				m.resetPasteBurst()
				return
			}
		}
	}
	// Sticky-end backspace must stay sticky. Using moveInputCursor(-1) after
	// shortening the string treated caret as (newLen) then -1 → before the
	// last remaining rune ("ab|" BS → "|a" instead of "a|"), so retyping
	// produced reversed order (e.g. abc → bca).
	wasStickyEnd := m.inputCursor < 0
	out := append(append([]rune{}, runes[:cur-1]...), runes[cur:]...)
	m.inputValue = string(out)
	if wasStickyEnd || len(out) == 0 {
		m.inputCursor = -1
		return
	}
	m.setInputCaret(cur - 1)
}

func (m *AppModel) deleteInputAfterCursor() {
	runes := []rune(m.inputValue)
	cur := m.inputCaretIndex()
	if cur >= len(runes) || len(runes) == 0 {
		return
	}
	// CA-624 forward-delete: if caret is right before a paste token, remove whole token
	if len(m.pasteSegments) > 0 {
		for i, seg := range m.pasteSegments {
			tokenRunes := []rune(seg.token)
			tlen := len(tokenRunes)
			if tlen == 0 || cur+tlen > len(runes) {
				continue
			}
			match := true
			for k := 0; k < tlen; k++ {
				if runes[cur+k] != tokenRunes[k] {
					match = false
					break
				}
			}
			if match {
				out := append(append([]rune{}, runes[:cur]...), runes[cur+tlen:]...)
				m.inputValue = string(out)
				m.pasteSegments = append(m.pasteSegments[:i], m.pasteSegments[i+1:]...)
				if cur >= len(out) {
					m.inputCursor = -1
				} else {
					m.setInputCaret(cur)
				}
				m.resetPasteBurst()
				return
			}
		}
	}
	out := append(append([]rune{}, runes[:cur]...), runes[cur+1:]...)
	m.inputValue = string(out)
	if cur >= len(out) {
		m.inputCursor = -1
	} else {
		m.setInputCaret(cur)
	}
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
	// The right sidebar (CA-524) owns the x>=sideX region — never place the caret there.
	if c.sideActive && x >= c.sideX {
		return false
	}
	// Leave interactive chrome alone.
	if hitAttachChrome(c, x, y) {
		return false
	}
	if m.hitApprovalChrome(c, x, y) != "" {
		return false
	}
	if hitQuestionChrome(m, c, x, y) != "" {
		return false
	}
	if m.hitBlockedChrome(c, x, y) != "" {
		return false
	}
	if m.hitAttentionChip(x, y) != "" {
		return false
	}

	w := m.chatWidth()
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
	off := 0

	innerLead := 0
	if m.approval != nil {
		innerLead++
	}
	if m.question != nil {
		innerLead++
	}
	if bar := m.renderAttentionBar(); bar != "" {
		innerLead += strings.Count(bar, "\n") + 1
	}
	if bar := m.renderBlockedBar(); bar != "" {
		innerLead += strings.Count(bar, "\n") + 1
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
