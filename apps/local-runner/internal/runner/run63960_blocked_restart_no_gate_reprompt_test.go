package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// run-63960: after Review Loop parks at cap (loop.Status=blocked), a runner
// restart must NOT re-run the post-turn gate / gate-reprompt. That path used to
// hit startTurn → flow_awaiting_user 409, map desktop to Failed, hide
// Continue/Stop, and risk clobbering durable LoopState.
//
// Provider-agnostic: reconstruct/resumePendingFlowGate branch on LoopState only.
// additive-tests-only: new file.

func TestRun63960BlockedRestartClearsStaleGateSettleAndPreservesLoop(t *testing.T) {
	providers := []ProviderKey{ProviderKeyCodex, ProviderKeyClaude, ProviderKeyGrok}
	for _, pk := range providers {
		pk := pk
		t.Run(string(pk), func(t *testing.T) {
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

			const hubID = "run-63960-blocked-restart"
			now := time.Now().UTC().Format(time.RFC3339Nano)
			// Crash-window shape: loop already blocked at cap, but disk still has
			// pending_flow_gate_settle + gate reprompt from the park turn.
			if err := store.UpsertProviderSession(context.Background(), ProviderSessionState{
				RunID:                     hubID,
				ProjectID:                 "proj-63960",
				ProviderKey:               pk,
				ProviderSessionID:         "sess-hub-63960",
				WorkingDirectory:          cwd,
				Status:                    RunStatusRunning,
				RunKind:                   "chat",
				AutoOrchestrate:           true,
				ActiveFlowNodes:           []agentpack.FlowNode{{ID: "coder"}, {ID: "synthesis"}},
				PendingFlowGateSettle:     true,
				PendingFlowGateFinalMsg:   "synthesis final",
				PendingFlowGateOccurredAt: now,
				PendingFlowGateTurnID:     "turn-69560",
				PendingGateRepromptPrompt: "gate: open issues remain",
				PendingGateRepromptStepID: "synthesis",
				PendingGateRepromptGen:    3,
				StartedAt:                 now,
				UpdatedAt:                 now,
				LoopState: AgentLoopState{
					Status:      "blocked",
					BlockReason: "cap",
					GateReason:  "cap 3 reached with 2 open issue(s)",
					Round:       3,
					Cap:         3,
					RoundCap:    3,
					Mode:        "explicit",
					OpenIssues:  2,
					ActiveNode:  "synthesis",
				},
			}); err != nil {
				t.Fatalf("Upsert: %v", err)
			}

			// Restart process: load store + reconstruct hub.
			store2, err := NewLocalFileSessionStore(dir)
			if err != nil {
				t.Fatalf("reload store: %v", err)
			}
			var turns atomic.Int32
			reg := newProviderRegistry()
			reg.register(ProviderRegistration{
				Key: pk, Status: ProviderStatusAvailable,
				Capabilities: ProviderCapabilities{Streaming: true},
				newAdapter: func() ProviderRuntimeAdapter {
					return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
						turns.Add(1)
						b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "must not run"})
						return nil
					})
				},
			})
			// Register sibling providers as available so registry is complete.
			for _, other := range []ProviderKey{ProviderKeyCodex, ProviderKeyClaude, ProviderKeyGrok} {
				if other == pk {
					continue
				}
				other := other
				reg.register(ProviderRegistration{
					Key: other, Status: ProviderStatusAvailable,
					Capabilities: ProviderCapabilities{Streaming: true},
					newAdapter: func() ProviderRuntimeAdapter {
						return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
							b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "noop"})
							return nil
						})
					},
				})
			}
			svc := newInteractiveService(reg, newInteractiveCatalog(), store2)
			st, ok, getErr := store2.GetProviderSession(context.Background(), hubID)
			if getErr != nil || !ok {
				t.Fatalf("GetProviderSession: ok=%v err=%v", ok, getErr)
			}
			if !st.PendingFlowGateSettle {
				t.Fatal("fixture must have PendingFlowGateSettle before reconstruct")
			}

			rs, apiErr := svc.reconstructRun(st)
			if apiErr != nil {
				t.Fatalf("reconstructRun: %s", apiErr.msg)
			}
			if rs == nil {
				t.Fatal("reconstructRun returned nil run")
			}

			// In-memory: settle/reprompt payload dropped; gen high-water kept; loop blocked.
			svc.mu.Lock()
			live := svc.runs[hubID]
			if live == nil {
				svc.mu.Unlock()
				t.Fatal("hub missing after reconstruct")
			}
			if live.pendingFlowGateSettle {
				svc.mu.Unlock()
				t.Fatal("pendingFlowGateSettle must be cleared on blocked reconstruct")
			}
			if live.pendingGateRepromptPrompt != "" || live.pendingGateRepromptStepID != "" {
				svc.mu.Unlock()
				t.Fatalf("gate reprompt must clear: prompt=%q step=%q",
					live.pendingGateRepromptPrompt, live.pendingGateRepromptStepID)
			}
			if live.pendingGateRepromptGen != 3 {
				svc.mu.Unlock()
				t.Fatalf("pendingGateRepromptGen = %d, want 3 high-water (run-23820)", live.pendingGateRepromptGen)
			}
			svc.mu.Unlock()

			loop := svc.agentOrchestrator.loopStateFor(hubID)
			if loop.Status != "blocked" || loop.BlockReason != "cap" {
				t.Fatalf("loop = %+v, want blocked/cap", loop)
			}

			// resumePendingFlowGate must no-op (settle already false; blocked guard).
			svc.resumePendingFlowGate(hubID)
			if turns.Load() != 0 {
				t.Fatalf("resumePendingFlowGate started %d turn(s); want 0", turns.Load())
			}

			// startTurn still rejects with flow_awaiting_user (Continue form path).
			_, turnErr := svc.startTurn(hubID, TurnInput{StepID: "chat-step", Prompt: "ok continue?"}, "", "")
			if turnErr == nil || turnErr.code != "flow_awaiting_user" {
				t.Fatalf("startTurn after blocked restart: err=%v, want flow_awaiting_user", turnErr)
			}

			// Durable disk: settle/reprompt cleared, LoopState still blocked.
			time.Sleep(50 * time.Millisecond)
			disk, ok, err := store2.GetProviderSession(context.Background(), hubID)
			if err != nil || !ok {
				t.Fatalf("post-reconstruct GetProviderSession: ok=%v err=%v", ok, err)
			}
			if disk.PendingFlowGateSettle {
				t.Fatal("disk PendingFlowGateSettle must clear so next restart does not re-arm")
			}
			if strings.TrimSpace(disk.PendingGateRepromptPrompt) != "" {
				t.Fatalf("disk PendingGateRepromptPrompt still set: %q", disk.PendingGateRepromptPrompt)
			}
			if disk.LoopState.Status != "blocked" {
				t.Fatalf("disk LoopState.Status = %q, want blocked (must not wipe)", disk.LoopState.Status)
			}
			if disk.PendingGateRepromptGen != 3 {
				t.Fatalf("disk PendingGateRepromptGen = %d, want 3 high-water", disk.PendingGateRepromptGen)
			}

			// Second reconstruct (another restart): settle must stay cleared.
			store3, err := NewLocalFileSessionStore(dir)
			if err != nil {
				t.Fatalf("store3: %v", err)
			}
			svc2 := newInteractiveService(reg, newInteractiveCatalog(), store3)
			st2, ok2, err2 := store3.GetProviderSession(context.Background(), hubID)
			if err2 != nil || !ok2 {
				t.Fatalf("second Get: ok=%v err=%v", ok2, err2)
			}
			if st2.PendingFlowGateSettle {
				t.Fatal("second boot fixture still has settle — first persist failed")
			}
			if _, apiErr2 := svc2.reconstructRun(st2); apiErr2 != nil {
				t.Fatalf("second reconstruct: %s", apiErr2.msg)
			}
			loop2 := svc2.agentOrchestrator.loopStateFor(hubID)
			if loop2.Status != "blocked" {
				t.Fatalf("second reconstruct loop = %q, want blocked", loop2.Status)
			}
		})
	}
}

// stopped/done also clear stale settle so restarts do not re-schedule gate forever.
func TestRun63960ResumePendingFlowGateClearsSettleWhenStoppedOrDone(t *testing.T) {
	for _, status := range []string{"stopped", "done"} {
		status := status
		t.Run(status, func(t *testing.T) {
			reg := newProviderRegistry()
			reg.register(ProviderRegistration{
				Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
				Capabilities: ProviderCapabilities{Streaming: true},
				newAdapter: func() ProviderRuntimeAdapter {
					return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
						b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "no"})
						return nil
					})
				},
			})
			svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
			parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
			if err != nil {
				t.Fatalf("createRun: %v", err)
			}
			parentID := parent.RunID
			svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: status, Cap: 3, Round: 1})
			svc.mu.Lock()
			rs := svc.runs[parentID]
			rs.flowEngineDriven = true
			rs.pendingFlowGateSettle = true
			rs.pendingFlowGateFinalMsg = "stale"
			rs.pendingGateRepromptPrompt = "stale"
			rs.pendingGateRepromptStepID = "synthesis"
			rs.pendingGateRepromptGen = 5
			svc.mu.Unlock()

			svc.resumePendingFlowGate(parentID)

			svc.mu.Lock()
			rs = svc.runs[parentID]
			if rs.pendingFlowGateSettle {
				svc.mu.Unlock()
				t.Fatal("settle must clear for terminal loop")
			}
			if rs.pendingGateRepromptPrompt != "" {
				svc.mu.Unlock()
				t.Fatal("reprompt payload must clear")
			}
			if rs.pendingGateRepromptGen != 5 {
				svc.mu.Unlock()
				t.Fatalf("gen = %d, want 5 high-water", rs.pendingGateRepromptGen)
			}
			svc.mu.Unlock()
		})
	}
}

// Child gate resume must no-op when parent loop is blocked (cap park).
func TestRun63960ChildGateResumeSkippedWhenParentBlocked(t *testing.T) {
	reg := newProviderRegistry()
	var turns atomic.Int32
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				turns.Add(1)
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "no"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	parentID := parent.RunID
	childID := "child-63960-gate"
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{
		Status: "blocked", BlockReason: "cap", Cap: 3, Round: 3, RoundCap: 3,
	})
	svc.mu.Lock()
	svc.runs[parentID].flowEngineDriven = true
	svc.runs[childID] = &interactiveRun{
		id: childID, parentRunID: parentID, status: RunStatusRunning,
		subs: map[int64]chan ProviderEvent{}, idempotency: map[string]string{},
		pendingFlowGateSettle: true, pendingFlowGateFinalMsg: "child done",
		pendingGateRepromptPrompt: "child reprompt", pendingGateRepromptStepID: "coder",
		pendingGateRepromptGen: 2,
	}
	svc.mu.Unlock()

	svc.resumePendingFlowGate(childID)
	if turns.Load() != 0 {
		t.Fatalf("child gate started %d turns while parent blocked", turns.Load())
	}
	svc.mu.Lock()
	child := svc.runs[childID]
	if child.pendingFlowGateSettle {
		svc.mu.Unlock()
		t.Fatal("child settle must clear when parent blocked")
	}
	if child.pendingGateRepromptGen != 2 {
		svc.mu.Unlock()
		t.Fatalf("child gen = %d, want 2", child.pendingGateRepromptGen)
	}
	svc.mu.Unlock()
}

// resumePendingFlowGate when loop is already blocked must clear settle and
// never start a turn — even if reconstruct did not run the clear path.
func TestRun63960ResumePendingFlowGateNoOpWhenBlocked(t *testing.T) {
	reg := newProviderRegistry()
	var turns atomic.Int32
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				turns.Add(1)
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "no"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	parentID := parent.RunID
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{
		Status: "blocked", BlockReason: "cap", Cap: 3, Round: 3, RoundCap: 3,
		GateReason: "cap 3 reached with 2 open issue(s)",
	})
	svc.mu.Lock()
	rs := svc.runs[parentID]
	rs.flowEngineDriven = true
	rs.pendingFlowGateSettle = true
	rs.pendingFlowGateFinalMsg = "stale"
	rs.pendingGateRepromptPrompt = "stale reprompt"
	rs.pendingGateRepromptStepID = "synthesis"
	svc.mu.Unlock()

	svc.resumePendingFlowGate(parentID)

	if turns.Load() != 0 {
		t.Fatalf("turns started = %d, want 0", turns.Load())
	}
	svc.mu.Lock()
	rs = svc.runs[parentID]
	if rs.pendingFlowGateSettle {
		svc.mu.Unlock()
		t.Fatal("pendingFlowGateSettle must clear when blocked")
	}
	if rs.pendingGateRepromptPrompt != "" {
		svc.mu.Unlock()
		t.Fatalf("reprompt still set: %q", rs.pendingGateRepromptPrompt)
	}
	svc.mu.Unlock()

	loop := svc.agentOrchestrator.loopStateFor(parentID)
	if loop.Status != "blocked" {
		t.Fatalf("loop.Status = %q, want blocked", loop.Status)
	}
}
