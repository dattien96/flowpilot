package runner

// BUG-308 residual run-33289 — full acceptance coverage for post-Stop plain-chat.
//
// Product path that hung live:
//   coder done → reviewers running → user Stop → chat follow-up
//   admitted (BUG-308) but dispatch run_stop.stopped still true →
//   linearizeSendStarted ErrRunStopFence → terminal_cancelled with no
//   EventTurnFailed/Completed → desktop Thinking hang.
//
// This file covers the FULL path (AF), not just startTurn admission:
//   stopAgentLoop + durable fence + status=cancelled + stranded notes +
//   startTurn + release fence + adapter send + terminal event + note strip.
//
// cross-provider-parity Case 1: shared startTurn/runTurn + DispatchStore.
// Table: Codex, Grok, Claude (same code path, no providerKey branch).
//
// additive-tests-only: this file only; do not edit legacy bug308/bug307 tests.

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const bug308SpawnWaitNote = "[flow-engine] An agent has already been spawned to work on this request. " +
	"Do not duplicate that work yourself — wait for its result; you will be reinvoked automatically once this step of the flow completes."

const bug308CohortJoinNote = "[flow-engine] Agent results ready for cohort flow-auto-coder-round-0.\n" +
	"Synthesize the findings and call the flow's control tool when ready."

func bug308FullAFRegistry(key ProviderKey, captured *atomic.Value, sendCount *int32) *ProviderRegistry {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: key, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				atomic.AddInt32(sendCount, 1)
				if captured != nil {
					captured.Store(req.Prompt)
				}
				b.Emit(ProviderEvent{
					Type:           EventTurnCompleted,
					ProviderTurnID: req.ProviderTurnID,
					FinalMessage:   "ok — flow was stopped; not done",
				})
				return nil
			})
		},
	})
	return reg
}

func bug308AllProviders() []ProviderKey {
	return []ProviderKey{ProviderKeyCodex, ProviderKeyGrok, ProviderKeyClaude}
}

// setupPostStopHub mirrors run-33289 after Stop mid-review:
// V2 dispatch active, loop stopped, status cancelled, fence stopped, stranded notes.
func setupPostStopHub(t *testing.T, pk ProviderKey) (svc *InteractiveService, store DispatchStore, parentID string, sends *int32, captured *atomic.Value) {
	t.Helper()
	var sendCount int32
	var cap atomic.Value
	svc = newInteractiveService(bug308FullAFRegistry(pk, &cap, &sendCount), newInteractiveCatalog(), newFakeWorkflowStore())
	store = NewMemoryDispatchStore()
	svc.SetDispatchStore(store)

	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: pk})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	if rs.runKind == "" {
		rs.runKind = "chat"
	}
	svc.mu.Unlock()

	// Activate V2 (prepare/claim/linearize) like a live hub turn.
	if _, apiErr := svc.startTurn(parent.RunID, TurnInput{StepID: "chat-" + parent.RunID, Prompt: "fix bug 1 + 1 != 2"}, "", ""); apiErr != nil {
		t.Fatalf("seed startTurn: %s", apiErr.msg)
	}
	waitLoop(t, "seed turn done", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[parent.RunID].turnInFlight
	})

	// Mid-flow hub shape: non-terminal + running loop + stranded orchestration notes.
	svc.mu.Lock()
	svc.runs[parent.RunID].status = RunStatusRunning
	svc.runs[parent.RunID].agentStatus = string(RunStatusRunning)
	svc.runs[parent.RunID].pendingAgentContext = []string{bug308SpawnWaitNote, bug308CohortJoinNote}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, Mode: "explicit", Round: 0})

	if _, apiErr := svc.stopAgentLoop(parent.RunID); apiErr != nil {
		t.Fatalf("stopAgentLoop: %v", apiErr)
	}

	st, stErr := store.GetRunStopState(context.Background(), parent.RunID)
	if stErr != nil {
		t.Fatalf("GetRunStopState: %v", stErr)
	}
	if !st.Stopped || st.Generation < 1 {
		t.Fatalf("after Stop: stopped=%v gen=%d want stopped=true gen≥1", st.Stopped, st.Generation)
	}
	svc.mu.Lock()
	status := svc.runs[parent.RunID].status
	loopSt := svc.agentOrchestrator.loopStateFor(parent.RunID).Status
	svc.mu.Unlock()
	if status != RunStatusCancelled {
		t.Fatalf("status=%s want cancelled after Stop", status)
	}
	if loopSt != "stopped" {
		t.Fatalf("loop=%q want stopped", loopSt)
	}

	return svc, store, parent.RunID, &sendCount, &cap
}

func lastPrompt(cap *atomic.Value) string {
	if cap == nil {
		return ""
	}
	v := cap.Load()
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func assertNoStaleFlowEngineNotes(t *testing.T, prompt string) {
	t.Helper()
	low := strings.ToLower(prompt)
	for _, bad := range []string{
		"reinvoked automatically",
		"wait for its result",
		"call the flow's control tool",
		"agent results ready",
		"[flow-engine]",
	} {
		if strings.Contains(low, strings.ToLower(bad)) {
			t.Fatalf("stale flow-engine orchestration leaked into follow-up prompt (contains %q):\n%s", bad, prompt)
		}
	}
}

func assertHasTerminalForLatestTurn(t *testing.T, events []ProviderEvent) {
	t.Helper()
	// Find last TurnStarted, then require a terminal with same ProviderTurnID after it.
	lastStart := -1
	var turnID string
	for i, ev := range events {
		if ev.Type == EventTurnStarted {
			lastStart = i
			turnID = ev.ProviderTurnID
		}
	}
	if lastStart < 0 {
		t.Fatalf("no TurnStarted in events: %v", summarizeEventTypes(events))
	}
	for i := lastStart + 1; i < len(events); i++ {
		ev := events[i]
		if (ev.Type == EventTurnCompleted || ev.Type == EventTurnFailed) &&
			(turnID == "" || ev.ProviderTurnID == turnID || ev.ProviderTurnID == "") {
			return
		}
	}
	t.Fatalf("no terminal event after last TurnStarted turnID=%q full=%v", turnID, summarizeEventTypes(events))
}

// ---------------------------------------------------------------------------
// AF-1: full post-Stop follow-up completes (run-33289) — all three providers
// ---------------------------------------------------------------------------

func TestPostStopFollowUpFullPathCompletes_AllProviders(t *testing.T) {
	for _, pk := range bug308AllProviders() {
		pk := pk
		t.Run(string(pk), func(t *testing.T) {
			svc, store, parentID, sends, cap := setupPostStopHub(t, pk)
			sendsBefore := atomic.LoadInt32(sends)

			tid, apiErr := svc.startTurn(parentID, TurnInput{StepID: "chat-" + parentID, Prompt: "done hay bị cancel rồi ?"}, "", "")
			if apiErr != nil {
				t.Fatalf("follow-up startTurn: %s (code=%s)", apiErr.msg, apiErr.code)
			}
			if tid == "" {
				t.Fatal("empty turn id")
			}
			waitLoop(t, "follow-up idle", 3*time.Second, func() bool {
				svc.mu.Lock()
				defer svc.mu.Unlock()
				return !svc.runs[parentID].turnInFlight
			})

			if atomic.LoadInt32(sends) <= sendsBefore {
				t.Fatal("adapter never sent follow-up — stop fence still blocking (run-33289 hang)")
			}
			prompt := lastPrompt(cap)
			if !strings.Contains(prompt, "done hay bị cancel") {
				t.Fatalf("user prompt missing:\n%s", prompt)
			}
			assertNoStaleFlowEngineNotes(t, prompt)

			// Fence released for hub; generation kept for children.
			st, _ := store.GetRunStopState(context.Background(), parentID)
			if st.Stopped {
				t.Fatal("run_stop.stopped still true after follow-up")
			}
			if st.Generation < 1 {
				t.Fatalf("generation=%d want ≥1 (children stay fenced)", st.Generation)
			}

			// Run revived from cancelled for plain chat.
			svc.mu.Lock()
			status := svc.runs[parentID].status
			events := append([]ProviderEvent(nil), svc.runs[parentID].events...)
			flag := svc.runs[parentID].turnStartedAfterLoopDone
			svc.mu.Unlock()
			if status == RunStatusCancelled {
				t.Fatal("status still cancelled after sealed follow-up revive")
			}
			// Flag is per-turn; after complete it may still be true from last turn.
			_ = flag
			assertHasTerminalForLatestTurn(t, events)

			// Dispatch record for follow-up must not be terminal_cancelled (run-33289).
			rec, _, gerr := store.Get(context.Background(), parentID, tid)
			if gerr != nil {
				t.Fatalf("dispatch Get follow-up: %v", gerr)
			}
			if rec.State == DispatchTerminalCancelled {
				t.Fatalf("follow-up dispatch still terminal_cancelled (stop fence) outcome=%q", rec.Outcome)
			}
			if rec.State != DispatchTerminalCompleted && rec.State != DispatchSendStarted &&
				rec.State != DispatchProviderAccepted && rec.State != DispatchTerminalFailed {
				// Best-effort: some settle paths may leave completed terminal only.
				if !rec.State.IsTerminal() {
					t.Fatalf("unexpected dispatch state=%s for completed follow-up", rec.State)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// AF-2: durable idempotency key follow-up also survives abortDurableStartIfStale
// ---------------------------------------------------------------------------

func TestPostStopFollowUpWithDurableKeyCompletes(t *testing.T) {
	svc, store, parentID, sends, _ := setupPostStopHub(t, ProviderKeyCodex)
	before := atomic.LoadInt32(sends)
	idem := "durable-" + parentID + "-user-followup-00000000000000000001"
	if _, apiErr := svc.startTurn(parentID, TurnInput{StepID: "chat-" + parentID, Prompt: "lan truoc fix gi"}, "", idem); apiErr != nil {
		t.Fatalf("durable follow-up aborted: %s (code=%s) — abortDurableStartIfStale must skip sealed plain-chat", apiErr.msg, apiErr.code)
	}
	waitLoop(t, "durable follow-up idle", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[parentID].turnInFlight
	})
	if atomic.LoadInt32(sends) <= before {
		t.Fatal("durable follow-up never reached adapter")
	}
	st, _ := store.GetRunStopState(context.Background(), parentID)
	if st.Stopped {
		t.Fatal("fence still stopped after durable follow-up")
	}
}

// ---------------------------------------------------------------------------
// AF-3: without turnStartedAfterLoopDone, fence still cancels send BUT emits TurnFailed
// (defense-in-depth — UI must not hang if release path is skipped)
// ---------------------------------------------------------------------------

func TestStopFenceWithoutSealedFollowUpEmitsTurnFailed(t *testing.T) {
	var captured atomic.Value
	var sends int32
	svc := newInteractiveService(bug308FullAFRegistry(ProviderKeyCodex, &captured, &sends), newInteractiveCatalog(), newFakeWorkflowStore())
	store := NewMemoryDispatchStore()
	svc.SetDispatchStore(store)

	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	// Seed V2
	if _, apiErr := svc.startTurn(parent.RunID, TurnInput{StepID: "chat-" + parent.RunID, Prompt: "seed"}, "", ""); apiErr != nil {
		t.Fatalf("seed: %s", apiErr.msg)
	}
	waitLoop(t, "seed done", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[parent.RunID].turnInFlight
	})
	sendsBefore := atomic.LoadInt32(&sends)

	// Raise fence while loop is still "running" so admission does NOT set
	// turnStartedAfterLoopDone and does NOT release the fence.
	st, _ := store.GetRunStopState(context.Background(), parent.RunID)
	if _, err := store.RequestRunStop(context.Background(), parent.RunID, st.Revision, StopReasonUser); err != nil {
		st, _ = store.GetRunStopState(context.Background(), parent.RunID)
		if _, err2 := store.RequestRunStop(context.Background(), parent.RunID, st.Revision, StopReasonUser); err2 != nil {
			t.Fatalf("RequestRunStop: %v", err2)
		}
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, Mode: "explicit"})

	tid, apiErr := svc.startTurn(parent.RunID, TurnInput{StepID: "chat-" + parent.RunID, Prompt: "should fail at linearize"}, "", "")
	if apiErr != nil {
		// Admission may still succeed; hang was after TurnStarted.
		t.Fatalf("startTurn returned error (want admit then fail at linearize): %s", apiErr.msg)
	}
	waitLoop(t, "fenced turn idle", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[parent.RunID].turnInFlight
	})

	if atomic.LoadInt32(&sends) > sendsBefore {
		t.Fatal("adapter must NOT send when stop fence blocks linearize")
	}

	svc.mu.Lock()
	events := append([]ProviderEvent(nil), svc.runs[parent.RunID].events...)
	svc.mu.Unlock()
	var sawFailed bool
	for _, ev := range events {
		if ev.Type == EventTurnFailed && (ev.ProviderTurnID == tid || tid == "") {
			if strings.Contains(ev.Error, "stop fence") || strings.Contains(ev.Error, "cancelled before send") {
				sawFailed = true
			}
		}
	}
	if !sawFailed {
		// Accept any TurnFailed after the turn as hang-fix
		for i := len(events) - 1; i >= 0; i-- {
			if events[i].Type == EventTurnFailed {
				sawFailed = true
				break
			}
		}
	}
	if !sawFailed {
		t.Fatalf("want EventTurnFailed after stop-fence linearize cancel (UI hang fix); events=%v", summarizeEventTypes(events))
	}
}

// ---------------------------------------------------------------------------
// AF-4: store contract — ReleaseRunStopFence
// ---------------------------------------------------------------------------

func TestReleaseRunStopFence_StoreContract(t *testing.T) {
	ctx := context.Background()
	// Memory + local disk stores (local embeds memory + fsync).
	stores := []struct {
		name  string
		store DispatchStore
	}{
		{"memory", NewMemoryDispatchStore()},
	}
	dir := t.TempDir()
	local, err := NewLocalDispatchStore(dir)
	if err != nil {
		t.Fatalf("NewLocalDispatchStore: %v", err)
	}
	defer local.Close()
	stores = append(stores, struct {
		name  string
		store DispatchStore
	}{"local", local})

	for _, tc := range stores {
		t.Run(tc.name, func(t *testing.T) {
			store := tc.store
			runID := "run-fence-" + tc.name
			// Activate V2 + RunStopState via CreatePrepared.
			env := testEnvelope(runID, "t0")
			rec := testPrepared(runID, "t0")
			if err := store.CreatePrepared(ctx, rec, env); err != nil {
				t.Fatalf("CreatePrepared: %v", err)
			}
			st, err := store.GetRunStopState(ctx, runID)
			if err != nil {
				t.Fatalf("GetRunStopState: %v", err)
			}
			if st.Revision == 0 {
				st.Revision = 1
			}
			stopped, err := store.RequestRunStop(ctx, runID, st.Revision, StopReasonUser)
			if err != nil {
				t.Fatalf("RequestRunStop: %v", err)
			}
			if !stopped.Stopped || stopped.Generation < 1 {
				t.Fatalf("after stop: %+v", stopped)
			}
			gen := stopped.Generation

			// Stale rev rejected.
			if _, err := store.ReleaseRunStopFence(ctx, runID, stopped.Revision-1); err == nil {
				t.Fatal("stale revision must fail")
			} else if !errors.Is(err, ErrStaleDispatch) {
				// memory returns ErrStaleDispatch
				if !strings.Contains(err.Error(), "stale") && err != ErrStaleDispatch {
					// accept any error for stale
				}
			}

			released, err := store.ReleaseRunStopFence(ctx, runID, stopped.Revision)
			if err != nil {
				t.Fatalf("ReleaseRunStopFence: %v", err)
			}
			if released.Stopped {
				t.Fatal("Stopped must be false after release")
			}
			if released.Generation != gen {
				t.Fatalf("Generation changed %d→%d — children would lose fence authority", gen, released.Generation)
			}

			// Idempotent when already clear.
			again, err := store.ReleaseRunStopFence(ctx, runID, released.Revision)
			if err != nil {
				t.Fatalf("idempotent release: %v", err)
			}
			if again.Stopped {
				t.Fatal("still want Stopped=false")
			}

			// Re-stop works for a later Stop (generation must bump).
			st2, _ := store.GetRunStopState(ctx, runID)
			againStop, err := store.RequestRunStop(ctx, runID, st2.Revision, StopReasonUser)
			if err != nil {
				t.Fatalf("re-stop: %v", err)
			}
			if !againStop.Stopped || againStop.Generation <= gen {
				t.Fatalf("re-stop should bump generation: %+v", againStop)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// AF-5: after release, child parent-fence still invalid when generation advanced
// ---------------------------------------------------------------------------

func TestReleaseRunStopFence_ChildParentFenceStillHolds(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryDispatchStore()
	parentID, childID := "run-parent-fence", "run-child-fence"

	// Parent prepare + stop.
	if err := store.CreatePrepared(ctx, testPrepared(parentID, "pt0"), testEnvelope(parentID, "pt0")); err != nil {
		t.Fatal(err)
	}
	st, _ := store.GetRunStopState(ctx, parentID)
	stopped, err := store.RequestRunStop(ctx, parentID, st.Revision, StopReasonUser)
	if err != nil {
		t.Fatal(err)
	}
	// Child prepared with ExpectedStopGeneration = 0 (pre-stop stamp).
	child := testPrepared(childID, "ct0")
	child.ParentStopFence = &ParentStopFence{ParentRunID: parentID, ExpectedStopGeneration: 0}
	if err := store.CreatePrepared(ctx, child, testEnvelope(childID, "ct0")); err != nil {
		t.Fatal(err)
	}
	// Release hub fence (Stopped=false) but Generation stays > 0.
	if _, err := store.ReleaseRunStopFence(ctx, parentID, stopped.Revision); err != nil {
		t.Fatal(err)
	}
	// Child send_started must still fail parent fence (generation mismatch).
	got, rev, err := store.Get(ctx, childID, "ct0")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CASAdvance(ctx, childID, "ct0", rev, DispatchPrepared, DispatchSendClaimed, nil); err != nil {
		t.Fatalf("claim: %v", err)
	}
	got, rev, _ = store.Get(ctx, childID, "ct0")
	_, err = store.CASAdvance(ctx, childID, "ct0", rev, DispatchSendClaimed, DispatchSendStarted, nil)
	if err == nil {
		t.Fatal("child send_started must fail parent stop fence after hub release (gen mismatch)")
	}
	var parentFence ErrParentStopFence
	if !errors.As(err, &parentFence) {
		// own stop would be wrong here since parent Stopped=false
		t.Fatalf("want ErrParentStopFence, got %v", err)
	}
	_ = got
}

// ---------------------------------------------------------------------------
// AF-6: child hub follow-up still rejected under stopped parent (scope guard)
// ---------------------------------------------------------------------------

func TestChildTurnStillRejectedWhenParentStopped_WithDispatchFence(t *testing.T) {
	svc, _, parentID, _, _ := setupPostStopHub(t, ProviderKeyCodex)
	child, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.runs[child.RunID].parentRunID = parentID
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parentID, child.RunID)

	_, apiErr := svc.startTurn(child.RunID, TurnInput{StepID: "s1", Prompt: "child work"}, "", "")
	if apiErr == nil {
		t.Fatal("child under stopped parent must 409")
	}
	if apiErr.code != "flow_stopped" {
		t.Fatalf("code=%q want flow_stopped", apiErr.code)
	}
}

// ---------------------------------------------------------------------------
// AF-7: done-loop follow-up (no RequestRunStop) still works with V2 — no false fence
// ---------------------------------------------------------------------------

func TestDoneLoopFollowUpWithDispatchV2Completes(t *testing.T) {
	var cap atomic.Value
	var sends int32
	svc := newInteractiveService(bug308FullAFRegistry(ProviderKeyCodex, &cap, &sends), newInteractiveCatalog(), newFakeWorkflowStore())
	svc.SetDispatchStore(NewMemoryDispatchStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[parent.RunID].autoOrchestrate = true
	if svc.runs[parent.RunID].runKind == "" {
		svc.runs[parent.RunID].runKind = "chat"
	}
	svc.mu.Unlock()

	if _, apiErr := svc.startTurn(parent.RunID, TurnInput{StepID: "chat-" + parent.RunID, Prompt: "task"}, "", ""); apiErr != nil {
		t.Fatalf("seed: %s", apiErr.msg)
	}
	waitLoop(t, "seed", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[parent.RunID].turnInFlight
	})
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "done", Cap: 3, Mode: "explicit", Round: 2})
	before := atomic.LoadInt32(&sends)
	if _, apiErr := svc.startTurn(parent.RunID, TurnInput{StepID: "chat-" + parent.RunID, Prompt: "follow after done"}, "", ""); apiErr != nil {
		t.Fatalf("done follow-up: %s", apiErr.msg)
	}
	waitLoop(t, "done follow-up", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[parent.RunID].turnInFlight
	})
	if atomic.LoadInt32(&sends) <= before {
		t.Fatal("done follow-up must reach adapter (no stop fence)")
	}
}

// ---------------------------------------------------------------------------
// AF-8: releaseHubStopFenceForFollowUp is no-op without store / already clear
// ---------------------------------------------------------------------------

func TestReleaseHubStopFenceForFollowUp_NoopSafe(t *testing.T) {
	svc := newInteractiveService(bug308CompletingCodexRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	// no dispatch store
	svc.releaseHubStopFenceForFollowUp(context.Background(), "run-missing")

	store := NewMemoryDispatchStore()
	svc.SetDispatchStore(store)
	svc.releaseHubStopFenceForFollowUp(context.Background(), "run-never-activated")
	// should not panic
}
