package runner

// BUG-505 (live run-3688 / run-20370): a contract.freeze escalate for a
// missing planner proposal had no resolution path — Continue fed the raw
// operator feedback straight into runContractFreezeNode as plannerResult,
// so prose ("contract-planner must emit the JSON") was parse-failed verbatim
// ("invalid planner proposal: 'c'" — first char of the note) and the freeze
// re-escalated forever. Only an operator-supplied JSON draft unblocked it.
//
// Fixed behavior: prose feedback (or a bare continue) with no retrievable
// draft retries the planner delegate (preflight_contract_plan) with the note
// as guidance — the same delegate-retry path a failed writer takes.
// An operator-supplied parseable draft still feeds the freeze directly
// (live-verified escape hatch), and a draft retrievable from a child event
// or the parent cache proceeds on its own — neither needs a scout re-run.
// New file; no pre-existing test is modified.

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

func bug505PlanFreezeTopology() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	edges := []agentpack.FlowEdge{
		{From: "preflight_contract_plan", To: "preflight_contract_freeze", When: "done", Kind: "forward"},
		{From: "preflight_contract_freeze", To: "test_signatures", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "preflight_contract_plan", Behavior: "agent.delegate", Agent: "agents/contract-planner.md"},
		{ID: "preflight_contract_freeze", Behavior: "contract.freeze", Lifecycle: "once"},
		{ID: "test_signatures", Behavior: "agent.code", Agent: "agents/coder.md"},
	}
	return edges, nodes
}

func bug505SeedFailedPlanner(svc *InteractiveService, runID string) {
	planner := &interactiveRun{
		id:          "run-planner-505",
		parentRunID: runID,
		label:       "preflight_contract_plan",
		agentName:   "contract-planner",
		status:      RunStatusCompleted,
		subs:        map[int64]chan ProviderEvent{},
		events: []ProviderEvent{
			{Type: EventTurnCompleted, FinalMessage: "I reviewed the codebase and designed the contract — but emitted only prose."},
		},
	}
	svc.mu.Lock()
	svc.runs[planner.id] = planner
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(runID, planner.id)
}

func bug505DriveToFreezeEscalate(t *testing.T, svc *InteractiveService, runID string, edges []agentpack.FlowEdge, nodes []agentpack.FlowNode) {
	t.Helper()
	freezeNode, ok := findFlowNode(nodes, "preflight_contract_freeze")
	if !ok {
		t.Fatal("topology must contain preflight_contract_freeze")
	}
	if !svc.runContractFreezeNode(context.Background(), runID, edges, nodes, freezeNode, "contract-planner's prose output") {
		t.Fatal("freeze must escalate on an unparseable planner result")
	}
	if st := svc.agentOrchestrator.loopStateFor(runID); st.Status != "blocked" || st.BlockReason != "escalate" {
		t.Fatalf("loop = %+v, want blocked/escalate", st)
	}
}

// The live repro: prose continue on a freeze park must retry the planner,
// not consume the prose as the draft.
func TestBug505_FreezeEscalateProseRetriesPlanner(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := bug505PlanFreezeTopology()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	svc.reseedFlowStepRuntime(runID, nodes)
	bug505SeedFailedPlanner(svc, runID)
	bug505DriveToFreezeEscalate(t, svc, runID, edges, nodes)

	if _, err := svc.resumeFlowWithFeedback(runID, "retry: emit the contract JSON draft"); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}

	svc.mu.Lock()
	p := svc.runs["run-planner-505"]
	reinvoked := p != nil && (p.status == RunStatusRunning || p.activationSeq > 0)
	svc.mu.Unlock()
	if !reinvoked {
		t.Fatalf("prose continue must re-invoke the planner child; planner = %+v", p)
	}
	// The freeze must NOT have re-escalated synchronously by parse-failing the
	// prose — the planner retry owns the next attempt.
	if st := svc.agentOrchestrator.loopStateFor(runID); st.BlockReason == "escalate" {
		t.Fatalf("loop re-escalated on prose-as-draft: %+v", st)
	}
}

// A bare Continue (Retry chip, empty feedback) hits the same dead end live —
// it must retry the planner too, not re-parse "".
func TestBug505_FreezeEscalateBareContinueRetriesPlanner(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := bug505PlanFreezeTopology()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	svc.reseedFlowStepRuntime(runID, nodes)
	bug505SeedFailedPlanner(svc, runID)
	bug505DriveToFreezeEscalate(t, svc, runID, edges, nodes)

	if _, err := svc.resumeFlowWithFeedback(runID, ""); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}

	svc.mu.Lock()
	p := svc.runs["run-planner-505"]
	reinvoked := p != nil && (p.status == RunStatusRunning || p.activationSeq > 0)
	svc.mu.Unlock()
	if !reinvoked {
		t.Fatalf("bare continue must re-invoke the planner child; planner = %+v", p)
	}
}

// Escape hatch preserved: an operator-supplied valid draft feeds the freeze
// directly — no scout re-run, freeze completes.
func TestBug505_OperatorSuppliedDraftStillFeedsFreeze(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := bug505PlanFreezeTopology()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	svc.reseedFlowStepRuntime(runID, nodes)
	bug505SeedFailedPlanner(svc, runID)
	bug505DriveToFreezeEscalate(t, svc, runID, edges, nodes)

	if _, err := svc.resumeFlowWithFeedback(runID, validPlannerDraft); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}
	waitLoop(t, "freeze completes on operator-supplied draft", 3_000_000_000, func() bool {
		return flowStepStatus(t, svc, runID, "preflight_contract_freeze") == StepStatusDone
	})
	// The planner child must NOT have been re-invoked for a draft the
	// operator already supplied.
	svc.mu.Lock()
	p := svc.runs["run-planner-505"]
	svc.mu.Unlock()
	if p != nil && p.status == RunStatusRunning {
		t.Fatal("planner must not re-run when operator supplied the draft")
	}
}

// Near-miss: a draft that becomes retrievable (e.g. a re-driven scout emitted
// it after the escalate) means the freeze can proceed on its own — prose
// continue must NOT re-run the planner.
func TestBug505_RetrievableDraftSkipsPlannerRetry(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	edges, nodes := bug505PlanFreezeTopology()
	runID := newFreezeTestRun(t, svc, dir, edges, nodes, head)
	svc.reseedFlowStepRuntime(runID, nodes)
	bug505SeedFailedPlanner(svc, runID)
	bug505DriveToFreezeEscalate(t, svc, runID, edges, nodes)

	// After the park, a valid draft lands on the planner child (re-drive
	// between escalate and operator continue — the recovery path).
	svc.mu.Lock()
	svc.runs["run-planner-505"].events = append(svc.runs["run-planner-505"].events,
		ProviderEvent{Type: EventTurnCompleted, FinalMessage: validPlannerDraft})
	svc.mu.Unlock()

	if _, err := svc.resumeFlowWithFeedback(runID, "prose note"); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}
	waitLoop(t, "freeze proceeds on the retrievable draft", 3_000_000_000, func() bool {
		return flowStepStatus(t, svc, runID, "preflight_contract_freeze") == StepStatusDone
	})
	svc.mu.Lock()
	p := svc.runs["run-planner-505"]
	svc.mu.Unlock()
	if p != nil && p.status == RunStatusRunning {
		t.Fatal("planner must not re-run when its draft is retrievable")
	}
}
