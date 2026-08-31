//go:build windows

package app

import "time"

const consoleRearmInterval = 2 * time.Second

// maybeRearmConsoleInput re-sends mouse-off ANSI periodically. SetConsoleMode
// (QuickEdit) must not run after bubbletea starts its reader — only stdout ANSI.
func (m *AppModel) maybeRearmConsoleInput(now time.Time) {
	if !m.lastConsoleRearmAt.IsZero() && now.Sub(m.lastConsoleRearmAt) < consoleRearmInterval {
		return
	}
	ensureMouseTrackingOff()
	m.lastConsoleRearmAt = now
}
