package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

func TestCA812_ResumeOKReleasesStopFence(t *testing.T) {
	svc, store, parentID, _, _ := setupPostStopHub(t, ProviderKeyCodex)
	nodes := []agentpack.FlowNode{
		{ID: "validate", Behavior: "command.validate"},
		{ID: "synthesis", Behavior: "hub.notify"},
	}
	edges := []agentpack.FlowEdge{
		{From: "validate", To: "synthesis", When: "done", Kind: "forward"},
	}
	svc.mu.Lock()
	rs := svc.runs[parentID]
	rs.workingMode = workingmode.Vibe
	rs.vibeResumeConfirm = true
	rs.vibeResumeFromNode = "validate"
	rs.activeFlowNodes = nodes
	rs.activeFlowEdges = edges
	rs.flowEngineDriven = true
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parentID, nodes)
	svc.setFlowStepStatus(context.Background(), parentID, "validate", StepStatusDone)
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{
		Status: "blocked", BlockReason: vibeResumePausedReason, Cap: 3, RoundCap: 3, Mode: "explicit",
	})
	st, err := store.GetRunStopState(context.Background(), parentID)
	if err != nil || !st.Stopped {
		t.Fatalf("precondition fence stopped=%v err=%v", st.Stopped, err)
	}
	if e := svc.SubmitGateDecision(parentID, "ok", ""); e != nil {
		t.Fatalf("ok: %v", e)
	}
	st2, err := store.GetRunStopState(context.Background(), parentID)
	if err != nil {
		t.Fatalf("GetRunStopState: %v", err)
	}
	if st2.Stopped {
		t.Fatal("OK resume must release hub stop fence")
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		rs = svc.runs[parentID]
		var failed string
		for _, ev := range rs.events {
			if ev.Type == EventTurnFailed && strings.Contains(ev.Error, "stop fence") {
				failed = ev.Error
				break
			}
		}
		hub := rs.activeHubNodeID
		svc.mu.Unlock()
		if failed != "" {
			t.Fatalf("synthesis turn hit stop fence: %s", failed)
		}
		if hub == "synthesis" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("OK must resume validate → synthesis without stop fence")
}
