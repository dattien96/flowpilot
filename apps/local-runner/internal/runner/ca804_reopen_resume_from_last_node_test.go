package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

func TestCA804_ReopenParksResumeFromCoderWhenValidatePending(t *testing.T) {
	nodes := []agentpack.FlowNode{
		{ID: "tdd", Behavior: "agent.code"},
		{ID: "coder", Behavior: "agent.code"},
		{ID: "validate", Behavior: "hub.notify"},
	}
	edges := []agentpack.FlowEdge{
		{From: "tdd", To: "coder", When: "done", Kind: "forward"},
		{From: "coder", To: "validate", When: "done", Kind: "forward"},
	}
	svc, _ := newTestServer(t)
	runID := "run-reopen-coder"
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
	svc.setFlowStepStatus(context.Background(), runID, "tdd", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), runID, "coder", StepStatusDone)
	svc.maybeParkVibeResumeConfirm(runID)
	if !rs.vibeResumeConfirm {
		t.Fatal("reopen with coder DONE + validate pending must show resume gate")
	}
	if rs.vibeResumeFromNode != "coder" {
		t.Fatalf("from=%q want coder", rs.vibeResumeFromNode)
	}
	view, err := svc.runSnapshot(runID)
	if err != nil {
		t.Fatalf("runSnapshot: %v", err)
	}
	if view.PendingGate == nil {
		t.Fatal("snapshot must include ok/cancel resume gate")
	}
	if e := svc.SubmitGateDecision(runID, "ok", ""); e != nil {
		t.Fatalf("ok: %v", e)
	}
	svc.mu.Lock()
	hub := svc.runs[runID].activeHubNodeID
	svc.mu.Unlock()
	if hub != "validate" {
		t.Fatalf("OK must resume from coder into validate, activeHubNodeID=%q", hub)
	}
}
