package app

import (
	"testing"
)

// Wheel-only: AltScreen+Filter, wheel enabled via 1000h in Init (no CellMotion).
func TestBug328_TuiProgramOpts_NoMouseCellMotion(t *testing.T) {
	opts := tuiProgramOpts()
	if len(opts) != 2 {
		t.Fatalf("opts must be AltScreen+Filter (wheel-only), got %d", len(opts))
	}
}
