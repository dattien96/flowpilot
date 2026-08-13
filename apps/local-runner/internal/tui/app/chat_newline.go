package app

import tea "github.com/charmbracelet/bubbletea"

// enterModifierHeld reports whether Ctrl or Shift is down. On Windows, Ctrl+Enter
// and Shift+Enter arrive as plain KeyEnter (console ignores those modifiers).
var enterModifierHeld = defaultEnterModifiers

func defaultEnterModifiers() (ctrl, shift bool) {
	return false, false
}

func isModifiedEnterNewline(msg tea.KeyMsg) bool {
	if msg.Type != tea.KeyEnter {
		return false
	}
	ctrl, shift := enterModifierHeld()
	return ctrl || shift
}
