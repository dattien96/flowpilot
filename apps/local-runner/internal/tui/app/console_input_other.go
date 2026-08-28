//go:build !windows

package app

import "time"

// macOS/Linux keep application mouse tracking ON (WithMouseCellMotion) so that
// drag-select and Shift+click copy reach the app. Do NOT re-send mouse-off ANSI
// here: that would turn tracking off after the first keystroke and silently
// kill the copy affordance (user request).
func (m *AppModel) maybeRearmConsoleInput(now time.Time) {
	_ = now
}
