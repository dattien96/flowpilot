package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func flowModelForSettleTest() *AppModel {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.mode = ModeFlow
	m.launch = LaunchArm{Label: "grok-flow", Mode: ModeFlow, WorkflowID: "wf"}
	m.runHandle = &client.RunHandle{RunID: "run-189839", Status: "running"}
	m.connStatus = ConnWaiting
	m.statusMsg = "flow running…"
	return m
}

// A finished flow whose steps are all terminal, the loop reports done, and no
// agent is still live must drop [stop] and the stale running/streaming chrome,
// even though the raw handle status is still "running" (run-189839).
func TestFlowDone_StepsRuntimeMsgSettlesChrome(t *testing.T) {
	m := flowModelForSettleTest()
	m.flowLoopStatus = "done"
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-189839", AgentName: "main", Role: "main", Status: "completed"},
	}

	m2, _ := m.Update(StepsRuntimeMsg{
		RunID: "run-189839",
		Steps: []client.WorkflowStepRuntime{
			{StepID: "s1", NodeID: "grok-context", Status: "DONE"},
			{StepID: "s2", NodeID: "grok-coder", Status: "DONE"},
			{StepID: "s3", NodeID: "grok-review", Status: "DONE"},
			{StepID: "s4", NodeID: "grok-synthesis", Status: "DONE"},
		},
	})
	am := m2.(*AppModel)
	if am.connStatus != ConnIdle {
		t.Fatalf("connStatus=%v want ConnIdle", am.connStatus)
	}
	if am.statusMsg != "done" {
		t.Fatalf("statusMsg=%q want done", am.statusMsg)
	}
	if am.runHandle.Status != "completed" {
		t.Fatalf("handle status=%q want completed", am.runHandle.Status)
	}
	if am.turnIsActive() {
		t.Fatal("finished flow must not keep [stop] armed")
	}
	line := stripANSI(am.renderStatusLine())
	if strings.Contains(line, "[stop]") || strings.Contains(line, "running") || strings.Contains(line, "streaming") {
		t.Fatalf("settled statusline must be clean:\n%s", line)
	}
}

func TestFlowDone_NotSettledWhileChildRunning(t *testing.T) {
	m := flowModelForSettleTest()
	m.flowLoopStatus = "done"
	m.flowSteps = []client.WorkflowStepRuntime{
		{StepID: "s1", NodeID: "grok-context", Status: "DONE"},
		{StepID: "s2", NodeID: "grok-synthesis", Status: "RUNNING"},
	}
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-189839", AgentName: "main", Role: "main", Status: "running"},
		{RunID: "run-189844", AgentName: "my-reviewer", Status: "running"},
	}

	m2, _ := m.Update(StepsRuntimeMsg{
		RunID: "run-189839",
		Steps: m.flowSteps,
	})
	am := m2.(*AppModel)
	if am.connStatus == ConnIdle || am.statusMsg == "done" {
		t.Fatalf("flow with a live child must not settle (status=%v %q)", am.connStatus, am.statusMsg)
	}
	if !am.turnIsActive() {
		t.Fatal("flow with live child must keep [stop] armed")
	}
	if am.runHandle.Status != "running" {
		t.Fatalf("handle status must stay running, got %q", am.runHandle.Status)
	}
}

func TestFlowDone_NotSettledWhileTurnStreamLive(t *testing.T) {
	m := flowModelForSettleTest()
	m.flowLoopStatus = "done"
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-189839", AgentName: "main", Role: "main", Status: "completed"},
	}
	m.turnStream = &turnStreamState{}

	m2, _ := m.Update(StepsRuntimeMsg{
		RunID: "run-189839",
		Steps: []client.WorkflowStepRuntime{
			{StepID: "s1", NodeID: "grok-context", Status: "DONE"},
		},
	})
	am := m2.(*AppModel)
	if am.connStatus == ConnIdle {
		t.Fatal("live turn stream must prevent settle")
	}
	if am.runHandle.Status != "running" {
		t.Fatalf("handle status must stay running, got %q", am.runHandle.Status)
	}
}

func TestFlowDone_NotSettledUntilLoopDone(t *testing.T) {
	m := flowModelForSettleTest()
	m.flowLoopStatus = "running"
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-189839", AgentName: "main", Role: "main", Status: "completed"},
	}

	m2, _ := m.Update(StepsRuntimeMsg{
		RunID: "run-189839",
		Steps: []client.WorkflowStepRuntime{
			{StepID: "s1", NodeID: "grok-context", Status: "DONE"},
		},
	})
	am := m2.(*AppModel)
	if am.connStatus == ConnIdle {
		t.Fatal("loop still running must prevent settle")
	}
	if !am.turnIsActive() {
		t.Fatal("loop still running must keep [stop] armed")
	}
}

func TestAgentGraphMsg_SetsLoopStatusAndSettles(t *testing.T) {
	m := flowModelForSettleTest()
	m.flowLoopStatus = ""
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-189839", AgentName: "main", Role: "main", Status: "completed"},
	}

	m2, _ := m.Update(AgentGraphMsg{Graph: &client.AgentGraphSnapshot{
		LoopState: client.AgentLoopState{Status: "done", Round: 1, RoundCap: 3},
		Runs: []client.AgentRunSummary{
			{RunID: "run-189839", AgentName: "main", Role: "main", Status: "completed"},
			{RunID: "run-189844", AgentName: "my-reviewer", Status: "completed"},
		},
	}})
	am := m2.(*AppModel)
	if am.flowLoopStatus != "done" {
		t.Fatalf("flowLoopStatus=%q want done", am.flowLoopStatus)
	}
	if am.connStatus != ConnIdle || am.statusMsg != "done" {
		t.Fatalf("graph done must settle (status=%v %q)", am.connStatus, am.statusMsg)
	}
	if am.turnIsActive() {
		t.Fatal("settled graph must not keep [stop] armed")
	}
}