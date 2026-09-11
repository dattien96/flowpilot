package runner

import (
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

func TestCA811_ParkDoesNotHoldMutexDuringCancel(t *testing.T) {
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
	rs.flowEngineDriven = true
	rs.workingMode = workingmode.Vibe
	rs.activeFlowNodes = []agentpack.FlowNode{{ID: "synthesis", Behavior: "hub.inline"}}
	rs.turnInFlight = true
	rs.turnCancel = func() {
		svc.mu.Lock()
		svc.mu.Unlock()
	}
	svc.mu.Unlock()

	done := make(chan struct{})
	go func() {
		svc.parkFlowForAwaitingUser(parent.RunID)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("parkFlowForAwaitingUser deadlocked holding s.mu during turnCancel")
	}
}

func TestCA811_SummaryScanDoesNotParkResumeGate(t *testing.T) {
	svc, _ := newTestServer(t)
	nodes := []agentpack.FlowNode{
		{ID: "validate", Behavior: "command.validate"},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	edges := []agentpack.FlowEdge{
		{From: "validate", To: "synthesis", When: "done", Kind: "forward"},
	}
	rs, apiErr := svc.reconstructRunDeferred(ProviderSessionState{
		RunID:           "run-scan-no-park",
		ProjectID:       "proj",
		ProviderKey:     ProviderKeyCodex,
		RunKind:         "chat",
		WorkingMode:     workingmode.Vibe,
		ChatFlowRef:     workingmode.PackPrefix + vibeSprintFlowID,
		Status:          RunStatusRunning,
		ActiveFlowNodes: nodes,
		ActiveFlowEdges: edges,
		StartedAt:       "2026-09-09T00:00:00Z",
		UpdatedAt:       "2026-09-09T00:05:00Z",
	})
	if apiErr != nil {
		t.Fatalf("reconstructRunDeferred: %v", apiErr)
	}
	if rs.vibeResumeConfirm {
		t.Fatal("boot summary reconstruct must not park a resume gate")
	}
}
