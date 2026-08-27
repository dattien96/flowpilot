package app

import "strings"

// promptHistoryMax caps the recalled prompt ring so memory stays bounded.
const promptHistoryMax = 100

// recordPromptHistory appends a sent chat prompt for Up/Down recall (bash-style).
// Slash commands are excluded (they are fast to retype and pollute recall), and
// consecutive duplicates collapse so Up/Down never sits on the same entry.
// Browsing is reset so the next Up starts from the newest prompt.
func (m *AppModel) recordPromptHistory(input string) {
	p := strings.TrimSpace(input)
	if p == "" || strings.HasPrefix(p, "/") {
		return
	}
	if n := len(m.promptHistory); n > 0 && m.promptHistory[n-1] == p {
		return
	}
	m.promptHistory = append(m.promptHistory, p)
	if len(m.promptHistory) > promptHistoryMax {
		m.promptHistory = m.promptHistory[len(m.promptHistory)-promptHistoryMax:]
	}
	m.promptHistIdx = -1
	m.promptDraft = ""
}

// navigatePromptHistory moves the composer through the sent-prompt ring.
// dir > 0 recalls older prompts (Up), dir < 0 moves toward the live draft
// (Down). Returns true when the input value was changed.
func (m *AppModel) navigatePromptHistory(dir int) bool {
	n := len(m.promptHistory)
	if n == 0 {
		return false
	}
	if dir > 0 {
		// Up: from draft -> newest, then walk older.
		if m.promptHistIdx < 0 {
			if m.inputValue != "" {
				m.promptDraft = m.inputValue
			}
			m.promptHistIdx = n - 1
		} else if m.promptHistIdx > 0 {
			m.promptHistIdx--
		} else {
			return false
		}
	} else {
		// Down: walk newer toward the draft, then back to the draft itself.
		if m.promptHistIdx < 0 {
			return false
		}
		if m.promptHistIdx < n-1 {
			m.promptHistIdx++
		} else {
			m.promptHistIdx = -1
			m.inputValue = m.promptDraft
			m.inputCursor = -1
			if m.mirrorReady() {
				m.syncTextareaValue()
			}
			return true
		}
	}
	m.inputValue = m.promptHistory[m.promptHistIdx]
	m.inputCursor = -1
	if m.mirrorReady() {
		m.syncTextareaValue()
	}
	return true
}

