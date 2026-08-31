package app

import (
	"testing"
)

// Mouse CellMotion is ON for wheel scroll (history only on Up/Down keys).
func TestBug328_TuiProgramOpts_NoMouseCellMotion(t *testing.T) {
	opts := tuiProgramOpts()
	if len(opts) != 3 {
		t.Fatalf("opts must be AltScreen+Filter+MouseCellMotion, got %d", len(opts))
	}
}
