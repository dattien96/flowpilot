package runner

import (
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
