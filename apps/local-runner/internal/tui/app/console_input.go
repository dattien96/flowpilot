package app

import (
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// mouseTrackingOffANSI disables common xterm mouse modes left over from a prior
// session or another app in the same terminal (BUG-328). Without this, Windows
// Terminal / Cursor can keep delivering mouse records that starve KeyMsg.
const mouseTrackingOffANSI = "\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l"

func ensureMouseTrackingOff() {
	_, _ = os.Stdout.WriteString(mouseTrackingOffANSI)
}

// primeConsoleBeforeProgram clears QuickEdit (Windows) and disables mouse
// tracking once, before tea.NewProgram. Never call SetConsoleMode after the
// bubbletea coninput reader starts — session pid 4768 logged 0 KeyMsg while
// async runner msgs kept flowing (BUG-328).
func primeConsoleBeforeProgram() {
	ensureMouseTrackingOff()
	disableConsoleQuickEdit()
}

// ensureConsoleInputReady is kept for tests; production uses primeConsoleBeforeProgram.
func ensureConsoleInputReady() {
	primeConsoleBeforeProgram()
}

func cmdEnsureMouseTrackingOff() tea.Cmd {
	return func() tea.Msg {
		ensureMouseTrackingOff()
		return nil
	}
}
