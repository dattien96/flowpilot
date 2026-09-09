package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

func TestCA813_ReconstructFailedHubStalledParksResume(t *testing.T) {
	nodes := []agentpack.FlowNode{
		{ID: "validate", Behavior: "command.validate"},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	edges := []agentpack.FlowEdge{
		{From: "validate", To: "synthesis", When: "done", Kind: "forward"},
	}
	svc, store := ca810ServiceWithLog(t)
	runID := "run-220036-failed-stall"
	ts := "2026-09-09T08:30:00Z"
	for _, line := range []stepTransitionLine{
		{RunID: runID, NodeID: "validate", Status: string(StepStatusDone), TS: ts},
		{RunID: runID, NodeID: "synthesis", Status: string(StepStatusRunning), TS: ts},
	} {
		if err := store.AppendStepTransition(context.Background(), runID, line); err != nil {
			t.Fatalf("AppendStepTransition: %v", err)
		}
	}
	rs, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID:           runID,
		ProjectID:       "proj",
		ProviderKey:     ProviderKeyCodex,
		RunKind:         "chat",
		WorkingMode:     workingmode.Vibe,
		ChatFlowRef:     workingmode.PackPrefix + vibeSprintFlowID,
		Status:          RunStatusFailed,
		ActiveFlowNodes: nodes,
		ActiveFlowEdges: edges,
		StartedAt:       "2026-09-09T00:00:00Z",
		UpdatedAt:       ts,
		LoopState: AgentLoopState{
			Status: "blocked", BlockReason: "hub_stalled",
			GateReason: "hub has made no progress for 2m0s (no turn, gate, or reinvoke in flight)",
			Cap: 3, RoundCap: 3, Mode: "explicit",
		},
	})
	if apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	if rs.status == RunStatusFailed {
		t.Fatal("unfinished vibe must not stay Failed on reopen")
	}
	if !rs.vibeResumeConfirm {
		t.Fatal("Failed+hub_stalled reopen must park resume gate")
	}
	if rs.vibeResumeFromNode != "validate" {
		t.Fatalf("from=%q want validate", rs.vibeResumeFromNode)
	}
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.BlockReason == "hub_stalled" {
		t.Fatalf("stale hub_stalled must not survive reopen park: %+v", loop)
	}
	if loop.BlockReason != vibeResumePausedReason {
		t.Fatalf("BlockReason=%q want paused", loop.BlockReason)
	}
	view, err := svc.runSnapshot(runID)
	if err != nil {
		t.Fatalf("runSnapshot: %v", err)
	}
	if view.PendingGate == nil {
		t.Fatal("snapshot must include ok/cancel resume gate, not hub_stalled")
	}
}
