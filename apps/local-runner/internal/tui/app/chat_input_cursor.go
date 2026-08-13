package app

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
