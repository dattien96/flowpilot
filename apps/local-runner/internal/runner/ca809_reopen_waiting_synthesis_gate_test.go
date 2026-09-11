package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

func TestCA809_ReopenParksWhenSynthesisWaiting(t *testing.T) {
	nodes := []agentpack.FlowNode{
		{ID: "validate", Behavior: "command.validate"},
		{ID: "synthesis", Behavior: "hub.notify"},
	}
	edges := []agentpack.FlowEdge{
		{From: "validate", To: "synthesis", When: "done", Kind: "forward"},
	}
	svc, _ := newTestServer(t)
	runID := "run-reopen-synth-wait"
	rs, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID:           runID,
		ProjectID:       "proj",
		ProviderKey:     ProviderKeyCodex,
		RunKind:         "chat",
		WorkingMode:     workingmode.Vibe,
		ChatFlowRef:     workingmode.PackPrefix + vibeSprintFlowID,
		Status:          RunStatusCancelled,
		ActiveFlowNodes: nodes,
		ActiveFlowEdges: edges,
		StartedAt:       "2026-09-09T00:00:00Z",
		UpdatedAt:       "2026-09-09T00:05:00Z",
	})
	if apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	svc.reseedFlowStepRuntime(runID, nodes)
	svc.setFlowStepStatus(context.Background(), runID, "validate", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), runID, "synthesis", StepStatusWaitingUserApr)
	svc.maybeParkVibeResumeConfirm(runID)
	if !rs.vibeResumeConfirm {
		t.Fatal("reopen with validate DONE + synthesis WAITING must show resume gate")
	}
	if rs.vibeResumeFromNode != "validate" {
		t.Fatalf("from=%q want validate", rs.vibeResumeFromNode)
	}
	view, err := svc.runSnapshot(runID)
	if err != nil {
		t.Fatalf("runSnapshot: %v", err)
	}
	if view.PendingGate == nil {
		t.Fatal("snapshot must include ok/cancel resume gate")
	}
}

func TestCA809_ReopenParksGhostRunningSynthesis(t *testing.T) {
	nodes := []agentpack.FlowNode{
		{ID: "validate", Behavior: "command.validate"},
		{ID: "synthesis", Behavior: "hub.notify"},
	}
	edges := []agentpack.FlowEdge{
		{From: "validate", To: "synthesis", When: "done", Kind: "forward"},
	}
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
	rs.workingMode = workingmode.Vibe
	rs.flowEngineDriven = true
	rs.activeFlowNodes = nodes
	rs.activeFlowEdges = edges
	rs.turnInFlight = false
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	svc.setFlowStepStatus(context.Background(), parent.RunID, "validate", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), parent.RunID, "synthesis", StepStatusRunning)
	svc.maybeParkVibeResumeConfirm(parent.RunID)
	svc.mu.Lock()
	confirm := svc.runs[parent.RunID].vibeResumeConfirm
	from := svc.runs[parent.RunID].vibeResumeFromNode
	svc.mu.Unlock()
	if !confirm {
		t.Fatal("ghost RUNNING synthesis with no live turn must park resume")
	}
	if from != "validate" {
		t.Fatalf("from=%q want validate", from)
	}
}

func TestCA809_LiveHubTurnDoesNotPark(t *testing.T) {
	nodes := []agentpack.FlowNode{
		{ID: "validate", Behavior: "command.validate"},
		{ID: "synthesis", Behavior: "hub.notify"},
	}
	edges := []agentpack.FlowEdge{
		{From: "validate", To: "synthesis", When: "done", Kind: "forward"},
	}
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
	rs.workingMode = workingmode.Vibe
	rs.flowEngineDriven = true
	rs.activeFlowNodes = nodes
	rs.activeFlowEdges = edges
	rs.turnInFlight = true
	rs.activeHubNodeID = "synthesis"
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	svc.setFlowStepStatus(context.Background(), parent.RunID, "validate", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), parent.RunID, "synthesis", StepStatusRunning)
	svc.maybeParkVibeResumeConfirm(parent.RunID)
	svc.mu.Lock()
	confirm := svc.runs[parent.RunID].vibeResumeConfirm
	svc.mu.Unlock()
	if confirm {
		t.Fatal("live hub turn must not park a resume gate")
	}
}
