package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// Opening a finished flow still attaches orch SSE for late events, but [stop]
// must not appear (run-98153 reopen).

func TestTurnIsActive_CompletedFlowOrchDoesNotArmStop(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf", Label: "grok-flow"}
	m.runHandle = &client.RunHandle{RunID: "run-98153", Status: "completed"}
	m.connStatus = ConnIdle
	m.orchStream = &orchStreamState{}
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-98153", AgentName: "main", Role: "main", Status: "completed"},
		{RunID: "run-98158", AgentName: "coder", Label: "grok-coder", Status: "completed"},
	}
	if m.turnIsActive() {
		t.Fatal("completed flow + orch listener must not arm [stop]")
	}
	if strings.Contains(stripANSI(m.renderStatusLine()), "[stop]") {
		t.Fatalf("status must not show [stop]:\n%s", m.renderStatusLine())
	}
}

func TestChatOpenedMsg_CompletedFlowDoesNotShowStop(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m2, _ := m.Update(ChatOpenedMsg{
		Handle: client.RunHandle{
			RunID: "run-98153", RunKind: "workflow", WorkflowID: "wf-grok",
			// Resume may omit status; snapshot is ground truth.
		},
		Snapshot: client.RunSnapshot{RunID: "run-98153", Status: "completed"},
		HistoryMeta: client.RunHistoryItem{
			RunID: "run-98153", RunKind: "workflow", WorkflowID: "wf-grok", Status: "completed",
		},
		Messages: []ChatMessage{{Role: "user", Content: "fix bug"}},
	})
	am := m2.(*AppModel)
	// Simulate orch attach that ChatOpened schedules (listener only).
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
}

func TestTurnIsActive_RunningFlowOrchStillArmsStop(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m.runHandle = &client.RunHandle{RunID: "run-live", Status: "running"}
	m.connStatus = ConnIdle
	m.orchStream = &orchStreamState{}
	if !m.turnIsActive() {
		t.Fatal("running flow orch must still arm [stop]")
	}
}
