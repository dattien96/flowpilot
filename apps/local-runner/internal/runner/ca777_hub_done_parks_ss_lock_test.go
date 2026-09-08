package runner

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
)

func TestAdvanceHubDone_ParksVibeSSLockNotEscalate(t *testing.T) {
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
	rs.activeHubNodeID = "ss_validator"
	rs.currentTurnID = "turn-ss-lock"
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "ss_validator", Behavior: "hub.inline"},
		{ID: "ss_lock", Behavior: "user.confirm"},
	}
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "ss_validator", To: "ss_lock", When: "done", Kind: "forward"},
	}
	svc.mu.Unlock()

	res, handled := svc.advanceHubDoneThroughEdge(parent.RunID, FlowControlInput{Status: "done"})
	if !handled {
		t.Fatal("hub done to ss_lock must be handled")
	}
	if res.NextAction != "awaiting_user" {
		t.Fatalf("NextAction=%q want awaiting_user (got escalate/dispatch fail? %+v)", res.NextAction, res)
	}
	st := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if st.BlockReason != vibeLockBlockReason {
		t.Fatalf("BlockReason=%q want %q GateReason=%q", st.BlockReason, vibeLockBlockReason, st.GateReason)
	}
	if st.ActiveNode != vibeSSLockNodeID {
		t.Fatalf("ActiveNode=%q want ss_lock", st.ActiveNode)
	}
}
