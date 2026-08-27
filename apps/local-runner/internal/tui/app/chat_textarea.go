package app

import (
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"
)

// newChatTextArea creates a focused textarea for the chat composer (Task-308).
// Width is set via SetWidth before first View; height is 3 lines and grows
// up to 8 lines when the user types multi-line input.
func newChatTextArea(width int) textarea.Model {
	ta := textarea.New()
	ta.Placeholder = "Type a message, /command, or @file..."
	ta.Prompt = ""
	ta.CharLimit = 0
	ta.ShowLineNumbers = false
	ta.SetWidth(width)
	ta.SetHeight(3)
	// Disable textarea's built-in Enter->newline and Ctrl+V paste so handleKey
	// owns send vs newline and Alt+V clipboard. Navigation keys stay but are
	// not used in this slice (see handleKey revert below).
	ta.KeyMap.InsertNewline = key.NewBinding()
	ta.KeyMap.Paste = key.NewBinding()
	ta.FocusedStyle.CursorLine = lipgloss.NewStyle()
	ta.FocusedStyle.Placeholder = styleSystem
	ta.FocusedStyle.Prompt = lipgloss.NewStyle()
	ta.FocusedStyle.Text = styleUser
	ta.BlurredStyle.CursorLine = lipgloss.NewStyle()
	ta.Focus()
	return ta
}

// useTextareaView reports whether the composer should be rendered via
// textarea.View() (sticky-end with draft) or the legacy custom renderer.
// Sticky-end with draft covers the common typing case where SetValue leaves
// the caret at the right place. Empty idle, mid-string caret, skill highlight,
// attach chip, burst, live turn, and blocked/attention chrome stay on the
// legacy path so the CA-633 idle-pin/caching contract and CA-560 no-clamp
// are preserved.
func (m *AppModel) useTextareaView() bool {
	if !m.mirrorReady() || m.authPhase != AuthNone || m.viewingChild() || m.modeSetupModalOpen || m.pasteBurst.active {
		return false
	}
	if m.inputCursor >= 0 {
		return false
	}
	if strings.TrimSpace(m.inputValue) == "" {
		return false
	}
	if len(m.selectedSkills) > 0 {
		return false
	}
	if strings.TrimSpace(m.inputAttachChipPlain()) != "" {
		return false
	}
	if m.flowLoopBlocked() || m.gate != nil || m.question != nil || m.approval != nil || len(m.attention) > 0 {
		return false
	}
	if m.turnIsActive() || m.workIsLive() {
		return false
	}
	return true
}

// normalizeComposerForMirror rewrites inputValue the same way bubbles'
// runeutil.Sanitizer will when the text lands in the buffer: \r and \n become
// \n, \t expands to four spaces, other C0 control chars are dropped. Pushing
// this normalized form keeps the mirror byte-faithful (M1): after SetValue,
// textarea.Value() == normalizeComposerForMirror(inputValue). The live
// composer (m.inputValue) is never rewritten — send still uses the original.
// Known residual: Value() TrimSuffixes one trailing \n, so an input ending in
// two or more newlines stays one newline shorter in the mirror (Phase-2
// constraint, documented in Task-308).
func normalizeComposerForMirror(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\r', '\n':
			return '\n'
		case '\t':
			return r
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s)
}

// expandMirrorTabs mirrors the \t -> four spaces expansion the bubbles
// sanitizer performs (it cannot be expressed as a single-rune map above).
func expandMirrorTabs(s string) string {
	if !strings.Contains(s, "\t") {
		return s
	}
	return strings.ReplaceAll(s, "\t", "    ")
}

// syncTextareaValue mirrors inputValue into textarea when it is safe to do so.
// It is value-only — no caret poke via reflect/unsafe. Auth passwords must
// never land in the textarea buffer (would leak via View).
func (m *AppModel) syncTextareaValue() {
	if !m.textareaReady || m.authPhase != AuthNone {
		return
	}
	norm := expandMirrorTabs(normalizeComposerForMirror(m.inputValue))
	// Avoid churn: SetValue allocates and sanitizes.
	if m.textarea.Value() == norm {
		return
	}
	m.textarea.SetValue(norm)
}

// mirrorReady reports whether the textarea mirror exists (initialized in New).
func (m *AppModel) mirrorReady() bool {
	return m.textareaReady
}

// isTextareaReady reports whether the textarea mirror can be used.
// Kept for compatibility; new code should also check auth/child/modal.
func (m *AppModel) isTextareaReady() bool {
	return m.textareaReady && m.authPhase == AuthNone && !m.viewingChild() && !m.modeSetupModalOpen
}
