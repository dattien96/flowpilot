package runner

import (
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// BUG-571 (live run-100368, turn-1252xx/turn-126711): pendingFlowGateSettle
// armed on a run whose dispatch record was never listed recoverable —
// gate eval died with the process or the settle-driver re-kick was consumed
// by a concurrent continue. Neither the boot ladder nor the settle sweep
// enumerated it (no SettleOwed record), so the run stayed armed forever until
// an operator Continue poked resumePendingFlowGate. The wedge sweep must
// re-drive armed settle directly off live run state.
func TestBug571_SweepConvergesArmedGateSettle(t *testing.T) {
	svc := bug289Service(t)
	svc.dispatchStore = NewMemoryDispatchStore()
	runID := "run-571a"

	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:                    runID,
		status:                RunStatusRunning,
		pendingFlowGateSettle: true,
		pendingFlowGateTurnID: "turn-571a",
		lastTurnID:            "turn-571a",
		subs:                  map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()

	svc.sweepWedgedFlowWork()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		armed := svc.runs[runID].pendingFlowGateSettle
		svc.mu.Unlock()
		if !armed {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("pendingFlowGateSettle still armed after sweep — wedged run never converges")
}

// An armed settle with a live post-turn gate eval owns its disposition — the
// sweep must not double-eval it (single-flight contract).
func TestBug571_SweepLeavesLiveGateEvalAlone(t *testing.T) {
	svc := bug289Service(t)
	svc.dispatchStore = NewMemoryDispatchStore()
	runID := "run-571b"

	svc.mu.Lock()
	rs := &interactiveRun{
		id:                    runID,
		status:                RunStatusRunning,
		pendingFlowGateSettle: true,
		pendingFlowGateTurnID: "turn-571b",
		lastTurnID:            "turn-571b",
		postTurnGateCancel:    func() {},
		postTurnGateStartedAt: time.Now().UTC(),
		gateClaimID:           "claim-live",
		subs:                  map[int64]chan ProviderEvent{},
	}
	svc.runs[runID] = rs
	svc.mu.Unlock()

	svc.sweepWedgedFlowWork()
	time.Sleep(100 * time.Millisecond)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if rs.gateClaimID != "claim-live" {
		t.Fatalf("sweep clobbered a live gate claim: %q", rs.gateClaimID)
	}
	if !rs.pendingFlowGateSettle {
		t.Fatal("sweep cleared pendingFlowGateSettle while a live eval owned it")
	}
}

// BUG-581 + BUG-579 (live run-100368): a run holding waiting_question /
// waiting_approval / waiting_user_approval past the grace window with NO
// live card id is a phantom/orphan wait — the emit-then-stamp window is
// sub-second so a minute of silence is unambiguous. The reconstruct heal
// already treats a durable-free waiting_question as fake (BUG-470); the
// in-session sweep must do the same.
func TestBug581_SweepHealsPhantomWaitingQuestion(t *testing.T) {
	svc := bug289Service(t)
	svc.dispatchStore = NewMemoryDispatchStore()
	runID := "run-581"

	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:                  runID,
		status:              RunStatusWaitingQuestion,
		agentStatus:         string(RunStatusWaitingQuestion),
		lastProviderEventAt: time.Now().UTC().Add(-2 * wedgedWaitGrace),
		subs:                map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()

	svc.sweepWedgedFlowWork()

	svc.mu.Lock()
	got := svc.runs[runID].status
	svc.mu.Unlock()
	if got != RunStatusRunning {
		t.Fatalf("phantom waiting_question not healed: status=%s", got)
	}
}

// A waiting_approval run whose resume-reconciliation id points at a card
// record that does not exist is orphaned (BUG-579: q-120784 on leg
// run-124283 — card gone from the durable store, parent blocked on it
// forever). The sweep must release the reconcile id and heal the wait.
func TestBug579_SweepHealsOrphanedResumeApproval(t *testing.T) {
	svc := bug289Service(t)
	svc.dispatchStore = NewMemoryDispatchStore()
	runID := "run-579"

	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:                      runID,
		status:                  RunStatusWaitingApproval,
		agentStatus:             string(RunStatusWaitingApproval),
		pendingResumeApprovalID: "q-orphaned",
		lastProviderEventAt:     time.Now().UTC().Add(-2 * wedgedWaitGrace),
		subs:                    map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()

	svc.sweepWedgedFlowWork()

	svc.mu.Lock()
	rs := svc.runs[runID]
	gotStatus := rs.status
	gotID := rs.pendingResumeApprovalID
	svc.mu.Unlock()
	if gotID != "" {
		t.Fatalf("orphaned resume-approval id not released: %q", gotID)
	}
	if gotStatus != RunStatusRunning {
		t.Fatalf("orphaned wait not healed: status=%s", gotStatus)
	}
}

// A wait backed by a live card id is real work — never heal it.
func TestBug581_SweepKeepsCardBackedWait(t *testing.T) {
	svc := bug289Service(t)
	svc.dispatchStore = NewMemoryDispatchStore()
	runID := "run-581b"

	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:                  runID,
		status:              RunStatusWaitingQuestion,
		pendingQuestionID:   "q-live",
		lastProviderEventAt: time.Now().UTC().Add(-2 * wedgedWaitGrace),
		subs:                map[int64]chan ProviderEvent{},
	}
	svc.questions["q-live"] = &questionRecord{id: "q-live", runID: runID, status: "pending"}
	svc.mu.Unlock()

	svc.sweepWedgedFlowWork()

	svc.mu.Lock()
	got := svc.runs[runID].status
	svc.mu.Unlock()
	if got != RunStatusWaitingQuestion {
		t.Fatalf("sweep healed a card-backed wait: status=%s", got)
	}
}

// BUG-572 (live run-100368): a PENDING artifact.audit_draft node whose
// defer reason is gone — cohort drained, no RUNNING sprint step, forward-done
// predecessors all terminal — sat PENDING forever; only an operator Continue
// re-dispatched it. The sweep must re-drive it through the normal inline
// path.
func TestBug572_SweepRedispatchesDeferredAudit(t *testing.T) {
	svc := bug289Service(t)
	svc.dispatchStore = NewMemoryDispatchStore()
	runID := "run-572"

	synthesis := agentpack.FlowNode{ID: "synthesis", Behavior: "agent.reason"}
	audit := agentpack.FlowNode{ID: "audit", Behavior: "artifact.audit_draft"}
	nodes := []agentpack.FlowNode{synthesis, audit}
	edges := []agentpack.FlowEdge{{
		From: "synthesis", To: "audit", When: "done", Kind: "forward",
	}}

	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:               runID,
		status:           RunStatusRunning,
		flowEngineDriven: true,
		activeFlowNodes:  nodes,
		activeFlowEdges:  edges,
		subs:             map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.workflowStore.(*fakeWorkflowStore).seed(runID, []RuntimeWorkflowStep{
		{ID: "synthesis", NodeID: "synthesis", Status: StepStatusDone},
		{ID: "audit", NodeID: "audit", Status: StepStatusPending},
	})
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "advancing"})

	svc.sweepWedgedFlowWork()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		st := svc.lookupFlowStepStatus(runID, "audit")
		if st != StepStatusPending {
			return // re-dispatched (RUNNING or settled)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("deferred audit still PENDING after sweep: status=%s",
		svc.lookupFlowStepStatus(runID, "audit"))
}

// A deferred audit whose defer reason still stands — sprint step still
// RUNNING — must stay parked.
func TestBug572_SweepKeepsAuditDeferredWhileSprintStepRuns(t *testing.T) {
	svc := bug289Service(t)
	svc.dispatchStore = NewMemoryDispatchStore()
	runID := "run-572b"

	synthesis := agentpack.FlowNode{ID: "synthesis", Behavior: "agent.reason"}
	coder := agentpack.FlowNode{ID: "coder", Behavior: "agent.code"}
	audit := agentpack.FlowNode{ID: "audit", Behavior: "artifact.audit_draft"}
	nodes := []agentpack.FlowNode{synthesis, coder, audit}
	edges := []agentpack.FlowEdge{{
		From: "synthesis", To: "audit", When: "done", Kind: "forward",
	}}

	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:               runID,
		status:           RunStatusRunning,
		flowEngineDriven: true,
		activeFlowNodes:  nodes,
		activeFlowEdges:  edges,
		subs:             map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.workflowStore.(*fakeWorkflowStore).seed(runID, []RuntimeWorkflowStep{
		{ID: "synthesis", NodeID: "synthesis", Status: StepStatusDone},
		{ID: "coder", NodeID: "coder", Status: StepStatusRunning},
		{ID: "audit", NodeID: "audit", Status: StepStatusPending},
	})
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "advancing"})

	svc.sweepWedgedFlowWork()
	time.Sleep(100 * time.Millisecond)

	if st := svc.lookupFlowStepStatus(runID, "audit"); st != StepStatusPending {
		t.Fatalf("sweep dispatched audit while a sprint step was RUNNING: status=%s", st)
	}
}
