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

// primeConsoleBeforeProgram clears QuickEdit (Windows), disables mouse
// tracking, and flushes the input queue once, before tea.NewProgram. Never
// call SetConsoleMode after the bubbletea coninput reader starts — session
// pid 4768 logged 0 KeyMsg while async runner msgs kept flowing (BUG-328).
//
// Bracketed paste (?2004h) is deliberately NOT enabled: on the native
// coninput record path the 200~/201~ markers arrive as plain records and
// would be typed into the composer as "[200~" garbage (the raw-flood guard
// already rejects Ctrl+V floods on Windows).
func primeConsoleBeforeProgram() {
	ensureMouseTrackingOff()
	disableConsoleQuickEdit()
	flushConsoleInputBuffer()
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
