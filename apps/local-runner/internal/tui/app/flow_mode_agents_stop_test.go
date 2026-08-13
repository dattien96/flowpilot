package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestStatusLine_FlowModeKeepsAccount(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.5"}, "http://127.0.0.1:4317")
	m.width = 72
	m.mode = ModeFlow
	m.launch = LaunchArm{Label: "flow-claude-2-review", Mode: ModeFlow, WorkflowID: "wf"}
	m.accountLabel = "grok-ready"
	m.account = &client.ProviderAccountSummary{ProviderKey: "grok", DisplayLabel: "grok-ready", IsActive: true}
	m.flowStepsActive = "my-reviewer"
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-main", AgentName: "main", Role: "main", Status: "running"},
		{RunID: "run-rev", AgentName: "my-reviewer", Status: "running"},
	}
	got := stripANSI(m.renderStatusLine())
	if !strings.Contains(got, "grok-ready") {
		t.Fatalf("flow statusline dropped current account:\n%s", got)
	}
}

func TestTurnIsActive_FlowChildrenKeepStop(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.runHandle = &client.RunHandle{RunID: "run-91842"}
	m.connStatus = ConnIdle
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-91842", AgentName: "main", Role: "main", Status: "running"},
		{RunID: "run-92245", AgentName: "my-reviewer", Status: "running"},
	}
	if !m.turnIsActive() {
		t.Fatal("flow with running children must keep [stop] armed")
	}
	if !strings.Contains(stripANSI(m.renderStatusLine()), "[stop]") {
		t.Fatalf("missing [stop]:\n%s", m.renderStatusLine())
	}
}

func TestCmdFocusAgent_DisablesSendAndRestoresMain(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.runHandle = &client.RunHandle{RunID: "run-main"}
	m.addMessage("assistant", "hub text", "")
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-main", AgentName: "main", Role: "main", Status: "running"},
		{RunID: "run-child", AgentName: "my-reviewer", Status: "running"},
	}
	_ = m.cmdFocusAgent("run-child")
	if !m.viewingChild() {
		t.Fatal("expected child viewport")
	}
	if m.canSend() {
		t.Fatal("child transcript must be read-only")
	}
	m2, _ := m.processInput("please continue")
	am := m2.(*AppModel)
	joined := strings.Join(am.renderMessages(), "\n")
	if !strings.Contains(joined, "Child transcript is read-only") {
		t.Fatalf("missing read-only send block:\n%s", joined)
	}
	if !strings.Contains(strings.Join(m.renderMessages(), "\n"), "Child transcript") {
		t.Fatal("missing child chrome")
	}
	m.restoreMainTranscript()
	if m.viewingChild() {
		t.Fatal("expected main viewport")
	}
	mainJoined := strings.Join(m.renderMessages(), "\n")
	if !strings.Contains(mainJoined, "hub text") {
		t.Fatalf("main transcript lost:\n%s", mainJoined)
	}
}

func TestSlashAgent_OpensNamedChild(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m.runHandle = &client.RunHandle{RunID: "run-main"}
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-main", AgentName: "main", Role: "main", Status: "running"},
		{RunID: "run-child", AgentName: "my-reviewer", Status: "running"},
	}
	m2, cmd := m.handleSlashCommand("/agent my-reviewer")
	am := m2.(*AppModel)
	if cmd == nil {
		t.Fatal("expected child stream cmd")
	}
	if am.focusRunID != "run-child" {
		t.Fatalf("focusRunID=%q", am.focusRunID)
	}
}

func TestStoppedMsg_ClearsOrchAndFocus(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.runHandle = &client.RunHandle{RunID: "run-main"}
	m.orchStream = &orchStreamState{}
	m.focusRunID = "run-child"
	m2, _ := m.Update(StoppedMsg{})
	am := m2.(*AppModel)
	if am.orchStream != nil || am.viewingChild() {
		t.Fatal("stop should drop orch stream and child focus")
	}
	if am.connStatus != ConnIdle {
		t.Fatalf("status=%v", am.connStatus)
	}
}
