package runner

import (
	"testing"
	"time"
)

// Plan-phase round reset (dual-loop cap fix): task-harness and
// bug-plan-harness share ONE LoopState.Round across the plan loop and the
// code loop, so a contested plan used to starve the code review loop of its
// cap. A successfully dispatched plan_synthesis --done-->
// preflight_contract_freeze now resets Round to 0, giving the code phase a
// fresh budget. New file — no pre-existing test is modified.
//
// R2: advanceHubDoneThroughEdge / resetPlanPhaseRound / resumePlanApproval
// take no providerKey and never branch on one (grep), so the path is
// provider-agnostic by construction; the subtest matrix over
// Claude/Codex/Grok below locks that against future provider-specific drift.

// TestPlanPhaseRoundResetOnFreezeDispatch drives the reported repro:
// Round=2 (two plan review continues) then plan approve must dispatch freeze
// AND reset Round to 0 — the code phase that follows must not inherit the
// plan loop's spent budget.
func TestPlanPhaseRoundResetOnFreezeDispatch(t *testing.T) {
	providers := []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
	for _, pk := range providers {
		t.Run(string(pk), func(t *testing.T) {
			svc, runID, _ := run201295HubFreezeService(t, pk)
			svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
				st.Round = 2
				return st
			})

			res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: validPlannerDraft})
			if !handled {
				t.Fatalf("%s: advanceHubDoneThroughEdge must take over plan_synthesis -> freeze", pk)
			}
			if res.Round != 0 {
				t.Fatalf("%s: result Round = %d, want 0 (fresh code-phase budget)", pk, res.Round)
			}
			if st := svc.agentOrchestrator.loopStateFor(runID); st.Round != 0 {
				t.Fatalf("%s: loop Round = %d after plan approve, want 0", pk, st.Round)
			}
			if got := flowStepStatus(t, svc, runID, "preflight_contract_freeze"); got != StepStatusDone {
				t.Fatalf("%s: preflight_contract_freeze = %v, want DONE (reset must not skip the dispatch)", pk, got)
			}
			waitLoop(t, string(pk)+": writer spawned after freeze", 3*time.Second, func() bool {
				return countChildrenWithLabel(svc, runID, "test_signatures") == 1
			})
		})
	}
}

// TestPlanPhaseRoundResetOnResumeApprove covers the second reset site: a
// churned plan parks for human approval (Task-325) and the human approves —
// resumePlanApproval drives freeze directly, so it must reset Round too,
// or the park+approve path would keep the old starvation bug.
func TestPlanPhaseRoundResetOnResumeApprove(t *testing.T) {
	providers := []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
	for _, pk := range providers {
		t.Run(string(pk), func(t *testing.T) {
			svc, runID, _ := run201295HubFreezeService(t, pk)
			svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
				st.Round = 3
				return st
			})
			// Live the scout ran before the plan loop, so freeze parses the
			// stashed draft (BUG-360) instead of the empty approve message.
			svc.mu.Lock()
			svc.runs[runID].preflightDraftResult = validPlannerDraft
			svc.mu.Unlock()

			snap := svc.agentOrchestrator.graphSnapshot(runID)
			if _, ok := svc.resumePlanApproval(runID, "", snap); !ok {
				t.Fatalf("%s: resumePlanApproval (approve) must advance to freeze", pk)
			}
			if st := svc.agentOrchestrator.loopStateFor(runID); st.Round != 0 {
				t.Fatalf("%s: loop Round = %d after resume-approve, want 0", pk, st.Round)
			}
			if got := flowStepStatus(t, svc, runID, "preflight_contract_freeze"); got != StepStatusDone {
				t.Fatalf("%s: preflight_contract_freeze = %v, want DONE after resume-approve", pk, got)
			}
		})
	}
}

// TestCodePhaseHubDoesNotResetRound is the near-miss: the code-loop hub
// (synthesis --done--> audit) must keep counting toward cap — the reset is
// scoped to the plan_synthesis -> preflight_contract_freeze edge only.
func TestCodePhaseHubDoesNotResetRound(t *testing.T) {
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
	rs.currentTurnID = "turn-audit-noreset"
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	svc.agentOrchestrator.mutateLoop(parent.RunID, func(st AgentLoopState) AgentLoopState {
		st.Round = 2
		return st
	})

	res, handled := svc.advanceHubDoneThroughEdge(parent.RunID, FlowControlInput{Status: "done", Summary: "review approved"})
	if !handled {
		t.Fatal("expected advanceHubDoneThroughEdge to take over synthesis -> audit")
	}
	if res.Round != 2 {
		t.Fatalf("result Round = %d, want 2 (code-phase done must not reset)", res.Round)
	}
	if st := svc.agentOrchestrator.loopStateFor(parent.RunID); st.Round != 2 {
		t.Fatalf("loop Round = %d, want 2 (code-phase done must not reset)", st.Round)
	}
}

// TestPlanContinueStillCapsWithinSinglePhase locks the other half of the
// contract: the fresh budget is per-phase, not unlimited — a continue that
// reaches the (new, larger) cap still parks blocked/cap.
func TestPlanContinueStillCapsWithinSinglePhase(t *testing.T) {
	svc, runID := newFlowEngineTestRun(t)
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{
		Status: "running", Cap: 5, Round: 4, RoundCap: 5, Mode: "explicit",
	})
	svc.mu.Lock()
	svc.runs[runID].currentTurnID = "turn-plan-cap"
	svc.mu.Unlock()

	result, err := svc.applyFlowControl(runID, FlowControlInput{
		Status:  "continue",
		Summary: "plan still contested at the last round",
	})
	if err != nil {
		t.Fatalf("applyFlowControl: %v", err)
	}
	if result.Status != "blocked" || result.NextAction != "awaiting_user" {
		t.Fatalf("result = %+v, want blocked/awaiting_user at cap 5", result)
	}
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.Status != "blocked" || loop.BlockReason != "cap" {
		t.Fatalf("loop = %+v, want blocked/cap", loop)
	}
	if loop.Round != 5 {
		t.Fatalf("loop Round = %d, want 5 (continue still counts within a phase)", loop.Round)
	}
}

// TestBugSubModeOffersNoOrchestrationAfterReviewLoopHide locks the picker
// half of this change: with review-loop hidden (selectableIn []), the Chat
// Bug sub-mode offers no built-in orchestration, and the old review-loop ref
// is rejected instead of silently accepted.
func TestBugSubModeOffersNoOrchestrationAfterReviewLoopHide(t *testing.T) {
	for _, subMode := range []string{"bug", "normal", "task", ""} {
		opts, err := BuiltinOrchestrationOptions(subMode)
		if err != nil {
			t.Fatalf("BuiltinOrchestrationOptions(%q): %v", subMode, err)
		}
		for _, opt := range opts {
			if opt.FlowRef == "flowpilot-core-flow-pack/review-loop" {
				t.Fatalf("review-loop must stay hidden for subMode=%q, got %#v", subMode, opts)
			}
		}
	}
	if err := validateChatOrchestrationSelection("bug", "flowpilot-core-flow-pack/review-loop"); err == nil {
		t.Error("expected error selecting the hidden review-loop under bug sub-mode")
	}
}
