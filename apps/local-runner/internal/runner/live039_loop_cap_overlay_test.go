package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// live-039 residual (run-183756): startResolvedFlowFromNode seeds the shared
// loop's Cap/RoundCap/ExtendBy/NegotiationCap from the MOUNTED flow's policy
// on every mount. When the owner-debate overlay mounted over a parked
// vibe-sprint (cap=20), the debate's policy overwrote the sprint's budget —
// the sprint's own round counter then reported "round cap reached 5/5"
// against the debate leg's policy, and restoreVibeFlowAfterDebate never
// re-seeds, so the corruption outlived the debate. An overlay borrows the
// parked flow's loop; its own policy is bounded by the mount/retry counters.
func TestLive039_OverlayMountKeepsParkedLoopBudget(t *testing.T) {
	svc, _ := newTestServer(t)
	store := newFakeFlowDefinitionStore()
	store.byRef[workingmode.PackPrefix+vibeOwnerDebateFlowID] = FlowDefinitionRecord{
		FlowRef: workingmode.PackPrefix + vibeOwnerDebateFlowID,
		Name:    "Owner Debate",
		Source:  "builtin",
		Definition: agentpack.FlowDefinition{
			ID: vibeOwnerDebateFlowID,
			Nodes: []agentpack.FlowNode{
				{ID: "debate_trigger", Behavior: "hub.inline", Agent: "agents/synthesizer.md"},
			},
			Policy: agentpack.FlowPolicy{Cap: 5, ExtendBy: 1, NegotiationCap: 2},
		},
	}
	svc.SetFlowDefinitionStore(store)

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, WorkingMode: "vibe", Client: "tui"})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Status: "running", Cap: 20, RoundCap: 20, ExtendBy: 3, NegotiationCap: 7,
	})
	// The overlay shape: a sprint flow parked underneath the debate mount.
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = workingmode.Vibe
	rs.vibeParkedNodes = []agentpack.FlowNode{{ID: "coder", Behavior: "agent.delegate"}}
	svc.mu.Unlock()

	svc.startResolvedFlow(context.Background(), parent.RunID, workingmode.PackPrefix+vibeOwnerDebateFlowID, "owner debate mount")

	st := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if st.Cap != 20 || st.RoundCap != 20 {
		t.Fatalf("overlay mount overwrote parked loop budget: Cap=%d RoundCap=%d, want 20/20", st.Cap, st.RoundCap)
	}
	if st.ExtendBy != 3 || st.NegotiationCap != 7 {
		t.Fatalf("overlay mount overwrote parked loop budget fields: ExtendBy=%d NegotiationCap=%d, want 3/7", st.ExtendBy, st.NegotiationCap)
	}
}

// Guard: a first-mount flow (nothing parked underneath) still seeds its own
// policy budget — the BUG-631 semantics are preserved for real mounts.
func TestLive039_PrimaryMountStillSeedsPolicy(t *testing.T) {
	svc, _ := newTestServer(t)
	svc.SetFlowDefinitionStore(bug631FlowStore())

	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.startResolvedFlow(context.Background(), parent.RunID, bug631FlowRef, "first mount")
	if st := svc.agentOrchestrator.loopStateFor(parent.RunID); st.Cap != 3 {
		t.Fatalf("primary mount Cap = %d, want 3 (policy seed)", st.Cap)
	}
}
