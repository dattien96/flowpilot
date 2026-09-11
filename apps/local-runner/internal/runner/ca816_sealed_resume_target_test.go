package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// CA-816 reconciles CA-803 (Stop wins on a sealed loop with no resume
// target) with CA-812 (operator OK on a genuine resume gate releases the
// stop fence). The discriminator is vibeResumeFromNode: empty means poison,
// set means explicit resume intent.

// Sealed loop + genuine resume target: OK heals, releases the fence, unseals.
func TestCA816_SealedLoopWithResumeTargetOKResumes(t *testing.T) {
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
	// NOTE: loop stays "stopped" from setupPostStopHub — the live post-Stop
	// /open shape CA-812's own test does not cover (it re-blocks the loop).
	if got := svc.agentOrchestrator.loopStateFor(parentID).Status; got != "stopped" {
		t.Fatalf("precondition loop=%q want stopped", got)
	}
	if e := svc.SubmitGateDecision(parentID, "ok", ""); e != nil {
		t.Fatalf("ok: %v", e)
	}
	svc.mu.Lock()
	st := svc.runs[parentID].status
	svc.mu.Unlock()
	if st != RunStatusRunning {
		t.Fatalf("OK with resume target must heal Cancelled, status=%q", st)
	}
	if got := svc.agentOrchestrator.loopStateFor(parentID).Status; got == "stopped" || got == "done" {
		t.Fatalf("OK with resume target must unseal loop, loop=%q", got)
	}
	st2, err := store.GetRunStopState(context.Background(), parentID)
	if err != nil {
		t.Fatalf("GetRunStopState: %v", err)
	}
	if st2.Stopped {
		t.Fatal("OK with resume target must release hub stop fence")
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

// Sealed loop + NO resume target: OK stays a no-op and keeps the fence.
func TestCA816_SealedLoopWithoutTargetOKKeepsFence(t *testing.T) {
	svc, store, parentID, _, _ := setupPostStopHub(t, ProviderKeyCodex)
	svc.mu.Lock()
	rs := svc.runs[parentID]
	rs.vibeResumeConfirm = true
	rs.vibeResumeFromNode = ""
	svc.mu.Unlock()
	if got := svc.agentOrchestrator.loopStateFor(parentID).Status; got != "stopped" {
		t.Fatalf("precondition loop=%q want stopped", got)
	}
	if e := svc.SubmitGateDecision(parentID, "ok", ""); e != nil {
		t.Fatalf("ok: %v", e)
	}
	svc.mu.Lock()
	st := svc.runs[parentID].status
	svc.mu.Unlock()
	if st != RunStatusCancelled {
		t.Fatalf("Stop must win without resume target: status=%q", st)
	}
	if got := svc.agentOrchestrator.loopStateFor(parentID).Status; got != "stopped" {
		t.Fatalf("OK must not unseal a stopped loop without target, loop=%q", got)
	}
	st2, err := store.GetRunStopState(context.Background(), parentID)
	if err != nil {
		t.Fatalf("GetRunStopState: %v", err)
	}
	if !st2.Stopped {
		t.Fatal("poison-gate OK must not release the hub stop fence")
	}
}
