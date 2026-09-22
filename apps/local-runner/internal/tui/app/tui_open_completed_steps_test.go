package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// TestChatOpenedMsg_CompletedFlowArmsOneShotStepsRefresh guards run-189839:
// /open of a terminal workflow must still fetch the step timeline once so the
// F2 panel renders step rows (CA-516). The cursor auto-poll cadence stays gated
// by shouldPollStepsRuntime (CA-508/CA-514) — only the one-shot is unconditional.
func TestChatOpenedMsg_CompletedFlowArmsOneShotStepsRefresh(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.width, m.height = 100, 30
			m.flowWorkflows = []client.Workflow{{ID: "wf-grok", Name: "grok-flow"}}
			m2, _ := m.Update(ChatOpenedMsg{
				Handle: client.RunHandle{
					RunID: "run-189839", RunKind: "workflow",
					WorkflowID: "wf-grok", FlowRef: "wf-grok", Status: "completed",
					ProviderKey: pk,
				},
				Snapshot: client.RunSnapshot{RunID: "run-189839", Status: "completed"},
				HistoryMeta: client.RunHistoryItem{
					RunID: "run-189839", RunKind: "workflow", WorkflowID: "wf-grok",
					Status: "completed", ProviderKey: pk,
				},
				Messages: []ChatMessage{{Role: "user", Content: "fix bug 1+1 != 2"}},
			})
			am := m2.(*AppModel)
			if am.mode != ModeFlow {
				t.Fatalf("mode=%v want ModeFlow", am.mode)
			}
			if !am.stepsPollInFlight {
				t.Fatalf("completed flow open must arm one-shot steps refresh (got inFlight=false)")
			}
			// Auto-poll must stay off: completed + idle must not re-arm every 1.6s.
			if am.shouldPollStepsRuntime() {
				t.Fatal("completed flow open must NOT enable auto-poll (CA-508/CA-514)")
			}
		})
	}
}

// TestChatOpenedMsg_CompletedFlowNoStop_OneShotRefreshArmed combines the CA-508
// no-[stop] contract with the CA-516 one-shot refresh: opening a completed flow
// arms a steps fetch without surfacing [stop] chrome.
func TestChatOpenedMsg_CompletedFlowNoStop_OneShotRefreshArmed(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.flowWorkflows = []client.Workflow{{ID: "wf-grok", Name: "grok-flow"}}
	m2, _ := m.Update(ChatOpenedMsg{
		Handle: client.RunHandle{
			RunID: "run-98153", RunKind: "workflow",
			WorkflowID: "wf-grok", FlowRef: "wf-grok",
			ProviderKey: "grok",
		},
		Snapshot: client.RunSnapshot{RunID: "run-98153", Status: "completed"},
		HistoryMeta: client.RunHistoryItem{
			RunID: "run-98153", RunKind: "workflow", WorkflowID: "wf-grok", Status: "completed",
		},
		Messages: []ChatMessage{{Role: "user", Content: "fix bug"}},
	})
	am := m2.(*AppModel)
	am.orchStream = &orchStreamState{}
	if am.runHandle == nil || am.runHandle.Status != "completed" {
		t.Fatalf("handle status=%v want completed from snapshot", am.runHandle)
	}
	if am.turnIsActive() {
		t.Fatal("opened completed flow must not be turn-active")
	}
	if strings.Contains(stripANSI(am.renderStatusLine()), "[stop]") {
		t.Fatalf("opened completed flow must not show [stop]:\n%s", am.renderStatusLine())
	}
	if !am.stepsPollInFlight {
		t.Fatal("completed flow open must still arm one-shot steps refresh")
	}
}

// TestChatOpenedMsg_CompletedFlowStepsRenderAfterRefresh drives the full CA-516
// path: open a completed flow, deliver the steps snapshot and agent graph, then
// confirm the F2 panel lists step rows with [open] (run-189839 symptom).
func TestChatOpenedMsg_CompletedFlowStepsRenderAfterRefresh(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.width, m.height = 120, 36
	m.asciiMode = true
	m.flowWorkflows = []client.Workflow{{ID: "wf-grok", Name: "grok-flow"}}
	m2, _ := m.Update(ChatOpenedMsg{
		Handle: client.RunHandle{
			RunID: "run-189839", RunKind: "workflow",
			WorkflowID: "wf-grok", FlowRef: "wf-grok", Status: "completed",
			ProviderKey: "grok",
		},
		Snapshot: client.RunSnapshot{RunID: "run-189839", Status: "completed"},
		HistoryMeta: client.RunHistoryItem{
			RunID: "run-189839", RunKind: "workflow", WorkflowID: "wf-grok", Status: "completed",
		},
		Messages: []ChatMessage{{Role: "user", Content: "fix bug 1+1 != 2"}},
	})
	am := m2.(*AppModel)

	// Completed step timeline delivered by the one-shot refresh.
	m3, _ := am.Update(StepsRuntimeMsg{
		RunID: "run-189839",
		Steps: []client.WorkflowStepRuntime{
			{StepID: "s1", NodeID: "coder", AgentRef: "coder", Status: "DONE"},
			{StepID: "s2", NodeID: "reviewer", AgentRef: "reviewer", Status: "DONE"},
		},
	})
	am = m3.(*AppModel)

	// Child agents hydrated → step rows expose [open].
	m4, _ := am.Update(agentRunsHydratedMsg{
		ParentRunID: "run-189839",
		Runs: []client.AgentRunSummary{
			{RunID: "run-189839", AgentName: "main", Role: "main", Status: "completed"},
			{RunID: "run-coder", AgentName: "coder", Status: "completed"},
			{RunID: "run-rev", AgentName: "reviewer", Status: "completed"},
		},
	})
	am = m4.(*AppModel)

	joined := strings.Join(am.flowStepsPanelLines(), "\n")
	if strings.Contains(joined, "Steps 2:") {
		t.Fatalf("F2 must not show the redundant 'Steps N:' count line (CA-542):\n%s", joined)
	}
	if !strings.Contains(joined, "coder") || !strings.Contains(joined, "reviewer") {
		t.Fatalf("F2 must list the completed steps:\n%s", joined)
	}
	if strings.Contains(joined, "[open]") {
		t.Fatalf("F2 step rows must NOT expose [open] chip (keyboard-only via /agents):\n%s", joined)
	}
	if strings.Contains(stripANSI(am.renderStatusLine()), "[stop]") {
		t.Fatalf("completed flow with steps must not show [stop]:\n%s", am.renderStatusLine())
	}
}

// TestChatOpenedMsg_RunningFlowStillArmsRefresh keeps CA-502 behavior: a live
// (running) workflow open arms the one-shot refresh and still auto-polls.
func TestChatOpenedMsg_RunningFlowStillArmsRefresh(t *testing.T) {
	m := New(config.ChatConfig{Provider: "claude"}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.flowWorkflows = []client.Workflow{{ID: "wf-grok", Name: "grok-flow"}}
	m2, _ := m.Update(ChatOpenedMsg{
		Handle: client.RunHandle{
			RunID: "run-98153", RunKind: "workflow",
			WorkflowID: "wf-grok", FlowRef: "wf-grok", Status: "running",
			ProviderKey: "claude",
		},
		Messages: []ChatMessage{{Role: "user", Content: "fix bug 1+1 != 2"}},
	})
	am := m2.(*AppModel)
	if am.mode != ModeFlow {
		t.Fatalf("mode=%v", am.mode)
	}
	if !am.stepsPollInFlight {
		t.Fatal("running flow open must arm one-shot steps refresh")
	}
	if !am.shouldPollStepsRuntime() {
		t.Fatal("running workflow open must keep auto-poll")
	}
}
