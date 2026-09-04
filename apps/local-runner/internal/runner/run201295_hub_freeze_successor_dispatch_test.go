package runner

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// run-201295 (CP-58 S6 / task-harness): the plan_synthesis hub called
// submit_review_outcome(approved) successfully and CA-731 stamped the
// one-decision guard — but the successor (preflight_contract_freeze) was never
// dispatched because advanceToNextInlineOrDelegate's switch had no
// contract.freeze / context.produce cases (they only lived in
// tryAdvanceFlowThroughInline, CA-732). advanceHubDoneThroughEdge reported
// "advancing" anyway, so BUG-226's escalate was gated off and the flow idled
// RUNNING until the 2m watchdog parked hub_stalled (plan_synthesis
// WAITING_USER_APPROVAL, no freeze, no card). New file; no pre-existing test is
// modified.

// run201295HubFreezeTopology returns edges/nodes for a plan hub whose approved
// "done" edge leads into a real contract.freeze node, then an agent.code writer.
func run201295HubFreezeTopology() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	edges := []agentpack.FlowEdge{
		{From: "plan_synthesis", To: "preflight_contract_freeze", When: "done", Kind: "forward"},
		{From: "preflight_contract_freeze", To: "test_signatures", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "plan_synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke", Join: "all"},
		{ID: "preflight_contract_freeze", Behavior: "contract.freeze", Lifecycle: "once"},
		{ID: "test_signatures", Behavior: "agent.code", Agent: "agents/coder.md", Lifecycle: "once"},
	}
	return edges, nodes
}

// run201295HubFreezeService builds a workspace-backed flow-engine run with the
// hub->freeze->writer topology across the given provider key, mirroring the
// real flow start (workspace, flowStartGitHead, baseline fingerprint) so
// runContractFreezeNode can actually freeze and advance the chain.
func run201295HubFreezeService(t *testing.T, pk ProviderKey) (*InteractiveService, string, string) {
	t.Helper()
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestServiceForProvider(t, pk)
	edges, nodes := run201295HubFreezeTopology()
	runID := newFreezeTestRunForProvider(t, svc, pk, dir, edges, nodes, head)
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.activeHubNodeID = "plan_synthesis"
	rs.autoOrchestrate = true
	rs.currentTurnID = "turn-201295"
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(runID, nodes)
	return svc, runID, dir
}

// TestRun201295ApprovedDoneDispatchesFreezeSuccessor locks the reported repro
// across Claude/Codex/Grok: a hub's submit_review_outcome(approved) routed
// through advanceHubDoneThroughEdge must dispatch the contract.freeze
// successor in-process — plan_synthesis DONE, freeze DONE, the writer spawned
// — and must NOT silently report "advancing" while freezing nothing (the
// run-201295 2-minute hub_stalled).
func TestRun201295ApprovedDoneDispatchesFreezeSuccessor(t *testing.T) {
	providers := []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
	for _, pk := range providers {
		t.Run(string(pk), func(t *testing.T) {
			svc, runID, _ := run201295HubFreezeService(t, pk)

			res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: validPlannerDraft})
			if !handled {
				t.Fatalf("%s: advanceHubDoneThroughEdge must take over plan_synthesis -> freeze", pk)
			}
			if res.Status == "" || res.NextAction == "looping" {
				t.Fatalf("%s: result must carry an accepted status (never looping); got %+v", pk, res)
			}
			// The one-decision guard must be stamped so BUG-226 cannot re-escalate.
			if !svc.flowControlSubmittedForTurn(runID, "turn-201295") {
				t.Fatalf("%s: hub turn must count as a flow-control decision after a real successor dispatch", pk)
			}
			if got := flowStepStatus(t, svc, runID, "plan_synthesis"); got != StepStatusDone {
				t.Fatalf("%s: plan_synthesis = %v, want DONE (hub accepted)", pk, got)
			}
			if got := flowStepStatus(t, svc, runID, "preflight_contract_freeze"); got != StepStatusDone {
				t.Fatalf("%s: preflight_contract_freeze = %v, want DONE (freeze must actually run)", pk, got)
			}
			waitLoop(t, string(pk)+": writer spawned after freeze", 3*time.Second, func() bool {
				return countChildrenWithLabel(svc, runID, "test_signatures") == 1
			})
			if st := svc.agentOrchestrator.loopStateFor(runID); st.BlockReason == "hub_stalled" {
				t.Fatalf("%s: must not stall after an approved done with a real successor", pk)
			}
		})
	}
}

// TestRun201295UndispatchableSuccessorEscalatesImmediately locks the fail-closed
// near-miss: a "done" edge whose successor node has an unknown behavior must
// NOT report "advancing" and idle 2 minutes into hub_stalled — it must escalate
// immediately on the very turn so the operator gets an actionable card.
func TestRun201295UndispatchableSuccessorEscalatesImmediately(t *testing.T) {
	providers := []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
	for _, pk := range providers {
		t.Run(string(pk), func(t *testing.T) {
			svc, runID, _ := run201295HubFreezeService(t, pk)
			svc.mu.Lock()
			rs := svc.runs[runID]
			rs.activeFlowEdges = []agentpack.FlowEdge{
				{From: "plan_synthesis", To: "bad_node", When: "done", Kind: "forward"},
			}
			rs.activeFlowNodes = []agentpack.FlowNode{
				{ID: "plan_synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke", Join: "all"},
				{ID: "bad_node", Behavior: "totally.unknown", Lifecycle: "once"},
			}
			svc.mu.Unlock()

			res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: "approved"})
			if !handled {
				t.Fatalf("%s: undispatchable successor must still be claimed (escalated), got handled=false", pk)
			}
			st := svc.agentOrchestrator.loopStateFor(runID)
			if st.Status != "blocked" {
				t.Fatalf("%s: loop = %q, want blocked (immediate escalate, not silent advance)", pk, st.Status)
			}
			if st.BlockReason == "hub_stalled" {
				t.Fatalf("%s: must escalate immediately, not wait 2m for hub_stalled", pk)
			}
			if !strings.Contains(st.GateReason, "could not be dispatched") {
				t.Fatalf("%s: gateReason = %q, want the undispatchable-successor reason", pk, st.GateReason)
			}
			if got := flowStepStatus(t, svc, runID, "plan_synthesis"); got != StepStatusWaitingUserApr {
				t.Fatalf("%s: plan_synthesis = %v, want WAITING_USER_APPROVAL (actionable card)", pk, got)
			}
			if res.NextAction != "awaiting_user" {
				t.Fatalf("%s: result NextAction = %q, want awaiting_user", pk, res.NextAction)
			}
		})
	}
}

// TestRun201295ProseVerdictApprovedDrivesFreeze locks the CA-736 gap: a prose
// hub turn whose cohort recorded approved machine verdicts must route its
// derived "done" through advanceHubDoneThroughEdge (freeze actually runs +
// writer spawns), NOT settle the whole flow via applyFlowControl("done") which
// skipped the freeze node entirely.
func TestRun201295ProseVerdictApprovedDrivesFreeze(t *testing.T) {
	providers := []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
	for _, pk := range providers {
		t.Run(string(pk), func(t *testing.T) {
			svc, runID, _ := run201295HubFreezeService(t, pk)

			// The flow's entry planner already ran (real task-harness shape): its
			// completed child carries the preflight draft the freeze node reuses
			// when the hub's prose summary is not itself a draft.
			svc.mu.Lock()
			child := &interactiveRun{
				id:           "run-planner-" + string(pk),
				parentRunID:  runID,
				label:        "preflight_contract_plan",
				agentName:    "contract-planner",
				status:       RunStatusCompleted,
				providerKey:  pk,
				subs:         map[int64]chan ProviderEvent{},
				events:       []ProviderEvent{{Type: EventTurnCompleted, FinalMessage: validPlannerDraft}},
			}
			svc.runs[child.id] = child
			svc.agentOrchestrator.registerChild(runID, child.id)
			svc.runs[runID].lastReviewCohortVerdicts = map[string]string{"plan_reviewer": "approved"}
			svc.mu.Unlock()

			if !svc.advanceHubFromCohortMachineVerdicts(runID) {
				t.Fatalf("%s: approved prose verdicts must derive a transition", pk)
			}
			if got := flowStepStatus(t, svc, runID, "plan_synthesis"); got != StepStatusDone {
				t.Fatalf("%s: plan_synthesis = %v, want DONE", pk, got)
			}
			if got := flowStepStatus(t, svc, runID, "preflight_contract_freeze"); got != StepStatusDone {
				t.Fatalf("%s: preflight_contract_freeze = %v, want DONE (CA-736 prose-done must run freeze)", pk, got)
			}
			waitLoop(t, string(pk)+": writer spawned from prose-approved verdicts", 3*time.Second, func() bool {
				return countChildrenWithLabel(svc, runID, "test_signatures") == 1
			})
			if st := svc.agentOrchestrator.loopStateFor(runID); st.BlockReason == "hub_stalled" {
				t.Fatalf("%s: must not stall on prose-approved verdicts", pk)
			}
		})
	}
}

// TestRun201295ProseVerdictContinueStillReentersWriter guards the near-miss:
// changes_requested still routes to continue (plan_writer re-entry), not into
// the done-successor path.
func TestRun201295ProseVerdictContinueStillReentersWriter(t *testing.T) {
	svc, runID, ch := run198699HarnessService(t, ProviderKeyCodex)
	run199617HarnessTopology(t, svc, runID)
	svc.mu.Lock()
	svc.runs[runID].lastReviewCohortVerdicts = map[string]string{"plan_reviewer": "changes_requested"}
	svc.mu.Unlock()

	if !svc.advanceHubFromCohortMachineVerdicts(runID) {
		t.Fatal("changes_requested verdicts must derive continue")
	}
	select {
	case req := <-ch:
		if strings.TrimSpace(req.Prompt) == "" {
			t.Fatalf("continue-spawned plan_writer prompt empty: %q", req.Prompt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("plan_writer was not re-entered after changes_requested continue")
	}
	if got := flowStepStatus(t, svc, runID, "plan_synthesis"); got == StepStatusDone {
		t.Fatal("plan_synthesis must not be DONE on changes_requested continue")
	}
}

// seedRun201295PlannerChild records a completed entry-planner child carrying a
// valid preflight draft — the real task-harness shape the freeze node reuses
// via findPlannerResultForFreeze when the hub's own summary is prose, not a draft.
func seedRun201295PlannerChild(t *testing.T, svc *InteractiveService, runID string, pk ProviderKey) {
	t.Helper()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	child := &interactiveRun{
		id:          "run-planner-" + string(pk),
		parentRunID: runID,
		label:       "preflight_contract_plan",
		agentName:   "contract-planner",
		status:      RunStatusCompleted,
		providerKey: pk,
		subs:        map[int64]chan ProviderEvent{},
		events:      []ProviderEvent{{Type: EventTurnCompleted, FinalMessage: validPlannerDraft}},
	}
	svc.runs[child.id] = child
	svc.agentOrchestrator.registerChild(runID, child.id)
}

// TestRun201295ProseSummaryWithPlannerDraftDispatchesFreeze locks the exact
// live tool-call shape: the hub's submit_review_outcome(approved) carries a
// PROSE summary ("Synthesized: 1/1 ..."), not a preflight draft — the freeze
// must still run by reusing the entry planner child's draft
// (findPlannerResultForFreeze), and the writer must spawn.
func TestRun201295ProseSummaryWithPlannerDraftDispatchesFreeze(t *testing.T) {
	providers := []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
	for _, pk := range providers {
		t.Run(string(pk), func(t *testing.T) {
			svc, runID, _ := run201295HubFreezeService(t, pk)
			seedRun201295PlannerChild(t, svc, runID, pk)

			prose := "Synthesized: 1/1 reviewer approved (plan_reviewer). No conflicts. Advancing."
			res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done", Summary: prose})
			if !handled {
				t.Fatalf("%s: prose-summary done must take over plan_synthesis -> freeze", pk)
			}
			if res.NextAction == "looping" {
				t.Fatalf("%s: result must not read as rejected; got %+v", pk, res)
			}
			if !svc.flowControlSubmittedForTurn(runID, "turn-201295") {
				t.Fatalf("%s: hub turn must count as a flow-control decision", pk)
			}
			if got := flowStepStatus(t, svc, runID, "plan_synthesis"); got != StepStatusDone {
				t.Fatalf("%s: plan_synthesis = %v, want DONE", pk, got)
			}
			if got := flowStepStatus(t, svc, runID, "preflight_contract_freeze"); got != StepStatusDone {
				t.Fatalf("%s: preflight_contract_freeze = %v, want DONE (planner-draft fallback must fire)", pk, got)
			}
			waitLoop(t, string(pk)+": writer spawned from prose summary", 3*time.Second, func() bool {
				return countChildrenWithLabel(svc, runID, "test_signatures") == 1
			})
			if st := svc.agentOrchestrator.loopStateFor(runID); st.BlockReason == "hub_stalled" {
				t.Fatalf("%s: must not stall on a prose-summary approved done", pk)
			}
		})
	}
}

// run201295InTurnService builds the workspace-backed hub->freeze->writer run
// with a fake adapter that replays the live run-201295 two-turn shape: turn 1
// is the hub's pre-cohort prose turn (no tool call), turn 2 is the synthesis
// turn that calls submit_review_outcome(approved) MID-TURN with a prose summary
// (Codex item/tool/call -> SubmitFlowControl), then emits the prose completion.
func run201295InTurnService(t *testing.T, pk ProviderKey) (*InteractiveService, string) {
	t.Helper()
	dir, head := newContractFreezeTestRepo(t)
	reg := newProviderRegistry()
	var hubID string
	var hubTurns int64
	reg.register(ProviderRegistration{
		Key: pk, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				if hubID != "" && req.RunID == hubID {
					n := atomic.AddInt64(&hubTurns, 1)
					if n == 1 {
						b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "Planning context gathered."})
						return nil
					}
					if n == 2 {
						if _, err := b.SubmitFlowControl(FlowControlInput{Status: "done", Summary: "Synthesized: 1/1 reviewer approved (plan_reviewer). No conflicts. Advancing."}); err != nil {
							t.Errorf("hub synthesis SubmitFlowControl: %v", err)
						}
						b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "Synthesized: 1/1 reviewer approved (plan_reviewer). No conflicts. Advancing."})
						return nil
					}
					b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "Hub follow-up, nothing to do."})
					return nil
				}
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "coded"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: pk})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	hubID = parent.RunID
	svc.agentOrchestrator.setLoop(hubID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 3, RoundCap: 3})
	edges, nodes := run201295HubFreezeTopology()
	svc.mu.Lock()
	rs := svc.runs[hubID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.workspaceCwd = dir
	rs.flowStartGitHead = head
	rs.flowStartWorktreeFingerprint = baselineWorktreeFingerprint(dir)
	rs.activeHubNodeID = "plan_synthesis"
	rs.autoOrchestrate = true
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(hubID, nodes)
	seedRun201295PlannerChild(t, svc, hubID, pk)
	return svc, hubID
}

// TestRun201295InTurnSubmitFlowControlDispatchesFreeze is the end-to-end proof
// for the run-201295 class: a REAL in-turn SubmitFlowControl (the tool-call
// path every adapter routes through) with a prose summary must dispatch the
// freeze successor synchronously, and the turn's completion must NOT trip
// BUG-226 (the guard was stamped mid-turn) — plan_synthesis DONE, freeze DONE,
// writer spawned, loop still running.
func TestRun201295InTurnSubmitFlowControlDispatchesFreeze(t *testing.T) {
	providers := []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
	for _, pk := range providers {
		t.Run(string(pk), func(t *testing.T) {
			svc, runID := run201295InTurnService(t, pk)

			if _, apiErr := svc.startTurn(runID, TurnInput{StepID: "step-hub-1", Prompt: "first hub turn"}, "", ""); apiErr != nil {
				t.Fatalf("%s: startTurn(1): %s", pk, apiErr.msg)
			}
			waitHubTurnSettled(t, svc, runID, "hub turn 1", func() bool {
				svc.mu.Lock()
				defer svc.mu.Unlock()
				return !svc.runs[runID].turnInFlight
			})

			if _, apiErr := svc.startTurn(runID, TurnInput{StepID: "step-hub-2", Prompt: "synthesis turn"}, "", ""); apiErr != nil {
				t.Fatalf("%s: startTurn(2): %s", pk, apiErr.msg)
			}
			waitLoop(t, string(pk)+": plan_synthesis DONE after in-turn submit", 5*time.Second, func() bool {
				return flowStepStatus(t, svc, runID, "plan_synthesis") == StepStatusDone
			})

			if got := flowStepStatus(t, svc, runID, "preflight_contract_freeze"); got != StepStatusDone {
				t.Fatalf("%s: preflight_contract_freeze = %v, want DONE (in-turn submit must run freeze)", pk, got)
			}
			waitLoop(t, string(pk)+": writer spawned from in-turn submit", 3*time.Second, func() bool {
				return countChildrenWithLabel(svc, runID, "test_signatures") == 1
			})
			st := svc.agentOrchestrator.loopStateFor(runID)
			if st.Status == "blocked" {
				t.Fatalf("%s: loop = blocked after an accepted in-turn submit (%s) — BUG-226 must not fire on a decided turn", pk, st.GateReason)
			}
			if st.BlockReason == "hub_stalled" {
				t.Fatalf("%s: must not stall after an in-turn approved submit", pk)
			}
		})
	}
}