package runner

import (
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// CP-61 P-1: harness hubs (plan_synthesis / synthesis / cp_synthesis) with a
// real done-successor cannot dispatch it on a self-grade alone — the inbound
// cohort must have recorded approved via submit_review_outcome.
// New file; no pre-existing test is modified.

func cp61PlanTopology() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	edges := []agentpack.FlowEdge{
		{From: "plan_reviewer", To: "plan_synthesis", When: "done", Kind: "forward"},
		{From: "plan_synthesis", To: "plan_writer", When: "continue", Kind: "back"},
		{From: "plan_synthesis", To: "preflight_contract_freeze", When: "done", Kind: "forward"},
		{From: "preflight_contract_freeze", To: "test_signatures", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "plan_writer", Behavior: "agent.delegate", Agent: "agents/doc-writer.md", Lifecycle: "reinvoke"},
		{ID: "plan_reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md", Lifecycle: "spawn", Cohort: "plan"},
		{ID: "plan_synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke", Join: "all"},
		{ID: "preflight_contract_freeze", Behavior: "contract.freeze", Lifecycle: "once"},
		{ID: "test_signatures", Behavior: "agent.code", Agent: "agents/coder.md", Lifecycle: "once"},
	}
	return edges, nodes
}

func cp61PlanService(t *testing.T, pk ProviderKey) (*InteractiveService, string) {
	t.Helper()
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestServiceForProvider(t, pk)
	edges, nodes := cp61PlanTopology()
	runID := newFreezeTestRunForProvider(t, svc, pk, dir, edges, nodes, head)
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.activeHubNodeID = "plan_synthesis"
	rs.autoOrchestrate = true
	rs.currentTurnID = "turn-cp61-plan"
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 5, RoundCap: 5})
	svc.reseedFlowStepRuntime(runID, nodes)
	return svc, runID
}

func cp61CodeTopology() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	edges := []agentpack.FlowEdge{
		{From: "reviewer", To: "synthesis", When: "done", Kind: "forward"},
		{From: "synthesis", To: "implement", When: "continue", Kind: "back"},
		{From: "synthesis", To: "audit", When: "done", Kind: "forward"},
		{From: "audit", To: "done", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "implement", Behavior: "agent.code", Agent: "agents/coder.md", Lifecycle: "reinvoke"},
		{ID: "reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md", Lifecycle: "spawn", Cohort: "review"},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke", Join: "all"},
		{ID: "audit", Behavior: "artifact.audit_draft", Lifecycle: "once"},
	}
	return edges, nodes
}

func cp61CodeService(t *testing.T, pk ProviderKey) (*InteractiveService, string) {
	t.Helper()
	svc := newFreezeTestServiceForProvider(t, pk)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: pk})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	edges, nodes := cp61CodeTopology()
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
	rs.currentTurnID = "turn-cp61-code"
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 5, RoundCap: 5})
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	return svc, parent.RunID
}

func cp61CpTopology() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	edges := []agentpack.FlowEdge{
		{From: "cp_reviewer", To: "cp_synthesis", When: "done", Kind: "forward"},
		{From: "cp_synthesis", To: "cp_plan_writer", When: "continue", Kind: "back"},
		{From: "cp_synthesis", To: "task_splitter", When: "done", Kind: "forward"},
		{From: "task_splitter", To: "audit", When: "done", Kind: "forward"},
		{From: "audit", To: "done", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "cp_plan_writer", Behavior: "agent.delegate", Agent: "agents/doc-writer.md", Lifecycle: "reinvoke"},
		{ID: "cp_reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md", Lifecycle: "spawn", Cohort: "plan"},
		{ID: "cp_synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke", Join: "all"},
		{ID: "task_splitter", Behavior: "agent.delegate", Agent: "agents/doc-writer.md", Lifecycle: "once"},
		{ID: "audit", Behavior: "artifact.audit_draft", Lifecycle: "once"},
	}
	return edges, nodes
}

func cp61CpService(t *testing.T, pk ProviderKey) (*InteractiveService, string) {
	t.Helper()
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestServiceForProvider(t, pk)
	edges, nodes := cp61CpTopology()
	runID := newFreezeTestRunForProvider(t, svc, pk, dir, edges, nodes, head)
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.activeHubNodeID = "cp_synthesis"
	rs.autoOrchestrate = true
	rs.currentTurnID = "turn-cp61-cp"
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 5, RoundCap: 5})
	svc.reseedFlowStepRuntime(runID, nodes)
	return svc, runID
}

func cp61SeedVerdicts(t *testing.T, svc *InteractiveService, runID string, verdicts map[string]string) {
	t.Helper()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if rs := svc.runs[runID]; rs != nil {
		rs.lastReviewCohortVerdicts = verdicts
	}
}

// cp61StampOneDecision burns the turn's flow-control slot so the next
// applyFlowControl errors (the exotic applyErr path).
func cp61StampOneDecision(t *testing.T, svc *InteractiveService, runID string) {
	t.Helper()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	if rs == nil || rs.currentTurnID == "" {
		t.Fatalf("run %q missing currentTurnID", runID)
	}
	rs.lastFlowControlTurnID = rs.currentTurnID
}

func cp61Providers() []ProviderKey {
	return []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
}

func TestCP61HubDone(t *testing.T) {
	t.Run("plan_synthesis_missing_blocks", func(t *testing.T) {
		for _, pk := range cp61Providers() {
			t.Run(string(pk), func(t *testing.T) {
				svc, runID := cp61PlanService(t, pk)
				svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
					st.Round = 2
					return st
				})
				cp61SeedVerdicts(t, svc, runID, nil)

				res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: validPlannerDraft})
				if !handled {
					t.Fatalf("%s: missing verdict must be claimed (escalated), got handled=false", pk)
				}
				if res.NextAction != "awaiting_user" {
					t.Fatalf("%s: result NextAction = %q, want awaiting_user", pk, res.NextAction)
				}
				if got := flowStepStatus(t, svc, runID, "preflight_contract_freeze"); got == StepStatusRunning || got == StepStatusDone {
					t.Fatalf("%s: freeze = %v, want not RUNNING/DONE without a PASS verdict", pk, got)
				}
				if st := svc.agentOrchestrator.loopStateFor(runID); st.Status == "done" {
					t.Fatalf("%s: loop must not reach done without verdicts", pk)
				} else if st.Round != 2 {
					t.Fatalf("%s: loop Round = %d, want 2 (no reset on block)", pk, st.Round)
				}
				if st := svc.agentOrchestrator.loopStateFor(runID); st.BlockReason == "hub_stalled" {
					t.Fatalf("%s: must escalate immediately, not stall", pk)
				}
			})
		}
	})

	t.Run("plan_synthesis_changes_requested_continues", func(t *testing.T) {
		for _, pk := range cp61Providers() {
			t.Run(string(pk), func(t *testing.T) {
				svc, runID := cp61PlanService(t, pk)
				cp61SeedVerdicts(t, svc, runID, map[string]string{"plan_reviewer": "changes_requested"})

				res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: validPlannerDraft})
				if !handled {
					t.Fatalf("%s: changes_requested must be claimed (continue), got handled=false", pk)
				}
				if res.NextAction == "awaiting_user" {
					t.Fatalf("%s: changes_requested must continue, not park: %+v", pk, res)
				}
				if got := flowStepStatus(t, svc, runID, "preflight_contract_freeze"); got == StepStatusDone {
					t.Fatalf("%s: freeze must NOT run on changes_requested", pk)
				}
				if got := flowStepStatus(t, svc, runID, "plan_writer"); got != StepStatusRunning {
					t.Fatalf("%s: plan_writer = %v, want RUNNING (continue re-entry)", pk, got)
				}
				if st := svc.agentOrchestrator.loopStateFor(runID); st.Status == "done" || st.Status == "blocked" {
					t.Fatalf("%s: loop = %q, want running (continue)", pk, st.Status)
				}
			})
		}
	})

	t.Run("plan_synthesis_approved_dispatches_freeze", func(t *testing.T) {
		for _, pk := range cp61Providers() {
			t.Run(string(pk), func(t *testing.T) {
				svc, runID := cp61PlanService(t, pk)
				cp61SeedVerdicts(t, svc, runID, map[string]string{"plan_reviewer": "approved"})

				res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: validPlannerDraft})
				if !handled {
					t.Fatalf("%s: approved plan must dispatch freeze, got handled=false", pk)
				}
				if res.NextAction == "awaiting_user" {
					t.Fatalf("%s: approved plan must not park: %+v", pk, res)
				}
				if got := flowStepStatus(t, svc, runID, "plan_synthesis"); got != StepStatusDone {
					t.Fatalf("%s: plan_synthesis = %v, want DONE", pk, got)
				}
				if got := flowStepStatus(t, svc, runID, "preflight_contract_freeze"); got != StepStatusDone {
					t.Fatalf("%s: freeze = %v, want DONE", pk, got)
				}
				waitLoop(t, string(pk)+": writer spawned after freeze", 3*time.Second, func() bool {
					return countChildrenWithLabel(svc, runID, "test_signatures") == 1
				})
				if st := svc.agentOrchestrator.loopStateFor(runID); st.BlockReason == "hub_stalled" {
					t.Fatalf("%s: must not stall on approved plan", pk)
				}
			})
		}
	})

	t.Run("plan_synthesis_wrong_cohort_blocks", func(t *testing.T) {
		for _, pk := range cp61Providers() {
			t.Run(string(pk), func(t *testing.T) {
				svc, runID := cp61PlanService(t, pk)
				cp61SeedVerdicts(t, svc, runID, map[string]string{"reviewer": "approved"})

				res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: validPlannerDraft})
				if !handled {
					t.Fatalf("%s: code-cohort leftover must not satisfy plan hub, got handled=false", pk)
				}
				if res.NextAction != "awaiting_user" {
					t.Fatalf("%s: result NextAction = %q, want awaiting_user (blocked)", pk, res.NextAction)
				}
				if got := flowStepStatus(t, svc, runID, "preflight_contract_freeze"); got == StepStatusDone {
					t.Fatalf("%s: freeze must NOT run on wrong-cohort verdict", pk)
				}
				if st := svc.agentOrchestrator.loopStateFor(runID); st.Status == "done" {
					t.Fatalf("%s: loop must not reach done on wrong-cohort verdict", pk)
				}
			})
		}
	})

	t.Run("synthesis_missing_blocks", func(t *testing.T) {
		for _, pk := range cp61Providers() {
			t.Run(string(pk), func(t *testing.T) {
				svc, runID := cp61CodeService(t, pk)
				cp61SeedVerdicts(t, svc, runID, nil)

				res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: "review approved"})
				if !handled {
					t.Fatalf("%s: missing verdict must be claimed (escalated), got handled=false", pk)
				}
				if res.NextAction != "awaiting_user" {
					t.Fatalf("%s: result NextAction = %q, want awaiting_user", pk, res.NextAction)
				}
				if got := flowStepStatus(t, svc, runID, "audit"); got == StepStatusDone || got == StepStatusRunning {
					t.Fatalf("%s: audit must NOT dispatch without a PASS verdict, got %v", pk, got)
				}
				if st := svc.agentOrchestrator.loopStateFor(runID); st.Status == "done" {
					t.Fatalf("%s: loop must not reach done without verdicts", pk)
				}
			})
		}
	})

	t.Run("synthesis_approved_dispatches_audit", func(t *testing.T) {
		for _, pk := range cp61Providers() {
			t.Run(string(pk), func(t *testing.T) {
				svc, runID := cp61CodeService(t, pk)
				// Must NOT require plan_reviewer: only the review cohort gates synthesis.
				cp61SeedVerdicts(t, svc, runID, map[string]string{"reviewer": "approved"})

				res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: "review approved"})
				if !handled {
					t.Fatalf("%s: approved synthesis must dispatch audit, got handled=false", pk)
				}
				// The verdict gate passed when synthesis marks DONE and audit
				// dispatches (RUNNING). Audit's own verification may still
				// escalate (blocked_validation_failed) — that is downstream of
				// the gate, so res may be awaiting_user; what matters is the
				// successor dispatched instead of a verdict block.
				_ = res
				if got := flowStepStatus(t, svc, runID, "synthesis"); got != StepStatusDone {
					st := svc.agentOrchestrator.loopStateFor(runID)
					t.Fatalf("%s: synthesis = %v, want DONE (gate passed): loop=%+v", pk, got, st)
				}
				if got := flowStepStatus(t, svc, runID, "audit"); got != StepStatusDone && got != StepStatusRunning {
					st := svc.agentOrchestrator.loopStateFor(runID)
					t.Fatalf("%s: audit = %v, want RUNNING or DONE (dispatched): loop=%+v", pk, got, st)
				}
				if st := svc.agentOrchestrator.loopStateFor(runID); st.BlockReason == "hub_stalled" {
					t.Fatalf("%s: must not stall on approved synthesis", pk)
				}
			})
		}
	})

	t.Run("cp_synthesis_missing_blocks", func(t *testing.T) {
		for _, pk := range cp61Providers() {
			t.Run(string(pk), func(t *testing.T) {
				svc, runID := cp61CpService(t, pk)
				cp61SeedVerdicts(t, svc, runID, nil)

				res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: "cp approved"})
				if !handled {
					t.Fatalf("%s: missing cp verdict must be claimed, got handled=false", pk)
				}
				if res.NextAction != "awaiting_user" {
					t.Fatalf("%s: result NextAction = %q, want awaiting_user", pk, res.NextAction)
				}
				if got := flowStepStatus(t, svc, runID, "task_splitter"); got == StepStatusRunning || got == StepStatusDone {
					t.Fatalf("%s: splitter = %v, want not RUNNING/DONE without a PASS verdict", pk, got)
				}
				if n := countChildrenWithLabel(svc, runID, "task_splitter"); n != 0 {
					t.Fatalf("%s: splitter children = %d, want 0 (must not dispatch)", pk, n)
				}
			})
		}
	})

	t.Run("cp_synthesis_approved_dispatches_splitter", func(t *testing.T) {
		for _, pk := range cp61Providers() {
			t.Run(string(pk), func(t *testing.T) {
				svc, runID := cp61CpService(t, pk)
				cp61SeedVerdicts(t, svc, runID, map[string]string{"cp_reviewer": "approved"})

				res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: "cp approved"})
				if !handled {
					t.Fatalf("%s: approved cp plan must dispatch splitter, got handled=false", pk)
				}
				if res.NextAction == "awaiting_user" {
					t.Fatalf("%s: approved cp plan must not park: %+v", pk, res)
				}
				if got := flowStepStatus(t, svc, runID, "cp_synthesis"); got != StepStatusDone {
					t.Fatalf("%s: cp_synthesis = %v, want DONE", pk, got)
				}
				if got := flowStepStatus(t, svc, runID, "task_splitter"); got != StepStatusRunning {
					t.Fatalf("%s: splitter = %v, want RUNNING (delegate dispatched)", pk, got)
				}
				if n := countChildrenWithLabel(svc, runID, "task_splitter"); n != 1 {
					t.Fatalf("%s: splitter children = %d, want 1", pk, n)
				}
			})
		}
	})

	t.Run("cp_reviewer_records_verdict", func(t *testing.T) {
		for _, pk := range cp61Providers() {
			t.Run(string(pk), func(t *testing.T) {
				svc, runID := cp61CpService(t, pk)
				const childID = "child-cp-reviewer"
				svc.mu.Lock()
				svc.runs[childID] = &interactiveRun{
					id:           childID,
					parentRunID:  runID,
					flowCohortId: "plan-cohort",
					label:        "cp_reviewer",
					providerKey:  pk,
					subs:         map[int64]chan ProviderEvent{},
					idempotency:  map[string]string{},
				}
				child := svc.runs[childID]
				svc.mu.Unlock()

				fc, err := reviewOutcomeToFlowControl(ReviewOutcomeInput{Status: "approved", Feedback: "cp looks good"})
				if err != nil {
					t.Fatalf("reviewOutcomeToFlowControl: %v", err)
				}
				res, err := (&turnBridge{svc: svc, rs: child}).SubmitFlowControl(fc)
				if err != nil {
					t.Fatalf("cp_reviewer record-only submit_review_outcome: %v", err)
				}
				if res.NextAction != "review_verdict_recorded" {
					t.Fatalf("NextAction = %q, want review_verdict_recorded", res.NextAction)
				}
				if st := svc.agentOrchestrator.loopStateFor(runID); st.Status == "done" {
					t.Fatal("reviewer verdict must not advance loop to done")
				}
				svc.mu.Lock()
				got := svc.runs[runID].pendingReviewVerdictByLabel["cp_reviewer"]
				svc.mu.Unlock()
				if got != "approved" {
					t.Fatalf("pendingReviewVerdictByLabel = %q, want approved", got)
				}
			})
		}
	})

	t.Run("normal_chat_unaffected", func(t *testing.T) {
		for _, pk := range cp61Providers() {
			t.Run(string(pk), func(t *testing.T) {
				svc := newFreezeTestServiceForProvider(t, pk)
				parent, runErr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: pk})
				if runErr != nil {
					t.Fatalf("createRun: %v", runErr)
				}
				svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

				fc, err := reviewOutcomeToFlowControl(ReviewOutcomeInput{Status: "approved"})
				if err != nil {
					t.Fatalf("reviewOutcomeToFlowControl: %v", err)
				}
				if _, err := svc.applyFlowControl(parent.RunID, fc); err != nil {
					t.Fatalf("normal chat approved→done must not require cohort verdicts: %v", err)
				}
			})
		}
	})

	t.Run("churned_plan_pass_parks_missing_escalates", func(t *testing.T) {
		for _, pk := range cp61Providers() {
			t.Run(string(pk), func(t *testing.T) {
				// PASS + churned still parks for human read.
				svc, runID := cp61PlanService(t, pk)
				task325SeedWriter(t, svc, runID, pk, 1)
				cp61SeedVerdicts(t, svc, runID, map[string]string{"plan_reviewer": "approved"})

				res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: validPlannerDraft})
				if !handled {
					t.Fatalf("%s: churned PASS must be claimed (parked), got handled=false", pk)
				}
				if res.NextAction != "awaiting_user" {
					t.Fatalf("%s: churned PASS result = %+v, want awaiting_user (park)", pk, res)
				}
				if st := svc.agentOrchestrator.loopStateFor(runID); st.BlockReason != planApprovalBlockReason {
					t.Fatalf("%s: loop = %+v, want blocked/plan_approval", pk, st)
				}
				if got := flowStepStatus(t, svc, runID, "preflight_contract_freeze"); got == StepStatusDone {
					t.Fatalf("%s: freeze must NOT run before human approval", pk)
				}

				// Missing verdict + churned must NOT park — the verdict gate
				// fires first (escalate), never the approval park.
				svc2, runID2 := cp61PlanService(t, pk)
				task325SeedWriter(t, svc2, runID2, pk, 1)
				cp61SeedVerdicts(t, svc2, runID2, nil)

				res2, handled2 := svc2.advanceHubDoneThroughEdge(runID2, FlowControlInput{Status: "done", Summary: validPlannerDraft})
				if !handled2 {
					t.Fatalf("%s: churned missing verdict must be claimed, got handled=false", pk)
				}
				if st := svc2.agentOrchestrator.loopStateFor(runID2); st.BlockReason == planApprovalBlockReason {
					t.Fatalf("%s: missing verdict must escalate, not park plan_approval: %+v", pk, res2)
				}
				if got := flowStepStatus(t, svc2, runID2, "preflight_contract_freeze"); got == StepStatusDone {
					t.Fatalf("%s: freeze must NOT run without a PASS verdict", pk)
				}
			})
		}
	})

	t.Run("apply_err_missing_verdict_awaiting_user", func(t *testing.T) {
		for _, pk := range cp61Providers() {
			t.Run(string(pk), func(t *testing.T) {
				svc, runID := cp61PlanService(t, pk)
				cp61SeedVerdicts(t, svc, runID, nil)
				cp61StampOneDecision(t, svc, runID)

				res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: validPlannerDraft})
				if !handled {
					t.Fatalf("%s: applyErr escalate must still be claimed, got handled=false", pk)
				}
				if res.NextAction != "awaiting_user" || res.Status != "blocked" {
					t.Fatalf("%s: applyErr result = %+v, want blocked/awaiting_user (not empty success)", pk, res)
				}
				if got := flowStepStatus(t, svc, runID, "preflight_contract_freeze"); got == StepStatusRunning || got == StepStatusDone {
					t.Fatalf("%s: freeze = %v, must not dispatch after applyErr", pk, got)
				}
				if st := svc.agentOrchestrator.loopStateFor(runID); st.Status == "done" {
					t.Fatalf("%s: loop must not settle done on applyErr", pk)
				}
			})
		}
	})

	t.Run("apply_err_changes_requested_no_continue", func(t *testing.T) {
		for _, pk := range cp61Providers() {
			t.Run(string(pk), func(t *testing.T) {
				svc, runID := cp61PlanService(t, pk)
				cp61SeedVerdicts(t, svc, runID, map[string]string{"plan_reviewer": "changes_requested"})
				cp61StampOneDecision(t, svc, runID)

				res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: validPlannerDraft})
				if !handled {
					t.Fatalf("%s: applyErr continue must still be claimed, got handled=false", pk)
				}
				if res.NextAction != "awaiting_user" || res.Status != "blocked" {
					t.Fatalf("%s: applyErr continue result = %+v, want blocked/awaiting_user", pk, res)
				}
				if got := flowStepStatus(t, svc, runID, "preflight_contract_freeze"); got == StepStatusDone {
					t.Fatalf("%s: freeze must not run on applyErr continue", pk)
				}
				if got := flowStepStatus(t, svc, runID, "plan_writer"); got == StepStatusRunning {
					t.Fatalf("%s: plan_writer must not re-enter when continue applyErr", pk)
				}
				if n := countChildrenWithLabel(svc, runID, "plan_writer"); n != 0 {
					t.Fatalf("%s: plan_writer children = %d, want 0", pk, n)
				}
			})
		}
	})
}
