package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
)

// StepsRuntimeMsg carries a refreshed workflow steps-runtime snapshot.
type StepsRuntimeMsg struct {
	RunID string
	Steps []client.WorkflowStepRuntime
	// Provider/Model are the run-level posture from the snapshot body
	// (Task-322): per-step fallback when a step has no own provider/model.
	Provider string
	Model    string
	Err      string
}

const (
	stepsRuntimePollTimeout = 4 * time.Second
	// After this many consecutive dial/timeout poll failures, pause auto-poll
	// until the user starts/opens a run again (avoids flooding a dead runner).
	runnerPollFailPauseAfter = 3
)

func (m *AppModel) shouldPollStepsRuntime() bool {
	if m.runHandle == nil {
		return false
	}
	if !(m.launch.IsCatalogWorkflow() || m.mode == ModeFlow || m.mode == ModeStep) {
		return false
	}
	// Dead runner: stop auto-poll so the TUI stays interactive.
	if m.runnerPollFailStreak >= runnerPollFailPauseAfter {
		return false
	}
	// Live handle (e.g. /open of still-running workflow) even if ConnIdle chrome.
	// Empty status is treated as non-terminal (unknown) — poll until snapshot says done.
	if !runStatusIsTerminal(m.runHandle.Status) {
		return true
	}
	// Terminal handle: only while a turn/child stream is actually live.
	// NOTE: orchStream alone is NOT enough — /open always attaches orch as a
	// late-event listener (CA-502), and CA-513 added agent hydrate on every poll.
	// That combination flooded a dead runner and froze the TUI after open/flow work.
	if m.connStatus == ConnRunning || m.connStatus == ConnWaiting || m.connStatus == ConnConnecting {
		return true
	}
	if m.flowHasActiveAgents() {
		return true
	}
	if m.turnStream != nil || m.focusedChildLive() {
		return true
	}
	return false
}

// cmdRefreshStepsRuntime is a one-shot (or forced) steps fetch. Auto-poll cadence
// is gated by shouldPollStepsRuntime in the cursor tick; this cmd still works for
// /open and event-driven refreshes on idle terminal chrome.
func (m *AppModel) cmdRefreshStepsRuntime() tea.Cmd {
	if m.runHandle == nil {
		return nil
	}
	if !(m.launch.IsCatalogWorkflow() || m.mode == ModeFlow || m.mode == ModeStep) {
		return nil
	}
	if m.stepsPollInFlight {
		return nil
	}
	m.stepsPollInFlight = true
	runID := m.runHandle.RunID
	cl := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), stepsRuntimePollTimeout)
		defer cancel()
		snap, err := cl.GetWorkflowStepsRuntime(ctx, runID)
		if err != nil {
			return StepsRuntimeMsg{RunID: runID, Err: err.Error()}
		}
		return StepsRuntimeMsg{RunID: runID, Steps: snap.Steps, Provider: snap.Provider, Model: snap.Model}
	}
}

// runnerDialDeadErr is true for TCP-level dial/reset failures that mean the
// runner process itself is unreachable. "context deadline exceeded" and bare
// "i/o timeout" are NOT in this set: a live runner behind a slow Supabase
// catalog surfaces those exact strings when the client ctx/read budget runs
// out, so they must be classified as catalog-slow (retryable) and never as
// "runner not responding" (CA-514).
func runnerDialDeadErr(errText string) bool {
	s := strings.ToLower(strings.TrimSpace(errText))
	if s == "" {
		return false
	}
	return strings.Contains(s, "connection refused") ||
		strings.Contains(s, "connectex") ||
		strings.Contains(s, "dial tcp") ||
		strings.Contains(s, "connection reset") ||
		// Windows WSAECONNRESET (10054) surfaces as wsarecv/wsasend "An existing
		// connection was forcibly closed by the remote host" — an active reset,
		// the same dead-connection signal as Linux "connection reset by peer".
		strings.Contains(s, "forcibly closed by the remote host") ||
		strings.Contains(s, "no such host") ||
		strings.Contains(s, "network is unreachable")
}

// runnerUnreachableErr is the broad heuristic for poll-streak accounting: a
// timed-out steps/agent poll against a hung runner is still a failed poll, so
// it must count toward the auto-poll pause. Use runnerDialDeadErr when the
// question is "is the runner process actually unreachable?" (catalog paths).
func runnerUnreachableErr(errText string) bool {
	if runnerDialDeadErr(errText) {
		return true
	}
	s := strings.ToLower(strings.TrimSpace(errText))
	return strings.Contains(s, "context deadline exceeded") ||
		strings.Contains(s, "i/o timeout")
}

func (m *AppModel) noteRunnerPollResult(errText string) {
	if runnerUnreachableErr(errText) {
		m.runnerPollFailStreak++
		if m.runnerPollFailStreak == runnerPollFailPauseAfter {
			m.statusMsg = "runner offline — restart runner"
			m.connStatus = ConnError
		}
		return
	}
	if strings.TrimSpace(errText) == "" {
		m.runnerPollFailStreak = 0
	}
}

// flowLoopDone reports the orchestrator loop reached a terminal "done" state
// with no step still active and no agent still live. This is the authoritative
// "flow finished" signal even when the raw handle status still says "running"
// (interactive_handlers.historyStatusForLiveRun comment: hub provider runs stay
// "running" until the last SSE settles) — run-189839 showed [stop] + streaming…
// forever after all steps DONE / children completed / loop done.
func (m *AppModel) flowLoopDone() bool {
	if strings.ToLower(strings.TrimSpace(m.flowLoopStatus)) != "done" {
		return false
	}
	if strings.TrimSpace(m.flowStepsActive) != "" {
		return false
	}
	if m.hasLiveWorkingChild() {
		return false
	}
	return true
}

// flowLoopBlocked reports the loop is deliberately parked awaiting a user
// decision (BUG-231, run-189839) with no child still live. A blocked loop is a
// non-terminal "awaiting user" pause (escalate, or the round cap reached) — it
// must NOT read as live-running, or [stop] arms with no way for the very user
// the flow is waiting on to respond. Desktop parity: a running child still wins
// over a blocked loop (BUG-231 legacy), so a live child keeps [stop] armed.
// WAITING_USER_APPROVAL is a park stamp, not live work (run-136749).
func (m *AppModel) flowLoopBlocked() bool {
	if strings.ToLower(strings.TrimSpace(m.flowLoopStatus)) != "blocked" {
		return false
	}
	// YOLO/ask_user gate has its own Approve/Deny chips — do not show Continue.
	if m.approval != nil || m.question != nil || m.gate != nil {
		return false
	}
	if m.hasLiveWorkingChild() {
		return false
	}
	if m.focusedChildLive() {
		return false
	}
	return true
}

// applyAgentGraph records the latest loop state + agent runs from an agent
// graph snapshot and refreshes the awaiting-user banner. The caller guarantees
// the snapshot belongs to the current run (stale-parent guard).
func (m *AppModel) applyAgentGraph(g *client.AgentGraphSnapshot) {
	if g == nil {
		return
	}
	prevStatus := m.flowLoopStatus
	prevGateReason := m.flowGateReason
	m.flowLoopStatus = g.LoopState.Status
	m.flowBlockReason = g.LoopState.BlockReason
	m.flowGateReason = strings.TrimSpace(g.LoopState.GateReason)
	// Task-322: persist round/cap for the steps header "round R/C" chip.
	// Display uses Cap with RoundCap fallback (client.AgentLoopState contract).
	m.flowLoopRound = g.LoopState.Round
	m.flowLoopCap = g.LoopState.Cap
	if m.flowLoopCap == 0 {
		m.flowLoopCap = g.LoopState.RoundCap
	}
	m.agentRuns = g.Runs
	m.afterAgentRunsAdopted()
	if m.hasChildAgentRuns() {
		m.agentHydrateRetries = 0
	}
	// Banner when the loop just transitioned into a parked awaiting-user state,
	// or when a blocked loop finally received its GateReason (late gate —
	// run-142155: the park reason arrives with the same snapshot as the park,
	// but a missed SSE + poll can surface blocked first with an empty reason).
	if strings.ToLower(strings.TrimSpace(m.flowLoopStatus)) == "blocked" &&
		(!strings.EqualFold(strings.TrimSpace(prevStatus), "blocked") || (prevGateReason == "" && m.flowGateReason != "")) {
		m.showBlockedBanner(g.LoopState)
	}
	if strings.ToLower(strings.TrimSpace(m.flowLoopStatus)) == "done" {
		m.settleFlowIfDone()
	}
}

// showBlockedBanner surfaces the parked awaiting-user state (Desktop
// FlowAwaitingUserCard parity, BUG-231) so the user knows the flow is waiting
// on their Continue/Stop decision, not stuck running.
func (m *AppModel) showBlockedBanner(ls client.AgentLoopState) {
	reason := strings.TrimSpace(ls.BlockReason)
	gate := strings.TrimSpace(ls.GateReason)
	// Task-309: list only the chips actually rendered above the composer.
	// Allow appears only for frozen-contract scope drift (and not cap /
	// member_stalled), so a hub_stalled/cap park must not advertise it.
	chips := "Retry / Stop"
	if !strings.EqualFold(reason, "cap") && !strings.EqualFold(reason, "member_stalled") {
		if len(parseDriftedPaths(gate)) > 0 {
			chips += " / Allow"
		}
	}
	base := fmt.Sprintf("Flow is waiting for you (blocked) — click %s above, or /continue or /stop", chips)
	if reason != "" {
		base = fmt.Sprintf("Flow is waiting for you (blocked: %s) — click %s above, or /continue or /stop", reason, chips)
	}
	line := base + "."
	// CA-617 surfaced gate for delegate_failed; CA-619 extends to escalate/cap
	// so the review reason is visible without opening F2 (run-136749).
	if gate != "" && (reason == "delegate_failed" || reason == "escalate" || reason == "cap") {
		gate = truncateRunes(gate, 120)
		line = strings.TrimSuffix(base, ".") + " — " + strings.TrimSuffix(gate, ".") + "."
	} else if gate != "" && reason == "" {
		gate = truncateRunes(gate, 120)
		line = base + " — " + strings.TrimSuffix(gate, ".")
	}
	m.addMessage("system", line, "warn")
	m.connStatus = ConnWaiting
	m.statusMsg = "awaiting your decision"
}

// parseDriftedPaths extracts the scope-drift paths from a gate reason of the
// form "flow scope drift: wrote outside the frozen contract's declared paths: a.go, b.go".
// It mirrors gate_hook.go:863 and Task-309 T-1. Returns nil when not a drift gate or no paths.
func parseDriftedPaths(gate string) []string {
	const marker = "wrote outside the frozen contract's declared paths:"
	idx := strings.Index(gate, marker)
	if idx < 0 {
		return nil
	}
	rest := gate[idx+len(marker):]
	rest = strings.TrimSpace(rest)
	rest = strings.TrimSuffix(rest, ".")
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return nil
	}
	parts := strings.Split(rest, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.TrimSuffix(p, ".")
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// isMissingChangeAuditNoteGate reports whether a gate reason is the audit
// tier-3 / r-ca missing change-audit-note block (run-202550). Retry on that
// park continues by re-running the writer to write the note — not "old
// scope" — so the chip copy must say so.
func isMissingChangeAuditNoteGate(gate string) bool {
	return strings.Contains(strings.ToLower(gate), "no change-audit note")
}

// renderBlockedBar renders the awaiting-user action chips above the composer
// (Desktop FlowAwaitingUserCard parity, BUG-231) when the flow loop is parked
// on the user's Continue/Stop decision. Mirrors the Approve/Deny + attention
// chip pattern: no slash command needed — [Retry] unparks via
// agent-loop/continue (run again with old scope), [Stop] ends the parked loop,
// and when the gate is a frozen-contract scope drift, [Allow] widens the freeze
// via agent-loop/amend (continue with new scope match code changed) per Task-309.
// run-202550: on a missing change-audit-note park, [Retry] instead continues by
// re-running the writer to write the note, and the copy says exactly that.
func (m *AppModel) renderBlockedBar() string {
	if !m.flowLoopBlocked() {
		return ""
	}
	width := safeTermWidth(m.chatWidth())
	if width < 1 {
		if m.chatWidth() > 0 {
			width = m.chatWidth()
		} else {
			width = 80
		}
	}
	reason := strings.TrimSpace(m.flowBlockReason)
	head := styleGate.Render("flow") + " " + styleLink.Render("[blocked]") + " " +
		styleSystem.Render("awaiting your decision")
	if reason != "" {
		head += styleSystem.Render(" (" + reason + ")")
	}
	bar := head + "\n"
	if detail := m.blockedDecisionReason(); detail != "" {
		lead := "  reason: "
		indent := "          "
		avail := max(10, width-len([]rune(lead)))
		wrapped := wrapText(detail, avail)
		for i, wline := range wrapped {
			prefix := indent
			if i == 0 {
				prefix = lead
			}
			bar += styleSystem.Render(prefix+wline) + "\n"
		}
	}
	isCap := strings.EqualFold(strings.TrimSpace(m.flowBlockReason), "cap")
	isStalled := strings.EqualFold(strings.TrimSpace(m.flowBlockReason), "member_stalled")
	drifted := parseDriftedPaths(m.blockedDecisionReason())
	showAllow := !isCap && !isStalled && len(drifted) > 0

	var options []string
	retryHi := m.actionRingHighlighted("blocked", 0)
	stopHi := m.actionRingHighlighted("blocked", 1)
	// run-202550: a missing change-audit-note park continues by re-running
	// the writer to write the note — say so instead of "old scope". Every
	// other park keeps the established copy (pinned by
	// TestBlockedBar_RetryStopAlways_AllowOnlyOnDrift).
	retryDesc := " - run again with old scope"
	if isMissingChangeAuditNoteGate(m.blockedDecisionReason()) {
		retryDesc = " - continue: re-run the writer to write the missing change-audit note"
	}
	options = append(options, styleSystem.Render("  ")+renderActionRingChip("[Retry]", retryHi)+styleSystem.Render(retryDesc))
	options = append(options, styleSystem.Render("  ")+renderActionRingChip("[Stop]", stopHi)+styleSystem.Render(" - end flow"))
	if showAllow {
		allowHi := m.actionRingHighlighted("blocked", 2)
		options = append(options, styleSystem.Render("  ")+renderActionRingChip("[Allow]", allowHi)+styleSystem.Render(" - continue with new scope (match code changed)"))
	}
	// Task-325 UX (live-found run-594636): operators didn't know human
	// feedback rides "/continue <note>" — approve/retry paths need no text,
	// so the affordance was invisible. [Revise] prefills the composer with
	// "/continue " (type the note + Enter); it never continues by itself.
	// Highlight index tracks the ring order [Retry Stop (Allow) Revise]:
	// Revise sits at 2, or 3 when Allow is shown (live-found run-584646
	// follow-up: hardcoded 3 left Revise unhighlightable without drift,
	// so Tab appeared to die on [Stop]).
	reviseIdx := 2
	if showAllow {
		reviseIdx = 3
	}
	reviseHi := m.actionRingHighlighted("blocked", reviseIdx)
	options = append(options, styleSystem.Render("  ")+renderActionRingChip("[Revise]", reviseHi)+styleSystem.Render(" - type feedback note, revise"))
	bar += strings.Join(options, "\n")
	bar += "\n" + styleSystem.Render("  ← → select · Enter · type note + Enter to revise · /stop")
	return bar
}

// blockedDecisionReason returns the human explanation for a parked
// Continue/Stop decision: LoopState.GateReason first (run-142155: "Review
// cannot proceed: the Codex reviewer failed with ..."), else the first FAILED
// step's RejectionNote (stamped by the runner for cohort fails too since
// CA-632). Empty when the runner provided no detail.
func (m *AppModel) blockedDecisionReason() string {
	if g := strings.TrimSpace(m.flowGateReason); g != "" {
		return g
	}
	for _, s := range m.flowSteps {
		if strings.EqualFold(strings.TrimSpace(s.Status), "FAILED") {
			if n := strings.TrimSpace(s.RejectionNote); n != "" {
				return n
			}
		}
	}
	return ""
}

// settleFlowIfDone flips a finished flow to a clean ConnIdle "done" state so
// the statusline drops [stop] and no longer shows streaming…/flow running…
// once the loop is done, every step is terminal, and no agent is active.
// A live turn stream (follow-up chat turn after loop done) or a child focus
// transcript is left untouched — only the settled chrome is updated.
func (m *AppModel) settleFlowIfDone() {
	if !m.isFlowChrome() || !m.flowLoopDone() {
		return
	}
	// run-107774: a follow-up chat turn sets ConnRunning + "thinking…" in
	// processInput and arms turnSendPending, but the SSE stream only opens later
	// (turnStreamOpenedMsg clears it). A steps poll landing in that gap would see
	// flowLoopDone() + turnStream==nil and flash the chrome to "done" before the
	// reply even starts. A turn send in flight is live work — never settle.
	if m.turnSendPending {
		return
	}
	if m.turnStream != nil || m.focusedChildLive() {
		return
	}
	m.connStatus = ConnIdle
	m.statusMsg = "done"
	if m.runHandle != nil && !runStatusIsTerminal(m.runHandle.Status) {
		m.runHandle.Status = "completed"
	}
}

func activeStepName(steps []client.WorkflowStepRuntime) string {
	for _, s := range steps {
		st := strings.ToUpper(strings.TrimSpace(s.Status))
		if st == "RUNNING" || st == "WAITING_USER_APPROVAL" {
			return stepDisplayName(s)
		}
	}
	return ""
}

func stepDisplayName(s client.WorkflowStepRuntime) string {
	name := strings.TrimSpace(s.NodeID)
	if name == "" {
		name = strings.TrimSpace(s.StepType)
	}
	if name == "" {
		name = shortID(s.StepID)
	}
	return name
}

func formatStepChatLine(index int, s client.WorkflowStepRuntime, fallbackFailReason string) string {
	st := strings.ToUpper(strings.TrimSpace(s.Status))
	mark := " "
	switch st {
	case "RUNNING", "WAITING_USER_APPROVAL":
		mark = ">"
	case "DONE":
		mark = "+"
	case "FAILED":
		mark = "x"
	case "SKIPPED", "CANCELED":
		mark = "-"
	}
	line := fmt.Sprintf("%s %2d. [%s] %s", mark, index+1, st, stepDisplayName(s))
	if s.RetryCount > 0 {
		line += fmt.Sprintf(" (retry %d)", s.RetryCount)
	}
	if st == "FAILED" {
		reason := strings.TrimSpace(s.RejectionNote)
		if reason == "" {
			reason = strings.TrimSpace(fallbackFailReason)
		}
		if reason != "" {
			line += "\n  reason: " + reason
		} else {
			line += "\n  reason: (no detail from runner — check /status or Desktop timeline)"
		}
	}
	return line
}

// formatStepChatNotices returns short chat lines for the current step only
// (full step list stays in the top-right session panel).
func formatStepChatNotices(prev, next []client.WorkflowStepRuntime, prevActive, nextActive, fallbackFailReason string) []string {
	if len(next) == 0 {
		return nil
	}
	prevByID := make(map[string]client.WorkflowStepRuntime, len(prev))
	for _, s := range prev {
		prevByID[s.StepID] = s
	}

	var out []string
	// Terminal transitions for steps that left RUNNING/WAITING (e.g. FAILED).
	for i, s := range next {
		st := strings.ToUpper(strings.TrimSpace(s.Status))
		if st != "FAILED" && st != "DONE" && st != "SKIPPED" && st != "CANCELED" {
			continue
		}
		old, ok := prevByID[s.StepID]
		if !ok {
			continue
		}
		oldSt := strings.ToUpper(strings.TrimSpace(old.Status))
		if oldSt != "RUNNING" && oldSt != "WAITING_USER_APPROVAL" {
			continue
		}
		if oldSt == st {
			continue
		}
		// Prefer FAILED (and other terminals) in chat; skip DONE spam unless it was the active focus.
		if st == "DONE" && prevActive != "" && stepDisplayName(old) != prevActive && stepDisplayName(s) != prevActive {
			continue
		}
		out = append(out, formatStepChatLine(i, s, fallbackFailReason))
	}
	// CA-618: late RejectionNote, first-poll FAILED, and PENDING→FAILED.
	// First-poll (prev empty, e.g. /open): cap to at most one FAILED+note — the
	// last FAILED in declaration order that has no later RUNNING sibling.
	if len(prev) == 0 {
		lastIdx := -1
		for i, s := range next {
			if strings.ToUpper(strings.TrimSpace(s.Status)) != "FAILED" {
				continue
			}
			if strings.TrimSpace(s.RejectionNote) == "" {
				continue
			}
			lastIdx = i
		}
		if lastIdx >= 0 {
			// If a later step is still RUNNING/WAITING, the FAILED is history past
			// the active work — do not dump old-park reasons on open.
			hasLaterActive := false
			for j := lastIdx + 1; j < len(next); j++ {
				stj := strings.ToUpper(strings.TrimSpace(next[j].Status))
				if stj == "RUNNING" || stj == "WAITING_USER_APPROVAL" {
					hasLaterActive = true
					break
				}
			}
			if !hasLaterActive {
				out = append(out, formatStepChatLine(lastIdx, next[lastIdx], fallbackFailReason))
			}
		}
	} else {
		for i, s := range next {
			if strings.ToUpper(strings.TrimSpace(s.Status)) != "FAILED" {
				continue
			}
			if strings.TrimSpace(s.RejectionNote) == "" {
				continue
			}
			old, ok := prevByID[s.StepID]
			if !ok {
				continue
			}
			oldSt := strings.ToUpper(strings.TrimSpace(old.Status))
			if oldSt == "FAILED" && strings.TrimSpace(old.RejectionNote) == "" {
				out = append(out, formatStepChatLine(i, s, fallbackFailReason))
				continue
			}
			if oldSt != "RUNNING" && oldSt != "WAITING_USER_APPROVAL" && oldSt != "FAILED" {
				// PENDING (or unknown) → FAILED+note: poll skipped RUNNING.
				out = append(out, formatStepChatLine(i, s, fallbackFailReason))
			}
		}
	}

	// New current RUNNING/WAITING step.
	if nextActive != "" && nextActive != prevActive {
		for i, s := range next {
			st := strings.ToUpper(strings.TrimSpace(s.Status))
			if (st == "RUNNING" || st == "WAITING_USER_APPROVAL") && stepDisplayName(s) == nextActive {
				out = append(out, formatStepChatLine(i, s, ""))
				break
			}
		}
	}
	return out
}
