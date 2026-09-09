package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

func TestCA806_FailedParkDoesNotArm(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	nodes := []agentpack.FlowNode{
		{ID: "tdd", Behavior: "agent.code"},
		{ID: "coder", Behavior: "agent.code"},
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.status = RunStatusFailed
	rs.flowEngineDriven = true
	rs.activeFlowNodes = nodes
	rs.activeFlowEdges = []agentpack.FlowEdge{{From: "tdd", To: "coder", When: "done", Kind: "forward"}}
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	svc.setFlowStepStatus(context.Background(), parent.RunID, "tdd", StepStatusDone)
	svc.maybeParkVibeResumeConfirm(parent.RunID)
	svc.mu.Lock()
	confirm := svc.runs[parent.RunID].vibeResumeConfirm
	svc.mu.Unlock()
	if confirm {
		t.Fatal("Failed run must not arm resume gate")
	}
}

func TestCA806_FailedOKDoesNotFlipLoop(t *testing.T) {
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
	rs.status = RunStatusFailed
	rs.vibeResumeConfirm = true
	rs.vibeResumeFromNode = "coder"
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Status: "blocked", BlockReason: vibeResumePausedReason, Cap: 3, RoundCap: 3,
	})
	if e := svc.SubmitGateDecision(parent.RunID, "ok", ""); e != nil {
		t.Fatalf("ok: %v", e)
	}
	loop := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if loop.Status != "blocked" {
		t.Fatalf("Failed OK must not unblock loop, got %q", loop.Status)
	}
	svc.mu.Lock()
	st := svc.runs[parent.RunID].status
	svc.mu.Unlock()
	if st != RunStatusFailed {
		t.Fatalf("Failed OK must not heal status, got %q", st)
	}
}

func TestCA806_RetryReachesRealValidateEscalate(t *testing.T) {
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
		{ID: "validate", Behavior: "command.validate"},
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.status = RunStatusCancelled
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.workspaceCwd = t.TempDir()
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
	esc := svc.runs[parent.RunID].lastEscalatedInlineNodeID
	svc.mu.Unlock()
	if esc != "validate" {
		t.Fatalf("Retry must reach real command.validate (skipped_no_command escalate), escalated=%q", esc)
	}
}

func TestCA806_OKHealReachesRealValidateEscalate(t *testing.T) {
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
		{ID: "validate", Behavior: "command.validate"},
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.status = RunStatusCancelled
	rs.vibeResumeConfirm = true
	rs.vibeResumeFromNode = "coder"
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.workspaceCwd = t.TempDir()
	rs.activeFlowNodes = nodes
	rs.activeFlowEdges = []agentpack.FlowEdge{{From: "coder", To: "validate", When: "done", Kind: "forward"}}
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	svc.setFlowStepStatus(context.Background(), parent.RunID, "coder", StepStatusDone)
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Status: "blocked", BlockReason: vibeResumePausedReason, Cap: 3, RoundCap: 3,
	})
	if e := svc.SubmitGateDecision(parent.RunID, "ok", ""); e != nil {
		t.Fatalf("ok: %v", e)
	}
	svc.mu.Lock()
	esc := svc.runs[parent.RunID].lastEscalatedInlineNodeID
	svc.mu.Unlock()
	if esc != "validate" {
		t.Fatalf("OK must reach real command.validate (skipped_no_command escalate), escalated=%q", esc)
	}
}

func TestCA806_NoEvidenceNoAdvanceNoHeal(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.status = RunStatusCancelled
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.code"},
		{ID: "validate", Behavior: "hub.notify"},
	}
	rs.activeFlowEdges = []agentpack.FlowEdge{{From: "coder", To: "validate", When: "done", Kind: "forward"}}
	svc.mu.Unlock()
	if svc.maybeAdvancePendingValidateAfterCoder(parent.RunID) {
		t.Fatal("no DONE row and no child evidence must not claim handled")
	}
	svc.mu.Lock()
	st := svc.runs[parent.RunID].status
	svc.mu.Unlock()
	if st != RunStatusCancelled {
		t.Fatalf("refused advance must not heal, got %q", st)
	}
}

func TestCA806_CompletedChildEvidenceAdvances(t *testing.T) {
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
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.activeFlowNodes = nodes
	rs.activeFlowEdges = []agentpack.FlowEdge{{From: "coder", To: "validate", When: "done", Kind: "forward"}}
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	type upserter interface {
		UpsertProviderSession(context.Context, ProviderSessionState) error
	}
	up, ok := svc.workflowStore.(upserter)
	if !ok {
		t.Skip("store cannot persist child evidence")
	}
	if err := up.UpsertProviderSession(context.Background(), ProviderSessionState{
		RunID: "run-coder-ev", ProjectID: "proj",
		ParentRunID: parent.RunID, Label: "coder", Status: RunStatusCompleted,
	}); err != nil {
		t.Fatalf("upsert child: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	if !svc.maybeAdvancePendingValidateAfterCoder(parent.RunID) {
		t.Fatal("PENDING row + completed child evidence must advance")
	}
}
