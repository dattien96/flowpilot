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
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
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
	// V9-24 / Task-240 D-9: flow E2E must not leave steps stuck RUNNING.
	assertNoStepStuckRunning(t, svc, parentID)
}

// TestE2EReviewLoopApprovedPathPersistsTerminalLoopStateAfterHubTurnFinishes is
// the BUG-257 regression: a fully-completed flow run (all reviewers approved,
// hub called submit_review_outcome(done)) must persist LoopState.Status=="done"
// on its LATEST sessions.ndjson record — the one a server-restart resume
// actually reads. markFlowRunComplete's own persist (fired synchronously inside
// applyFlowControl, before the tool call returns) got this right, but runTurn's
// post-turn persist — which fires moments later when the hub's own synthesis
// turn actually finishes streaming — rebuilt its snapshot via sessionStateOf,
// which never carries LoopState, and clobbered the terminal "done" back to a
// zero value. Because GetProviderSession/resume only ever reads the last
// record, that trailing write made a genuinely-complete run's synthesis step
// resume as CANCELED instead of DONE after a restart (run-7804 in the live
// repro, ~4s between the tool call and the turn's own completion). The fake
// adapter sleeps briefly between the tool call and TurnCompleted to reproduce
// that same ordering deterministically instead of racing two goroutines. Uses
// a real LocalFileSessionStore (not the in-memory fake) so this test reads
// back exactly what a restart would see.
func TestE2EReviewLoopApprovedPathPersistsTerminalLoopStateAfterHubTurnFinishes(t *testing.T) {
	store, err := NewLocalFileSessionStore(filepath.Join(t.TempDir(), ".flowpilot", "chats"))
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}

	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				if autoAnswerPreflightContractPlanTurn(req, b) {
					return nil
				}
				if strings.Contains(req.Prompt, "Agent results ready") {
					if _, err := b.SubmitFlowControl(FlowControlInput{Status: "done", Summary: "all reviewers approved"}); err != nil {
						return err
					}
					// Give applyFlowControl's own `go persistParentSession` goroutine
					// time to land its "done" write before this turn finishes and
					// triggers runTurn's post-turn persist — matching the live repro's
					// multi-second gap between the tool call and the turn's own
					// TurnCompleted, and making the ordering deterministic instead of
					// racing two goroutines.
					time.Sleep(50 * time.Millisecond)
					b.Emit(ProviderEvent{Type: EventTurnCompleted, ProviderTurnID: req.ProviderTurnID, FinalMessage: "approved"})
					return nil
				}
				b.Emit(ProviderEvent{Type: EventTurnCompleted, ProviderTurnID: req.ProviderTurnID, FinalMessage: "ok"})
				return nil
			})
		},
	})

	svc := newInteractiveService(reg, newInteractiveCatalog(), store)
	ph, apiErr := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if apiErr != nil {
		t.Fatalf("createRun: %v", apiErr)
	}
	parentID := ph.RunID

	svc.startResolvedFlow(context.Background(), parentID, "flowpilot-core-flow-pack/review-loop", "fix the bug where 1+1 != 2")

	// Wait for the hub's synthesis turn to fully finish (not just for the loop
	// state to flip to "done", which happens mid-turn, before the sleep above and
	// before runTurn's post-turn persist even runs).
	waitLoop(t, "hub turn settles and persists as completed", 8*time.Second, func() bool {
		st, found, err := store.GetProviderSession(context.Background(), parentID)
		return err == nil && found && st.Status == RunStatusCompleted
	})

	st, found, err := store.GetProviderSession(context.Background(), parentID)
	if err != nil || !found {
		t.Fatalf("GetProviderSession(%q): found=%v err=%v", parentID, found, err)
	}
	if st.LoopState.Status != "done" {
		t.Fatalf("persisted LoopState.Status = %q, want done — the latest record (what resume reads) lost the terminal loop state", st.LoopState.Status)
	}
	if !st.AutoOrchestrate {
		t.Errorf("persisted AutoOrchestrate = false, want true (unrelated fields must survive the fix untouched)")
	}
}

// TestE2EReviewLoopMultiRoundChangesThenApprovedCompletes drives a REAL two-round
// review loop end to end through the built-in flow topology: round 1 synthesis
// requests changes (loop back to the coder), round 2 synthesis approves (flow
// done). This is the multi-round completion path that had NO coverage before
// BUG-234 — TestE2EReviewLoopApprovedPath drives only one round, and
// TestE2EReviewLoopChangesRequestedFeedbackReachesCoderPrompt stops after the
// first coder re-entry without ever driving the second synthesis. The live-
// testing regression (synthesis stuck RUNNING, a looped run never completing)
// surfaces here as a waitLoop timeout.
func TestE2EReviewLoopMultiRoundChangesThenApprovedCompletes(t *testing.T) {
	var synthMu sync.Mutex
	synthCalls := 0

	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				if autoAnswerPreflightContractPlanTurn(req, b) {
					return nil
				}
				// The hub synthesis turn is the only one whose prompt carries the
				// auto-reinvoke text ("Agent results ready"); coder/reviewer turns do not.
				if strings.Contains(req.Prompt, "Agent results ready") {
					synthMu.Lock()
					synthCalls++
					n := synthCalls
					synthMu.Unlock()
					// Round 1 → changes_requested (loop back to coder); round 2 → approved (done).
					in := FlowControlInput{Status: "done", Summary: "all reviewers approved"}
					if n == 1 {
						in = FlowControlInput{Status: "continue", Summary: "address the failing test from round 1"}
					}
					if _, err := b.SubmitFlowControl(in); err != nil {
						return err
					}
					b.Emit(ProviderEvent{Type: EventTurnCompleted, ProviderTurnID: req.ProviderTurnID, FinalMessage: "synthesis turn " + in.Status})
					return nil
				}
				// Coder and reviewer turns simply complete.
				b.Emit(ProviderEvent{Type: EventTurnCompleted, ProviderTurnID: req.ProviderTurnID, FinalMessage: "ok"})
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

	// Drive the real built-in review-loop flow: startResolvedFlow sets the flow
	// topology + autoOrchestrate + explicit mode + flowEngineDriven and spawns the
	// coder entry node. Auto-advance then carries coder → reviewer cohort →
	// synthesis → (continue) coder → reviewer cohort → synthesis → done, with no
	// further manual driving.
	svc.startResolvedFlow(context.Background(), parentID, "flowpilot-core-flow-pack/review-loop", "fix the bug where 1+1 != 2")

	waitLoop(t, "loop.Status==done after a changes→approved multi-round", 8*time.Second, func() bool {
		return svc.agentOrchestrator.loopStateFor(parentID).Status == "done"
	})

	st := svc.agentOrchestrator.loopStateFor(parentID)
	synthMu.Lock()
	n := synthCalls
	synthMu.Unlock()
	if st.Status != "done" {
		t.Fatalf("loop.Status = %q, want done (synthCalls=%d, round=%d) — multi-round flow failed to complete", st.Status, n, st.Round)
	}
	if n < 2 {
		t.Errorf("synthesis turn fired %d time(s), want >= 2 (round 1 changes_requested + round 2 approved)", n)
	}
}

// TestE2EReviewLoopMultiRoundSlowSynthesisStillCompletes forces the deferred-
// reinvoke window (BUG-234 probe): the round-1 synthesis turn calls
// submit_review_outcome(changes_requested) — which schedules the coder re-entry —
// and then STALLS before emitting TurnCompleted. With a fast fake coder/reviewer,
// the round-2 cohort joins while the round-1 synthesis turn is still in flight, so
// maybeAutoReinvokeHubWithNote must DEFER the round-2 synthesis (pendingHubReinvoke)
// and the turn-completion retry must fire it. If that retry path is broken, the
// hub node stays RUNNING and the flow never completes (the Ảnh 5 hang shape).
func TestE2EReviewLoopMultiRoundSlowSynthesisStillCompletes(t *testing.T) {
	var synthMu sync.Mutex
	synthCalls := 0

	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				if autoAnswerPreflightContractPlanTurn(req, b) {
					return nil
				}
				if strings.Contains(req.Prompt, "Agent results ready") {
					synthMu.Lock()
					synthCalls++
					n := synthCalls
					synthMu.Unlock()
					in := FlowControlInput{Status: "done", Summary: "all reviewers approved"}
					if n == 1 {
						in = FlowControlInput{Status: "continue", Summary: "address the failing test from round 1"}
					}
					if _, err := b.SubmitFlowControl(in); err != nil {
						return err
					}
					// Round 1: hold the turn open past the round-2 cohort join to force
					// the deferred-reinvoke path. The coder re-entry + round-2 reviewers
					// (instant fakes) complete inside this window.
					if n == 1 {
						time.Sleep(150 * time.Millisecond)
					}
					b.Emit(ProviderEvent{Type: EventTurnCompleted, ProviderTurnID: req.ProviderTurnID, FinalMessage: "synthesis turn " + in.Status})
					return nil
				}
				b.Emit(ProviderEvent{Type: EventTurnCompleted, ProviderTurnID: req.ProviderTurnID, FinalMessage: "ok"})
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
	svc.startResolvedFlow(context.Background(), parentID, "flowpilot-core-flow-pack/review-loop", "fix the bug where 1+1 != 2")

	waitLoop(t, "loop.Status==done despite a slow round-1 synthesis", 8*time.Second, func() bool {
		return svc.agentOrchestrator.loopStateFor(parentID).Status == "done"
	})

	st := svc.agentOrchestrator.loopStateFor(parentID)
	synthMu.Lock()
	n := synthCalls
	synthMu.Unlock()
	if st.Status != "done" {
		t.Fatalf("loop.Status = %q, want done (synthCalls=%d) — deferred round-2 synthesis was never retried", st.Status, n)
	}
	if n < 2 {
		t.Errorf("synthesis turn fired %d time(s), want >= 2", n)
	}
}

// TestE2EReviewLoopBlockedByProseOnlyRoundStopsAdvancing is the BUG-234 (#4)
// regression: in a looped run, a round-2 synthesis that COMPLETES WITHOUT calling
// submit_review_outcome (prose-only — the realistic Haiku failure) trips the
// CA-226 fallback, which escalates the loop to blocked and settles the hub node
// to WAITING_USER_APPROVAL. The flow must then STOP: no more reviewer spawns, and
// the hub node must STAY WAITING_USER_APPROVAL (not flip back to RUNNING). Before
// the fix, the auto-advance paths had no loop-status guard, so late/continuing
// completions re-spawned reviewers and flipped the hub node back to RUNNING
// forever — the "synthesis step spins, run never completes" hang.
func TestE2EReviewLoopBlockedByProseOnlyRoundStopsAdvancing(t *testing.T) {
	var mu sync.Mutex
	synthCalls := 0
	reviewerSpawns := 0

	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				if autoAnswerPreflightContractPlanTurn(req, b) {
					return nil
				}
				if strings.Contains(req.Prompt, "Agent results ready") {
					mu.Lock()
					synthCalls++
					n := synthCalls
					mu.Unlock()
					if n == 1 {
						if _, err := b.SubmitFlowControl(FlowControlInput{Status: "continue", Summary: "address round 1"}); err != nil {
							return err
						}
						b.Emit(ProviderEvent{Type: EventTurnCompleted, ProviderTurnID: req.ProviderTurnID, FinalMessage: "changes requested"})
						return nil
					}
					// Round 2+: complete WITHOUT calling submit_review_outcome (prose only).
					b.Emit(ProviderEvent{Type: EventTurnCompleted, ProviderTurnID: req.ProviderTurnID, FinalMessage: "I am not sure, looks plausible."})
					return nil
				}
				if strings.Contains(req.Prompt, "Review this result") {
					mu.Lock()
					reviewerSpawns++
					mu.Unlock()
				}
				b.Emit(ProviderEvent{Type: EventTurnCompleted, ProviderTurnID: req.ProviderTurnID, FinalMessage: "ok"})
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
	svc.startResolvedFlow(context.Background(), parentID, "flowpilot-core-flow-pack/review-loop", "fix the bug where 1+1 != 2")
	// Mirror what handleStartTurn does for a real flow-engine launch so the
	// CA-226 fallback (gated on flowEngineDriven) and the step timeline engage.
	svc.markFlowEngineDriven(parentID)
	svc.mu.Lock()
	nodes := append([]agentpack.FlowNode(nil), svc.runs[parentID].activeFlowNodes...)
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parentID, nodes)

	// Wait for the loop to reach blocked (the round-2 prose-only synthesis escalates).
	waitLoop(t, "loop.Status==blocked after a prose-only round-2 synthesis", 8*time.Second, func() bool {
		return svc.agentOrchestrator.loopStateFor(parentID).Status == "blocked"
	})

	// The pre-fix bug was an UNBOUNDED runaway: after the escalate, reviewers kept
	// re-spawning and the hub node kept flipping RUNNING → WAITING → RUNNING every
	// round, forever — the synthesis step spun and the run never settled. The fix
	// gates every auto-advance/spawn and the cohort-join hub-RUNNING write on the
	// loop still being active. A single reviewer turn already in flight at the
	// exact block instant may still land (a benign, one-shot boundary race that
	// cascades no further because the reinvoke and hub-RUNNING writes are gated),
	// so the guarantee we assert is: the count SETTLES (stops growing once any
	// in-flight turn lands), the loop STAYS blocked, and — the actual user-facing
	// symptom — the hub node STAYS WAITING_USER_APPROVAL rather than flapping back
	// to RUNNING. Sample after a generous drain so any in-flight turn has landed.
	time.Sleep(800 * time.Millisecond)
	mu.Lock()
	countA := reviewerSpawns
	mu.Unlock()
	time.Sleep(800 * time.Millisecond)
	mu.Lock()
	countB := reviewerSpawns
	mu.Unlock()

	st := svc.agentOrchestrator.loopStateFor(parentID)
	if st.Status != "blocked" {
		t.Fatalf("loop.Status = %q, want blocked (must stay settled, not resume)", st.Status)
	}
	hubStatus := flowStepStatus(t, svc, parentID, hubInlineNodeID(nodes))
	if hubStatus != StepStatusWaitingUserApr {
		t.Errorf("hub node status = %v, want WAITING_USER_APPROVAL (must not flap back to RUNNING after block)", hubStatus)
	}
	if countB != countA {
		t.Errorf("reviewer turns still firing after the loop blocked and drained: %d → %d across 800ms (runaway auto-advance not gated)", countA, countB)
	}
}

// TestE2EReviewLoopChangesRequestedFeedbackReachesCoderPrompt verifies that
// applyFlowControl("continue", summary=...) triggers a coder re-entry whose
// prompt contains the supplied feedback text. Drives the full path:
//
//	applyFlowControl → maybeReinvokeCoderForContinue → scheduleChildTurn → runTurn adapter
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

// TestE2EReviewLoopCoderReentryIncrementsActivationSeqAndEmitsSpawnEvent is
// the regression test for BUG-242 (Bug 1): a review-loop round-2+ coder
// re-entry goes through applyFlowControl("continue") ->
// maybeReinvokeCoderForContinue, which used to carry its own older inline
// reinvoke logic that never incremented activationSeq or emitted
// EventAgentSpawnedByUser — only the separate reinvokeExistingFlowChild
// (forward-edge reuse path) had those BUG-Rnd2 fixes. Without activationSeq
// incrementing, the desktop's monotonic terminal-status guard
// (mergeAgentRunsById) discarded the completed->running transition as a stale
// snapshot, so the coder stayed miscategorized in "Recently closed" with no
// new main-chat card even though the backend had genuinely restarted it.
// Both call sites now delegate to the single reinvokeMatchingFlowChild.
func TestE2EReviewLoopCoderReentryIncrementsActivationSeqAndEmitsSpawnEvent(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})

	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	ph, _ := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	parentID := ph.RunID
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	spawnResult, err := svc.spawnChildRun(context.Background(), parentID, SpawnAgentInput{
		Agent: "coder", Prompt: "implement auth middleware", Wait: true,
	})
	if err != nil {
		t.Fatalf("spawnChildRun(coder): %v", err)
	}
	coderRunID := spawnResult.RunID

	summaryBefore, ok := svc.agentOrchestrator.currentSummary(parentID, coderRunID)
	if !ok {
		t.Fatalf("no summary found for coder run %q before continue", coderRunID)
	}
	if summaryBefore.Status != RunStatusCompleted {
		t.Fatalf("coder status before continue = %q, want completed", summaryBefore.Status)
	}

	if _, err := svc.applyFlowControl(parentID, FlowControlInput{
		Status:  "continue",
		Summary: "fix the null pointer dereference",
	}); err != nil {
		t.Fatalf("applyFlowControl(continue): %v", err)
	}

	waitLoop(t, "coder summary reflects reinvoke", 3*time.Second, func() bool {
		summary, ok := svc.agentOrchestrator.currentSummary(parentID, coderRunID)
		return ok && summary.ActivationSeq > summaryBefore.ActivationSeq
	})

	summaryAfter, _ := svc.agentOrchestrator.currentSummary(parentID, coderRunID)
	if summaryAfter.ActivationSeq <= summaryBefore.ActivationSeq {
		t.Fatalf("activationSeq = %d after continue, want > %d (before) so the desktop recognizes a genuine reinvoke, not a stale snapshot",
			summaryAfter.ActivationSeq, summaryBefore.ActivationSeq)
	}

	svc.mu.Lock()
	rs := svc.runs[parentID]
	var sawSpawnEvent bool
	for _, ev := range rs.events {
		if ev.Type == EventAgentSpawnedByUser && ev.ChildRunID == coderRunID {
			sawSpawnEvent = true
			break
		}
	}
	svc.mu.Unlock()
	if !sawSpawnEvent {
		t.Error("expected an EventAgentSpawnedByUser for the reinvoked coder run on the parent's timeline, so the main chat renders a new agent card for this turn")
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
	// Cap path may leave hub WAITING (not RUNNING) — no stuck RUNNING nodes.
	assertNoStepStuckRunning(t, svc, runID)
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

	if !strings.Contains(capturedPrompt, "## Context") {
		t.Errorf("coding prompt missing '## Context'; prompt[:300] = %.300q", capturedPrompt)
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
		Type:               EventFlowContextPackage,
		WorkflowRunID:      "run-sidecar-1",
		WorkflowStepRunID:  "step-plan",
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

// TestE2EReviewLoopSynthesisFallbackEscalates verifies that if the hub synthesis turn
// completes without calling submit_review_outcome, the synthesis step is marked FAILED,
// the loop is escalated/blocked, and no second auto-reinvoke loop is created.
func TestE2EReviewLoopSynthesisFallbackEscalates(t *testing.T) {
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
					// Do NOT call b.SubmitFlowControl (submit_review_outcome tool).
					// Answer in prose only.
					b.Emit(ProviderEvent{
						Type:           EventTurnCompleted,
						ProviderTurnID: req.ProviderTurnID,
						FinalMessage:   "Blocked: joined note unavailable",
					})
					return nil
				}
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	store := newFakeWorkflowStore()
	svc = newInteractiveService(reg, newInteractiveCatalog(), store)
	ph, _ := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if ph.RunID == "" {
		t.Fatal("createRun returned empty RunID")
	}
	parentID = ph.RunID

	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.mu.Lock()
	svc.runs[parentID].autoOrchestrate = true
	svc.runs[parentID].flowEngineDriven = true
	svc.runs[parentID].turnCount = 1 // simulate hub's first turn already run
	svc.runs[parentID].activeFlowNodes = []agentpack.FlowNode{
		{ID: "step-synth", Behavior: "hub.inline"},
	}
	svc.mu.Unlock()

	// Seed steps into workflow store
	svc.reseedFlowStepRuntime(parentID, svc.runs[parentID].activeFlowNodes)

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

	// Wait for the loop state to become "blocked".
	waitLoop(t, "loop.Status==blocked", 3*time.Second, func() bool {
		return svc.agentOrchestrator.loopStateFor(parentID).Status == "blocked"
	})

	st := svc.agentOrchestrator.loopStateFor(parentID)
	if st.Status != "blocked" {
		t.Errorf("loop.Status = %q, want blocked", st.Status)
	}
	// BUG-233: the awaiting-user card renders GateReason verbatim, so the
	// fallback must surface the reviewers' actual findings (their joined cohort
	// note) instead of the internal "completed without calling
	// submit_review_outcome" diagnostic sentence.
	if strings.Contains(st.GateReason, "completed without calling submit_review_outcome") {
		t.Errorf("GateReason still contains the internal diagnostic sentence: %q", st.GateReason)
	}
	for _, want := range []string{"reviewer-correctness", "reviewer-security"} {
		if !strings.Contains(st.GateReason, want) {
			t.Errorf("expected GateReason to include reviewer findings (%q), got: %q", want, st.GateReason)
		}
	}

	// BUG-233: the hub node must settle to WAITING_USER_APPROVAL, not FAILED —
	// this fallback is a non-terminal awaiting-user pause (same contract BUG-231
	// established for the escalate/cap-reached paths), and FAILED reads as a
	// terminal error.
	steps, err := store.LoadRunSteps(context.Background(), parentID)
	if err != nil {
		t.Fatalf("LoadRunSteps: %v", err)
	}
	foundSynth := false
	for _, step := range steps {
		if step.ID == "step-synth" {
			foundSynth = true
			if step.Status != StepStatusWaitingUserApr {
				t.Errorf("expected synthesis step status to be WAITING_USER_APPROVAL, got: %v", step.Status)
			}
		}
	}
	if !foundSynth {
		t.Error("synthesis step not found in workflow store")
	}

	// Assert no second auto-reinvoke loop is created (hubCalls should remain 1).
	time.Sleep(50 * time.Millisecond)
	if hubCalls != 1 {
		t.Errorf("expected exactly 1 hub call, got %d", hubCalls)
	}
}
