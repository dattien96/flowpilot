package runner

import (
	"context"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// Completed + leftover blocked:paused must not deadlock and must read as
// non-terminal when confirm was cleared (StateMachine P1).
func TestCA805_CompletedLeftoverPausedNotTerminal(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].status = RunStatusCompleted
	svc.runs[parent.RunID].vibeResumeConfirm = false
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Status: "blocked", BlockReason: vibeResumePausedReason, Cap: 3,
	})
	done := make(chan bool, 1)
	go func() {
		done <- svc.flowRunTerminalLocked(parent.RunID)
	}()
	select {
	case terminal := <-done:
		if terminal {
			t.Fatal("Completed + leftover paused (confirm cleared) must not be terminal")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("flowRunTerminalLocked deadlocked on Completed + blocked:paused")
	}
}

// maybeAdvance must not mutate Cancelled→Running when it claims nothing.
func TestCA805_NoHealWhenAdvanceRefused(t *testing.T) {
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
	rs.activeFlowNodes = []agentpack.FlowNode{{ID: "coder", Behavior: "agent.code"}}
	rs.activeFlowEdges = []agentpack.FlowEdge{}
	svc.mu.Unlock()
	if svc.maybeAdvancePendingValidateAfterCoder(parent.RunID) {
		t.Fatal("no validate node must not claim handled")
	}
	svc.mu.Lock()
	st := svc.runs[parent.RunID].status
	svc.mu.Unlock()
	if st != RunStatusCancelled {
		t.Fatalf("refused advance must not heal status, got %q", st)
	}
}

// Snapshot gate must carry the resume node for TUI render.
func TestCA805_SnapshotGateCarriesResumeFrom(t *testing.T) {
	nodes := []agentpack.FlowNode{
		{ID: "tdd", Behavior: "agent.code"},
		{ID: "coder", Behavior: "agent.code"},
		{ID: "validate", Behavior: "command.validate"},
	}
	edges := []agentpack.FlowEdge{
		{From: "tdd", To: "coder", When: "done", Kind: "forward"},
		{From: "coder", To: "validate", When: "done", Kind: "forward"},
	}
	svc, _ := newTestServer(t)
	rs, apiErr := svc.reconstructRun(ProviderSessionState{
		RunID:           "run-ca805-snap",
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
	_ = rs
	svc.reseedFlowStepRuntime("run-ca805-snap", nodes)
	svc.setFlowStepStatus(context.Background(), "run-ca805-snap", "tdd", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), "run-ca805-snap", "coder", StepStatusDone)
	svc.maybeParkVibeResumeConfirm("run-ca805-snap")
	view, err := svc.runSnapshot("run-ca805-snap")
	if err != nil {
		t.Fatalf("runSnapshot: %v", err)
	}
	if view.PendingGate == nil {
		t.Fatal("want resume gate")
	}
	if view.PendingGate.ResumeFrom != "coder" {
		t.Fatalf("ResumeFrom=%q want coder", view.PendingGate.ResumeFrom)
	}
}
