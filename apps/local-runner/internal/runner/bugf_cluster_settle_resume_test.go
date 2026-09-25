package runner

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
)

// Cluster F — vibe/settle/resume fixes (BUG-401, 402, 403, 404, 424, 432, 437).
// Each test pins the contract the live-verification wave caught broken.

func clusterFService(t *testing.T) (*InteractiveService, string) {
	t.Helper()
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
	run, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: status=%d code=%q msg=%q", err.status, err.code, err.msg)
	}
	return svc, run.RunID
}

// BUG-437: a park triggered synchronously by the hub turn's own flow_control
// call must NOT cancel that turn — the verdict tool call is the one in flight
// (cp-harness run-10564: "Canceled due to user interrupt" stranded the hub's
// own submit_review_outcome).
func TestBug437_ParkPreservesFlowControlSubmittingTurn(t *testing.T) {
	svc, runID := clusterFService(t)
	var cancelled atomic.Bool
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.status = RunStatusRunning
	rs.turnInFlight = true
	rs.currentTurnID = "turn-verdict"
	// The agent bridge arms parkPreserveTurnID when this very turn's
	// flow_control tool call is executing — engine-internal stamps of
	// lastFlowControlTurnID must NOT preserve (run-203966 contract).
	rs.parkPreserveTurnID = "turn-verdict"
	rs.turnCancel = func() { cancelled.Store(true) }
	svc.mu.Unlock()

	svc.parkFlowForAwaitingUser(runID)

	if cancelled.Load() {
		t.Fatal("park cancelled the very turn whose flow_control triggered it — verdict call stranded mid-flight")
	}
	svc.mu.Lock()
	rs = svc.runs[runID]
	if rs.pendingGateRepromptPrompt != "" || rs.pendingFlowGateSettle {
		t.Fatal("park must still drop queued auto-intents")
	}
	svc.mu.Unlock()
}

// BUG-437 converse: an in-flight turn that did NOT submit flow control is
// still cancelled (the freeze semantics of run-1675 must hold).
func TestBug437_ParkStillCancelsNonDecisionTurn(t *testing.T) {
	svc, runID := clusterFService(t)
	var cancelled atomic.Bool
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.status = RunStatusRunning
	rs.turnInFlight = true
	rs.currentTurnID = "turn-work"
	// Engine-internal stamps (audit escalate, machine-verdict credit) set
	// lastFlowControlTurnID but never arm parkPreserveTurnID — the in-flight
	// turn must still be cancelled.
	rs.lastFlowControlTurnID = "turn-work"
	rs.turnCancel = func() { cancelled.Store(true) }
	svc.mu.Unlock()

	svc.parkFlowForAwaitingUser(runID)

	if !cancelled.Load() {
		t.Fatal("park must still cancel an in-flight turn that did not submit the decision")
	}
	svc.mu.Lock()
	rs = svc.runs[runID]
	if !rs.parkCancelCause || !rs.parkCancelSuppress {
		t.Fatal("park-cancel must arm cause+suppress so finishTurn stays non-terminal")
	}
	svc.mu.Unlock()
}

// BUG-432: after flow done, a waiting_user_approval child must be reconciled
// to completed instead of orphaning forever (live run-27148).
func TestBug432_ReconcileSettlesWaitingUserApprovalChild(t *testing.T) {
	svc, runID := clusterFService(t)
	svc.mu.Lock()
	child := &interactiveRun{
		id: "child-1", parentRunID: runID, projectID: "proj",
		status: RunStatusWaitingUserApr, agentStatus: "waiting_user_approval",
		agentName: "coder", label: "tdd",
	}
	svc.runs[child.id] = child
	svc.agentOrchestrator.registerChild(runID, child.id)
	svc.agentOrchestrator.upsertSummary(runID, AgentRunSummary{
		RunID: child.id, ParentRunID: runID, AgentName: "coder", Label: "tdd",
		Status: RunStatusWaitingUserApr, AgentStatus: "waiting_user_approval",
	})
	svc.mu.Unlock()

	svc.reconcileChildRunsOnFlowDone(runID)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if got := svc.runs[child.id].status; got != RunStatusCompleted {
		t.Fatalf("waiting_user_approval child orphaned — status=%q want completed", got)
	}
}

// BUG-432: markFlowRunComplete must clear a stale pendingGateBlock — else
// /gate-decision stays live on a dead loop and reprompts into the void.
func TestBug432_FlowDoneClearsPendingGateBlock(t *testing.T) {
	svc, runID := clusterFService(t)
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.flowEngineDriven = true
	rs.pendingGateBlock = &gateBlockInfo{regressedTests: []string{"TestX"}}
	svc.mu.Unlock()

	svc.markFlowRunComplete(context.Background(), runID)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.runs[runID].pendingGateBlock != nil {
		t.Fatal("stale pendingGateBlock survived flow completion — /gate-decision would reprompt a dead loop")
	}
}

// BUG-432: /gate-decision on a run with no pending gate must fail closed —
// previously it accepted and spawned a reprompt on a dead loop.
func TestBug432_GateDecisionRejectsWhenNothingPending(t *testing.T) {
	svc, runID := clusterFService(t)
	err := svc.SubmitGateDecision(runID, "keep-test-fix-code", "")
	if err == nil {
		t.Fatal("gate decision accepted with no pendingGateBlock — dead option reprompts a dead loop")
	}
	if err.code != "no_pending_gate_decision" {
		t.Fatalf("wrong error code %q — want no_pending_gate_decision", err.code)
	}
}

// BUG-432: a flow child must not spawn while the parent loop is blocked —
// live run-29375 was created and FAILED in the same second under `blocked cap`.
func TestBug432_SpawnRefusedWhileParentLoopBlocked(t *testing.T) {
	svc, runID := clusterFService(t)
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "blocked", BlockReason: "cap"})

	_, err := svc.spawnChildRun(context.Background(), runID, SpawnAgentInput{
		Agent: "coder", Prompt: "retry",
	})
	if err == nil {
		t.Fatal("spawnChildRun created a child under a blocked parent loop")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("refusal must name the blocked loop, got %v", err)
	}
}

// BUG-401: a debate_synthesis done verdict on a terminal done-edge must restore
// the stashed sprint topology, not settle the whole flow (run-21751/run-9).
func TestBug401_DebateSynthesisDoneRestoresParkedSprint(t *testing.T) {
	svc, runID := clusterFService(t)
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.flowEngineDriven = true
	rs.activeHubNodeID = vibeDebateSynthesisNodeID
	rs.activeFlowNodes = []agentpack.FlowNode{{ID: vibeDebateSynthesisNodeID, Behavior: "hub.inline"}}
	rs.activeFlowEdges = []agentpack.FlowEdge{{From: vibeDebateSynthesisNodeID, To: "done", Kind: "forward"}}
	rs.vibeParkedNodes = []agentpack.FlowNode{{ID: "sprint_impl", Behavior: "agent.code"}}
	rs.vibeParkedEdges = []agentpack.FlowEdge{{From: "sprint_impl", To: "done", Kind: "forward"}}
	rs.vibeParkedFlowRef = "vibe-sprint"
	rs.currentTurnID = "turn-syn"
	svc.mu.Unlock()

	res, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done"})
	if !handled {
		t.Fatal("debate_synthesis done on terminal edge was not handled — fell through to whole-run settle")
	}
	if res.Status != "done" || res.NextAction != "advancing" {
		t.Fatalf("want done/advancing, got %+v", res)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs = svc.runs[runID]
	if len(rs.vibeParkedNodes) != 0 {
		t.Fatal("parked topology not consumed")
	}
	if len(rs.activeFlowNodes) != 1 || rs.activeFlowNodes[0].ID != "sprint_impl" {
		t.Fatalf("sprint topology not restored: %+v", rs.activeFlowNodes)
	}
	if rs.chatFlowRef != "vibe-sprint" {
		t.Fatalf("parked flowRef not restored: %q", rs.chatFlowRef)
	}
}

// BUG-401: with nothing stashed, a terminal done-edge keeps legacy semantics
// (unhandled → caller's applyFlowControl settles).
func TestBug401_NoStashedSprint_FallsThrough(t *testing.T) {
	svc, runID := clusterFService(t)
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.flowEngineDriven = true
	rs.activeHubNodeID = "synthesis"
	rs.activeFlowNodes = []agentpack.FlowNode{{ID: "synthesis", Behavior: "hub.inline"}}
	rs.activeFlowEdges = []agentpack.FlowEdge{{From: "synthesis", To: "done", Kind: "forward"}}
	svc.mu.Unlock()

	if _, handled := svc.advanceHubDoneThroughEdge(runID, FlowControlInput{Status: "done"}); handled {
		t.Fatal("terminal done-edge with no stashed sprint must stay unhandled (legacy whole-run settle)")
	}
}

// BUG-402: a done verdict while vibe sprint tasks remain must refuse and
// escalate to the operator, never settle (run-9, run-2290: settled 1/3 done).
// The refusal applies to agent-initiated verdicts only — an operator settle
// (boundary cancel/decline) is the human decision and must not wedge.
func TestBug402_DoneRefusedWhileSprintTasksRemain(t *testing.T) {
	svc, runID := clusterFService(t)
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.flowEngineDriven = true
	rs.vibeTaskPlan = []string{"task-a", "task-b", "task-c"}
	rs.vibeSprintIndex = 1 // 1/3 delivered, 2 remain
	rs.currentTurnID = "turn-hub"
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running"})

	_, err := svc.applyFlowControl(runID, FlowControlInput{Status: "done", agentInitiated: true})
	if err == nil {
		t.Fatal("agent done accepted while sprint tasks remained — false-done wedge")
	}
	if !strings.Contains(err.Error(), "sprint") {
		t.Fatalf("refusal must name the unmet sprint contract, got %v", err)
	}
	st := svc.agentOrchestrator.loopStateFor(runID)
	if st.Status == "done" {
		t.Fatal("loop settled done despite undelivered sprint tasks")
	}
	if st.Status != "blocked" {
		t.Fatalf("loop must escalate to the operator (blocked), got %q", st.Status)
	}

	// Operator settle (sprint-boundary cancel/decline) must NOT be refused:
	// the human explicitly ended the plan — refusing wedges the gate forever.
	// Operator decisions are not bound to a provider turn; clear the stamp.
	svc.mu.Lock()
	rs.currentTurnID = ""
	rs.lastFlowControlTurnID = ""
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "blocked", BlockReason: "vibe_sprint_boundary"})
	if _, err := svc.applyFlowControl(runID, FlowControlInput{
		Status: "done", Summary: "Operator declined further sprints.",
	}); err != nil {
		t.Fatalf("operator done must not be refused: %v", err)
	}
	if got := svc.agentOrchestrator.loopStateFor(runID).Status; got != "done" {
		t.Fatalf("operator settle must publish done, got %q", got)
	}
}

// BUG-404: the debate-parked sprint topology and buffered coder batches must
// round-trip through the durable session record — a restart mid-negotiation
// previously lost them and false-done'd the flow.
func TestBug404_ParkedSprintStateRoundTripsSession(t *testing.T) {
	svc, runID := clusterFService(t)
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.vibeParkedNodes = []agentpack.FlowNode{{ID: "sprint_impl", Behavior: "agent.code"}}
	rs.vibeParkedEdges = []agentpack.FlowEdge{{From: "sprint_impl", To: "audit", Kind: "forward"}}
	rs.vibeParkedAcceptance = []string{"audit"}
	rs.vibeParkedFlowRef = "vibe-sprint"
	rs.pendingBatchSignatureByStep = map[string][]CoderBatchSignatureRequest{
		"impl": {{Symbol: "Add", File: "calc.go", ProposedSignature: "Add(a,b int) int"}},
	}
	snap := sessionStateOf(rs)
	svc.mu.Unlock()

	if len(snap.VibeParkedNodes) != 1 || snap.VibeParkedNodes[0].ID != "sprint_impl" {
		t.Fatalf("vibeParkedNodes not persisted: %+v", snap.VibeParkedNodes)
	}
	if len(snap.VibeParkedEdges) != 1 || snap.VibeParkedEdges[0].To != "audit" {
		t.Fatalf("vibeParkedEdges not persisted: %+v", snap.VibeParkedEdges)
	}
	if len(snap.VibeParkedAcceptance) != 1 || snap.VibeParkedAcceptance[0] != "audit" {
		t.Fatalf("vibeParkedAcceptance not persisted: %v", snap.VibeParkedAcceptance)
	}
	if snap.VibeParkedFlowRef != "vibe-sprint" {
		t.Fatalf("vibeParkedFlowRef not persisted: %q", snap.VibeParkedFlowRef)
	}
	if len(snap.PendingBatchSignatureByStep["impl"]) != 1 ||
		snap.PendingBatchSignatureByStep["impl"][0].Symbol != "Add" {
		t.Fatalf("pendingBatchSignatureByStep not persisted: %+v", snap.PendingBatchSignatureByStep)
	}
}

// BUG-403: the missing-verdict reprompt site inside
// settleFlowChildTurnCompletedLocked must arm reinvokeInFlight so a
// turn_in_progress rejection enters the drain/re-arm recovery instead of
// vanishing (CP58-1 deterministic wedge).
func TestBug403_VerdictRepromptArmsReinvokeRecovery(t *testing.T) {
	svc, parentID := clusterFService(t)
	svc.mu.Lock()
	parent := svc.runs[parentID]
	parent.flowEngineDriven = true
	// synthesis in the acceptance set → flowRequiresSynthesisMachineVerdict
	// is true, so a verdict-less cohort completion takes the reprompt branch.
	parent.activeFlowAcceptanceNodes = []string{synthesisAcceptanceNodeID}
	parent.activeFlowNodes = []agentpack.FlowNode{
		{ID: "reviewer", Behavior: "agent.delegate", Cohort: "reviewers"},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	child := &interactiveRun{
		id: "child-rev", parentRunID: parentID, projectID: "proj",
		status: RunStatusRunning, agentName: "reviewer", label: "reviewer",
		flowCohortId: "reviewers", stepID: "review-step",
	}
	svc.runs[child.id] = child
	svc.agentOrchestrator.registerChild(parentID, child.id)
	svc.settleFlowChildTurnCompletedLocked(child, "review done", ProviderEvent{Type: EventTurnCompleted})
	svc.mu.Unlock()

	svc.mu.Lock()
	child = svc.runs["child-rev"]
	armed := child.reinvokeInFlight
	count := child.verdictRepromptCount
	svc.mu.Unlock()
	if count != 1 {
		t.Fatalf("verdictRepromptCount=%d want 1 — reprompt branch not taken", count)
	}
	if !armed {
		t.Fatal("reprompt scheduled without reinvokeInFlight — a turn_in_progress reject escapes the drain/re-arm recovery (verdict silently lost)")
	}
}

// BUG-424: the amend-resume retry prompt must carry the change.contract scope
// block like the original writer spawn — after an amend the coder was never
// told which new paths the amendment allowed.
func TestBug424_ResumeRetryPromptCarriesContractScope(t *testing.T) {
	dir := t.TempDir()

	promptCh := make(chan TurnRequest, 4)
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				select {
				case promptCh <- req:
				default:
				}
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	run, cerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if cerr != nil {
		t.Fatalf("createRun: status=%d code=%q msg=%q", cerr.status, cerr.code, cerr.msg)
	}
	parentID := run.RunID
	svc.mu.Lock()
	parent := svc.runs[parentID]
	parent.flowEngineDriven = true
	parent.workspaceCwd = dir
	parent.activeFlowNodes = []agentpack.FlowNode{
		{ID: "implement", Behavior: "agent.delegate", Agent: "coder"},
		{ID: "audit", Behavior: "artifact.audit_draft"},
	}
	parent.activeFlowEdges = []agentpack.FlowEdge{
		{From: "implement", To: "audit", Kind: "forward"},
	}
	parent.lastFailedDelegateNodeID = "implement"
	svc.mu.Unlock()

	// The contract is keyed to the REAL parent run id — the inject looks it up
	// under .flowpilot/contracts in the workspace.
	cstore, err := changecontract.NewStore(dir)
	if err != nil {
		t.Fatalf("contract store: %v", err)
	}
	if err := cstore.Save(changecontract.Contract{
		RunID: parentID, StepID: "implement", FeatureKey: "f",
		DeclaredPaths: []string{"src/amended.go"},
		Confidence:    changecontract.ConfidenceDeclared,
		DeclaredAt:    time.Now().UTC(),
	}); err != nil {
		t.Fatalf("save contract: %v", err)
	}
	child, cerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if cerr != nil {
		t.Fatalf("createRun(child): status=%d code=%q msg=%q", cerr.status, cerr.code, cerr.msg)
	}
	svc.mu.Lock()
	crs := svc.runs[child.RunID]
	crs.parentRunID = parentID
	crs.agentName = "coder"
	crs.label = "implement"
	crs.role = "coder"
	crs.status = RunStatusFailed
	crs.stepID = "implement"
	crs.workspaceCwd = dir
	svc.agentOrchestrator.registerChild(parentID, child.RunID)
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "blocked", BlockReason: "escalate", ActiveNode: "implement"})

	if _, err := svc.resumeFlowWithFeedback(parentID, "contract amended — src/amended.go is now in scope"); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}

	var got string
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case req := <-promptCh:
			if strings.Contains(req.Prompt, "Retrying failed delegate") {
				got = req.Prompt
			}
		case <-time.After(20 * time.Millisecond):
		}
		if got != "" {
			break
		}
	}
	if got == "" {
		t.Fatal("no delegate retry prompt was dispatched")
	}
	// Assert on the contract block itself — not just "amended.go", which the
	// feedback text already contains.
	if !strings.Contains(got, "Change Contract") {
		t.Fatalf("retry prompt lacks the change.contract scope block — coder never learns the amended paths:\n%s", got)
	}
	if !strings.Contains(got, "src/amended.go") {
		t.Fatalf("contract block must carry the declared amended path:\n%s", got)
	}
}

// BUG-411: a gate-decision posted to a child whose parent flow is parked
// awaiting the human's remediation choice (owner-debate / escalate card) must
// route the option into resumeFlowWithFeedback on the parent. The old
// fire-and-forget startTurn died on the flow_awaiting_user fence — accepted
// but dead (live run-21689/run-25555).
func TestBug411_GateDecisionOnParkedChildRoutesToParentResume(t *testing.T) {
	promptCh := make(chan TurnRequest, 8)
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				select {
				case promptCh <- req:
				default:
				}
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, cerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if cerr != nil {
		t.Fatalf("createRun: status=%d code=%q", cerr.status, cerr.code)
	}
	parentID := parent.RunID
	child, cerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if cerr != nil {
		t.Fatalf("createRun(child): status=%d code=%q", cerr.status, cerr.code)
	}
	svc.mu.Lock()
	prs := svc.runs[parentID]
	prs.flowEngineDriven = true
	prs.autoOrchestrate = true
	// Owner-debate topology: debate_trigger is the parked hub.inline node.
	prs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "debate_trigger", Behavior: "hub.inline", Agent: "synthesizer"},
		{ID: "owner_1", Behavior: "agent.delegate", Agent: "owner"},
		{ID: "owner_2", Behavior: "agent.delegate", Agent: "owner"},
		{ID: "debate_synthesis", Behavior: "hub.inline", Agent: "synthesizer"},
	}
	crs := svc.runs[child.RunID]
	crs.parentRunID = parentID
	crs.agentName = "coder"
	crs.label = "tdd"
	crs.status = RunStatusWaitingUserApr // parked by the debate park
	crs.stepID = "step-tdd"
	crs.pendingGateBlock = &gateBlockInfo{stepID: "step-tdd", regressedTests: []string{"TestX"}}
	svc.agentOrchestrator.registerChild(parentID, child.RunID)
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "blocked", BlockReason: "hub_stalled", ActiveNode: "debate_trigger", Cap: 5, RoundCap: 5})

	if aerr := svc.SubmitGateDecision(child.RunID, "keep-test-fix-code", ""); aerr != nil {
		t.Fatalf("gate decision on parked child must not fail: %d %q %q", aerr.status, aerr.code, aerr.msg)
	}

	// The decision must lift the parent's park and re-drive the hub.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if svc.agentOrchestrator.loopStateFor(parentID).Status == "running" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if st := svc.agentOrchestrator.loopStateFor(parentID); st.Status != "running" {
		t.Fatalf("parent loop still %q after gate decision — dead option", st.Status)
	}
	// And the remediation must reach the hub's reinvoke prompt.
	var hubPrompt string
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && hubPrompt == "" {
		select {
		case req := <-promptCh:
			if strings.Contains(req.Prompt, "keep-test-fix-code") || strings.Contains(req.Prompt, "keep the test") {
				hubPrompt = req.Prompt
			}
		case <-time.After(20 * time.Millisecond):
		}
	}
	if hubPrompt == "" {
		t.Fatal("decision never reached the hub reinvoke prompt — remediation is still dead")
	}
	// The child's stale gate block must be consumed (superseded by the resume).
	svc.mu.Lock()
	stillBlocked := svc.runs[child.RunID].pendingGateBlock != nil
	svc.mu.Unlock()
	if stillBlocked {
		t.Fatal("child pendingGateBlock left behind — a later decision could double-fire")
	}
}

// BUG-411: same routing when the decision lands on the blocked hub run itself
// (operator picked the option on the parent's own card).
func TestBug411_GateDecisionOnBlockedHubResumesFlow(t *testing.T) {
	promptCh := make(chan TurnRequest, 8)
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				select {
				case promptCh <- req:
				default:
				}
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	run, cerr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if cerr != nil {
		t.Fatalf("createRun: status=%d code=%q", cerr.status, cerr.code)
	}
	svc.mu.Lock()
	rs := svc.runs[run.RunID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "debate_trigger", Behavior: "hub.inline", Agent: "synthesizer"},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(run.RunID, AgentLoopState{Status: "blocked", BlockReason: "escalate", ActiveNode: "debate_trigger", Cap: 5, RoundCap: 5})

	if aerr := svc.SubmitGateDecision(run.RunID, "custom", "remove the drifted file and retry tdd"); aerr != nil {
		t.Fatalf("gate decision on blocked hub: %d %q %q", aerr.status, aerr.code, aerr.msg)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if svc.agentOrchestrator.loopStateFor(run.RunID).Status == "running" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if st := svc.agentOrchestrator.loopStateFor(run.RunID); st.Status != "running" {
		t.Fatalf("loop still %q — the decision did not lift the park", st.Status)
	}
	var hubPrompt string
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && hubPrompt == "" {
		select {
		case req := <-promptCh:
			if strings.Contains(req.Prompt, "remove the drifted file") {
				hubPrompt = req.Prompt
			}
		case <-time.After(20 * time.Millisecond):
		}
	}
	if hubPrompt == "" {
		t.Fatal("custom decision never reached the hub reinvoke prompt")
	}
}

// BUG-411: when the reprompt turn cannot dispatch at all, the API must return
// the real error AND restore the pending block — never "accepted"-and-dead.
func TestBug411_GateDecisionSurfacesStartTurnReject(t *testing.T) {
	svc, runID := clusterFService(t)
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.status = RunStatusRunning
	rs.turnInFlight = true // force startTurn → turn_in_progress
	rs.pendingGateBlock = &gateBlockInfo{stepID: "step-1", regressedTests: []string{"TestX"}}
	svc.mu.Unlock()

	aerr := svc.SubmitGateDecision(runID, "keep-test-fix-code", "")
	if aerr == nil {
		t.Fatal("gate decision must surface the dispatch rejection, not read accepted-and-dead")
	}
	if aerr.code != "turn_in_progress" {
		t.Fatalf("expected turn_in_progress, got %d %q %q", aerr.status, aerr.code, aerr.msg)
	}
	svc.mu.Lock()
	restored := svc.runs[runID].pendingGateBlock != nil
	svc.mu.Unlock()
	if !restored {
		t.Fatal("pendingGateBlock was consumed even though the reprompt never dispatched — decision lost")
	}
}

// BUG-410 Symptom A: a flow parent reconstructed from a mid-flight crash
// snapshot carries loop status "running" with a stale blockReason
// ("hub_stalled" written by the watchdog before the kill). The reasons must
// not survive onto a non-blocked loop — and resume must re-drive the quiet
// hub instead of wedging.
func TestBug410_ReconstructClearsStaleBlockReason(t *testing.T) {
	svc, _ := clusterFService(t)
	store, ok := svc.workflowStore.(interface {
		UpsertProviderSession(context.Context, ProviderSessionState) error
	})
	if !ok {
		t.Skip("workflow store does not persist provider sessions")
	}
	sess := ProviderSessionState{
		RunID: "run-crashed", ProjectID: "proj", ProviderKey: ProviderKeyCodex,
		Status:          RunStatusCancelled,
		RunKind:         "workflow",
		AutoOrchestrate: true,
		ActiveFlowNodes: []agentpack.FlowNode{
			{ID: "synthesis", Behavior: "hub.inline", Agent: "synthesizer"},
		},
		LoopState: AgentLoopState{
			Status: "running", Mode: "explicit", Cap: 3, RoundCap: 3,
			BlockReason: "hub_stalled", GateReason: "hub made no progress",
		},
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := store.UpsertProviderSession(context.Background(), sess); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if _, aerr := svc.loadPersistedRun("run-crashed"); aerr != nil {
		t.Fatalf("loadPersistedRun: %d %q %q", aerr.status, aerr.code, aerr.msg)
	}
	loop := svc.agentOrchestrator.loopStateFor("run-crashed")
	if loop.BlockReason != "" {
		t.Fatalf("stale blockReason %q survived reconstruct on a %q loop", loop.BlockReason, loop.Status)
	}
	if loop.GateReason != "" {
		t.Fatalf("stale gateReason %q survived reconstruct on a %q loop", loop.GateReason, loop.Status)
	}
}

// BUG-410 Symptom A: resuming a crashed mid-flight flow (loop "running",
// nothing in flight, nothing queued) must re-drive the hub — previously every
// resume surface returned a snapshot but dispatched nothing (live run-4325).
func TestBug410_ResumeRedrivesQuietMidFlightFlow(t *testing.T) {
	promptCh := make(chan TurnRequest, 8)
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				select {
				case promptCh <- req:
				default:
				}
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	store, ok := svc.workflowStore.(interface {
		UpsertProviderSession(context.Context, ProviderSessionState) error
	})
	if !ok {
		t.Skip("workflow store does not persist provider sessions")
	}
	sess := ProviderSessionState{
		RunID: "run-dead", ProjectID: "proj", ProviderKey: ProviderKeyCodex,
		Status:            RunStatusCancelled, // normalizeResumedFlowRun's crash stamp
		RunKind:           "workflow",
		ProviderAccountID: "default",
		AutoOrchestrate:   false, // stripped by normalize
		ActiveFlowNodes: []agentpack.FlowNode{
			{ID: "synthesis", Behavior: "hub.inline", Agent: "synthesizer"},
		},
		LoopState: AgentLoopState{Status: "running", Mode: "explicit", Cap: 3, RoundCap: 3},
		UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := store.UpsertProviderSession(context.Background(), sess); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	// /agent-loop/resume — the surface the bug report shows returning a
	// snapshot while dispatching nothing.
	if _, aerr := svc.loadPersistedRun("run-dead"); aerr != nil {
		t.Fatalf("loadPersistedRun: %d %q %q", aerr.status, aerr.code, aerr.msg)
	}
	svc.resumeAgentLoop("run-dead")
	select {
	case req := <-promptCh:
		if strings.TrimSpace(req.Prompt) == "" {
			t.Fatal("hub re-drive dispatched an empty prompt")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("resume dispatched nothing — the quiet mid-flight flow is still wedged")
	}
	// And the crash-cancelled status must be healed so later advances land.
	// The fake adapter settles the re-driven turn instantly, so the run may
	// already read completed — the invariant is that it is no longer the
	// dead "cancelled" stamp normalizeResumedFlowRun left behind.
	svc.mu.Lock()
	st := svc.runs["run-dead"].status
	svc.mu.Unlock()
	if st == RunStatusCancelled {
		t.Fatalf("run status stayed %q after re-drive — crash stamp not healed", st)
	}
}

// BUG-410 Symptom B: a terminal commit that loses the revision CAS must retry
// with a fresh revision — the old reconcile was a wake marker that only
// Get()ed, so one stale commit wedged the record non-terminal forever and the
// outer run stayed "running" over a done loop (live run-3914).
func TestBug410_TerminalCommitRetriesOnStaleRevision(t *testing.T) {
	ctx := context.Background()
	svc, runID := clusterFService(t)
	store := NewMemoryDispatchStore()
	svc.dispatchStore = store
	rec := testPrepared(runID, "turn-9")
	if err := store.CreatePrepared(ctx, rec, testEnvelope(runID, "turn-9")); err != nil {
		t.Fatal(err)
	}
	rev := int64(1)
	var err error
	if rev, err = store.CASAdvance(ctx, runID, "turn-9", rev, DispatchPrepared, DispatchSendClaimed, nil); err != nil {
		t.Fatal(err)
	}
	if rev, err = store.CASAdvance(ctx, runID, "turn-9", rev, DispatchSendClaimed, DispatchSendStarted, nil); err != nil {
		t.Fatal(err)
	}

	// The retry path must land the terminal commit with the FRESH revision.
	proof := TerminalEvidence{ProviderKey: "codex", EvidenceKind: "test", Outcome: "completed", PayloadSHA256: "h"}
	svc.retryTerminalCommit(runID, "turn-9", proof)
	got, _, err := store.Get(ctx, runID, "turn-9")
	if err != nil {
		t.Fatal(err)
	}
	if !got.State.IsTerminal() {
		t.Fatalf("record still non-terminal (%s) after retry — the wedge persists", got.State)
	}
	if got.SettleOwed && got.SettlePhase == "" {
		t.Fatal("settle_owed record must advance to settle_pending after the retried commit")
	}
}

// BUG-410 Symptom B (same shape through the real bridge): a Terminal() call
// racing a concurrent write loses the CAS once — the retry must still land it.
func TestBug410_BridgeTerminalRecoversFromStaleCAS(t *testing.T) {
	ctx := context.Background()
	svc, runID := clusterFService(t)
	store := NewMemoryDispatchStore()
	svc.dispatchStore = store
	rec := testPrepared(runID, "turn-9")
	if err := store.CreatePrepared(ctx, rec, testEnvelope(runID, "turn-9")); err != nil {
		t.Fatal(err)
	}
	rev := int64(1)
	var err error
	if rev, err = store.CASAdvance(ctx, runID, "turn-9", rev, DispatchPrepared, DispatchSendClaimed, nil); err != nil {
		t.Fatal(err)
	}
	if rev, err = store.CASAdvance(ctx, runID, "turn-9", rev, DispatchSendClaimed, DispatchSendStarted, nil); err != nil {
		t.Fatal(err)
	}

	svc.mu.Lock()
	rs := svc.runs[runID]
	// Stale cached revision — simulates the race where the store moved on.
	rs.dispatch = map[string]*DispatchRecord{("turn-9"): {Revision: rev - 1}}
	svc.mu.Unlock()

	bridge := &turnBridge{svc: svc, rs: rs, ctx: ctx, turnID: "turn-9"}
	proof := TerminalEvidence{ProviderKey: "codex", EvidenceKind: "test", Outcome: "completed", PayloadSHA256: "h"}
	bridge.Terminal(proof)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		got, _, gerr := store.Get(ctx, runID, "turn-9")
		if gerr == nil && got.State.IsTerminal() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	got, _, _ := store.Get(ctx, runID, "turn-9")
	t.Fatalf("bridge terminal never committed after stale CAS — state=%s", got.State)
}
