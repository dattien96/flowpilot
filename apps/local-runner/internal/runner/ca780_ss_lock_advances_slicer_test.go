package runner

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
)

func TestResumeVibeLock_DispatchesSprintSlicer(t *testing.T) {
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
	rs.flowEngineDriven = true
	rs.vibeAwaitingLock = true
	rs.vibeLockNodeID = vibeSSLockNodeID
	rs.activeHubNodeID = "ss_validator"
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "ss_lock", Behavior: "user.confirm"},
		{ID: "sprint_slicer", Behavior: "hub.inline", Agent: "agents/synthesizer.md"},
	}
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "ss_lock", To: "sprint_slicer", When: "done", Kind: "forward"},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.mutateLoop(parent.RunID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = vibeLockBlockReason
		st.ActiveNode = vibeSSLockNodeID
		return st
	})

	if _, ok := svc.resumeVibeLock(parent.RunID, "continue", AgentGraphSnapshot{}); !ok {
		t.Fatal("resumeVibeLock")
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs = svc.runs[parent.RunID]
	if rs.activeHubNodeID != vibeSprintSlicerNodeID {
		t.Fatalf("activeHub=%q want sprint_slicer", rs.activeHubNodeID)
	}
}
