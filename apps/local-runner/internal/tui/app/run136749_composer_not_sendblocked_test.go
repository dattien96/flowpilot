package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-620: blocked escalate park with ConnWaiting must NOT block composer.
// WAITING stamp (run-136749) previously hid chips and kept sendBlocked true
// (prefix "next" + "turn in progress" toast). Chips now appear and input stays
// enabled for /continue, and Alt+V still dispatches clipboard.

func blockedEscalateWithWaitingModel() *AppModel {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.asciiMode = true
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf", Label: "rag-harness"}
	m.runHandle = &client.RunHandle{RunID: "run-136749", Status: "running"}
	m.flowLoopStatus = "blocked"
	m.flowBlockReason = "escalate"
	m.connStatus = ConnWaiting
	m.statusMsg = "awaiting your decision"
	m.flowSteps = []client.WorkflowStepRuntime{
		{NodeID: "implement", Status: "WAITING_USER_APPROVAL"},
	}
	m.flowStepsActive = "implement"
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-136749", AgentName: "main", Role: "main", Status: "completed"},
		{RunID: "run-136750", AgentName: "coder", Label: "implement", Status: "waiting_user_approval"},
	}
	// Simulate banner having set ConnWaiting
	m.sessionLoading = false
	return m
}

func TestCA620_BlockedPark_DoesNotBlockSend(t *testing.T) {
	m := blockedEscalateWithWaitingModel()
	if !m.flowLoopBlocked() {
		t.Fatal("must be flowLoopBlocked")
	}
	if m.sendBlocked() {
		t.Fatal("sendBlocked must be false when park blocked (awaiting Continue/Stop)")
	}
	// Normal /continue slash must still work (handled before sendBlocked)
	if _, cmd := m.handleSlashCommand("/continue"); cmd == nil {
		t.Fatal("/continue must return cmd even when blocked park")
	}
}

func TestCA620_BlockedPark_StillShowsChipsAndNoThinking(t *testing.T) {
	m := blockedEscalateWithWaitingModel()
	view := stripANSI(m.View())
	if !strings.Contains(view, "[Continue]") {
		t.Fatalf("blocked park must show [Continue], got:\n%s", view)
	}
	if m.workIsLive() {
		t.Fatal("workIsLive must be false for parked WAITING (Thinking off)")
	}
	if m.turnIsActive() {
		t.Fatal("turnIsActive must be false for parked WAITING")
	}
}

func TestCA620_NormalTurnStillBlocksSend(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.asciiMode = true
	m.mode = ModeFlow
	m.runHandle = &client.RunHandle{RunID: "run-1", Status: "running"}
	m.connStatus = ConnRunning
	m.flowLoopStatus = "running"
	if !m.sendBlocked() {
		t.Fatal("ConnRunning without park must be sendBlocked")
	}
}

func TestCA620_AltV_StillDispatchesWhenParkBlocked(t *testing.T) {
	m := blockedEscalateWithWaitingModel()
	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v"), Alt: true})
	_ = m2
	if cmd == nil {
		t.Fatal("Alt+V must return clipboard cmd even when park blocked")
	}
	// Should not have inserted a literal "v"
	am := m2.(*AppModel)
	if strings.Contains(am.inputValue, "v") && am.inputValue == "v" {
		t.Fatalf("Alt+V must not insert literal v, got %q", am.inputValue)
	}
}

func TestCA620_AltV_DoesNotNeedSendBlocked(t *testing.T) {
	// Ensure Alt+V path is via handleKey, not processInput sendBlocked
	m := blockedEscalateWithWaitingModel()
	// Even if sendBlocked were true, Alt+V must still work — handleKey is before sendBlocked
	m.connStatus = ConnRunning // would normally be sendBlocked if not for park fix
	// But park makes sendBlocked false, so double-check both
	if m.sendBlocked() {
		t.Fatal("park should make sendBlocked false, so Alt+V test is meaningful")
	}
	m2, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v"), Alt: true})
	_ = m2
	if cmd == nil {
		t.Fatal("Alt+V must still dispatch when park")
	}
}

func TestCA620_YOLOApprovalStillBlocksSend(t *testing.T) {
	m := blockedEscalateWithWaitingModel()
	m.approval = &ApprovalState{ID: "ap-1", Kind: "exec", Command: "go test"}
	// approval present makes flowLoopBlocked false → sendBlocked true
	if m.flowLoopBlocked() {
		t.Fatal("approval present must make flowLoopBlocked false")
	}
	if !m.sendBlocked() {
		t.Fatal("approval present must be sendBlocked")
	}
}
