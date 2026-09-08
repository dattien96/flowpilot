package runner

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

func TestCA792_OwnerDebateDoneSkipsReviewCohortVerdict(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = workingmode.Vibe
	rs.activeFlowAcceptanceNodes = []string{"validate", "synthesis", "audit"}
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "owner_1", Behavior: "agent.delegate", Cohort: "owner_debate"},
		{ID: "owner_2", Behavior: "agent.delegate", Cohort: "owner_debate"},
		{ID: vibeDebateSynthesisNodeID, Behavior: "hub.inline"},
	}
	svc.mu.Unlock()

	if err := svc.synthesisDoneVerdictError(parent.RunID); err != nil {
		t.Fatalf("owner_debate must not require review cohort: %v", err)
	}
}

func TestCA792_HarnessStillRequiresReviewCohort(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "dev", FlowRef: "task-harness", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowAcceptanceNodes = []string{"synthesis"}
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	svc.mu.Unlock()
	verdictErr := svc.synthesisDoneVerdictError(parent.RunID)
	if verdictErr == nil {
		t.Fatal("harness synthesis with no review cohort must still error")
	}
}

func TestCA792_DebateRestorePutsSprintGraphBack(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = workingmode.Vibe
	rs.chatFlowRef = workingmode.PackPrefix + vibeSprintFlowID
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "tdd", Behavior: "agent.delegate"},
		{ID: "coder", Behavior: "agent.code"},
	}
	rs.activeFlowAcceptanceNodes = []string{"validate", "synthesis", "audit"}
	svc.mu.Unlock()

	svc.stashVibeFlowForDebate(parent.RunID)
	svc.mu.Lock()
	rs = svc.runs[parent.RunID]
	rs.chatFlowRef = workingmode.PackPrefix + vibeOwnerDebateFlowID
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: vibeDebateSynthesisNodeID, Behavior: "hub.inline"},
	}
	rs.activeFlowAcceptanceNodes = nil
	svc.mu.Unlock()

	svc.onVibeCpNodeDone(parent.RunID, vibeDebateSynthesisNodeID)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs = svc.runs[parent.RunID]
	if workingmode.BareFlowID(rs.chatFlowRef) != vibeSprintFlowID {
		t.Fatalf("chatFlowRef=%q want vibe-sprint", rs.chatFlowRef)
	}
	if len(rs.activeFlowNodes) != 2 || rs.activeFlowNodes[0].ID != "tdd" {
		t.Fatalf("sprint graph not restored: %+v", rs.activeFlowNodes)
	}
	if len(rs.vibeParkedNodes) != 0 {
		t.Fatal("parked graph must clear after restore")
	}
}

func TestCA792_InlineEntryCopiesAcceptanceNodes(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = workingmode.Vibe
	rs.flowEngineDriven = true
	rs.activeFlowAcceptanceNodes = []string{"validate", "synthesis", "audit"}
	svc.mu.Unlock()

	svc.stashVibeFlowForDebate(parent.RunID)
	svc.startResolvedFlow(t.Context(), parent.RunID, workingmode.PackPrefix+vibeOwnerDebateFlowID, "gate")

	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs = svc.runs[parent.RunID]
	if len(rs.activeFlowAcceptanceNodes) != 0 {
		t.Fatalf("owner-debate acceptance=%v want empty (not leftover sprint synthesis)", rs.activeFlowAcceptanceNodes)
	}
	has1, has2 := false, false
	for _, n := range rs.activeFlowNodes {
		if n.ID == "owner_1" {
			has1 = true
		}
		if n.ID == "owner_2" {
			has2 = true
		}
	}
	if !has1 || !has2 {
		t.Fatalf("want both owners in graph, got %+v", rs.activeFlowNodes)
	}
}
