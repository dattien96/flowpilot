//go:build windows

package app

import "time"

const consoleRearmInterval = 2 * time.Second

// maybeRearmConsoleInput keeps mouse tracking ON for wheel scroll.
func (m *AppModel) maybeRearmConsoleInput(now time.Time) {
	_ = now
	// No-op: keep WithMouseCellMotion enabled (wheel=scroll, not history).
}
