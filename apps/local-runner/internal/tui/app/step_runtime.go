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
	Err   string
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
	if m.turnStream != nil || m.focusStream != nil {
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
		return StepsRuntimeMsg{RunID: runID, Steps: snap.Steps}
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
	if m.flowHasActiveAgents() {
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
func (m *AppModel) flowLoopBlocked() bool {
	if strings.ToLower(strings.TrimSpace(m.flowLoopStatus)) != "blocked" {
		return false
	}
	if m.flowHasActiveAgents() {
		return false
	}
	if m.turnStream != nil || m.focusStream != nil {
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
	m.flowLoopStatus = g.LoopState.Status
	m.flowBlockReason = g.LoopState.BlockReason
	m.agentRuns = g.Runs
	if m.focusedAgentIdx >= len(m.agentRuns) {
		m.focusedAgentIdx = 0
	}
	m.expandSessionPanelForChildAgents()
	// Banner when the loop just transitioned into a parked awaiting-user state.
	if strings.ToLower(strings.TrimSpace(m.flowLoopStatus)) == "blocked" &&
		!strings.EqualFold(strings.TrimSpace(prevStatus), "blocked") {
		m.showBlockedBanner(g.LoopState)
	}
	m.settleFlowIfDone()
}

// showBlockedBanner surfaces the parked awaiting-user state (Desktop
// FlowAwaitingUserCard parity, BUG-231) so the user knows the flow is waiting
// on their Continue/Stop decision, not stuck running.
func (m *AppModel) showBlockedBanner(ls client.AgentLoopState) {
	reason := strings.TrimSpace(ls.BlockReason)
	line := "Flow is waiting for you (blocked) — /continue to proceed or /stop to end."
	if reason != "" {
		line = fmt.Sprintf("Flow is waiting for you (blocked: %s) — /continue to proceed or /stop to end.", reason)
	}
	m.addMessage("system", line, "warn")
	m.connStatus = ConnWaiting
	m.statusMsg = "awaiting your decision"
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
	if m.turnStream != nil || m.focusStream != nil {
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
