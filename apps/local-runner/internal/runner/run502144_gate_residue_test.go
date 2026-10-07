package runner

import (
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// Live-defect regressions captured from run-490265 / run-502144 / run-504394
// (PrivateVault CP-04 lane, Devin quota kill-cycle):
//
//  1. A leg parked waiting_* whose pendingApprovalID/pendingQuestionID names a
//     record that no longer exists is stranded forever — every "card backed"
//     check trusted the bare id, so no sweep healed it and no card ever
//     surfaced for the operator to answer (run-504394 parked ~1h).
//  2. A labelled child stamped cancelled/failed while its parent step mirror
//     reads WAITING_USER_APPROVAL left a phantom wait: the node row stayed
//     parked with no decision card behind it (run-502144 synthesis row sat
//     WAITING ~7min while the run kept running, then ~3min while dead).
//  3. A renegotiation batch buffered by a NON-coder contract node
//     (agent.scaffold — the TDD leg) could never reach the negotiation hub:
//     the completion route only matched canonical "agent.code", and the leg
//     parked on a contract-frozen write before ever completing (contract
//     e0446f86 / RaspEngine.h read_only; batch renegotiation_recorded x3 with
//     no adjudication turn).
//  4. Answering "ok" on the vibe resume-confirm gate of a FAILED run consumed
//     the gate and the resume target, then silently no-oped — the operator's
//     resume decision destroyed the only recovery surface (run-490265).

// (1a) A parked leg whose pendingApprovalID is dangling (no record) must not
// count as card-backed: the orphan sweep clears the residue id and re-drives
// the owed step instead of skipping it forever.
func TestRun504394_OrphanSweepClearsDanglingApprovalID(t *testing.T) {
	dir, _ := newContractFreezeTestRepo(t)
	svc, parentID := newP4CodeWriterFixture(t, dir)
	child := newP4ChildRun(svc, "child-spec", parentID, dir, "")
	child.label = "spec_align"
	child.status = RunStatusWaitingUserApr
	child.agentStatus = "waiting_user_approval"
	child.pendingApprovalID = "appr-gone" // no s.approvals record — residue
	svc.agentOrchestrator.registerChild(parentID, child.id)

	svc.workflowStore.(*fakeWorkflowStore).seed(parentID, []RuntimeWorkflowStep{
		{ID: "spec_align", NodeID: "spec_align", Status: StepStatusRunning},
	})

	redrived, _ := svc.redriveParkedFlowOrphans(parentID, nil)
	if !redrived {
		t.Fatal("parked leg with a dangling approval id must be re-driven")
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if child.pendingApprovalID != "" {
		t.Fatalf("dangling approval id must be cleared, still %q", child.pendingApprovalID)
	}
	if child.status != RunStatusRunning {
		t.Fatalf("child status = %q, want running after orphan redrive", child.status)
	}
}

// (1b) Same class through the wedge sweep: a leg waiting_approval on a card
// that no longer exists heals to running and drops the residue id.
func TestRun504394_WedgeSweepHealsDanglingApprovalWait(t *testing.T) {
	svc, parentID := bug641FlowFixture(t)
	svc.mu.Lock()
	svc.runs["leg-coder"] = &interactiveRun{
		id:                  "leg-coder",
		parentRunID:         parentID,
		label:               "coder",
		stepID:              "coder",
		status:              RunStatusWaitingApproval,
		agentStatus:         string(RunStatusWaitingApproval),
		pendingApprovalID:   "appr-gone",
		lastProviderEventAt: time.Now().UTC().Add(-2 * wedgedWaitGrace),
		subs:                map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.workflowStore.(*fakeWorkflowStore).seed(parentID, []RuntimeWorkflowStep{
		{ID: "coder", NodeID: "coder", Status: StepStatusWaitingUserApr},
	})

	svc.sweepWedgedFlowWork()

	svc.mu.Lock()
	defer svc.mu.Unlock()
	leg := svc.runs["leg-coder"]
	if leg.pendingApprovalID != "" {
		t.Fatalf("dangling approval id must be cleared, still %q", leg.pendingApprovalID)
	}
	if leg.status != RunStatusRunning {
		t.Fatalf("orphaned wait not healed: %s", leg.status)
	}
	if st := svc.lookupFlowStepStatus(parentID, "coder"); st != StepStatusRunning {
		t.Fatalf("mirrored step stamp must heal with the leg: status=%s, want RUNNING", st)
	}
}

// (1c) The live-record counterpart: a pending approval record backing the id
// is a real card — the leg stays parked for the operator.
func TestRun504394_LiveApprovalRecordStillBlocksHeal(t *testing.T) {
	svc, parentID := bug641FlowFixture(t)
	svc.mu.Lock()
	svc.approvals["appr-live"] = &approvalRecord{id: "appr-live", runID: "leg-coder", status: "pending"}
	svc.runs["leg-coder"] = &interactiveRun{
		id:                  "leg-coder",
		parentRunID:         parentID,
		label:               "coder",
		stepID:              "coder",
		status:              RunStatusWaitingApproval,
		agentStatus:         string(RunStatusWaitingApproval),
		pendingApprovalID:   "appr-live",
		lastProviderEventAt: time.Now().UTC().Add(-2 * wedgedWaitGrace),
		subs:                map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.workflowStore.(*fakeWorkflowStore).seed(parentID, []RuntimeWorkflowStep{
		{ID: "coder", NodeID: "coder", Status: StepStatusWaitingUserApr},
	})

	svc.sweepWedgedFlowWork()

	svc.mu.Lock()
	defer svc.mu.Unlock()
	leg := svc.runs["leg-coder"]
	if leg.status != RunStatusWaitingApproval {
		t.Fatalf("card-backed wait healed: %s", leg.status)
	}
	if leg.pendingApprovalID != "appr-live" {
		t.Fatalf("live approval id cleared: %q", leg.pendingApprovalID)
	}
}

// (2a) Stopping the parent while a leg's step mirror reads
// WAITING_USER_APPROVAL must settle the mirror terminal — a cancelled leg can
// never surface the card the step claims to wait on.
func TestRun502144_StopSettlesWaitingStepMirror(t *testing.T) {
	svc, parentID := bug641FlowFixture(t)
	svc.mu.Lock()
	svc.runs["leg-coder"] = &interactiveRun{
		id:          "leg-coder",
		parentRunID: parentID,
		label:       "coder",
		stepID:      "coder",
		status:      RunStatusWaitingUserApr,
		agentStatus: string(RunStatusWaitingUserApr),
		subs:        map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, "leg-coder")
	svc.workflowStore.(*fakeWorkflowStore).seed(parentID, []RuntimeWorkflowStep{
		{ID: "coder", NodeID: "coder", Status: StepStatusWaitingUserApr},
	})

	if _, apiErr := svc.stopAgentLoop(parentID); apiErr != nil {
		t.Fatalf("stopAgentLoop: %v", apiErr)
	}
	if st := svc.lookupFlowStepStatus(parentID, "coder"); st != StepStatusCanceled {
		t.Fatalf("terminal leg must settle the wait mirror: status=%s, want CANCELED", st)
	}
}

// (2b) A live same-label sibling still holding a real park keeps the stamp —
// the terminal settle must not clobber a wait it does not own (BUG-639 I-2).
func TestRun502144_TerminalSettleKeepsSiblingOwnedStamp(t *testing.T) {
	svc, parentID := bug641FlowFixture(t)
	svc.mu.Lock()
	svc.runs["leg-old"] = &interactiveRun{
		id:          "leg-old",
		parentRunID: parentID,
		label:       "coder",
		stepID:      "coder",
		status:      RunStatusCancelled,
		subs:        map[int64]chan ProviderEvent{},
	}
	svc.runs["leg-new"] = &interactiveRun{
		id:                "leg-new",
		parentRunID:       parentID,
		label:             "coder",
		stepID:            "coder",
		status:            RunStatusWaitingUserApr,
		pendingApprovalID: "appr-live",
		subs:              map[int64]chan ProviderEvent{},
	}
	svc.approvals["appr-live"] = &approvalRecord{id: "appr-live", runID: "leg-new", status: "pending"}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, "leg-old")
	svc.agentOrchestrator.registerChild(parentID, "leg-new")
	svc.workflowStore.(*fakeWorkflowStore).seed(parentID, []RuntimeWorkflowStep{
		{ID: "coder", NodeID: "coder", Status: StepStatusWaitingUserApr},
	})

	svc.mu.Lock()
	svc.settleFlowChildStepTerminalLocked(svc.runs["leg-old"], StepStatusCanceled)
	svc.mu.Unlock()

	if st := svc.lookupFlowStepStatus(parentID, "coder"); st != StepStatusWaitingUserApr {
		t.Fatalf("sibling-owned wait stamp clobbered: %s, want WAITING_USER_APPROVAL", st)
	}
}

// (3a) The negotiation batch must route on ANY agent delegate's completion —
// the TDD scaffold leg (agent.scaffold) submits renegotiate_signatures too.
// Gating on canonical "agent.code" alone stranded its batch forever.
func TestRun502144_ScaffoldCompletionRoutesBatchToNegotiationHub(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 5, RoundCap: 5})
	nodes, edges := cp67NegotiationTopology()
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.flowEngineDriven = true
	prs.activeFlowNodes = nodes
	prs.activeFlowEdges = edges
	prs.stepID = "step-4"
	prs.turnInFlight = true // hub reinvoke defers into pendingHubReinvoke
	svc.mu.Unlock()

	svc.bufferCoderBatchSignatures(parent.RunID, []CoderBatchSignatureRequest{
		{Symbol: "scanFridaPort", File: "core/security-rasp/include/RaspEngine.h", CurrentSignature: "", ProposedSignature: "static bool scanFridaPort(uint16_t,int) noexcept", Rationale: "Task-046 port probe"},
	})

	// The TDD node completes with its batch buffered — it must reach the
	// negotiation hub, not the normal forward edge to implement.
	if !svc.tryAdvanceFlowFromNode(parent.RunID, "test_signatures", "done") {
		t.Fatal("scaffold completion with a pending batch must dispatch the negotiation hub")
	}
	svc.mu.Lock()
	prs = svc.runs[parent.RunID]
	hubID := prs.activeHubNodeID
	reinvoke := prs.pendingHubReinvoke
	reinvokePrompt := prs.pendingHubReinvokePrompt
	svc.mu.Unlock()
	if hubID != "synthesis_negotiation" {
		t.Fatalf("activeHubNodeID = %q, want synthesis_negotiation", hubID)
	}
	if !reinvoke || !strings.Contains(reinvokePrompt, "scanFridaPort") {
		t.Fatalf("batch must be injected into the hub prompt (reinvoke=%v)", reinvoke)
	}
	if got := svc.snapshotCoderBatchSignatures(parent.RunID); len(got) != 0 {
		t.Fatalf("batch must be consumed once dispatched, got %d rows", len(got))
	}
}

// (3b) The deadlock face: the submitting leg parks awaiting a user decision
// (contract-frozen write surfaced as a permission card) and never reaches
// completion — the buffered batch must still reach the negotiation hub so
// the Main Agent can adjudicate it.
func TestRun502144_ParkedBatchLegDispatchesNegotiationHub(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 5, RoundCap: 5})
	nodes, edges := cp67NegotiationTopology()
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.flowEngineDriven = true
	prs.activeFlowNodes = nodes
	prs.activeFlowEdges = edges
	prs.stepID = "step-4"
	prs.turnInFlight = true
	svc.mu.Unlock()

	svc.bufferCoderBatchSignatures(parent.RunID, []CoderBatchSignatureRequest{
		{Symbol: "verifyApkSignature", File: "core/security-rasp/include/RaspEngine.h", Rationale: "Task-046 signature verify"},
	})

	svc.maybeDispatchNegotiationHubForBufferedBatch(parent.RunID)

	svc.mu.Lock()
	prs = svc.runs[parent.RunID]
	hubID := prs.activeHubNodeID
	reinvokePrompt := prs.pendingHubReinvokePrompt
	svc.mu.Unlock()
	if hubID != "synthesis_negotiation" {
		t.Fatalf("activeHubNodeID = %q, want synthesis_negotiation", hubID)
	}
	if !strings.Contains(reinvokePrompt, "verifyApkSignature") {
		t.Fatalf("hub prompt missing batch row: %q", reinvokePrompt)
	}
	if got := svc.snapshotCoderBatchSignatures(parent.RunID); len(got) != 0 {
		t.Fatalf("batch must be consumed once dispatched, got %d rows", len(got))
	}
}

// (3c) No negotiation hub in the topology -> the batch stays buffered for the
// completion route; nothing is silently dropped or misrouted.
func TestRun502144_ParkedBatchWithoutHubStaysBuffered(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 5, RoundCap: 5})
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.flowEngineDriven = true
	prs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "implement", Behavior: "agent.code"},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	prs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "implement", To: "synthesis", When: "done", Kind: "forward"},
	}
	svc.mu.Unlock()

	svc.bufferCoderBatchSignatures(parent.RunID, []CoderBatchSignatureRequest{
		{Symbol: "Add", File: "calc/calc.go", Rationale: "need the sum"},
	})
	svc.maybeDispatchNegotiationHubForBufferedBatch(parent.RunID)
	if got := svc.snapshotCoderBatchSignatures(parent.RunID); len(got) != 1 {
		t.Fatalf("no declared hub — batch must stay buffered, got %d rows", len(got))
	}
}

// (4) CA-806 pins the gate contract: a genuinely Failed run never resumes
// through the confirm gate (healVibeFailedForReopenPark converts Failed ->
// Cancelled first and re-arms). What the live run-490265 defect needed was
// the healed path below: Cancelled + armed gate + OK resumes AND reconciles
// the stale FAILED ancestor rows, so the resumed flow is not re-killed by
// residue from the leg that died pre-resume.
func TestRun490265_ResumeConfirmOkReconcilesStaleAncestors(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "blocked", BlockReason: vibeResumePausedReason, Cap: 5, RoundCap: 5})
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.status = RunStatusCancelled
	prs.agentStatus = string(RunStatusCancelled)
	prs.vibeResumeConfirm = true
	prs.vibeResumeFromNode = "coder"
	prs.flowEngineDriven = true
	prs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "tdd", Behavior: "agent.scaffold"},
		{ID: "coder", Behavior: "agent.code"},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	prs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "tdd", To: "coder", When: "done", Kind: "forward"},
		{From: "coder", To: "synthesis", When: "done", Kind: "forward"},
	}
	svc.mu.Unlock()
	svc.workflowStore.(*fakeWorkflowStore).seed(parent.RunID, []RuntimeWorkflowStep{
		{ID: "tdd", NodeID: "tdd", Status: StepStatusFailed},
		{ID: "coder", NodeID: "coder", Status: StepStatusPending},
	})

	if apiErr := svc.SubmitGateDecision(parent.RunID, "ok", ""); apiErr != nil {
		t.Fatalf("SubmitGateDecision: %v", apiErr)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if prs.status == RunStatusCancelled {
		t.Fatal("operator resume on a healed run must flip status to running")
	}
	if prs.vibeResumeConfirm {
		t.Fatal("gate must be consumed by the decision")
	}
	if st := svc.lookupFlowStepStatus(parent.RunID, "tdd"); st != StepStatusDone {
		t.Fatalf("stale FAILED ancestor upstream of resume target must reconcile to DONE, tdd=%s", st)
	}
}

// (4b) Resume-confirm reconciliation: a FAILED/CANCELED step strictly
// upstream of the chosen checkpoint is kill-residue — the flow provably
// adjudicated past it. It normalizes to DONE so later settles never count a
// stale failure (live run-490265: preflight_contract_plan read FAILED while
// the resumed debate legs were already running).
func TestRun490265_ResumeReconcilesStaleFailedPredecessors(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.flowEngineDriven = true
	prs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "plan", Behavior: "agent.delegate"},
		{ID: "freeze", Behavior: "agent.delegate"},
		{ID: "tdd", Behavior: "agent.scaffold"},
	}
	prs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "plan", To: "freeze", When: "done", Kind: "forward"},
		{From: "freeze", To: "tdd", When: "done", Kind: "forward"},
		{From: "tdd", To: "plan", When: "continue", Kind: "back"}, // retry loop — not progress
	}
	svc.mu.Unlock()
	svc.workflowStore.(*fakeWorkflowStore).seed(parent.RunID, []RuntimeWorkflowStep{
		{ID: "plan", NodeID: "plan", Status: StepStatusFailed},
		{ID: "freeze", NodeID: "freeze", Status: StepStatusDone},
		{ID: "tdd", NodeID: "tdd", Status: StepStatusPending},
	})

	svc.reconcileResumedPredecessorSteps(parent.RunID, "tdd")

	if st := svc.lookupFlowStepStatus(parent.RunID, "plan"); st != StepStatusDone {
		t.Fatalf("stale FAILED predecessor must normalize: plan=%s, want DONE", st)
	}
	if st := svc.lookupFlowStepStatus(parent.RunID, "tdd"); st != StepStatusPending {
		t.Fatalf("the resume node itself must stay untouched: tdd=%s", st)
	}
}
