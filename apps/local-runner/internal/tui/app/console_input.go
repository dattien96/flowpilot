package app

import (
	"os"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"
)

// shouldDisableMouseTracking reports whether the platform must keep application
// mouse tracking OFF. Windows conhost steals keyboard focus from the host when
// the TUI enables mouse mode (BUG-328), so it stays off there. Every other
// platform keeps it ON so drag-select / Shift+click copy works (user request).
func shouldDisableMouseTracking() bool {
	return runtime.GOOS == "windows"
}

// initMouseCmd returns the mouse-tracking-off cmd for platforms that must keep
// tracking disabled (Windows), else nil so drag-select copy keeps working.
func initMouseCmd() tea.Cmd {
	if shouldDisableMouseTracking() {
		return cmdEnsureMouseTrackingOff()
	}
	return nil
}

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
