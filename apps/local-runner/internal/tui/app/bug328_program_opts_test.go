package app

import (
	"testing"
)

// BUG-328: mouse cell-motion is removed so Windows conhost does not steal
// keyboard focus from the host terminal.
func TestBug328_TuiProgramOpts_NoMouseCellMotion(t *testing.T) {
	opts := tuiProgramOpts()
	if len(opts) != 2 {
		t.Fatalf("opts must be AltScreen+Filter only, got %d", len(opts))
	}
}
