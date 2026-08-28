package app

import (
	"testing"
)

// User wants drag-select auto-copy back, so mouse is re-enabled despite BUG-328.
func TestBug328_TuiProgramOpts_NoMouseCellMotion(t *testing.T) {
	opts := tuiProgramOpts()
	if len(opts) != 3 {
		t.Fatalf("opts must be AltScreen+MouseCellMotion+Filter (drag-select), got %d", len(opts))
	}
}
