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
