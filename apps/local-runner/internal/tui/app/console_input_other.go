//go:build !windows

package app

import "time"

func (m *AppModel) maybeRearmConsoleInput(now time.Time) {
	if !m.lastWheelAt.IsZero() && time.Since(m.lastWheelAt) < 500*time.Millisecond {
		return
	}
	ensureWheelMouseOn()
}
