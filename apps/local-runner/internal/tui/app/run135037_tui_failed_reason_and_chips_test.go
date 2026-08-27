package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-616: hub-less delegate fail must surface the real RejectionNote and
// show blocked chips, without Thinking.
func TestRun135037_TUI_FailedPlannerShowsReasonAndBlockedChips(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.width, m.height = 100, 30
			m.asciiMode = true
			m.mode = ModeFlow
			m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf", Label: pk + "-flow"}
			m.runHandle = &client.RunHandle{RunID: "run-135037", Status: "running"}
			// Simulate blocked delegate_failed like runner now emits.
			m.flowLoopStatus = "blocked"
			m.flowBlockReason = "delegate_failed"
			m.agentRuns = []client.AgentRunSummary{
				{RunID: "run-135037", AgentName: "main", Role: "main", Status: "completed"},
				{RunID: "run-135042", AgentName: "contract-planner", Label: "preflight_contract_plan", Role: "contract-planner", Status: "failed"},
			}
			// Steps: planner FAILED with RejectionNote containing the provider error.
			step := client.WorkflowStepRuntime{StepID: "preflight_contract_plan", NodeID: "preflight_contract_plan", Status: "FAILED", RejectionNote: "The 'gpt-5.4' model is not supported when using Codex with a ChatGPT account."}
			m.flowSteps = []client.WorkflowStepRuntime{step}
			// RejectionNote must be rendered via formatStepChatLine (chat notice) and not lost.
			line := formatStepChatLine(0, step, "")
			if !strings.Contains(line, "gpt-5.4") {
				t.Fatalf("%s: formatStepChatLine must surface RejectionNote gpt-5.4, got %q", pk, line)
			}
			view := stripANSI(m.View())
			if !strings.Contains(view, "[Retry]") || !strings.Contains(view, "[Stop]") {
				t.Fatalf("%s: blocked delegate_failed must render [Continue]/[Stop], got:\n%s", pk, view)
			}
			if m.workIsLive() {
				t.Fatalf("%s: blocked failed must not be workIsLive (Thinking off)", pk)
			}
			if !m.flowLoopBlocked() {
				t.Fatalf("%s: flowLoopBlocked must be true", pk)
			}
		})
	}
}

func TestRun135037_TUI_FailedReasonFallsBackWhenNoteEmpty(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.asciiMode = true
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf", Label: "flow"}
	m.runHandle = &client.RunHandle{RunID: "run-x", Status: "running"}
	m.flowLoopStatus = "blocked"
	m.flowBlockReason = "delegate_failed"
	m.agentRuns = []client.AgentRunSummary{{RunID: "run-x", AgentName: "main", Status: "completed"}}
	m.flowSteps = []client.WorkflowStepRuntime{
		{StepID: "preflight_contract_plan", NodeID: "preflight_contract_plan", Status: "FAILED", RejectionNote: ""},
	}
	// formatStepChatLine fallback path
	line := formatStepChatLine(0, m.flowSteps[0], "The 'gpt-5.4' model is not supported")
	if !strings.Contains(line, "gpt-5.4") {
		t.Fatalf("fallback reason must appear, got %q", line)
	}
	// when both empty, must still show no-detail line (not panic)
	line2 := formatStepChatLine(0, m.flowSteps[0], "")
	if !strings.Contains(line2, "no detail from runner") {
		t.Fatalf("empty reason must show fallback, got %q", line2)
	}
}
