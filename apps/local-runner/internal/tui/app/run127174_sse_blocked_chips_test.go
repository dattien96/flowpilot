package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// run-127174: blocked audit showed WAITING but no [Continue]/[Stop] and Thinking kept spinning because
// SSE agent_graph_updated used agentGraphSnapshot while TUI only decoded agentGraph.

func waitingSteps() []client.WorkflowStepRuntime {
	return []client.WorkflowStepRuntime{
		{NodeID: "context", Status: "DONE"},
		{NodeID: "implement", Status: "DONE"},
		{NodeID: "validate", Status: "DONE"},
		{NodeID: "audit", Status: "WAITING_USER_APPROVAL"},
	}
}

func TestRun127174_SSEAgentGraphSnapshotShowsBlockedChips(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.width, m.height = 100, 30
			m.asciiMode = true
			m.mode = ModeFlow
			m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf", Label: pk + "-flow"}
			m.runHandle = &client.RunHandle{RunID: "run-127174", Status: "running"}
			m.flowSteps = waitingSteps()
			m.flowStepsActive = "audit"
			m.flowLoopStatus = "running"

			// Simulate SSE event that runner actually emitted: agentGraphSnapshot with blocked+escalate
			ev := client.ProviderEvent{
				ID:   "evt-127989",
				Seq:  12,
				Type: "agent_graph_updated",
				AgentGraphSnapshot: &client.AgentGraphSnapshot{
					ParentRunID: "run-127174",
					LoopState:   client.AgentLoopState{Status: "blocked", BlockReason: "escalate", GateReason: "Audit blocked: feature key missing"},
					Runs:        []client.AgentRunSummary{{RunID: "run-127174", AgentName: "main", Status: "completed"}},
				},
			}
			if ev.EffectiveAgentGraph() == nil {
				t.Fatal("EffectiveAgentGraph nil")
			}
			m2, _ := m.handleEvent(ev)
			am := m2.(*AppModel)
			if am.flowLoopStatus != "blocked" {
				t.Fatalf("%s: flowLoopStatus=%q want blocked", pk, am.flowLoopStatus)
			}
			if am.flowBlockReason != "escalate" {
				t.Fatalf("%s: flowBlockReason=%q want escalate", pk, am.flowBlockReason)
			}
			// Must render Continue/Stop and not be "live" (Thinking off, banner on)
			view := stripANSI(am.View())
			if !strings.Contains(view, "[Retry]") {
				t.Fatalf("%s: blocked view must contain [Continue]:\n%s", pk, view)
			}
			if !strings.Contains(view, "[Stop]") {
				t.Fatalf("%s: blocked view must contain [Stop]:\n%s", pk, view)
			}
			if am.workIsLive() {
				t.Fatalf("%s: blocked WAITING must not be workIsLive (Thinking must stop)", pk)
			}
			if !hasWarnBanner(am.messages) {
				t.Fatalf("%s: blocked must surface warn banner", pk)
			}
		})
	}
}

func TestRun127174_LegacyAgentGraphStillShowsBlocked(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.width, m.height = 100, 30
			m.asciiMode = true
			m.mode = ModeFlow
			m.runHandle = &client.RunHandle{RunID: "run-127174", Status: "running"}
			m.flowSteps = waitingSteps()
			m.flowLoopStatus = "running"
			ev := client.ProviderEvent{
				ID:   "e2",
				Seq:  2,
				Type: "agent_graph_updated",
				AgentGraph: &client.AgentGraphSnapshot{
					ParentRunID: "run-127174",
					LoopState:   client.AgentLoopState{Status: "blocked", BlockReason: "cap"},
				},
			}
			m2, _ := m.handleEvent(ev)
			am := m2.(*AppModel)
			if am.flowLoopStatus != "blocked" {
				t.Fatalf("%s: flowLoopStatus=%q want blocked", pk, am.flowLoopStatus)
			}
			if am.flowBlockReason != "cap" {
				t.Fatalf("%s: flowBlockReason=%q want cap", pk, am.flowBlockReason)
			}
		})
	}
}

func TestRun127174_WaitingStepsHydratesBlockedGraph(t *testing.T) {
	// When SSE missed due to tag, StepsRuntime WAITING should hydrate so banner still appears.
	// We verify the hydrate path exists: StepsRuntimeMsg with WAITING + not blocked queues a hydrate cmd.
	// The cmd itself is tested via the blocked-path above; here just ensure we don't lose WAITING.
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.width, m.height = 100, 30
			m.asciiMode = true
			m.mode = ModeFlow
			m.runHandle = &client.RunHandle{RunID: "run-127174", Status: "running"}
			m.flowLoopStatus = "running"
			m.flowSteps = []client.WorkflowStepRuntime{{NodeID: "audit", Status: "RUNNING"}}
			// Receive WAITING via poll
			m2, cmd := m.Update(StepsRuntimeMsg{RunID: "run-127174", Steps: waitingSteps()})
			am := m2.(*AppModel)
			if am.flowStepsActive != "audit" {
				t.Fatalf("%s: flowStepsActive=%q want audit", pk, am.flowStepsActive)
			}
			// cmd should include hydrate attempts (at least one non-nil)
			if cmd == nil {
				t.Fatalf("%s: StepsRuntime WAITING should queue hydrates", pk)
			}
		})
	}
}
