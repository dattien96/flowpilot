//go:build !windows

package app

import "time"

func (m *AppModel) maybeRearmConsoleInput(now time.Time) {
	_ = now
	ensureWheelMouseOn()
}
