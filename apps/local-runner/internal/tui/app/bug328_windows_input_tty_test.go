package app

import "testing"

// BUG-328: Run() must NOT wrap stdin. On Windows Bubble Tea's default
// coninput reader (readConInputs over the console record queue) is the only
// path proven to deliver F2/F4/Esc and IME text (tui.log pid 19296/14012);
// wrapping stdin pulls in readAnsiInputs + localereader, which mangles IME
// commits and drops ESC-prefixed sequences after them (pid 11948).
func TestBug328_RunOptsDoNotWrapWindowsInput(t *testing.T) {
	opts := tuiRunProgramOpts()
	base := tuiProgramOpts()
	if len(opts) != len(base) {
		t.Fatalf("Run opts must equal tuiProgramOpts on every platform (no WithInput wrapper), got %d want %d", len(opts), len(base))
	}
}

func TestBug328_PendingConsoleEventsSanity(t *testing.T) {
	if n := pendingConsoleInputEvents(); n < -1 {
		t.Fatalf("pendingConsoleInputEvents must return >= -1, got %d", n)
	}
}