//go:build !windows

package app

import "time"

func (m *AppModel) maybeRearmConsoleInput(now time.Time) {
	_ = now
	// Keep MouseCellMotion ON for wheel scroll (scroll != history).
}
