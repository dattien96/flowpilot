package runner

import (
	"context"
	"errors"
	"testing"
	"time"
)

// bug288_round11_test.go: focused regression coverage for BUG-288 "Vongf 11"
// findings P1-01, P1-04, P1-05, P1-07, P1-08, P1-09, P2-01, P2-02, P2-03.
// P1-11/TOCTOU is covered separately in source_excerpt_open_unix_test.go
// (unix-only) plus the existing TestReadSourceExcerptsRejectsNonRegular.

// ---- test doubles -----------------------------------------------------

// failingSessionUpsertStore wraps a *fakeWorkflowStore and lets a test force
// UpsertProviderSession/UpsertApproval/UpsertQuestion to fail N times before
// falling through to the real (in-memory) implementation.
type failingSessionUpsertStore struct {
	*fakeWorkflowStore
	failSessionTimes  int
	failApprovalTimes int
}

func (f *failingSessionUpsertStore) UpsertProviderSession(ctx context.Context, session ProviderSessionState) error {
	if f.failSessionTimes > 0 {
		f.failSessionTimes--
		return errors.New("injected session persist failure")
	}
	return f.fakeWorkflowStore.UpsertProviderSession(ctx, session)
}

func (f *failingSessionUpsertStore) UpsertApproval(ctx context.Context, approval ProviderApprovalState) error {
	if f.failApprovalTimes > 0 {
		f.failApprovalTimes--
		return errors.New("injected approval persist failure")
	}
	return f.fakeWorkflowStore.UpsertApproval(ctx, approval)
}

// failingTransitionLogStore wraps *fakeWorkflowStore and always fails
// AppendStepTransition, for the P2-03 degraded-marker test.
type failingTransitionLogStore struct {
	*fakeWorkflowStore
}

func (f *failingTransitionLogStore) AppendStepTransition(context.Context, string, stepTransitionLine) error {
	return errors.New("injected transition log append failure")
}
func (f *failingTransitionLogStore) LoadStepTransitions(context.Context, string) ([]stepTransitionLine, error) {
	return nil, nil
}
func (f *failingTransitionLogStore) DeleteStepTransitions(context.Context, string) error { return nil }

// failOnCompletedSessionStore fails exactly the first UpsertProviderSession
// call that persists targetRunID with a Completed status, regardless of how
// many other session persists (spawn, turn-started, etc.) happen first or
// after — used to pin down the exact post-gate-pass persist in runTurn's live
// path (BUG-288 R11 #3), which a plain call-count-based fail-N-times double
// cannot target reliably.
type failOnCompletedSessionStore struct {
	*fakeWorkflowStore
	targetRunID string
	fired       bool
}

func (f *failOnCompletedSessionStore) UpsertProviderSession(ctx context.Context, session ProviderSessionState) error {
	if !f.fired && session.RunID == f.targetRunID && session.Status == RunStatusCompleted {
		f.fired = true
		return errors.New("injected session persist failure")
	}
	return f.fakeWorkflowStore.UpsertProviderSession(ctx, session)
}

// epochBumpOnCompletedSessionStore succeeds the first UpsertProviderSession
// call that persists targetRunID with a Completed status (unlike
// failOnCompletedSessionStore), but bumps gateEpoch as a side effect first —
// simulating a concurrent Stop landing while runTurn's live post-gate persist
// call is in flight (persistProviderSession always runs with s.mu unlocked).
// Used to pin down BUG-288 R11 #4 on the live runTurn path (the sibling of
// resumePendingFlowGate's already-covered post-persist epoch revalidation).
type epochBumpOnCompletedSessionStore struct {
	*fakeWorkflowStore
	svc         *InteractiveService
	targetRunID string
	fired       bool
}

func (f *epochBumpOnCompletedSessionStore) UpsertProviderSession(ctx context.Context, session ProviderSessionState) error {
	if !f.fired && session.RunID == f.targetRunID && session.Status == RunStatusCompleted {
		f.fired = true
		f.svc.mu.Lock()
		if rs := f.svc.runs[f.targetRunID]; rs != nil {
			rs.gateEpoch++
		}
		f.svc.mu.Unlock()
	}
	return f.fakeWorkflowStore.UpsertProviderSession(ctx, session)
}

// ---- P1-01: resolving/retry does not falsely report success -----------

// TestSubmitApprovalDecisionRetryAfterPersistFailureCompletes proves that a
// retry of the SAME approval decision after a transient persistence failure
// (a) does not report false success on the first (failing) call, and (b)
// actually completes the durable write and signals the waiter on the retry,
// instead of short-circuiting to nil because RAM had already flipped to
// "resolved" before persistence was confirmed (BUG-288 P1-01).
func TestSubmitApprovalDecisionRetryAfterPersistFailureCompletes(t *testing.T) {
	svc := NewInteractiveService()
	base := svc.workflowStore.(*fakeWorkflowStore)
	failing := &failingSessionUpsertStore{fakeWorkflowStore: base, failApprovalTimes: 1}
	svc.workflowStore = failing

	rs := &interactiveRun{id: "run-p101", providerKey: ProviderKeyClaude, workspaceCwd: t.TempDir()}
	rec := &approvalRecord{
		id:      "appr-p101",
		runID:   rs.id,
		status:  "pending",
		resolve: make(chan string, 1),
		details: ApprovalDetails{
			Command: "npm test", Kind: "exec",
			Decisions: []ApprovalDecisionOption{{Value: "approve", Label: "Approve"}, {Value: "deny", Label: "Deny"}},
		},
	}
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	svc.approvals[rec.id] = rec
	svc.mu.Unlock()

	if apiErr := svc.SubmitApprovalDecision(rec.id, "approve"); apiErr == nil {
		t.Fatal("expected first submit to surface the injected persist failure, got nil")
	}
	svc.mu.Lock()
	status := svc.approvals[rec.id].status
	svc.mu.Unlock()
	if status != "resolving" {
		t.Fatalf("status after failed persist = %q, want %q (transitional, not falsely resolved)", status, "resolving")
	}

	// Retry: must redo the missing durable write and actually complete, not
	// short-circuit to a false nil success while nothing is durable.
	if apiErr := svc.SubmitApprovalDecision(rec.id, "approve"); apiErr != nil {
		t.Fatalf("retry SubmitApprovalDecision: %v", apiErr)
	}
	svc.mu.Lock()
	status = svc.approvals[rec.id].status
	svc.mu.Unlock()
	if status != "resolved" {
		t.Fatalf("status after successful retry = %q, want resolved", status)
	}
	select {
	case d := <-rec.resolve:
		if d != "approve" {
			t.Fatalf("resolve = %q, want approve", d)
		}
	default:
		t.Fatal("expected the waiter to be signaled after the retry completed durably")
	}
	if st, ok := base.approvals[rec.id]; !ok || st.Status != "resolved" {
		t.Fatalf("durable approval = %+v, want resolved", st)
	}
}

// ---- P1-04: stale parent stop-generation blocks a child gate resurrect --

// TestResumePendingFlowGateRejectsStaleParentStopGeneration proves a child
// whose last checkpoint predates its parent's current stop generation cannot
// resurrect its deferred gate (BUG-288 P1-04).
func TestResumePendingFlowGateRejectsStaleParentStopGeneration(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir() // empty workspace: gate would trivially pass if it ran
	svc.mu.Lock()
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[parent.RunID].activeFlowNodes = reviewLoopTestNodes()
	svc.runs[parent.RunID].stopGeneration = 2 // Stop happened twice on the parent
	crs := svc.runs[child.RunID]
	crs.parentRunID = parent.RunID
	crs.label = "coder"
	crs.workspaceCwd = dir
	crs.pendingFlowGateSettle = true
	crs.pendingFlowGateFinalMsg = "should not settle"
	crs.status = RunStatusRunning
	crs.parentStopGenSeen = 1 // child's last checkpoint predates the parent's stop
	svc.mu.Unlock()

	svc.resumePendingFlowGate(child.RunID)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	crs = svc.runs[child.RunID]
	if !crs.pendingFlowGateSettle {
		t.Fatal("stale-generation child gate must remain pending (no-op), not settle")
	}
	if crs.status == RunStatusCompleted {
		t.Fatal("stale-generation child must not be marked Completed")
	}
}

// TestResumePendingFlowGateAllowsCurrentParentStopGeneration is the sibling
// fast-path: a child checkpointed AT the parent's current generation is not
// rejected by the new guard.
func TestResumePendingFlowGateAllowsCurrentParentStopGeneration(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	svc.mu.Lock()
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[parent.RunID].activeFlowNodes = reviewLoopTestNodes()
	svc.runs[parent.RunID].stopGeneration = 2
	crs := svc.runs[child.RunID]
	crs.parentRunID = parent.RunID
	crs.label = "coder"
	crs.workspaceCwd = dir
	crs.pendingFlowGateSettle = true
	crs.status = RunStatusRunning
	crs.parentStopGenSeen = 2 // checkpointed at-or-after the parent's stop gen
	svc.mu.Unlock()

	svc.resumePendingFlowGate(child.RunID)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	crs = svc.runs[child.RunID]
	if crs.status != RunStatusCompleted {
		t.Fatalf("status = %q, want Completed (guard must not block a current-generation child)", crs.status)
	}
}

// ---- P1-05: gate settlement persists durably before broadcast/finalize --

// TestResumePendingFlowGatePersistFailureDoesNotSettle proves a durable
// session-persist failure during the gate-pass path rolls RAM back and never
// reaches the broadcast/cohort/finalizer side effects (BUG-288 P1-05).
func TestResumePendingFlowGatePersistFailureDoesNotSettle(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	svc.mu.Lock()
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[parent.RunID].activeFlowNodes = reviewLoopTestNodes()
	crs := svc.runs[child.RunID]
	crs.parentRunID = parent.RunID
	crs.label = "coder"
	crs.workspaceCwd = dir
	crs.pendingFlowGateSettle = true
	crs.pendingFlowGateFinalMsg = "should not broadcast"
	crs.status = RunStatusRunning
	crs.events = nil
	svc.mu.Unlock()

	// Inject the persist failure only for the gate's own settlement write, not
	// for anything createRun above already did.
	base := svc.workflowStore.(*fakeWorkflowStore)
	svc.workflowStore = &failingSessionUpsertStore{fakeWorkflowStore: base, failSessionTimes: 1}

	svc.resumePendingFlowGate(child.RunID)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	crs = svc.runs[child.RunID]
	if crs.status != RunStatusRunning {
		t.Fatalf("status after persist failure = %q, want rolled back to Running", crs.status)
	}
	if !crs.pendingFlowGateSettle {
		t.Fatal("pendingFlowGateSettle must be restored on persist failure so a later resume retries")
	}
	for _, ev := range crs.events {
		if ev.Type == EventTurnCompleted {
			t.Fatal("TurnCompleted must not be broadcast when the durable settlement persist failed")
		}
	}
}

// ---- BUG-288 R11 #3: live post-gate persist precedes broadcast/settle --

// TestRunTurnPostGatePersistFailureDoesNotBroadcastCompletion proves runTurn's
// live post-gate-pass path (interactive_service.go, the branch mirroring
// resumePendingFlowGate) persists the Completed snapshot BEFORE
// signalChild/broadcast/settle — an injected persist failure must roll the
// child back to Running with pendingFlowGateSettle restored and must not have
// broadcast a TurnCompleted event or released dependents (BUG-288 R11 #3).
func TestRunTurnPostGatePersistFailureDoesNotBroadcastCompletion(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "reviewed, lgtm"})
				return nil
			})
		},
	})

	store := newFakeWorkflowStore()
	svc := newInteractiveService(reg, newInteractiveCatalog(), store)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = reviewLoopTestNodes()
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[parent.RunID].workspaceCwd = t.TempDir() // no git repo -> zero-cost gate, empty diff
	svc.mu.Unlock()

	spawned, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent: "agents/coder.md", Prompt: "review it", Label: "coder", Wait: false,
	})
	if spawnErr != nil {
		t.Fatalf("spawnChildRun: %v", spawnErr)
	}
	childID := spawned.RunID

	// Inject the persist failure only for THIS child's post-gate Completed
	// snapshot, regardless of how many other session persists (spawn,
	// turn-started) already happened.
	svc.mu.Lock()
	svc.workflowStore = &failOnCompletedSessionStore{fakeWorkflowStore: store, targetRunID: childID}
	svc.mu.Unlock()

	// Two-phase wait: turnInFlight starts false before the async spawn goroutine
	// has even called startTurn, so waiting only for "false" would race ahead of
	// the turn ever running. Wait for it to go true first, then back to false.
	waitLoop(t, "turn starts", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		rs := svc.runs[childID]
		return rs != nil && rs.turnInFlight
	})
	waitLoop(t, "turn settles (success or rolled-back) after the persist race", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		rs := svc.runs[childID]
		return rs != nil && !rs.turnInFlight
	})

	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[childID]
	if rs.status == RunStatusCompleted {
		t.Fatal("status must not be Completed when the post-gate persist failed")
	}
	if !rs.pendingFlowGateSettle {
		t.Fatal("pendingFlowGateSettle must be restored so a later resume retries")
	}
	// emitLocked always appends an in-memory placeholder EventTurnCompleted for
	// the raw provider event (deferGateCompleted only skips ITS OWN persistEvent
	// call) — the real invariant is that the gate-pass branch's own completedEv
	// never reached the durable store.
	store.mu.Lock()
	persisted := append([]ProviderEvent(nil), store.events[childID]...)
	store.mu.Unlock()
	for _, ev := range persisted {
		if ev.Type == EventTurnCompleted {
			t.Fatal("TurnCompleted must not be durably persisted when the post-gate persist failed")
		}
	}
}

// TestRunTurnPostGatePersistEpochBumpDoesNotFanOut proves runTurn's live
// post-gate-pass path revalidates gateEpoch AFTER the (successful) durable
// persist call returns — a Stop landing while persistProviderSession runs
// unlocked must suppress signalChild/event-broadcast/settle/release even
// though the Completed write itself already succeeded (BUG-288 R11 #4, the
// live-path sibling of resumePendingFlowGate's existing post-persist epoch
// check).
func TestRunTurnPostGatePersistEpochBumpDoesNotFanOut(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "reviewed, lgtm"})
				return nil
			})
		},
	})

	store := newFakeWorkflowStore()
	svc := newInteractiveService(reg, newInteractiveCatalog(), store)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = reviewLoopTestNodes()
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[parent.RunID].workspaceCwd = t.TempDir() // no git repo -> zero-cost gate, empty diff
	svc.mu.Unlock()

	spawned, spawnErr := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent: "agents/coder.md", Prompt: "review it", Label: "coder", Wait: false,
	})
	if spawnErr != nil {
		t.Fatalf("spawnChildRun: %v", spawnErr)
	}
	childID := spawned.RunID

	svc.mu.Lock()
	epochBefore := svc.runs[childID].gateEpoch
	hooked := &epochBumpOnCompletedSessionStore{fakeWorkflowStore: store, svc: svc, targetRunID: childID}
	svc.workflowStore = hooked
	svc.mu.Unlock()

	waitLoop(t, "turn starts", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		rs := svc.runs[childID]
		return rs != nil && rs.turnInFlight
	})
	waitLoop(t, "turn settles after the epoch-bump persist race", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		rs := svc.runs[childID]
		return rs != nil && !rs.turnInFlight
	})

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if !hooked.fired {
		t.Fatal("test setup: epoch-bump hook on the Completed persist did not fire")
	}
	rs := svc.runs[childID]
	if rs.gateEpoch == epochBefore {
		t.Fatal("test setup: gateEpoch was not actually bumped during the persist call")
	}
	store.mu.Lock()
	persisted := append([]ProviderEvent(nil), store.events[childID]...)
	store.mu.Unlock()
	for _, ev := range persisted {
		if ev.Type == EventTurnCompleted {
			t.Fatal("a gateEpoch bump landing during the unlocked post-gate persist call must suppress the live fan-out (no durably persisted TurnCompleted)")
		}
	}
}

// ---- P1-07 / P1-08: pending card TTL is durable and re-checked ---------

// TestRehydratePendingGatesDurablyExpiresElapsedApproval proves a pending
// approval whose durable TTL already elapsed while the process was down is
// expired on rehydrate rather than resurrected as a live pending card
// (BUG-288 P1-08).
func TestRehydratePendingGatesDurablyExpiresElapsedApproval(t *testing.T) {
	svc := NewInteractiveService()
	base := svc.workflowStore.(*fakeWorkflowStore)
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	if err := base.UpsertApproval(context.Background(), ProviderApprovalState{
		ApprovalID: "appr-ttl-1", RunID: "run-ttl-1", Status: "pending",
		Command: "npm test", ExpiresAt: past,
	}); err != nil {
		t.Fatalf("seed approval: %v", err)
	}
	rs := &interactiveRun{id: "run-ttl-1", providerKey: ProviderKeyClaude, status: RunStatusRunning}
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	svc.rehydratePendingGatesLocked(rs.id)
	rec := svc.approvals["appr-ttl-1"]
	svc.mu.Unlock()
	if rec == nil {
		t.Fatal("expected a record to be rehydrated")
	}
	if rec.status != "expired" {
		t.Fatalf("status = %q, want expired (elapsed TTL must not resurrect as pending)", rec.status)
	}
}

// TestSubmitApprovalDecisionRejectsElapsedTTLOnSubmit proves Submit re-checks
// the durable TTL under the lock even if no in-process timer has fired yet
// (BUG-288 P1-08) — e.g. immediately after a restart.
func TestSubmitApprovalDecisionRejectsElapsedTTLOnSubmit(t *testing.T) {
	svc := NewInteractiveService()
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	rs := &interactiveRun{id: "run-ttl-2", providerKey: ProviderKeyClaude}
	rec := &approvalRecord{
		id: "appr-ttl-2", runID: rs.id, status: "pending", resolve: make(chan string, 1),
		expiresAt: past,
		details:   ApprovalDetails{Decisions: []ApprovalDecisionOption{{Value: "approve", Label: "Approve"}, {Value: "deny", Label: "Deny"}}},
	}
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	svc.approvals[rec.id] = rec
	svc.mu.Unlock()

	apiErr := svc.SubmitApprovalDecision(rec.id, "approve")
	if apiErr == nil || apiErr.code != "question_expired" {
		t.Fatalf("SubmitApprovalDecision on elapsed TTL = %+v, want question_expired conflict", apiErr)
	}
	svc.mu.Lock()
	status := svc.approvals[rec.id].status
	svc.mu.Unlock()
	if status != "expired" {
		t.Fatalf("status = %q, want expired", status)
	}
}

// ---- P1-09: blocked durable intent + requeue hook -----------------------

// TestRequeueBlockedIntentClearsStateAndRetries proves the blocked-intent
// marker set when the permanent-fail budget is exhausted can be cleared via
// RequeueBlockedIntent, which resets the fail budget so a later flush retries
// instead of the run remaining inert forever (BUG-288 P1-09).
func TestRequeueBlockedIntentClearsStateAndRetries(t *testing.T) {
	svc, _ := newTestServer(t)
	root, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	rs := svc.runs[root.RunID]
	rs.intentBlockedKind = "resume"
	rs.intentBlockedReason = "provider_unavailable"
	rs.intentBlockedAt = time.Now().UTC().Format(time.RFC3339Nano)
	rs.pendingResumeFailCount = durableIntentMaxPermanentFails
	rs.pendingResumeFailGen = 3
	svc.mu.Unlock()

	if apiErr := svc.RequeueBlockedIntent(root.RunID); apiErr != nil {
		t.Fatalf("RequeueBlockedIntent: %v", apiErr)
	}
	svc.mu.Lock()
	rs = svc.runs[root.RunID]
	blockedKind, failCount := rs.intentBlockedKind, rs.pendingResumeFailCount
	svc.mu.Unlock()
	if blockedKind != "" {
		t.Fatalf("intentBlockedKind = %q, want cleared", blockedKind)
	}
	if failCount != 0 {
		t.Fatalf("pendingResumeFailCount = %d, want reset to 0", failCount)
	}

	// A second requeue with nothing blocked must report a conflict, not
	// silently succeed.
	if apiErr := svc.RequeueBlockedIntent(root.RunID); apiErr == nil || apiErr.code != "not_blocked" {
		t.Fatalf("second RequeueBlockedIntent = %+v, want not_blocked conflict", apiErr)
	}
}

// ---- P2-01: multi-select choices survive reconciliation verbatim -------

// TestRehydrateReconciliationPreservesMultiSelectChoices proves a crashed
// multi-select answer reconciles to its ORIGINAL slice, not a re-wrapped join
// of the display string (BUG-288 P2-01): ["a, b", "c"] must not become
// ["a, b, c"].
func TestRehydrateReconciliationPreservesMultiSelectChoices(t *testing.T) {
	svc := NewInteractiveService()
	base := svc.workflowStore.(*fakeWorkflowStore)
	if err := base.UpsertQuestion(context.Background(), ProviderQuestionState{
		QuestionID: "q-p201", RunID: "run-p201", Status: "pending", MultiSelect: true,
	}); err != nil {
		t.Fatalf("seed question: %v", err)
	}
	rs := &interactiveRun{
		id: "run-p201", providerKey: ProviderKeyClaude, status: RunStatusRunning,
		pendingResumeApprovalID:      "q-p201",
		pendingResumeDecision:        "a, b, c", // joined display string (lossy to re-split)
		pendingResumeQuestionChoices: []string{"a, b", "c"},
	}
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	svc.rehydratePendingGatesLocked(rs.id)
	rec := svc.questions["q-p201"]
	svc.mu.Unlock()
	if rec == nil {
		t.Fatal("expected question record to be reconciled")
	}
	if len(rec.choice) != 2 || rec.choice[0] != "a, b" || rec.choice[1] != "c" {
		t.Fatalf("reconciled choice = %#v, want [\"a, b\" \"c\"] verbatim (not flattened)", rec.choice)
	}
}

// ---- P2-02: Stop signals typed interruption, not a fake empty answer ----

// TestCancelPendingGatesSignalsInterruptedNotEmptyAnswer proves Stop's
// cancellation path sends a distinguishable error, not an empty-but-valid
// answer that could race ctx.Done() and look like a real submission
// (BUG-288 P2-02).
func TestCancelPendingGatesSignalsInterruptedNotEmptyAnswer(t *testing.T) {
	svc, _ := newTestServer(t)
	rs := &interactiveRun{id: "run-p202", providerKey: ProviderKeyClaude}
	rec := &questionRecord{
		id: "q-p202", runID: rs.id, status: "pending",
		resolve: make(chan questionResolveResult, 1),
	}
	svc.mu.Lock()
	svc.runs[rs.id] = rs
	svc.questions[rec.id] = rec
	svc.cancelPendingGatesForRunsLocked([]string{rs.id})
	svc.mu.Unlock()

	select {
	case r := <-rec.resolve:
		if r.err == nil {
			t.Fatalf("resolve result = %+v, want a non-nil interrupted error", r)
		}
		if !errors.Is(r.err, errInterrupted) {
			t.Fatalf("resolve err = %v, want errInterrupted", r.err)
		}
		if r.choices != nil {
			t.Fatalf("resolve choices = %#v, want nil on interruption", r.choices)
		}
	default:
		t.Fatal("expected a resolve result after cancelPendingGatesForRunsLocked")
	}
}

// ---- P2-03: transition log append failure is durably marked degraded ---

// TestAppendStepTransitionLogFailureMarksRunDegraded proves a failed
// transition-log append durably records a degraded marker instead of only a
// log-warn (BUG-288 P2-03), so restore/replay can detect the step timeline is
// not fully trustworthy.
func TestAppendStepTransitionLogFailureMarksRunDegraded(t *testing.T) {
	svc, _ := newTestServer(t)
	base := svc.workflowStore.(*fakeWorkflowStore)
	svc.workflowStore = &failingTransitionLogStore{fakeWorkflowStore: base}

	root, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.runs[root.RunID].flowEngineDriven = true
	svc.mu.Unlock()
	base.seed(root.RunID, []RuntimeWorkflowStep{{ID: "n1", Status: StepStatusPending}})

	svc.setFlowStepStatus(context.Background(), root.RunID, "n1", StepStatusRunning)

	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[root.RunID]
	if !rs.transitionLogDegraded {
		t.Fatal("expected transitionLogDegraded=true after a failed AppendStepTransition")
	}
	if rs.transitionLogDegradedReason == "" {
		t.Fatal("expected a non-empty transitionLogDegradedReason")
	}
}
