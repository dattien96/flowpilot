package app

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Console input watchdog (CA-645).
//
// On Windows the conhost input path can wedge while the app itself stays fully
// alive: the 64-slot ReadConsoleInput queue stalls, or bubbletea's
// readConInputs goroutine dies on a console-read error. The app keeps polling
// the runner (StepsRuntimeMsg/agentRunsHydratedMsg keep flowing every ~1.6s)
// but ZERO KeyMsg/MouseMsg ever arrive — session pid 25708 logged 0 KeyMsg for
// 9+ minutes after flow start while the user pressed keys and clicked
// [Continue]/[Stop]. Every prior fix hardened the renderer (CA-633/CA-636) and
// the input-adjacent guards (CA-610/612/615/621/630/631); this watchdog
// converts the remaining silent wedge into a logged fingerprint with the exact
// app state at the moment of the stall plus a one-time visible banner, so the
// operator knows to restart instead of staring at a dead screen, and the next
// occurrence carries full context in tui.log.

const (
	inputWatchdogInterval = 10 * time.Second
	inputStallThreshold   = 45 * time.Second
)

type inputWatchdogMsg struct{ at time.Time }

func cmdInputWatchdog() tea.Cmd {
	return tea.Tick(inputWatchdogInterval, func(t time.Time) tea.Msg {
		return inputWatchdogMsg{at: t}
	})
}

// markInputAlive stamps the last-seen console input time; called on every
// KeyMsg and MouseMsg that reaches Update, before normal handling.
func (m *AppModel) markInputAlive() {
	now := time.Now()
	if m.inputStallLogged {
		tuiLog("input-watchdog: console input recovered after %s stall", now.Sub(m.lastInputAt).Round(time.Second))
		m.statusMsg = ""
	}
	m.lastInputAt = now
	m.inputStallLogged = false
}

// checkInputWatchdog runs on inputWatchdogMsg. When no console input has
// arrived for inputStallThreshold while the app is still responsive, it logs
// one diagnostic fingerprint (conn status, flow block state, child view, live
// turn) and shows a one-time banner. The banner is not repeated; the next
// input or a restart clears it.
func (m *AppModel) checkInputWatchdog(at time.Time) (tea.Model, tea.Cmd) {
	if m.lastInputAt.IsZero() {
		m.lastInputAt = at
		return m, cmdInputWatchdog()
	}
	if !m.inputStallLogged && at.Sub(m.lastInputAt) >= inputStallThreshold {
		stall := at.Sub(m.lastInputAt)
		tuiLog("input-watchdog: console input stalled — no KeyMsg/MouseMsg for %s since %s; connStatus=%s flowBlocked=%v viewingChild=%v turnActive=%v; press any key or Ctrl+C to restart",
			stall.Round(time.Second), m.lastInputAt.Format(time.RFC3339),
			m.connStatus, m.flowLoopBlocked(), m.viewingChild(), m.turnIsActive())
		m.inputStallLogged = true
		m.statusMsg = "input stalled: no keys/mouse received (" + stall.Round(time.Second).String() + ") — press any key or Ctrl+C to restart"
		m.addMessage("system", "Input stalled: console is not delivering keys/mouse to the TUI (the session is otherwise alive). Press any key, or Ctrl+C and restart the TUI.", "gate")
	}
	return m, cmdInputWatchdog()
}