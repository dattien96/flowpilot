package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

func TestCA803_OKHealsCancelledThenInlineCanAdvance(t *testing.T) {
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
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.code"},
		{ID: "notify", Behavior: "hub.notify"},
	}
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "coder", To: "notify", When: "done", Kind: "forward"},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Status: "blocked", BlockReason: vibeResumePausedReason, Cap: 3, RoundCap: 3,
	})
	if e := svc.SubmitGateDecision(parent.RunID, "ok", ""); e != nil {
		t.Fatalf("ok: %v", e)
	}
	svc.mu.Lock()
	st := svc.runs[parent.RunID].status
	svc.mu.Unlock()
	if st == RunStatusCancelled {
		t.Fatal("pause-gate OK must heal Cancelled so coder→validate is not skipped")
	}
	if !svc.tryAdvanceFlowFromNode(parent.RunID, "coder", "coder finished") {
		t.Fatal("after OK heal, coder→inline must dispatch")
	}
}

func TestCA803_HubStalledRetryAdvancesPendingValidate(t *testing.T) {
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
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	nodes := []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.code"},
		{ID: "validate", Behavior: "hub.notify"},
	}
	rs.activeFlowNodes = nodes
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "coder", To: "validate", When: "done", Kind: "forward"},
	}
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
	hub := svc.runs[parent.RunID].activeHubNodeID
	svc.mu.Unlock()
	if hub != "validate" {
		t.Fatalf("Retry must dispatch pending validate, activeHubNodeID=%q", hub)
	}
}

func TestCA803_StoppedOKDoesNotHeal(t *testing.T) {
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
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "stopped", Cap: 3})
	if e := svc.SubmitGateDecision(parent.RunID, "ok", ""); e != nil {
		t.Fatalf("ok: %v", e)
	}
	svc.mu.Lock()
	st := svc.runs[parent.RunID].status
	svc.mu.Unlock()
	if st != RunStatusCancelled {
		t.Fatalf("Stop must win: status=%q", st)
	}
	if svc.agentOrchestrator.loopStateFor(parent.RunID).Status != "stopped" {
		t.Fatal("OK must not unseal a stopped loop")
	}
}

func TestCA803_FailedHubStalledRetryDoesNotClaimHandled(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.status = RunStatusFailed
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.code"},
		{ID: "validate", Behavior: "hub.notify"},
	}
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "coder", To: "validate", When: "done", Kind: "forward"},
	}
	svc.mu.Unlock()
	if svc.maybeAdvancePendingValidateAfterCoder(parent.RunID) {
		t.Fatal("Failed run must not claim hub_stalled Retry as handled")
	}
}

