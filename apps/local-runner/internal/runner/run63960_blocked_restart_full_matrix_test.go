package runner

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// Full R3 matrix for run-63960 blocked-after-restart (safe-fix-contract).
// Additive only. Provider-agnostic path; matrix still exercises all 3 providers.
//
// Axes:
//   - providers: codex / claude / grok
//   - blockReason: cap / escalate / member_stalled
//   - disk: crash settle+reprompt | settle-only | reprompt-only | clean blocked
//   - lifecycle: freeform 409, graph card data, Continue, Stop, second restart
//   - child startTurn under blocked parent
//   - real park (applyFlowControl) then restart

type blockedDiskShape string

const (
	shapeCrashSettleReprompt blockedDiskShape = "crash_settle_reprompt"
	shapeSettleOnly          blockedDiskShape = "settle_only"
	shapeRepromptOnly        blockedDiskShape = "reprompt_only"
	shapeCleanBlocked        blockedDiskShape = "clean_blocked"
)

func allProviders63960() []ProviderKey {
	return []ProviderKey{ProviderKeyCodex, ProviderKeyClaude, ProviderKeyGrok}
}

func allBlockReasons63960() []string {
	return []string{"cap", "escalate", "member_stalled"}
}

func allDiskShapes63960() []blockedDiskShape {
	return []blockedDiskShape{
		shapeCrashSettleReprompt,
		shapeSettleOnly,
		shapeRepromptOnly,
		shapeCleanBlocked,
	}
}

type blockedRestartFixture struct {
	hubID string
	dir   string
	cwd   string
	store *localFileSessionStore
	state ProviderSessionState
}

func seedBlockedRestartFixture(t *testing.T, pk ProviderKey, blockReason string, shape blockedDiskShape) blockedRestartFixture {
	t.Helper()
	root := t.TempDir()
	cwd := filepath.Join(root, "workspace")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	dir := filepath.Join(root, ".flowpilot", "chats")
	store, err := NewLocalFileSessionStore(dir)
	if err != nil {
		t.Fatalf("NewLocalFileSessionStore: %v", err)
	}
	hubID := "run-63960-m-" + string(pk) + "-" + blockReason + "-" + string(shape)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	st := ProviderSessionState{
		RunID:             hubID,
		ProjectID:         "proj-63960-matrix",
		ProviderKey:       pk,
		ProviderSessionID: "sess-" + hubID,
		WorkingDirectory:  cwd,
		Status:            RunStatusRunning,
		RunKind:           "chat",
		AutoOrchestrate:   true,
		ActiveFlowNodes:   []agentpack.FlowNode{{ID: "coder"}, {ID: "synthesis"}},
		StartedAt:         now,
		UpdatedAt:         now,
		LoopState: AgentLoopState{
			Status:      "blocked",
			BlockReason: blockReason,
			GateReason:  "matrix gate: " + blockReason,
			Round:       3,
			Cap:         3,
			RoundCap:    3,
			Mode:        "explicit",
			OpenIssues:  1,
			ActiveNode:  "synthesis",
		},
	}
	switch shape {
	case shapeCrashSettleReprompt:
		st.PendingFlowGateSettle = true
		st.PendingFlowGateFinalMsg = "stale final"
		st.PendingFlowGateOccurredAt = now
		st.PendingFlowGateTurnID = "turn-stale"
		st.PendingGateRepromptPrompt = "stale reprompt"
		st.PendingGateRepromptStepID = "synthesis"
		st.PendingGateRepromptGen = 7
	case shapeSettleOnly:
		st.PendingFlowGateSettle = true
		st.PendingFlowGateFinalMsg = "settle only"
		st.PendingFlowGateTurnID = "turn-settle"
	case shapeRepromptOnly:
		st.PendingGateRepromptPrompt = "reprompt only"
		st.PendingGateRepromptStepID = "synthesis"
		st.PendingGateRepromptGen = 5
	case shapeCleanBlocked:
		// nothing auto-intent
	}
	if err := store.UpsertProviderSession(context.Background(), st); err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	return blockedRestartFixture{hubID: hubID, dir: dir, cwd: cwd, store: store, state: st}
}

func reconstructBlockedFixture(t *testing.T, f blockedRestartFixture, pk ProviderKey) (*InteractiveService, *atomic.Int32) {
	t.Helper()
	store2, err := NewLocalFileSessionStore(f.dir)
	if err != nil {
		t.Fatalf("reload store: %v", err)
	}
	var turns atomic.Int32
	reg := newProviderRegistry()
	for _, p := range allProviders63960() {
		p := p
		reg.register(ProviderRegistration{
			Key: p, Status: ProviderStatusAvailable,
			Capabilities: ProviderCapabilities{Streaming: true},
			newAdapter: func() ProviderRuntimeAdapter {
				return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
					if p == pk {
						turns.Add(1)
					}
					b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "adapter"})
					return nil
				})
			},
		})
	}
	svc := newInteractiveService(reg, newInteractiveCatalog(), store2)
	st, ok, getErr := store2.GetProviderSession(context.Background(), f.hubID)
	if getErr != nil || !ok {
		t.Fatalf("GetProviderSession: ok=%v err=%v", ok, getErr)
	}
	if _, apiErr := svc.reconstructRun(st); apiErr != nil {
		t.Fatalf("reconstructRun: %s", apiErr.msg)
	}
	// Let any boot goroutine that slipped through start.
	time.Sleep(30 * time.Millisecond)
	return svc, &turns
}

func assertNoAutoTurnAndBlocked(t *testing.T, svc *InteractiveService, hubID string, blockReason string, turns *atomic.Int32) {
	t.Helper()
	if turns.Load() != 0 {
		t.Fatalf("auto turn started %d times after blocked reconstruct; want 0", turns.Load())
	}
	loop := svc.agentOrchestrator.loopStateFor(hubID)
	if loop.Status != "blocked" || loop.BlockReason != blockReason {
		t.Fatalf("loop=%+v, want blocked/%s", loop, blockReason)
	}
	snap := svc.agentGraphSnapshot(hubID)
	if snap.LoopState.Status != "blocked" || snap.LoopState.BlockReason != blockReason {
		t.Fatalf("agentGraphSnapshot loop=%+v, want blocked/%s (card data)", snap.LoopState, blockReason)
	}
	svc.mu.Lock()
	rs := svc.runs[hubID]
	if rs == nil {
		svc.mu.Unlock()
		t.Fatal("hub missing")
	}
	if rs.pendingFlowGateSettle {
		svc.mu.Unlock()
		t.Fatal("pendingFlowGateSettle must be clear after blocked reconstruct")
	}
	if rs.pendingGateRepromptPrompt != "" {
		svc.mu.Unlock()
		t.Fatalf("reprompt payload still set: %q", rs.pendingGateRepromptPrompt)
	}
	svc.mu.Unlock()
	if _, err := svc.startTurn(hubID, TurnInput{StepID: "chat", Prompt: "freeform after restart"}, "", ""); err == nil || err.code != "flow_awaiting_user" {
		t.Fatalf("hub freeform: %v, want flow_awaiting_user", err)
	}
}

// Disk shape × blockReason × provider: boot never auto-turns; card data intact.
func TestRun63960FullMatrix_ReconstructNoAutoTurn(t *testing.T) {
	for _, pk := range allProviders63960() {
		for _, reason := range allBlockReasons63960() {
			for _, shape := range allDiskShapes63960() {
				pk, reason, shape := pk, reason, shape
				name := string(pk) + "/" + reason + "/" + string(shape)
				t.Run(name, func(t *testing.T) {
					f := seedBlockedRestartFixture(t, pk, reason, shape)
					svc, turns := reconstructBlockedFixture(t, f, pk)
					assertNoAutoTurnAndBlocked(t, svc, f.hubID, reason, turns)
					// resumePendingFlowGate / flush must remain no-op.
					svc.resumePendingFlowGate(f.hubID)
					svc.flushDurableTurnIntents(f.hubID)
					time.Sleep(20 * time.Millisecond)
					if turns.Load() != 0 {
						t.Fatalf("post-flush turns=%d", turns.Load())
					}
				})
			}
		}
	}
}

// Continue after blocked restart works for every blockReason × provider.
func TestRun63960FullMatrix_ContinueAfterRestart(t *testing.T) {
	for _, pk := range allProviders63960() {
		for _, reason := range allBlockReasons63960() {
			pk, reason := pk, reason
			t.Run(string(pk)+"/"+reason, func(t *testing.T) {
				f := seedBlockedRestartFixture(t, pk, reason, shapeCrashSettleReprompt)
				svc, _ := reconstructBlockedFixture(t, f, pk)
				beforeCap := svc.agentOrchestrator.loopStateFor(f.hubID).Cap
				snap, err := svc.resumeFlowWithFeedback(f.hubID, "operator continue")
				if err != nil {
					t.Fatalf("Continue: %v", err)
				}
				if snap.LoopState.Status != "running" {
					t.Fatalf("status=%q want running", snap.LoopState.Status)
				}
				if snap.LoopState.BlockReason != "" {
					t.Fatalf("blockReason=%q want empty", snap.LoopState.BlockReason)
				}
				if reason == "cap" {
					if snap.LoopState.Cap <= beforeCap {
						t.Fatalf("cap block must auto-extend: before=%d after=%d", beforeCap, snap.LoopState.Cap)
					}
				} else if snap.LoopState.Cap != beforeCap {
					t.Fatalf("%s must not change cap: before=%d after=%d", reason, beforeCap, snap.LoopState.Cap)
				}
				// Freeform after Continue must not be flow_awaiting_user.
				_, turnErr := svc.startTurn(f.hubID, TurnInput{StepID: "chat", Prompt: "after continue"}, "", "")
				if turnErr != nil && turnErr.code == "flow_awaiting_user" {
					t.Fatal("still flow_awaiting_user after Continue")
				}
			})
		}
	}
}

// Stop after blocked restart for every blockReason × provider.
func TestRun63960FullMatrix_StopAfterRestart(t *testing.T) {
	for _, pk := range allProviders63960() {
		for _, reason := range allBlockReasons63960() {
			pk, reason := pk, reason
			t.Run(string(pk)+"/"+reason, func(t *testing.T) {
				f := seedBlockedRestartFixture(t, pk, reason, shapeCrashSettleReprompt)
				svc, turns := reconstructBlockedFixture(t, f, pk)
				snap, apiErr := svc.stopAgentLoop(f.hubID)
				if apiErr != nil {
					t.Fatalf("stop: %s", apiErr.msg)
				}
				if snap.LoopState.Status != "stopped" {
					t.Fatalf("status=%q want stopped", snap.LoopState.Status)
				}
				svc.resumePendingFlowGate(f.hubID)
				svc.flushDurableTurnIntents(f.hubID)
				time.Sleep(20 * time.Millisecond)
				if turns.Load() != 0 {
					t.Fatalf("turns after stop=%d", turns.Load())
				}
				_, turnErr := svc.startTurn(f.hubID, TurnInput{StepID: "chat", Prompt: "after stop"}, "", "")
				if turnErr == nil {
					t.Fatal("startTurn after Stop must fail")
				}
				// Stopped loops reject with flow_stopped (not awaiting_user).
				if turnErr.code == "flow_awaiting_user" {
					t.Fatalf("after Stop got flow_awaiting_user; want non-awaiting reject (%s)", turnErr.code)
				}
			})
		}
	}
}

// Child freeform rejected while parent blocked after restart.
func TestRun63960FullMatrix_ChildStartTurnWhileParentBlocked(t *testing.T) {
	for _, pk := range allProviders63960() {
		pk := pk
		t.Run(string(pk), func(t *testing.T) {
			f := seedBlockedRestartFixture(t, pk, "cap", shapeCrashSettleReprompt)
			// Seed child on disk under same store.
			childID := f.hubID + "-child"
			now := time.Now().UTC().Format(time.RFC3339Nano)
			if err := f.store.UpsertProviderSession(context.Background(), ProviderSessionState{
				RunID: childID, ProjectID: f.state.ProjectID, ProviderKey: pk,
				ProviderSessionID: "sess-child", ParentRunID: f.hubID,
				AgentName: "coder", Label: "coder", Role: "coder",
				Status: RunStatusRunning, WorkingDirectory: f.cwd,
				StartedAt: now, UpdatedAt: now, RunKind: "chat",
				PendingFlowGateSettle: true, PendingFlowGateFinalMsg: "child",
			}); err != nil {
				t.Fatalf("child upsert: %v", err)
			}
			svc, turns := reconstructBlockedFixture(t, f, pk)
			// Reconstruct child (may already be reconstructed via parent path).
			storeR, err := NewLocalFileSessionStore(f.dir)
			if err != nil {
				t.Fatalf("child store: %v", err)
			}
			stChild, ok, getErr := storeR.GetProviderSession(context.Background(), childID)
			if getErr == nil && ok {
				if svc.runs[childID] == nil {
					if _, apiErr := svc.reconstructRun(stChild); apiErr != nil {
						// Parent path may already have loaded; ignore not-found races.
						t.Logf("child reconstruct: %s", apiErr.msg)
					}
				}
			}
			// Ensure child in memory with settle for resumePendingFlowGate path.
			svc.mu.Lock()
			if svc.runs[childID] == nil {
				svc.runs[childID] = &interactiveRun{
					id: childID, parentRunID: f.hubID, status: RunStatusRunning,
					subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
					pendingFlowGateSettle: true, pendingFlowGateFinalMsg: "child",
				}
			} else {
				svc.runs[childID].pendingFlowGateSettle = true
			}
			svc.mu.Unlock()

			svc.resumePendingFlowGate(childID)
			if turns.Load() != 0 {
				t.Fatalf("child gate auto-turn while parent blocked: %d", turns.Load())
			}
			if _, turnErr := svc.startTurn(childID, TurnInput{StepID: "coder", Prompt: "child freeform"}, "", ""); turnErr == nil || turnErr.code != "flow_awaiting_user" {
				t.Fatalf("child startTurn: %v, want flow_awaiting_user", turnErr)
			}
		})
	}
}

// Real escalate park → capture durable snap → reconstruct stays blocked.
func TestRun63960FullMatrix_EscalateParkThenRestart(t *testing.T) {
	for _, pk := range allProviders63960() {
		pk := pk
		t.Run(string(pk), func(t *testing.T) {
			// Use the shared flow-engine test harness (createRun+nodes wired).
			svc, runID := newFlowEngineTestRun(t)
			svc.agentOrchestrator.setLoop(runID, AgentLoopState{
				Status: "running", Cap: 3, RoundCap: 3, Round: 1, Mode: "explicit",
			})
			svc.mu.Lock()
			// Align provider key for matrix labeling only — park path is agnostic.
			svc.runs[runID].providerKey = pk
			svc.mu.Unlock()

			if _, aerr := svc.applyFlowControl(runID, FlowControlInput{
				Status:  "escalate",
				Summary: "coder failed mid-review",
			}); aerr != nil {
				t.Fatalf("escalate: %v", aerr)
			}
			loopLive := svc.agentOrchestrator.loopStateFor(runID)
			if loopLive.Status != "blocked" || loopLive.BlockReason != "escalate" {
				t.Fatalf("live loop=%+v want blocked/escalate", loopLive)
			}

			// Crash-window durable shape on disk.
			root := t.TempDir()
			cwd := filepath.Join(root, "workspace")
			_ = os.MkdirAll(cwd, 0o755)
			dir := filepath.Join(root, ".flowpilot", "chats")
			store, err := NewLocalFileSessionStore(dir)
			if err != nil {
				t.Fatalf("store: %v", err)
			}
			svc.mu.Lock()
			live := svc.runs[runID]
			snap := sessionStateOf(live)
			snap.LoopState = svc.agentOrchestrator.loopStateFor(runID)
			snap.WorkingDirectory = cwd
			snap.ProviderKey = pk
			snap.PendingFlowGateSettle = true
			snap.PendingFlowGateFinalMsg = "crash settle"
			snap.PendingGateRepromptPrompt = "crash reprompt"
			snap.PendingGateRepromptStepID = "synthesis"
			snap.PendingGateRepromptGen = 2
			svc.mu.Unlock()
			if err := store.UpsertProviderSession(context.Background(), snap); err != nil {
				t.Fatalf("persist: %v", err)
			}

			store2, err := NewLocalFileSessionStore(dir)
			if err != nil {
				t.Fatalf("reload: %v", err)
			}
			var turns atomic.Int32
			reg2 := newProviderRegistry()
			for _, p := range allProviders63960() {
				p := p
				reg2.register(ProviderRegistration{
					Key: p, Status: ProviderStatusAvailable,
					Capabilities: ProviderCapabilities{Streaming: true},
					newAdapter: func() ProviderRuntimeAdapter {
						return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
							turns.Add(1)
							b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "no"})
							return nil
						})
					},
				})
			}
			svc2 := newInteractiveService(reg2, newInteractiveCatalog(), store2)
			loaded, ok, getErr := store2.GetProviderSession(context.Background(), runID)
			if getErr != nil || !ok {
				t.Fatalf("get: ok=%v err=%v", ok, getErr)
			}
			if _, apiErr := svc2.reconstructRun(loaded); apiErr != nil {
				t.Fatalf("reconstruct: %s", apiErr.msg)
			}
			time.Sleep(30 * time.Millisecond)
			if turns.Load() != 0 {
				t.Fatalf("auto turns after escalate restart: %d", turns.Load())
			}
			loop := svc2.agentOrchestrator.loopStateFor(runID)
			if loop.Status != "blocked" || loop.BlockReason != "escalate" {
				t.Fatalf("loop=%+v want blocked/escalate", loop)
			}
			if _, turnErr := svc2.startTurn(runID, TurnInput{StepID: "chat", Prompt: "x"}, "", ""); turnErr == nil || turnErr.code != "flow_awaiting_user" {
				t.Fatalf("startTurn: %v", turnErr)
			}
			if _, contErr := svc2.resumeFlowWithFeedback(runID, "go"); contErr != nil {
				t.Fatalf("Continue: %v", contErr)
			}
		})
	}
}

// Cap park clean path + second restart after Continue does not re-arm settle.
func TestRun63960FullMatrix_ContinueThenSecondRestart(t *testing.T) {
	f := seedBlockedRestartFixture(t, ProviderKeyCodex, "cap", shapeCrashSettleReprompt)
	svc, _ := reconstructBlockedFixture(t, f, ProviderKeyCodex)
	if _, err := svc.resumeFlowWithFeedback(f.hubID, "extend"); err != nil {
		t.Fatalf("Continue: %v", err)
	}
	// Persist post-continue and reconstruct again.
	svc.mu.Lock()
	live := svc.runs[f.hubID]
	snap := sessionStateOf(live)
	snap.LoopState = svc.agentOrchestrator.loopStateFor(f.hubID)
	svc.mu.Unlock()
	store2, err := NewLocalFileSessionStore(f.dir)
	if err != nil {
		t.Fatalf("store2: %v", err)
	}
	if err := store2.UpsertProviderSession(context.Background(), snap); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	loaded, ok, getErr := store2.GetProviderSession(context.Background(), f.hubID)
	if getErr != nil || !ok {
		t.Fatalf("get: ok=%v err=%v", ok, getErr)
	}
	if loaded.LoopState.Status != "running" {
		t.Fatalf("disk after Continue status=%q want running", loaded.LoopState.Status)
	}
	if loaded.PendingFlowGateSettle {
		t.Fatal("disk settle re-armed after Continue")
	}
	reg3 := newProviderRegistry()
	reg3.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc3 := newInteractiveService(reg3, newInteractiveCatalog(), store2)
	if _, apiErr := svc3.reconstructRun(loaded); apiErr != nil {
		t.Fatalf("second reconstruct: %s", apiErr.msg)
	}
	if st := svc3.agentOrchestrator.loopStateFor(f.hubID).Status; st != "running" {
		t.Fatalf("second restart status=%q want running", st)
	}
}

// normalizeResumedFlowStatus maps durable blocked loop to run status blocked.
func TestRun63960FullMatrix_NormalizeStatusBlocked(t *testing.T) {
	for _, reason := range allBlockReasons63960() {
		st := ProviderSessionState{
			Status:    RunStatusRunning,
			LoopState: AgentLoopState{Status: "blocked", BlockReason: reason, Cap: 3},
		}
		got := normalizeResumedFlowStatus(st)
		if got != RunStatus("blocked") {
			t.Fatalf("reason=%s normalize=%q want blocked", reason, got)
		}
	}
	// Failed/cancelled durable run status wins over blocked loop.
	stFail := ProviderSessionState{
		Status:    RunStatusFailed,
		LoopState: AgentLoopState{Status: "blocked", BlockReason: "cap"},
	}
	if got := normalizeResumedFlowStatus(stFail); got != RunStatusFailed {
		t.Fatalf("failed+blocked normalize=%q want failed", got)
	}
}

// Gen high-water preserved across all disk shapes that had reprompt gen.
func TestRun63960FullMatrix_RepromptGenHighWater(t *testing.T) {
	for _, shape := range []blockedDiskShape{shapeCrashSettleReprompt, shapeRepromptOnly} {
		f := seedBlockedRestartFixture(t, ProviderKeyGrok, "cap", shape)
		wantGen := int64(7)
		if shape == shapeRepromptOnly {
			wantGen = 5
		}
		svc, _ := reconstructBlockedFixture(t, f, ProviderKeyGrok)
		svc.mu.Lock()
		got := svc.runs[f.hubID].pendingGateRepromptGen
		svc.mu.Unlock()
		if got != wantGen {
			t.Fatalf("shape=%s gen=%d want %d high-water", shape, got, wantGen)
		}
	}
}
