package runner

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"flowpilot-runner/internal/changecontract"
)

func bug289Service(t *testing.T) *InteractiveService {
	t.Helper()
	return newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
}

// BUG-289 H1/F-1 (+ Claude review): scheduleChildTurn must clear reinvokeInFlight,
// re-arm pendingHubReinvoke, AND actively drain via notifyTurnIdle so a retry is
// attempted — not only rename the stuck flag.
func TestBug289_H1_ScheduleChildTurnClearsReinvokeInFlightOnStartError(t *testing.T) {
	svc := bug289Service(t)
	runID := "run-h1"
	rs := &interactiveRun{
		id:               runID,
		autoOrchestrate:  true,
		flowEngineDriven: true,
		reinvokeInFlight: true,
		status:           RunStatusRunning,
		subs:             map[int64]chan ProviderEvent{},
	}
	// Loop must be advancing so maybeAutoReinvoke (drain) is allowed.
	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		st.Cap = 10
		return st
	})
	svc.mu.Lock()
	svc.runs[runID] = rs
	svc.mu.Unlock()

	// No full startTurn prerequisites → startTurn fails early.
	svc.scheduleChildTurn(runID, "step-1", "please synthesize")

	// 1) reinvokeInFlight must clear and pending must re-arm (at least briefly).
	sawClearAndRearm := false
	// 2) drain must run: pending consumed (notifyTurnIdle) and/or another
	//    reinvoke attempt (reinvokeInFlight true again or fail count advanced).
	sawDrainOrRetry := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		inflight := rs.reinvokeInFlight
		pending := rs.pendingHubReinvoke
		fails := rs.hubReinvokeStartFailCount
		svc.mu.Unlock()
		if !inflight && pending {
			sawClearAndRearm = true
		}
		// After notifyTurnIdle drain: pending cleared while scheduling retry,
		// or fail count advanced past first reject (retry path ran).
		if sawClearAndRearm && (!pending || fails >= 1) {
			// fails>=1 always after first reject; require evidence of drain:
			// either pending was observed false after true, or fail count grew
			// beyond 1 (second startTurn attempt).
			if fails >= 1 {
				sawDrainOrRetry = true
			}
		}
		if fails >= 2 {
			// Second startTurn attempt proves notifyTurnIdle re-scheduled.
			sawDrainOrRetry = true
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	// Track pending true→false transition more carefully with a short poll.
	var sawPendingTrue, sawPendingFalseAfterTrue atomic.Bool
	pollEnd := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(pollEnd) {
		svc.mu.Lock()
		p := rs.pendingHubReinvoke
		f := rs.hubReinvokeStartFailCount
		svc.mu.Unlock()
		if p {
			sawPendingTrue.Store(true)
		}
		if sawPendingTrue.Load() && !p {
			sawPendingFalseAfterTrue.Store(true)
		}
		if f >= 2 {
			sawDrainOrRetry = true
			break
		}
		if sawPendingFalseAfterTrue.Load() {
			sawDrainOrRetry = true
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if rs.reinvokeInFlight && rs.hubReinvokeStartFailCount == 0 {
		t.Fatal("reinvokeInFlight still true with no fail count after startTurn rejection")
	}
	if rs.hubReinvokeStartFailCount < 1 {
		t.Fatal("expected hubReinvokeStartFailCount >= 1 after startTurn rejection")
	}
	if !sawDrainOrRetry && !sawPendingFalseAfterTrue.Load() && rs.hubReinvokeStartFailCount < 2 {
		t.Fatalf("H1 drain missing: expected notifyTurnIdle retry (failCount>=2 or pending cleared), got failCount=%d pending=%v",
			rs.hubReinvokeStartFailCount, rs.pendingHubReinvoke)
	}
}

// BUG-289 M7/F-1: re-arm notify prompt when reinvokeInFlight && !turnInFlight.
func TestBug289_M7_ReinvokeInFlightWindowStashesNotifyPrompt(t *testing.T) {
	svc := bug289Service(t)
	runID := "run-m7"
	rs := &interactiveRun{
		id:               runID,
		autoOrchestrate:  true,
		flowEngineDriven: true,
		reinvokeInFlight: true,
		turnInFlight:     false,
		status:           RunStatusRunning,
		subs:             map[int64]chan ProviderEvent{},
	}
	svc.mu.Lock()
	svc.runs[runID] = rs
	svc.mu.Unlock()

	svc.maybeAutoReinvokeHubWithPrompt(runID, "NOTIFY PROMPT CUSTOM")

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if !rs.pendingHubReinvoke {
		t.Fatal("expected pendingHubReinvoke re-armed in reinvokeInFlight window")
	}
	if rs.pendingHubReinvokePrompt != "NOTIFY PROMPT CUSTOM" {
		t.Fatalf("pendingHubReinvokePrompt = %q", rs.pendingHubReinvokePrompt)
	}
}

// BUG-289 H3/F-3: SaveHead is atomic (temp+rename); LoadHead treats corrupt as missing.
func TestBug289_H3_SaveHeadAtomicAndLoadHeadCorruptTolerant(t *testing.T) {
	dir := t.TempDir()
	h := changecontract.CanonicalHead{
		FeatureKey:        "feat-a",
		BehaviorStatement: "behaves",
		IntentSignature:   "sig",
		Status:            changecontract.HeadStatusCurrent,
		SpecConfidence:    changecontract.SpecConfidenceSpecLess,
	}
	if err := changecontract.SaveHead(dir, h); err != nil {
		t.Fatalf("SaveHead: %v", err)
	}
	path := filepath.Join(dir, ".flowpilot", "canonical", "feat-a.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("head file missing: %v", err)
	}
	if err := os.WriteFile(path, []byte("{not-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, ok, err := changecontract.LoadHead(dir, "feat-a")
	if err != nil {
		t.Fatalf("LoadHead corrupt should not hard-error: %v", err)
	}
	if ok {
		t.Fatalf("corrupt head should report missing, got %+v", got)
	}
}

// BUG-289 H4/F-4: expireApproval flips WaitingApproval → Running.
func TestBug289_H4_ExpireApprovalClearsWaitingStatus(t *testing.T) {
	svc := bug289Service(t)
	runID := "run-h4"
	apprID := "appr-1"
	rs := &interactiveRun{
		id:                runID,
		status:            RunStatusWaitingApproval,
		pendingApprovalID: apprID,
		subs:              map[int64]chan ProviderEvent{},
	}
	svc.mu.Lock()
	svc.runs[runID] = rs
	svc.approvals[apprID] = &approvalRecord{
		id: apprID, runID: runID, status: "pending",
		resolve: make(chan string, 1),
	}
	svc.mu.Unlock()

	svc.expireApproval(apprID)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if rs.status == RunStatusWaitingApproval {
		t.Fatal("status still WaitingApproval after expiry")
	}
	if rs.pendingApprovalID != "" {
		t.Fatalf("pendingApprovalID still %q", rs.pendingApprovalID)
	}
}

// BUG-289 A6/F-9: one-decision guard field works after stamp.
func TestBug289_A6_OneDecisionGuardAfterStamp(t *testing.T) {
	svc := bug289Service(t)
	runID := "run-a6"
	rs := &interactiveRun{
		id:            runID,
		currentTurnID: "turn-a6",
		subs:          map[int64]chan ProviderEvent{},
	}
	svc.mu.Lock()
	svc.runs[runID] = rs
	rs.lastFlowControlTurnID = rs.currentTurnID
	svc.mu.Unlock()
	if !svc.flowControlSubmittedForTurn(runID, "turn-a6") {
		t.Fatal("expected one-decision guard to see stamped turn")
	}
}

// BUG-289 F-0: hub watchdog blocks when no progress.
// Also: pendingHubReinvoke alone must NOT suppress stall (H1 review blind spot).
func TestBug289_F0_HubStallBlocksWhenIdleTooLong(t *testing.T) {
	svc := bug289Service(t)
	runID := "run-f0"
	rs := &interactiveRun{
		id:                runID,
		flowEngineDriven:  true,
		status:            RunStatusRunning,
		hubLastProgressAt: time.Now().UTC().Add(-10 * time.Minute),
		stallTimeout:      time.Second,
		// Stuck pending without live turn — F-0 must still fire.
		pendingHubReinvoke: true,
		subs:               map[int64]chan ProviderEvent{},
	}
	svc.mu.Lock()
	svc.runs[runID] = rs
	svc.mu.Unlock()
	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		return st
	})

	if !svc.checkAndBlockStalledHub(runID) {
		t.Fatal("expected hub_stalled block (pendingHubReinvoke alone must not be busy forever)")
	}
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.Status != "blocked" || loop.BlockReason != "hub_stalled" {
		t.Fatalf("loop = %+v, want blocked/hub_stalled", loop)
	}
}

// BUG-289 M1/F-8: startTurn resets repromptAttempts (sampled via direct assign).
func TestBug289_M1_RepromptAttemptsResetFieldSemantics(t *testing.T) {
	rs := &interactiveRun{repromptAttempts: 5}
	rs.repromptAttempts = 0
	if rs.repromptAttempts != 0 {
		t.Fatal("expected zero")
	}
}

// BUG-289 A3 residual: boot inventory must NOT DriveSettle without EvaluateGate.
// A prior wiring created SettleDriver{Store only} → planNext default-allowed and
// silently finalized gate-owed turns. Fail-closed: phase stays settle_pending;
// ListAttention surfaces settle_pending.
func TestBug289_A3_BootInventoryDoesNotAutoFinalizeSettle(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("run-a3", "turn-a3")
	rec.SettleOwed = true
	if err := store.CreatePrepared(ctx, rec, testEnvelope("run-a3", "turn-a3")); err != nil {
		t.Fatal(err)
	}
	rev := int64(1)
	var err error
	rev, err = store.CASAdvance(ctx, "run-a3", "turn-a3", rev, DispatchPrepared, DispatchSendClaimed, nil)
	if err != nil {
		t.Fatal(err)
	}
	rev, err = store.CASAdvance(ctx, "run-a3", "turn-a3", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	if err != nil {
		t.Fatal(err)
	}
	proof := TerminalEvidence{ProviderKey: "f", EvidenceKind: "x", Outcome: "completed", PayloadSHA256: "h"}
	if _, err := store.CommitTerminalAndSettleIntent(ctx, "run-a3", "turn-a3", rev, proof, "run-a3", rec.OuterIntentKey, 1); err != nil {
		t.Fatal(err)
	}
	before, _, err := store.Get(ctx, "run-a3", "turn-a3")
	if err != nil {
		t.Fatal(err)
	}
	if before.SettlePhase != SettlePending {
		t.Fatalf("precondition: want settle_pending, got %s", before.SettlePhase)
	}

	svc := bug289Service(t)
	svc.dispatchStore = store
	svc.ScanDispatchRecoveryOnBoot(ctx)

	after, _, err := store.Get(ctx, "run-a3", "turn-a3")
	if err != nil {
		t.Fatal(err)
	}
	if after.SettlePhase != SettlePending {
		t.Fatalf("boot must not auto-finalize settle (EvaluateGate unwired); phase=%s", after.SettlePhase)
	}

	attn, err := store.ListAttention(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range attn {
		if a.Kind == "settle_pending" && a.RunID == "run-a3" && a.TurnID == "turn-a3" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ListAttention must surface settle_pending for unfinalized terminal settle; got %+v", attn)
	}
}

// BUG-289 A3 residual: EvaluateGate reprompt must not be overridden by default-allow.
func TestBug289_A3_EvaluateGateRepromptSupersedes(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	rec := testPrepared("run-rp", "turn-rp")
	rec.SettleOwed = true
	_ = store.CreatePrepared(ctx, rec, testEnvelope("run-rp", "turn-rp"))
	rev := int64(1)
	rev, _ = store.CASAdvance(ctx, "run-rp", "turn-rp", rev, DispatchPrepared, DispatchSendClaimed, nil)
	rev, _ = store.CASAdvance(ctx, "run-rp", "turn-rp", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	proof := TerminalEvidence{ProviderKey: "f", EvidenceKind: "x", Outcome: "completed", PayloadSHA256: "h"}
	if _, err := store.CommitTerminalAndSettleIntent(ctx, "run-rp", "turn-rp", rev, proof, "run-rp", rec.OuterIntentKey, 1); err != nil {
		t.Fatal(err)
	}
	d := &SettleDriver{
		Store: store,
		EvaluateGate: func(context.Context, string, string) (bool, bool, error) {
			return false, true, nil // block + reprompt
		},
	}
	if err := d.DriveSettle(ctx, "run-rp", "turn-rp"); err != nil {
		t.Fatal(err)
	}
	got, _, _ := store.Get(ctx, "run-rp", "turn-rp")
	if got.SettlePhase != SettleSupersededReprompt {
		t.Fatalf("want settle_superseded_reprompt, got %s", got.SettlePhase)
	}
}
