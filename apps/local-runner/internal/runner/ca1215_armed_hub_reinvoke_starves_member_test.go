package runner

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// CA-1215 (live run-262417): a blocked hub.notify reinvoke re-arms
// pendingHubReinvokePrompt while the loop is parked on a review-verdict
// escalate. Continue drains the armed prompt FIRST and early-returns into a
// generic hub re-prompt — the hub re-submits done against the identical
// stale verdict, re-parks, and the reinvoke re-arms. The CA-1098 deficient
// member re-drive below never ran: observed 3 consecutive
// escalate→continue→hub-reinvoke→park cycles at 20:04/20:07/20:08 while the
// parked spec_align leg stayed waiting_user_approval. The verdict-gate
// resume must run before the armed-prompt short-circuit; the member settle
// reinvokes the hub on its own edges, so the armed prompt is redundant.
func TestCA1215_ArmedHubReinvokeDoesNotStarveDeficientMemberRedrive(t *testing.T) {
	dir, _ := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)

	child := newP4ChildRun(svc, "child-spec", parentID, dir, "")
	child.label = "spec_align"
	child.status = RunStatusWaitingUserApr
	child.agentStatus = "waiting_user_approval"
	svc.agentOrchestrator.registerChild(parentID, child.id)

	svc.mu.Lock()
	rs := svc.runs[parentID]
	rs.flowEngineDriven = true
	rs.activeHubNodeID = "synthesis"
	rs.pendingHubReinvoke = true
	rs.pendingHubReinvokePrompt = "armed hub.notify prompt re-armed while blocked"
	rs.lastReviewCohortVerdicts = map[string]string{"spec_align": "blocked", "reviewer": "approved"}
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "spec_align", Cohort: "review", Behavior: "agent.delegate"},
		{ID: "reviewer", Cohort: "review", Behavior: "agent.delegate"},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	svc.mu.Unlock()

	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{
		Status:      "blocked",
		BlockReason: "escalate",
		GateReason:  "advanceHubDoneThroughEdge: synthesis done blocked — reviewer verdict not approved: spec_align=blocked",
		Cap:         20,
		RoundCap:    20,
	})

	if _, err := svc.resumeFlowWithFeedback(parentID, "defer AC-4 to CP-10 device gate"); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if child.status != RunStatusRunning {
		t.Fatalf("deficient member leg = %q, want running — armed hub reinvoke must not starve the member re-drive", child.status)
	}
}
