//go:build windows

package app

import "time"

const consoleRearmInterval = 2 * time.Second

// maybeRearmConsoleInput keeps wheel-only mouse (1000h) alive so wheel scroll
// stays MouseWheel rather than degrading to KeyUp burst. No hover (1002/1003).
func (m *AppModel) maybeRearmConsoleInput(now time.Time) {
	if !m.lastConsoleRearmAt.IsZero() && now.Sub(m.lastConsoleRearmAt) < consoleRearmInterval {
		return
	}
	ensureWheelMouseOn()
	m.lastConsoleRearmAt = now
}
