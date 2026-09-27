package runner

// CP-58 A-58-4 (deep review, 2026-09-26): task-harness.yaml deliberately
// sets cap 5 per phase so a contested plan cannot starve the code review
// loop — but existing tests jumped straight to Round 4 behavior and never
// proved the boundary: four continues must still loop, the fifth must block,
// and a plan approve must reset the shared Round budget to zero for the
// code phase (resetPlanPhaseRound on plan_synthesis -> preflight freeze).

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

func TestTaskHarnessCap5ContinueThenBlockedThenPlanApproveResets(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	// task-harness.yaml declares cap: 5 (both loops share one Round budget).
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 5, RoundCap: 5})

	edges := []agentpack.FlowEdge{
		{From: "plan_synthesis", To: "plan_writer", When: "continue", Kind: "back"},
		{From: "plan_synthesis", To: "preflight_contract_freeze", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "plan_writer", Behavior: "agent.delegate", Agent: "agents/coder.md", Lifecycle: "reinvoke"},
		{ID: "plan_synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke", Join: "all"},
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.activeHubNodeID = "plan_synthesis"
	svc.mu.Unlock()

	continueOnce := func(want string) FlowControlResult {
		t.Helper()
		fc, fcErr := svc.applyFlowControl(parent.RunID, FlowControlInput{
			Status: "continue", Summary: "plan reviewer requested changes",
		})
		if fcErr != nil {
			t.Fatalf("applyFlowControl(continue): %v", fcErr)
		}
		if fc.NextAction != want {
			t.Fatalf("NextAction = %q, want %q (round=%d)", fc.NextAction, want, fc.Round)
		}
		return fc
	}

	// Rounds 1..4 keep looping — the fifth bumps Round to the cap and blocks.
	for i := 0; i < 4; i++ {
		fc := continueOnce("looping")
		if fc.Round != i+1 {
			t.Fatalf("round %d: fc.Round = %d", i+1, fc.Round)
		}
	}
	fc := continueOnce("awaiting_user")
	if fc.Status != "blocked" || fc.Round != 5 {
		t.Fatalf("fifth continue must hit cap: %+v", fc)
	}
	st := svc.agentOrchestrator.loopStateFor(parent.RunID)
	// cap reached → blocked. With the tournament-rescue flag on the loop may
	// immediately flip to tournament_escalation — BlockReason stays "cap"
	// either way; what matters is the flow is parked awaiting user.
	if st.BlockReason != "cap" || (st.Status != "blocked" && st.Status != "tournament_escalation") {
		t.Fatalf("cap must park the loop (blocked or tournament_escalation), got %+v", st)
	}

	// Plan approve, driven through the REAL hub edge: the hub's own
	// submit_review_outcome(done) routes through SubmitFlowControl →
	// advanceHubDoneThroughEdge, where plan_synthesis --done-->
	// preflight_contract_freeze fires resetPlanPhaseRound (the code phase gets
	// a fresh shared budget). applyFlowControl alone is the terminal settle —
	// it never consults edges — so the bridge call is the honest seam. A
	// contested (churned) plan parks on plan_approval first by design
	// (Task-325), so the approve is exercised on a clean run: same graph
	// shape as run-201295's hub->freeze->writer topology (real git
	// workspace so runContractFreezeNode can freeze, a reachable agent.code
	// writer, and a valid preflight draft in the verdict summary), no
	// re-entered writer (planLoopChurned=false), Round pre-seeded nonzero.
	dir, head := newContractFreezeTestRepo(t)
	freezeEdges, freezeNodes := run201295HubFreezeTopology()
	runID2 := newFreezeTestRunForProvider(t, svc, ProviderKeyCodex, dir, freezeEdges, freezeNodes, head)
	svc.mu.Lock()
	rs2 := svc.runs[runID2]
	rs2.activeHubNodeID = "plan_synthesis"
	rs2.autoOrchestrate = true
	rs2.currentTurnID = "turn-cap5-approve"
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(runID2, freezeNodes)
	svc.agentOrchestrator.setLoop(runID2, AgentLoopState{Status: "running", Mode: "explicit", Cap: 5, RoundCap: 5, Round: 4})
	bridge := &turnBridge{svc: svc, rs: rs2, ctx: context.Background()}
	fc, fcErr := bridge.SubmitFlowControl(FlowControlInput{
		Status: "done", Summary: validPlannerDraft,
	})
	if fcErr != nil {
		t.Fatalf("SubmitFlowControl(done): %v", fcErr)
	}
	st = svc.agentOrchestrator.loopStateFor(runID2)
	if st.Round != 0 {
		t.Fatalf("plan_synthesis --done--> freeze must reset the shared round budget via resetPlanPhaseRound, Round=%d (fc=%+v)", st.Round, fc)
	}
	if got := flowStepStatus(t, svc, runID2, "preflight_contract_freeze"); got != StepStatusDone {
		t.Fatalf("preflight_contract_freeze = %v, want DONE (the edge must dispatch the freeze node, not just report advancing)", got)
	}
}
