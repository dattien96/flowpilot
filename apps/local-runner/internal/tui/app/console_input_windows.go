//go:build windows

package app

import "time"

const consoleRearmInterval = 2 * time.Second

// maybeRearmConsoleInput keeps wheel-only mouse (1000h) alive so wheel scroll
// stays MouseWheel rather than degrading to KeyUp burst. No hover (1002/1003).
// Skipped during active wheel burst so the ANSI burst does not wedge the
// 64-slot queue alongside continuous View() (pid 20632 hang after 20s scroll).
func (m *AppModel) maybeRearmConsoleInput(now time.Time) {
	if !m.lastConsoleRearmAt.IsZero() && now.Sub(m.lastConsoleRearmAt) < consoleRearmInterval {
		return
	}
	if !m.lastWheelAt.IsZero() && now.Sub(m.lastWheelAt) < 500*time.Millisecond {
		return
	}
	ensureWheelMouseOn()
	m.lastConsoleRearmAt = now
}
