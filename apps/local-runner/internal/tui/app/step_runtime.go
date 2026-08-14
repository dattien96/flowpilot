package app

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
)

// StepsRuntimeMsg carries a refreshed workflow steps-runtime snapshot.
type StepsRuntimeMsg struct {
	RunID string
	Steps []client.WorkflowStepRuntime
	Err   string
}

func (m *AppModel) shouldPollStepsRuntime() bool {
	if m.runHandle == nil {
		return false
	}
	return m.launch.IsCatalogWorkflow() || m.mode == ModeFlow || m.mode == ModeStep
}

func (m *AppModel) cmdRefreshStepsRuntime() tea.Cmd {
	if !m.shouldPollStepsRuntime() {
		return nil
	}
	runID := m.runHandle.RunID
	cl := m.client
	return func() tea.Msg {
		snap, err := cl.GetWorkflowStepsRuntime(context.Background(), runID)
		if err != nil {
			return StepsRuntimeMsg{RunID: runID, Err: err.Error()}
		}
		return StepsRuntimeMsg{RunID: runID, Steps: snap.Steps}
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
