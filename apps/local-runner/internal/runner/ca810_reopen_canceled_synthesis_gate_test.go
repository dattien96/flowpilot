package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

type ca810StepLogStore struct {
	*fakeWorkflowStore
	lines map[string][]stepTransitionLine
}

func (m *ca810StepLogStore) AppendStepTransition(_ context.Context, runID string, line stepTransitionLine) error {
	if m.lines == nil {
		m.lines = map[string][]stepTransitionLine{}
	}
	m.lines[runID] = append(m.lines[runID], line)
	return nil
}

func (m *ca810StepLogStore) LoadStepTransitions(_ context.Context, runID string) ([]stepTransitionLine, error) {
	return append([]stepTransitionLine(nil), m.lines[runID]...), nil
}

func (m *ca810StepLogStore) DeleteStepTransitions(_ context.Context, runID string) error {
	delete(m.lines, runID)
	return nil
}

func ca810SprintGraph() ([]agentpack.FlowNode, []agentpack.FlowEdge) {
	nodes := []agentpack.FlowNode{
		{ID: "validate", Behavior: "command.validate"},
		{ID: "synthesis", Behavior: "hub.inline"},
		{ID: "audit", Behavior: "agent.audit"},
	}
	edges := []agentpack.FlowEdge{
		{From: "validate", To: "synthesis", When: "done", Kind: "forward"},
		{From: "synthesis", To: "audit", When: "done", Kind: "forward"},
	}
	return nodes, edges
}

func ca810ServiceWithLog(t *testing.T) (*InteractiveService, *ca810StepLogStore) {
	t.Helper()
	svc, _ := newTestServer(t)
	base, ok := svc.workflowStore.(*fakeWorkflowStore)
	if !ok {
		t.Fatalf("workflowStore=%T want *fakeWorkflowStore", svc.workflowStore)
	}
	logStore := &ca810StepLogStore{fakeWorkflowStore: base, lines: map[string][]stepTransitionLine{}}
	svc.workflowStore = logStore
	return svc, logStore
}

func TestCA810_ReconstructParksCanceledGhostSynthesis(t *testing.T) {
	nodes, edges := ca810SprintGraph()
	svc, store := ca810ServiceWithLog(t)
	runID := "run-220036-replay"
	ts := "2026-09-09T07:26:32Z"
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
		Status:          RunStatusRunning,
		ActiveFlowNodes: nodes,
		ActiveFlowEdges: edges,
		StartedAt:       "2026-09-09T00:00:00Z",
		UpdatedAt:       ts,
		LoopState:       AgentLoopState{Status: "running", Cap: 3, RoundCap: 3, Mode: "explicit"},
	})
	if apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	if got := flowStepStatus(t, svc, runID, "synthesis"); got != StepStatusCanceled {
		t.Fatalf("replay synthesis=%q want CANCELED (I-17 ghost RUNNING)", got)
	}
	if !rs.vibeResumeConfirm {
		t.Fatal("reconstruct must park resume when synthesis remapped to CANCELED")
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

func TestCA810_StoppedLoopStillParksCanceledSynthesis(t *testing.T) {
	nodes, edges := ca810SprintGraph()
	svc, store := ca810ServiceWithLog(t)
	runID := "run-220036-stopped"
	ts := "2026-09-09T07:26:32Z"
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
		Status:          RunStatusCancelled,
		ActiveFlowNodes: nodes,
		ActiveFlowEdges: edges,
		StartedAt:       "2026-09-09T00:00:00Z",
		UpdatedAt:       ts,
		LoopState: AgentLoopState{
			Status: "stopped", BlockReason: "stopped", GateReason: "stopped",
			Cap: 3, RoundCap: 3, Mode: "explicit",
		},
	})
	if apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	if !rs.vibeResumeConfirm {
		t.Fatal("Stop-sealed loop must still park reopen resume gate")
	}
	if rs.vibeResumeFromNode != "validate" {
		t.Fatalf("from=%q want validate", rs.vibeResumeFromNode)
	}
}
