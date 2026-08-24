package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestRun135037_TUI_FirstPollFailedWithNote(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			prev := []client.WorkflowStepRuntime{}
			next := []client.WorkflowStepRuntime{
				{StepID: "preflight_contract_plan", NodeID: "preflight_contract_plan", Status: "FAILED", RejectionNote: "The 'gpt-5.4' model is not supported when using Codex with a ChatGPT account."},
			}
			got := formatStepChatNotices(prev, next, "", "", "")
			if len(got) != 1 || !strings.Contains(got[0], "gpt-5.4") {
				t.Fatalf("%s: first-poll FAILED must surface note, got %v", pk, got)
			}
		})
	}
}

func TestRun135037_TUI_LateNoteAfterFailedEmpty(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			prev := []client.WorkflowStepRuntime{
				{StepID: "preflight_contract_plan", NodeID: "preflight_contract_plan", Status: "FAILED", RejectionNote: ""},
			}
			next := []client.WorkflowStepRuntime{
				{StepID: "preflight_contract_plan", NodeID: "preflight_contract_plan", Status: "FAILED", RejectionNote: "The 'gpt-5.4' model is not supported"},
			}
			got := formatStepChatNotices(prev, next, "", "", "")
			if len(got) != 1 || !strings.Contains(got[0], "gpt-5.4") {
				t.Fatalf("%s: late note FAILED->FAILED must surface, got %v", pk, got)
			}
		})
	}
}

func TestRun135037_TUI_FailedRunningStillOneLine(t *testing.T) {
	prev := []client.WorkflowStepRuntime{
		{StepID: "b", NodeID: "grok-coder", Status: "RUNNING"},
	}
	next := []client.WorkflowStepRuntime{
		{StepID: "b", NodeID: "grok-coder", Status: "FAILED", RejectionNote: "tool timed out"},
	}
	got := formatStepChatNotices(prev, next, "grok-coder", "", "")
	if len(got) != 1 {
		t.Fatalf("RUNNING->FAILED must still be 1 line, got %v", got)
	}
}

func TestRun135037_TUI_BannerShowsGateReason(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.applyAgentGraph(&client.AgentGraphSnapshot{
				LoopState: client.AgentLoopState{Status: "blocked", BlockReason: "delegate_failed", GateReason: "The 'gpt-5.4' model is not supported when using Codex with a ChatGPT account."},
				Runs: []client.AgentRunSummary{{RunID: "run-1", Status: "completed"}},
			})
			// banner is the last system message
			found := false
			for _, msg := range m.messages {
				if strings.Contains(msg.Content, "delegate_failed") && strings.Contains(msg.Content, "gpt-5.4") {
					found = true
				}
			}
			if !found {
				t.Fatalf("%s: banner must contain delegate_failed + gpt-5.4, messages=%v", pk, m.messages)
			}
		})
	}
}
