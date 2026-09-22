package runner

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// TestBug353HubDoneRealSuccessorStampsDecision locks the BUG-353 regression
// (run-198468): a hub synthesis turn whose submit_review_outcome(approved) was
// routed through advanceHubDoneThroughEdge's generic-successor branch
// (plan_synthesis --done--> preflight_contract_freeze) never stamped the
// one-decision guard, so BUG-226 escalated the hub to WAITING_USER_APPROVAL
// ("completed without submit_review_outcome") even though the tool call
// succeeded (reproduced 3x incl. a Retry). The hub.notify branch already
// stamps (BUG-289); this locks the generic branch to the same contract.
//
// Provider parity: the stamped path (SubmitFlowControl -> advanceHubDoneThroughEdge)
// never branches on providerKey (grep: advanceHubDoneThroughEdge takes no
// provider key; every adapter's submit_review_outcome handler routes through
// bridge.SubmitFlowControl), so the subtest matrix over all three provider
// keys guards a future provider-specific drift.
func TestBug353HubDoneRealSuccessorStampsDecision(t *testing.T) {
	providers := []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
	for _, pk := range providers {
		t.Run(string(pk), func(t *testing.T) {
			ch := make(chan TurnRequest, 1)
			reg := newProviderRegistry()
			registerKeyedCapture(reg, pk, ch)
			svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
			parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: pk})
			if err != nil {
				t.Fatalf("createRun: %v", err)
			}
			svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 3, RoundCap: 3})

			edges := []agentpack.FlowEdge{
				{From: "plan_synthesis", To: "preflight_contract_freeze", When: "done", Kind: "forward"},
			}
			nodes := []agentpack.FlowNode{
				{ID: "plan_synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke", Join: "all"},
				{ID: "preflight_contract_freeze", Behavior: "contract.freeze", Lifecycle: "once"},
			}
			svc.mu.Lock()
			rs := svc.runs[parent.RunID]
			rs.activeFlowEdges = edges
			rs.activeFlowNodes = nodes
			rs.flowEngineDriven = true
			rs.autoOrchestrate = true
			rs.activeHubNodeID = "plan_synthesis"
			rs.currentTurnID = "turn-198535"
			svc.mu.Unlock()

			res, handled := svc.advanceHubDoneThroughEdge(parent.RunID, FlowControlInput{Status: "done", Summary: "calc-core approved"})
			if !handled {
				t.Fatal("expected advanceHubDoneThroughEdge to take over plan_synthesis -> freeze")
			}
			// The one-decision guard must be stamped so BUG-226's escalate
			// fallback cannot fire on this turn.
			if !svc.flowControlSubmittedForTurn(parent.RunID, "turn-198535") {
				t.Fatal("expected the hub turn to count as a flow-control decision (BUG-353 stamp)")
			}
			// A second decision on the same turn must be rejected (one-decision guard).
			if _, err := svc.applyFlowControl(parent.RunID, FlowControlInput{Status: "done", Summary: "dup"}); err == nil {
				t.Fatal("expected duplicate flow_control on the same turn to be rejected")
			}
			// BUG-284 semantics (extended from hub.notify): the caller's verdict
			// is accepted — never "looping", which reads to the model as a
			// rejection and made it re-answer in prose.
			if res.NextAction == "looping" {
				t.Fatalf("NextAction = looping must not be reported for an accepted done; got %+v", res)
			}
			if res.Status == "" {
				t.Fatal("expected a non-empty flow result status from the successor dispatch")
			}
		})
	}
}

// TestBug353HubDoneAuditSuccessorAlsoStamps proves the same stamp applies to
// the other real-successor shape (rag-harness synthesis --done--> audit) that
// pre-existing coverage exercised only for step status, never for the
// one-decision guard — the near-miss that let BUG-353 ship.
func TestBug353HubDoneAuditSuccessorAlsoStamps(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	edges, nodes := synthesisBackEdgeFixture()
	dir := t.TempDir()
	initGitRepoForAuditFixture(t, dir)

	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.workspaceCwd = dir
	rs.currentTurnID = "turn-audit-1"
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parent.RunID, nodes)

	res, handled := svc.advanceHubDoneThroughEdge(parent.RunID, FlowControlInput{Status: "done", Summary: "review approved"})
	if !handled {
		t.Fatal("expected advanceHubDoneThroughEdge to take over synthesis -> audit")
	}
	if !svc.flowControlSubmittedForTurn(parent.RunID, "turn-audit-1") {
		t.Fatal("expected the synthesis turn to count as a decision on the audit successor (BUG-353 stamp)")
	}
	if got := flowStepStatus(t, svc, parent.RunID, "audit"); got != StepStatusRunning && got != StepStatusWaitingUserApr {
		t.Fatalf("audit step status = %v, want RUNNING or WAITING_USER_APPROVAL after synthesis approved", got)
	}
	if res.NextAction == "looping" {
		t.Fatalf("NextAction = looping must not be reported for an accepted done; got %+v", res)
	}
}
