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
//
// The stall is only flagged when the app is in a state that REQUIRES operator
// input (blocked flow, gate/approval/question card). Silence while idle,
// connecting, or watching a running turn is normal user behavior — sessions
// 21472/25580/12300 sat 1-3 minutes without any input while waiting on the
// runner and must not trigger the banner.

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

// inputStallEligible reports whether the app currently requires the operator
// to act — the only states where input silence is suspicious:
//   - flow loop parked "blocked" (Continue/Stop bar — the run-151954 wedge)
//   - a gate block, approval card, or question card waiting for a decision
func (m *AppModel) inputStallEligible() bool {
	if m.flowLoopBlocked() {
		return true
	}
	return m.approval != nil || m.question != nil || m.gate != nil
}

// markMotionAlive stamps hover-motion delivery. Hover motion is filtered out
// before Update (tuiMsgFilter), so it cannot mark a key/click stall as
// recovered by itself — but its arrival proves the console input pipe is
// delivering events at all, which separates "keys dropped upstream" from
// "console fully dead" in the stall fingerprint.
func (m *AppModel) markMotionAlive() {
	m.lastMotionAt = time.Now()
}

// markInputAlive stamps the last-seen console input time; called on every
// KeyMsg and MouseMsg that reaches Update, before normal handling.
func (m *AppModel) markInputAlive() {
	now := time.Now()
	m.inputHadRealKey = true
	m.maybeRearmConsoleInput(now)
	if m.inputStallLogged {
		since := m.lastInputAt
		if since.IsZero() {
			since = m.inputExpectedSince
		}
		if since.IsZero() {
			tuiLog("input-watchdog: console input recovered (no prior key baseline)")
		} else {
			tuiLog("input-watchdog: console input recovered after %s stall", now.Sub(since).Round(time.Second))
		}
		m.statusMsg = ""
	}
	m.lastInputAt = now
	m.inputStallLogged = false
}

// checkInputWatchdog runs on inputWatchdogMsg.
//
// The stall FINGERPRINT is always logged once per stall episode regardless of
// app state: session 9176 wedged at startup while idle (no flow, no block),
// and v2's eligibility gate kept that wedge entirely invisible. The log line
// is cheap and harmless when silence is just the user reading.
//
// The visible BANNER is only raised in input-requiring states (blocked flow,
// gate/approval/question card): silence there means the operator is probably
// trying to act and the wedge is real (the run-151954 case). Silence while
// idle/connecting/watching is normal user behavior — sessions
// 21472/25580/12300 sat 1-3 minutes without input and must not be alarmed.
func (m *AppModel) checkInputWatchdog(at time.Time) (tea.Model, tea.Cmd) {
	if m.lastInputAt.IsZero() {
		if !m.inputExpectedSince.IsZero() && at.Sub(m.inputExpectedSince) >= inputStallThreshold && !m.inputStallLogged {
			stall := at.Sub(m.inputExpectedSince)
			tuiLog("input-watchdog: startup input stalled — session ready but no KeyMsg for %s since %s; connStatus=%s flowBlocked=%v viewingChild=%v turnActive=%v; terminal not delivering keys",
				stall.Round(time.Second), m.inputExpectedSince.Format(time.RFC3339),
				m.connStatus, m.flowLoopBlocked(), m.viewingChild(), m.turnIsActive())
			m.inputStallLogged = true
			m.statusMsg = "input stalled: terminal is not delivering keys — close this window to exit (runner is cleaned up)"
			m.addMessage("system", "Input stalled: the terminal stopped delivering keys to the TUI (the session is otherwise alive). Close this terminal window to exit — the Go runner is terminated automatically.", "gate")
		}
		return m, cmdInputWatchdog()
	}
	if !m.inputStallLogged && at.Sub(m.lastInputAt) >= inputStallThreshold {
		stall := at.Sub(m.lastInputAt)
		motionLive := !m.lastMotionAt.IsZero() && at.Sub(m.lastMotionAt) < inputStallThreshold
		pending := pendingConsoleInputEvents()
		tuiLog("input-watchdog: console input stalled — no KeyMsg/MouseMsg for %s since %s; connStatus=%s flowBlocked=%v viewingChild=%v turnActive=%v motionLive=%v hadRealKey=%v pendingConsoleEvents=%d; use ← → Enter on cards or Ctrl+C",
			stall.Round(time.Second), m.lastInputAt.Format(time.RFC3339),
			m.connStatus, m.flowLoopBlocked(), m.viewingChild(), m.turnIsActive(), motionLive, m.inputHadRealKey, pending)
		m.inputStallLogged = true
		if m.inputStallEligible() {
			m.statusMsg = "input stalled: terminal is not delivering keys — close this window to exit (runner is cleaned up)"
			m.addMessage("system", "Input stalled: the terminal stopped delivering keys to the TUI (the session is otherwise alive). Close this terminal window to exit — the Go runner is terminated automatically.", "gate")
		}
	}
	return m, cmdInputWatchdog()
}
