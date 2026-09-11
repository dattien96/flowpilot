package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// run-220036: restart strips autoOrchestrate while flow topology survives;
// a hub dispatch IS orchestration, so the flag must come back, otherwise
// synthesis never starts and the run hangs with no card.
func TestCA808_DispatchRestoresAutoOrchestrate(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	nodes := []agentpack.FlowNode{
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = false
	rs.activeFlowNodes = nodes
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.dispatchHubNotifyNode(parent.RunID, nodes[0])
	svc.mu.Lock()
	flag := svc.runs[parent.RunID].autoOrchestrate
	hub := svc.runs[parent.RunID].activeHubNodeID
	svc.mu.Unlock()
	if !flag {
		t.Fatal("hub dispatch on engine-driven flow must restore autoOrchestrate")
	}
	if hub != "synthesis" {
		t.Fatalf("activeHubNodeID=%q want synthesis", hub)
	}
}

func TestCA808_OKRestoresAutoOrchestrate(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.status = RunStatusCancelled
	rs.vibeResumeConfirm = true
	rs.vibeResumeFromNode = "tdd"
	rs.flowEngineDriven = true
	rs.autoOrchestrate = false
	rs.activeFlowNodes = []agentpack.FlowNode{{ID: "tdd"}, {ID: "coder"}}
	rs.activeFlowEdges = []agentpack.FlowEdge{{From: "tdd", To: "coder", When: "done", Kind: "forward"}}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Status: "blocked", BlockReason: vibeResumePausedReason, Cap: 3, RoundCap: 3,
	})
	if e := svc.SubmitGateDecision(parent.RunID, "ok", ""); e != nil {
		t.Fatalf("ok: %v", e)
	}
	svc.mu.Lock()
	flag := svc.runs[parent.RunID].autoOrchestrate
	svc.mu.Unlock()
	if !flag {
		t.Fatal("OK on flow topology must restore autoOrchestrate")
	}
}

func TestCA808_RetryRestoresAutoOrchestrate(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	nodes := []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.code"},
		{ID: "validate", Behavior: "hub.notify"},
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.status = RunStatusCancelled
	rs.flowEngineDriven = true
	rs.autoOrchestrate = false
	rs.activeFlowNodes = nodes
	rs.activeFlowEdges = []agentpack.FlowEdge{{From: "coder", To: "validate", When: "done", Kind: "forward"}}
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	svc.setFlowStepStatus(context.Background(), parent.RunID, "coder", StepStatusDone)
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Status: "blocked", BlockReason: "hub_stalled", Cap: 3, RoundCap: 3,
	})
	if _, err := svc.resumeFlowWithFeedback(parent.RunID, ""); err != nil {
		t.Fatalf("resume: %v", err)
	}
	svc.mu.Lock()
	flag := svc.runs[parent.RunID].autoOrchestrate
	svc.mu.Unlock()
	if !flag {
		t.Fatal("hub_stalled Retry on flow topology must restore autoOrchestrate")
	}
}
