package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// run-207435: plan_approval re-parked AFTER freeze DONE. Live: churned plan
// approved → freeze DONE → code phase running → hub_stalled Retry reinvoked
// the hub while activeHubNodeID still pointed at plan_synthesis → stale done
// passed the (still-approved) verdict snapshot → planLoopChurned latch
// re-parked plan_approval with the stale "writer round 1" reason, cancelling
// the live implement turn. Freeze DONE must close the plan gate for good.
// New file; no pre-existing test is modified.

func run207435SeedVerdicts(t *testing.T, svc *InteractiveService, runID string, verdicts map[string]string) {
	t.Helper()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if rs := svc.runs[runID]; rs != nil {
		rs.lastReviewCohortVerdicts = verdicts
	}
}

// run207435PlanServiceWithCohort mirrors task325PlanService (same workspace,
// planner draft, loop, reseed) but declares plan_reviewer with Cohort "plan"
// so the CP-61 verdict gate actually evaluates on these subtests — the shared
// task325PlanTopology has no cohort nodes, which silently skips the gate.
// Lives here (not in the legacy helper) per additive-tests-only.
func run207435PlanServiceWithCohort(t *testing.T, pk ProviderKey) (*InteractiveService, string) {
	t.Helper()
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestServiceForProvider(t, pk)
	edges, nodes := task325PlanTopology()
	edges = append(edges, agentpack.FlowEdge{From: "plan_reviewer", To: "plan_synthesis", When: "done", Kind: "forward"})
	nodes = append(nodes, agentpack.FlowNode{ID: "plan_reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md", Lifecycle: "spawn", Cohort: "plan"})
	runID := newFreezeTestRunForProvider(t, svc, pk, dir, edges, nodes, head)
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.activeHubNodeID = "plan_synthesis"
	rs.autoOrchestrate = true
	rs.currentTurnID = "turn-207435-plan"
	planner := &interactiveRun{
		id:          "run-planner-207435",
		parentRunID: runID,
		label:       "preflight_contract_plan",
		agentName:   "contract-planner",
		status:      RunStatusCompleted,
		providerKey: pk,
		subs:        map[int64]chan ProviderEvent{},
		events:      []ProviderEvent{{Type: EventTurnCompleted, FinalMessage: validPlannerDraft}},
	}
	svc.runs[planner.id] = planner
	svc.agentOrchestrator.registerChild(runID, planner.id)
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 3, RoundCap: 3})
	svc.reseedFlowStepRuntime(runID, nodes)
	return svc, runID
}

func run207435CodeTopology() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	edges := []agentpack.FlowEdge{
		{From: "plan_synthesis", To: "preflight_contract_freeze", When: "done", Kind: "forward"},
		{From: "plan_synthesis", To: "plan_writer", When: "continue", Kind: "back"},
		{From: "reviewer", To: "synthesis", When: "done", Kind: "forward"},
		{From: "synthesis", To: "audit", When: "done", Kind: "forward"},
		{From: "audit", To: "done", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "plan_synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke", Join: "all"},
		{ID: "preflight_contract_freeze", Behavior: "contract.freeze", Lifecycle: "once"},
		{ID: "plan_writer", Behavior: "agent.delegate", Agent: "agents/doc-writer.md", Lifecycle: "reinvoke"},
		{ID: "reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md", Lifecycle: "spawn", Cohort: "review"},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke", Join: "all"},
		{ID: "audit", Behavior: "artifact.audit_draft", Lifecycle: "once"},
	}
	return edges, nodes
}

func run207435CodeService(t *testing.T, pk ProviderKey) (*InteractiveService, string) {
	t.Helper()
	svc := newFreezeTestServiceForProvider(t, pk)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: pk})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	edges, nodes := run207435CodeTopology()
	dir := t.TempDir()
	initGitRepoForAuditFixture(t, dir)
	writeBaseline(t, dir, "go version")
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.workspaceCwd = dir
	rs.activeHubNodeID = "synthesis"
	rs.currentTurnID = "turn-207435-code"
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 5, RoundCap: 5})
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	return svc, parent.RunID
}

func TestRun207435NoReparkAfterFreezeDone(t *testing.T) {
	providers := []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}

	t.Run("freeze_done_churned_done_advances", func(t *testing.T) {
		for _, pk := range providers {
			t.Run(string(pk), func(t *testing.T) {
				svc, runID := run207435PlanServiceWithCohort(t, pk)
				task325SeedWriter(t, svc, runID, pk, 1)
				run207435SeedVerdicts(t, svc, runID, map[string]string{"plan_reviewer": "approved"})
				svc.setFlowStepStatus(context.Background(), runID, "plan_synthesis", StepStatusDone)
				svc.setFlowStepStatus(context.Background(), runID, "preflight_contract_freeze", StepStatusDone)

				res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: validPlannerDraft})
				if !handled {
					t.Fatalf("%s: stale plan done after freeze must be claimed, got handled=false", pk)
				}
				if res.NextAction == "awaiting_user" {
					t.Fatalf("%s: must not re-park after freeze DONE: %+v", pk, res)
				}
				if st := svc.agentOrchestrator.loopStateFor(runID); st.BlockReason == planApprovalBlockReason {
					t.Fatalf("%s: loop re-parked plan_approval after freeze DONE: %+v", pk, st)
				}
				if st := svc.agentOrchestrator.loopStateFor(runID); st.Status != "running" {
					t.Fatalf("%s: loop = %q, want running (no mutation)", pk, st.Status)
				}
				if got := flowStepStatus(t, svc, runID, "preflight_contract_freeze"); got != StepStatusDone {
					t.Fatalf("%s: freeze = %v, want still DONE", pk, got)
				}
				if got := flowStepStatus(t, svc, runID, "plan_synthesis"); got == StepStatusWaitingUserApr {
					t.Fatalf("%s: plan_synthesis re-parked WAITING after freeze DONE", pk)
				}
				if !svc.flowControlSubmittedForTurn(runID, "turn-207435-plan") {
					t.Fatalf("%s: stale done must still count as a decision (BUG-226 guard)", pk)
				}
			})
		}
	})

	t.Run("freeze_pending_churned_still_parks", func(t *testing.T) {
		for _, pk := range providers {
			t.Run(string(pk), func(t *testing.T) {
				svc, runID := run207435PlanServiceWithCohort(t, pk)
				task325SeedWriter(t, svc, runID, pk, 1)
				run207435SeedVerdicts(t, svc, runID, map[string]string{"plan_reviewer": "approved"})

				res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: validPlannerDraft})
				if !handled {
					t.Fatalf("%s: first churned done must be claimed (parked), got handled=false", pk)
				}
				if res.NextAction != "awaiting_user" {
					t.Fatalf("%s: first park result = %+v, want awaiting_user", pk, res)
				}
				if st := svc.agentOrchestrator.loopStateFor(runID); st.BlockReason != planApprovalBlockReason {
					t.Fatalf("%s: loop = %+v, want blocked/plan_approval (CA-749 first park intact)", pk, st)
				}
			})
		}
	})

	t.Run("clean_plan_still_dispatches_freeze", func(t *testing.T) {
		for _, pk := range providers {
			t.Run(string(pk), func(t *testing.T) {
				svc, runID := run207435PlanServiceWithCohort(t, pk)
				task325SeedWriter(t, svc, runID, pk, 0)
				run207435SeedVerdicts(t, svc, runID, map[string]string{"plan_reviewer": "approved"})

				res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: validPlannerDraft})
				if !handled {
					t.Fatalf("%s: clean plan must dispatch freeze, got handled=false", pk)
				}
				if res.NextAction == "awaiting_user" {
					t.Fatalf("%s: clean plan must not park: %+v", pk, res)
				}
				if got := flowStepStatus(t, svc, runID, "preflight_contract_freeze"); got != StepStatusDone {
					t.Fatalf("%s: freeze = %v, want DONE (passthrough intact)", pk, got)
				}
			})
		}
	})

	t.Run("code_hub_after_freeze_no_plan_park", func(t *testing.T) {
		for _, pk := range providers {
			t.Run(string(pk), func(t *testing.T) {
				svc, runID := run207435CodeService(t, pk)
				task325SeedWriter(t, svc, runID, pk, 1)
				run207435SeedVerdicts(t, svc, runID, map[string]string{"reviewer": "approved"})
				svc.setFlowStepStatus(context.Background(), runID, "plan_synthesis", StepStatusDone)
				svc.setFlowStepStatus(context.Background(), runID, "preflight_contract_freeze", StepStatusDone)

				res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: "review approved"})
				if !handled {
					t.Fatalf("%s: code hub done must be claimed, got handled=false", pk)
				}
				_ = res
				if st := svc.agentOrchestrator.loopStateFor(runID); st.BlockReason == planApprovalBlockReason {
					t.Fatalf("%s: code hub must never park plan_approval: %+v", pk, st)
				}
				if got := flowStepStatus(t, svc, runID, "synthesis"); got != StepStatusDone {
					t.Fatalf("%s: synthesis = %v, want DONE", pk, got)
				}
				if got := flowStepStatus(t, svc, runID, "audit"); got != StepStatusDone && got != StepStatusRunning {
					st := svc.agentOrchestrator.loopStateFor(runID)
					t.Fatalf("%s: audit = %v, want RUNNING or DONE (dispatched): loop=%+v", pk, got, st)
				}
				if got := flowStepStatus(t, svc, runID, "preflight_contract_freeze"); got != StepStatusDone {
					t.Fatalf("%s: freeze = %v, want still DONE", pk, got)
				}
			})
		}
	})
}
