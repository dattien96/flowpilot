package runner

import (
	"context"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

func vibeOwnerDebateNodes() []agentpack.FlowNode {
	return []agentpack.FlowNode{
		{ID: "debate_trigger", Behavior: "hub.inline"},
		{ID: "owner_1", Behavior: "agent.delegate", Agent: "agents/owner.md", Cohort: "owner_debate"},
		{ID: "owner_2", Behavior: "agent.delegate", Agent: "agents/owner.md", Cohort: "owner_debate"},
		{ID: vibeDebateSynthesisNodeID, Behavior: "hub.inline"},
	}
}

func seedVibeOwnerFailRun(t *testing.T, pk ProviderKey) (*InteractiveService, string) {
	t.Helper()
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: pk,
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 5, RoundCap: 5, Round: 0})
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = workingmode.Vibe
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.chatFlowRef = workingmode.PackPrefix + vibeOwnerDebateFlowID
	rs.activeFlowNodes = vibeOwnerDebateNodes()
	rs.hubLastProgressAt = time.Now().UTC().Add(-3 * time.Minute)
	rs.stallTimeout = 2 * time.Minute
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parent.RunID, vibeOwnerDebateNodes())
	svc.setFlowStepStatus(context.Background(), parent.RunID, "debate_trigger", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), parent.RunID, "owner_1", StepStatusFailed)
	svc.setFlowStepStatus(context.Background(), parent.RunID, "owner_2", StepStatusFailed)
	return svc, parent.RunID
}

func TestCA796_OwnerFailDoesNotHubStall(t *testing.T) {
	svc, runID := seedVibeOwnerFailRun(t, ProviderKeyCodex)
	if svc.checkAndBlockStalledHub(runID) {
		st := svc.agentOrchestrator.loopStateFor(runID)
		if st.BlockReason == "hub_stalled" {
			t.Fatalf("hub_stalled after owner fail: %+v", st)
		}
	}
	st := svc.agentOrchestrator.loopStateFor(runID)
	if st.BlockReason == "hub_stalled" {
		t.Fatalf("loop hub_stalled: %+v", st)
	}
}

func TestCA796_OwnerFailRetriesThenCapNotHubStall(t *testing.T) {
	svc, runID := seedVibeOwnerFailRun(t, ProviderKeyCodex)
	svc.mu.Lock()
	svc.runs[runID].vibeOwnerFailRetries = maxVibeOwnerFailRetries
	svc.mu.Unlock()
	parked := svc.checkAndBlockStalledHub(runID)
	st := svc.agentOrchestrator.loopStateFor(runID)
	if st.BlockReason == "hub_stalled" {
		t.Fatalf("want cap park, got hub_stalled: %+v parked=%v", st, parked)
	}
	if st.BlockReason != "cap" {
		t.Fatalf("BlockReason=%q want cap", st.BlockReason)
	}
}

func TestCA796_HarnessReviewFailStillCanHubStall(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "reviewer", Behavior: "agent.delegate"},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	rs.hubLastProgressAt = time.Now().UTC().Add(-3 * time.Minute)
	rs.stallTimeout = 2 * time.Minute
	svc.mu.Unlock()
	if !svc.checkAndBlockStalledHub(parent.RunID) {
		t.Fatal("harness empty hub must still hub_stalled")
	}
	st := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if st.BlockReason != "hub_stalled" {
		t.Fatalf("BlockReason=%q want hub_stalled", st.BlockReason)
	}
}

func TestCA796_LiveOwnerBothFailSettlesWithoutWait(t *testing.T) {
	svc, runID := seedVibeOwnerFailRun(t, ProviderKeyCodex)
	if !svc.maybeSettleVibeOwnerDebate(runID) {
		t.Fatal("expected settle action on both owners failed")
	}
	st := svc.agentOrchestrator.loopStateFor(runID)
	if st.BlockReason == "hub_stalled" {
		t.Fatalf("settle must not hub_stalled: %+v", st)
	}
}

func TestCA796_RetryFromHubStalledUnblocksLoop(t *testing.T) {
	svc, runID := seedVibeOwnerFailRun(t, ProviderKeyCodex)
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{
		Status: "blocked", BlockReason: "hub_stalled", Cap: 5, RoundCap: 5,
	})
	if !svc.maybeSettleVibeOwnerDebate(runID) {
		t.Fatal("expected settle from hub_stalled")
	}
	st := svc.agentOrchestrator.loopStateFor(runID)
	if st.BlockReason == "hub_stalled" {
		t.Fatalf("retry must clear hub_stalled: %+v", st)
	}
	if st.Status == "blocked" && st.BlockReason != "cap" {
		t.Fatalf("loop still blocked: %+v", st)
	}
}

func TestCA796_DonePlusFailedDoesNotRetry(t *testing.T) {
	svc, runID := seedVibeOwnerFailRun(t, ProviderKeyCodex)
	svc.setFlowStepStatus(context.Background(), runID, "owner_1", StepStatusDone)
	if svc.maybeSettleVibeOwnerDebate(runID) {
		t.Fatal("Done+Failed must not full-respawn debate")
	}
}
