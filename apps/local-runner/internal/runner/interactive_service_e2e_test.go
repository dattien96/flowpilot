package runner

// E2E tests for CP-36 (agent review loop) and CP-41 (RAG harness flow mode).
// These tests drive InteractiveService end-to-end through spawnChildRun,
// applyFlowControl, and startTurn rather than calling internal helpers directly.
//
// Scenarios covered:
//   1.  Full cohort → hub auto-reinvoke → applyFlowControl("done") → loop done
//   2.  Hub calls applyFlowControl("continue") → coder re-entry prompt has feedback
//   3.  Round == Cap → applyFlowControl("continue") → blocked, awaiting_user
//   4.  applyFlowControl("escalate") → blocked with GateReason
//   5.  Blocked → extendCap → running, Cap raised by 2
//   6.  startTurn with coding StepID → provider prompt contains FlowContextPackage
//   7.  LocalFileSessionStore sidecar: persist + process-restart reload + FindFlowContextPackage
//   8.  BuildAuditDraft + PersistAuditDraft + FindAuditDraft round-trip
//   9.  Coding retry: second startTurn reuses cached PackageID (no rebuild)
//  10.  3-member parallel cohort → hub reinvoked exactly once

import (
	"context"
	"strings"
	"testing"
	"time"
)

// ── helper ───────────────────────────────────────────────────────────────────

// waitLoop polls cond at 1 ms intervals until it returns true or the deadline.
func waitLoop(t *testing.T, label string, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("waitLoop: timed out after %s waiting for %s", timeout, label)
}

// ── CP-36 E2E ─────────────────────────────────────────────────────────────────

// TestE2EReviewLoopApprovedPath drives the full cohort → hub-reinvoke → done
// path. Two reviewer children share a cohort; the last triggers exactly one hub
// turn that calls applyFlowControl("done"), closing the loop.
func TestE2EReviewLoopApprovedPath(t *testing.T) {
	var svc *InteractiveService
	var parentID string
	hubCalls := 0

	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				if strings.Contains(req.Prompt, "Agent results ready") {
					hubCalls++
					if svc != nil && parentID != "" {
						_, _ = svc.applyFlowControl(parentID, FlowControlInput{
							Status:  "done",
							Summary: "all reviewers approved",
						})
					}
				}
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc = newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	ph, _ := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if ph.RunID == "" {
		t.Fatal("createRun returned empty RunID")
	}
	parentID = ph.RunID

	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	svc.runs[parentID].autoOrchestrate = true
	svc.mu.Unlock()

	// Coder completes first.
	if _, e := svc.spawnChildRun(context.Background(), parentID, SpawnAgentInput{
		Agent: "coder", Prompt: "implement the feature", Wait: true,
	}); e != nil {
		t.Fatalf("spawnChildRun(coder): %v", e)
	}

	// Two reviewers in a cohort — last one triggers hub auto-reinvoke.
	for i, lbl := range []string{"reviewer-correctness", "reviewer-security"} {
		if _, e := svc.spawnChildRun(context.Background(), parentID, SpawnAgentInput{
			Agent: lbl, Prompt: "review the coder output", Wait: true,
			FlowCohortID: "review-1", Label: lbl, CohortSize: 2,
			AutoOrchestrate: i == 0,
		}); e != nil {
			t.Fatalf("spawnChildRun(%s): %v", lbl, e)
		}
	}

	// Hub is reinvoked asynchronously; wait for loop to reach "done".
	waitLoop(t, "loop.Status==done", 3*time.Second, func() bool {
		return svc.agentOrchestrator.loopStateFor(parentID).Status == "done"
	})

	st := svc.agentOrchestrator.loopStateFor(parentID)
	if st.Status != "done" {
		t.Errorf("loop.Status = %q, want done (hubCalls=%d)", st.Status, hubCalls)
	}
	if hubCalls != 1 {
		t.Errorf("hub called %d times, want exactly 1", hubCalls)
	}
}

// TestE2EReviewLoopChangesRequestedFeedbackReachesCoderPrompt verifies that
// applyFlowControl("continue", summary=...) triggers a coder re-entry whose
// prompt contains the supplied feedback text. Drives the full path:
//   applyFlowControl → maybeReinvokeCoderForContinue → scheduleChildTurn → runTurn adapter
func TestE2EReviewLoopChangesRequestedFeedbackReachesCoderPrompt(t *testing.T) {
	reentryPrompts := make(chan string, 5)

	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				// Only capture re-entry prompts — they contain the feedback text.
				if strings.Contains(req.Prompt, "null pointer") {
					reentryPrompts <- req.Prompt
				}
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})

	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	ph, _ := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if ph.RunID == "" {
		t.Fatal("createRun returned empty RunID")
	}
	parentID := ph.RunID
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	// Spawn coder and wait for its first turn to complete.
	if _, e := svc.spawnChildRun(context.Background(), parentID, SpawnAgentInput{
		Agent: "coder", Prompt: "implement auth middleware", Wait: true,
	}); e != nil {
		t.Fatalf("spawnChildRun(coder): %v", e)
	}

	// Simulate hub calling submit_review_outcome(changes_requested, feedback=...).
	if _, err := svc.applyFlowControl(parentID, FlowControlInput{
		Status:  "continue",
		Summary: "fix the null pointer dereference in the request handler",
	}); err != nil {
		t.Fatalf("applyFlowControl(continue): %v", err)
	}

	// Coder re-entry should fire with the feedback embedded in the prompt.
	var reentryPrompt string
	select {
	case reentryPrompt = <-reentryPrompts:
	case <-time.After(3 * time.Second):
		t.Fatal("coder re-entry did not fire within 3 s after applyFlowControl(continue)")
	}

	if !strings.Contains(reentryPrompt, "null pointer") {
		t.Errorf("re-entry prompt = %.200q, want feedback containing 'null pointer'", reentryPrompt)
	}

	st := svc.agentOrchestrator.loopStateFor(parentID)
	if st.Round < 1 {
		t.Errorf("loop.Round = %d after one continue cycle, want >= 1", st.Round)
	}
}

// TestE2EReviewLoopCapHitBlocked verifies that applyFlowControl("continue")
// when Round == Cap transitions the loop to blocked and returns NextAction=awaiting_user.
func TestE2EReviewLoopCapHitBlocked(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	// Place the loop exactly at the cap.
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running", Cap: 2, RoundCap: 2, Round: 2})

	result, err := svc.applyFlowControl(runID, FlowControlInput{Status: "continue", Summary: "still open issues"})
	if err != nil {
		t.Fatalf("applyFlowControl: %v", err)
	}
	if result.NextAction != "awaiting_user" {
		t.Errorf("NextAction = %q, want awaiting_user", result.NextAction)
	}

	st := svc.agentOrchestrator.loopStateFor(runID)
	if st.Status != "blocked" {
		t.Errorf("loop.Status = %q, want blocked", st.Status)
	}
}

// TestE2EReviewLoopEscalatePath verifies that applyFlowControl("escalate")
// moves the loop to blocked and stores the supplied GateReason.
func TestE2EReviewLoopEscalatePath(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running", Cap: 5, RoundCap: 5})

	result, err := svc.applyFlowControl(runID, FlowControlInput{
		Status:  "escalate",
		Summary: "security concern requires human review",
	})
	if err != nil {
		t.Fatalf("applyFlowControl(escalate): %v", err)
	}
	if result.Status != "blocked" {
		t.Errorf("result.Status = %q, want blocked", result.Status)
	}
	if result.NextAction != "awaiting_user" {
		t.Errorf("result.NextAction = %q, want awaiting_user", result.NextAction)
	}

	snap := svc.agentGraphSnapshot(runID)
	if snap.LoopState.Status != "blocked" {
		t.Errorf("snap.LoopState.Status = %q, want blocked", snap.LoopState.Status)
	}
	if snap.LoopState.GateReason == "" {
		t.Error("snap.LoopState.GateReason is empty, want non-empty reason")
	}
}

// TestE2EReviewLoopExtendCapResumesFromBlocked verifies that extendCap on a
// blocked loop raises the cap by 2 and moves status back to running.
func TestE2EReviewLoopExtendCapResumesFromBlocked(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "blocked", Cap: 3, RoundCap: 3, Round: 3})

	result, err := svc.extendCap(runID)
	if err != nil {
		t.Fatalf("extendCap: %v", err)
	}
	if result.Cap != 5 {
		t.Errorf("Cap after extend = %d, want 5 (3+2)", result.Cap)
	}

	st := svc.agentOrchestrator.loopStateFor(runID)
	if st.Status != "running" {
		t.Errorf("loop.Status = %q after extendCap, want running", st.Status)
	}
	if st.Cap != 5 {
		t.Errorf("loop.Cap = %d, want 5", st.Cap)
	}
	if st.ExtendCount != 1 {
		t.Errorf("loop.ExtendCount = %d, want 1", st.ExtendCount)
	}
}

// ── CP-41 E2E ─────────────────────────────────────────────────────────────────

// TestE2EPlanCodingFlowContextHandoff verifies that a turn started with a
// coding StepID has the rendered FlowContextPackage prepended in the provider
// prompt, end-to-end through startTurn → runTurn → injectFlowContextIfCoding.
func TestE2EPlanCodingFlowContextHandoff(t *testing.T) {
	workspace, _ := fcpFixture(t)
	store := newFakeWorkflowStore()

	var capturedPrompt string
	done := make(chan struct{})

	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				capturedPrompt = req.Prompt
				close(done)
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "coding done"})
				return nil
			})
		},
	})

	svc := newInteractiveService(reg, newInteractiveCatalog(), store)
	ph, _ := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if ph.RunID == "" {
		t.Fatal("createRun returned empty RunID")
	}
	runID := ph.RunID

	// Seed plan+coding steps and set the workspace.
	store.seed(runID, planCodingSteps("step-plan", "step-coding"))
	svc.mu.Lock()
	svc.runs[runID].workspaceCwd = workspace
	svc.mu.Unlock()

	if _, apiErr := svc.startTurn(runID, TurnInput{StepID: "step-coding", Prompt: "implement per the plan"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn: %v", apiErr)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("adapter was not called within 3 s")
	}

	if !strings.Contains(capturedPrompt, "## Flow Context Package") {
		t.Errorf("coding prompt missing '## Flow Context Package'; prompt[:300] = %.300q", capturedPrompt)
	}
	if !strings.Contains(capturedPrompt, "implement per the plan") {
		t.Error("original instruction must appear after the package block")
	}
}

// TestE2EFlowEventSidecarPersistAndReload verifies that EventFlowContextPackage
// events appended to a LocalFileSessionStore survive a process-restart simulation
// (new store instance on same dir) and can be found by FindFlowContextPackage.
func TestE2EFlowEventSidecarPersistAndReload(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	store1, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}

	pkg := FlowContextPackage{
		PackageID:     "pkg-e2e-sidecar",
		WorkflowRunID: "run-sidecar-1",
		SourceDocIDs:  []string{"Task-178"},
	}
	ev := ProviderEvent{
		Type:              EventFlowContextPackage,
		WorkflowRunID:     "run-sidecar-1",
		WorkflowStepRunID: "step-plan",
		FlowContextPackage: &pkg,
	}
	if err := store1.AppendEvent(ctx, ev); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	// Simulate process restart — new store pointing at the same directory.
	store2, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore (reload): %v", err)
	}

	evs, err := store2.LoadFlowEvents(ctx, "run-sidecar-1")
	if err != nil {
		t.Fatalf("LoadFlowEvents: %v", err)
	}
	if len(evs) == 0 {
		t.Fatal("LoadFlowEvents returned 0 events after sidecar reload")
	}

	found, ok := FindFlowContextPackage(evs, "step-plan")
	if !ok {
		t.Fatal("FindFlowContextPackage: event not found after reload")
	}
	if found.PackageID != "pkg-e2e-sidecar" {
		t.Errorf("found.PackageID = %q, want pkg-e2e-sidecar", found.PackageID)
	}
	if len(found.SourceDocIDs) == 0 || found.SourceDocIDs[0] != "Task-178" {
		t.Errorf("found.SourceDocIDs = %v, want [Task-178]", found.SourceDocIDs)
	}
}

// TestE2EAuditDraftBuildPersistAndFind covers the full CP-41 audit path:
// build → persist to fakeWorkflowStore → retrieve via FindAuditDraft.
func TestE2EAuditDraftBuildPersistAndFind(t *testing.T) {
	workspace, _ := auditFixture(t)
	store := newFakeWorkflowStore()
	ctx := context.Background()

	hints := FlowContextHints{
		WorkflowRunID: "run-audit-e2e",
		PlanStepRunID: "step-plan",
		UserPrompt:    "agent-flow-engine",
		SourceDocID:   "Task-178",
	}
	pkg, err := BuildFlowContextPackage(workspace, hints)
	if err != nil {
		t.Fatalf("BuildFlowContextPackage: %v", err)
	}

	input := auditDraftInput(workspace, pkg, "passed")
	input.WorkflowRunID = "run-audit-e2e"
	input.AuditStepID = "step-audit-e2e"
	draft := BuildAuditDraft(input)

	if draft.Status != "ready" {
		t.Errorf("draft.Status = %q, want ready", draft.Status)
	}

	if err := PersistAuditDraft(ctx, store, "run-audit-e2e", "step-audit-e2e", draft); err != nil {
		t.Fatalf("PersistAuditDraft: %v", err)
	}

	found, ok := FindAuditDraft(store.events["run-audit-e2e"], "step-audit-e2e")
	if !ok {
		t.Fatal("FindAuditDraft: event not found after PersistAuditDraft")
	}
	if found.Status != "ready" {
		t.Errorf("found.Status = %q, want ready", found.Status)
	}
	if found.WorkflowRunID != "run-audit-e2e" {
		t.Errorf("found.WorkflowRunID = %q, want run-audit-e2e", found.WorkflowRunID)
	}
	if found.AuditStepID != "step-audit-e2e" {
		t.Errorf("found.AuditStepID = %q, want step-audit-e2e", found.AuditStepID)
	}
}

// TestE2ECodingRetryReusesSamePackageID verifies that a second coding turn on
// the same run reuses the cached planContextPackage rather than rebuilding it
// (ensures no double-injection and stable PackageID across retries).
func TestE2ECodingRetryReusesSamePackageID(t *testing.T) {
	workspace, _ := fcpFixture(t)
	store := newFakeWorkflowStore()

	turnCount := 0
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				turnCount++
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "done"})
				return nil
			})
		},
	})

	svc := newInteractiveService(reg, newInteractiveCatalog(), store)
	ph, _ := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if ph.RunID == "" {
		t.Fatal("createRun returned empty RunID")
	}
	runID := ph.RunID

	store.seed(runID, planCodingSteps("step-plan", "step-coding"))
	svc.mu.Lock()
	svc.runs[runID].workspaceCwd = workspace
	svc.mu.Unlock()

	// First coding turn.
	if _, apiErr := svc.startTurn(runID, TurnInput{StepID: "step-coding", Prompt: "code it"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn #1: %v", apiErr)
	}
	waitLoop(t, "turn 1 complete", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[runID].turnInFlight
	})

	svc.mu.Lock()
	pkg1 := svc.runs[runID].planContextPackage
	svc.mu.Unlock()
	if pkg1 == nil {
		t.Fatal("planContextPackage is nil after first coding turn")
	}
	pkgID1 := pkg1.PackageID

	// Second coding turn (retry scenario).
	if _, apiErr := svc.startTurn(runID, TurnInput{StepID: "step-coding", Prompt: "retry after failure"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn #2: %v", apiErr)
	}
	waitLoop(t, "turn 2 complete", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[runID].turnInFlight
	})

	svc.mu.Lock()
	pkg2 := svc.runs[runID].planContextPackage
	svc.mu.Unlock()
	if pkg2 == nil {
		t.Fatal("planContextPackage is nil after second coding turn")
	}

	if pkg2.PackageID != pkgID1 {
		t.Errorf("PackageID changed on retry: %q → %q (want same ID)", pkgID1, pkg2.PackageID)
	}
	if turnCount != 2 {
		t.Errorf("turnCount = %d, want 2", turnCount)
	}
}

// TestE2EParallelCodingCohortReinvokesHub verifies that a 3-member cohort
// triggers at least one hub auto-reinvoke. Sequential Wait=true spawning fires
// one reinvoke per member completion; the single-flight (concurrent) guard is
// covered separately by TestAutoReinvokeHubSingleFlightConcurrent.
func TestE2EParallelCodingCohortReinvokesHub(t *testing.T) {
	var svc *InteractiveService
	var parentID string

	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "done"})
				return nil
			})
		},
	})
	svc = newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	ph, _ := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if ph.RunID == "" {
		t.Fatal("createRun returned empty RunID")
	}
	parentID = ph.RunID

	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Cap: 5, RoundCap: 5})
	svc.mu.Lock()
	svc.runs[parentID].autoOrchestrate = true
	svc.mu.Unlock()

	// 3-member cohort with Wait=true (sequential in test; barrier fires after the 3rd).
	for i := 0; i < 3; i++ {
		lbl := "coder-shard-" + string(rune('A'+i))
		ao := i == 0 // autoOrchestrate only on first member
		if _, e := svc.spawnChildRun(context.Background(), parentID, SpawnAgentInput{
			Agent: lbl, Prompt: "implement shard " + lbl, Wait: true,
			FlowCohortID: "coding-cohort", Label: lbl, CohortSize: 3,
			AutoOrchestrate: ao,
		}); e != nil {
			t.Fatalf("spawnChildRun(%s): %v", lbl, e)
		}
	}

	// Wait for hub to be reinvoked.
	waitLoop(t, "hub turnCount > 0", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return svc.runs[parentID].turnCount > 0
	})

	// Allow any in-flight hub turn to complete before reading turnCount.
	waitLoop(t, "hub turn complete", 2*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[parentID].turnInFlight
	})

	svc.mu.Lock()
	tc := svc.runs[parentID].turnCount
	svc.mu.Unlock()

	if tc == 0 {
		t.Error("hub was not auto-reinvoked after 3-member coding cohort completed")
	}
}
