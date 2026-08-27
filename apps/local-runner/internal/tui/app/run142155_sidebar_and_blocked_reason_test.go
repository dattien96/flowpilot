package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-632: run-142155 — two UI gaps on the rag-harness blocked park:
// (1) the right sidebar hard-capped at 8 steps hid the 9th (audit) as
// "… +1 more"; (2) [Continue]/[Stop] rendered without the review reason
// ("no connected local account found for provider \"codex\"").
//
// Sidebar now fills the available height (9 rag-harness steps fully visible
// at height 50) and the blocked bar shows LoopState.GateReason — falling back
// to the FAILED step's RejectionNote — so a decision chip is never shown
// without its reason.

func ragHarnessSteps142155() []client.WorkflowStepRuntime {
	return []client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "preflight_contract_plan", Status: "DONE"},
		{StepID: "s2", NodeID: "preflight_contract_freeze", Status: "DONE"},
		{StepID: "s3", NodeID: "context", Status: "DONE"},
		{StepID: "s4", NodeID: "test_signatures", Status: "DONE"},
		{StepID: "s5", NodeID: "implement", Status: "DONE"},
		{StepID: "s6", NodeID: "validate", Status: "DONE"},
		{StepID: "s7", NodeID: "reviewer", Status: "FAILED", RejectionNote: `no connected local account found for provider "codex"`},
		{StepID: "s8", NodeID: "synthesis", Status: "WAITING_USER_APPROVAL"},
		{StepID: "s9", NodeID: "audit", Status: "PENDING"},
	}
}

func TestRun142155_SidebarShowsAllNineSteps_ClaudeCodexGrok(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.width, m.height = 126, 50
			m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
			m.flowSteps = ragHarnessSteps142155()
			side := strings.Join(m.renderRightSidebar(50), "\n")
			if !strings.Contains(side, "audit") {
				t.Fatalf("[%s] sidebar must show the 9th step audit, got:\n%s", pk, side)
			}
			if strings.Contains(side, "+1 more") {
				t.Fatalf("[%s] sidebar must not truncate 9 steps at height 50, got:\n%s", pk, side)
			}
		})
	}
}

func TestRun142155_SidebarOverflowStillShowsMoreTail(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.flowSteps = ragHarnessSteps142155()
	// Height-aware budget smaller than the step count must still truncate.
	lines := m.flowStepsPanelLinesMax(5)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "… +4 more") {
		t.Fatalf("maxRows=5 must show +4 more, got:\n%s", joined)
	}
	// Legacy fixed-8 view keeps the pre-CA-632 contract for the overlay/tests.
	legacy := strings.Join(m.flowStepsPanelLines(), "\n")
	if !strings.Contains(legacy, "… +1 more") {
		t.Fatalf("legacy flowStepsPanelLines must stay capped at 8, got:\n%s", legacy)
	}
}

func TestRun142155_BlockedBarShowsGateReason_ClaudeCodexGrok(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.width, m.height = 100, 30
			m.asciiMode = true
			m.mode = ModeFlow
			m.flowSteps = ragHarnessSteps142155()
			m.applyAgentGraph(&client.AgentGraphSnapshot{
				LoopState: client.AgentLoopState{
					Status: "blocked", BlockReason: "escalate",
					GateReason: `Review cannot proceed: the Codex reviewer failed with "no connected local account found for provider \"codex\"".`,
				},
				Runs: []client.AgentRunSummary{{RunID: "run-142155", AgentName: "main", Status: "completed"}},
			})
			view := stripANSI(m.View())
			if !strings.Contains(view, "[Continue]") || !strings.Contains(view, "[Stop]") {
				t.Fatalf("[%s] blocked bar must render chips, got:\n%s", pk, view)
			}
			if !strings.Contains(view, "codex") {
				t.Fatalf("[%s] blocked bar must show GateReason with codex, got:\n%s", pk, view)
			}
		})
	}
}

func TestRun142155_BlockedBarFallsBackToRejectionNote(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.asciiMode = true
	m.mode = ModeFlow
	m.flowSteps = ragHarnessSteps142155()
	m.applyAgentGraph(&client.AgentGraphSnapshot{
		LoopState: client.AgentLoopState{Status: "blocked", BlockReason: "escalate"},
		Runs:      []client.AgentRunSummary{{RunID: "run-142155", AgentName: "main", Status: "completed"}},
	})
	if !m.flowLoopBlocked() {
		t.Fatal("must be flowLoopBlocked")
	}
	bar := stripANSI(m.renderBlockedBar())
	if !strings.Contains(bar, "[Continue]") || !strings.Contains(bar, "[Stop]") {
		t.Fatalf("chips must render, got:\n%s", bar)
	}
	if !strings.Contains(bar, `provider "codex"`) {
		t.Fatalf("empty gate must fall back to FAILED RejectionNote, got:\n%s", bar)
	}
}

func TestRun142155_BlockedBarNoReasonOmitsLine(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.asciiMode = true
	m.mode = ModeFlow
	m.flowSteps = []client.WorkflowStepRuntime{{StepID: "s1", NodeID: "reviewer", Status: "FAILED"}}
	m.applyAgentGraph(&client.AgentGraphSnapshot{
		LoopState: client.AgentLoopState{Status: "blocked", BlockReason: "escalate"},
		Runs:      []client.AgentRunSummary{{RunID: "run-142155", AgentName: "main", Status: "completed"}},
	})
	bar := stripANSI(m.renderBlockedBar())
	if !strings.Contains(bar, "[Continue]") || !strings.Contains(bar, "[Stop]") {
		t.Fatalf("chips must render even without a reason, got:\n%s", bar)
	}
	if strings.Contains(bar, "reason:") {
		t.Fatalf("no gate + no note must not fabricate a reason line, got:\n%s", bar)
	}
}

func TestRun142155_LateGateReasonRebanners(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.asciiMode = true
	m.mode = ModeFlow
	m.flowSteps = ragHarnessSteps142155()
	// First snapshot: blocked but gate empty (missed SSE / first poll).
	m.applyAgentGraph(&client.AgentGraphSnapshot{
		LoopState: client.AgentLoopState{Status: "blocked", BlockReason: "escalate"},
		Runs:      []client.AgentRunSummary{{RunID: "run-142155", AgentName: "main", Status: "completed"}},
	})
	if m.flowGateReason != "" {
		t.Fatalf("gate should start empty, got %q", m.flowGateReason)
	}
	// Late snapshot carries the gate — must re-banner so the reason is seen.
	m.applyAgentGraph(&client.AgentGraphSnapshot{
		LoopState: client.AgentLoopState{Status: "blocked", BlockReason: "escalate", GateReason: "codex account missing"},
		Runs:      []client.AgentRunSummary{{RunID: "run-142155", AgentName: "main", Status: "completed"}},
	})
	var last string
	for _, msg := range m.messages {
		if strings.Contains(msg.Content, "blocked") {
			last = msg.Content
		}
	}
	if !strings.Contains(last, "codex account missing") {
		t.Fatalf("late gate must re-banner with reason, last=%q", last)
	}
	if !strings.Contains(stripANSI(m.renderBlockedBar()), "codex account missing") {
		t.Fatal("blocked bar must show late gate reason")
	}
}

func TestRun142155_BlockedBarLongReasonWrapsWithoutTruncation(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 30
	m.asciiMode = true
	m.mode = ModeFlow
	longGate := "flow gate block: flow scope drift: wrote outside the frozen contract's declared paths: internal/tui/app/drift_probe.go, internal/tui/app/another_probe.go"
	m.applyAgentGraph(&client.AgentGraphSnapshot{
		LoopState: client.AgentLoopState{Status: "blocked", BlockReason: "escalate", GateReason: longGate},
		Runs:      []client.AgentRunSummary{{RunID: "run-142155", AgentName: "main", Status: "completed"}},
	})
	bar := stripANSI(m.renderBlockedBar())
	if !strings.Contains(bar, "another_probe.go") {
		t.Fatalf("long reason must not be truncated, got:\n%s", bar)
	}
	if strings.Contains(bar, "…") {
		t.Fatalf("long reason should wrap cleanly without truncation ellipsis, got:\n%s", bar)
	}
	view := stripANSI(m.View())
	if !strings.Contains(view, "another_probe.go") {
		t.Fatalf("full view must display the wrapped long reason without truncation, got:\n%s", view)
	}
}