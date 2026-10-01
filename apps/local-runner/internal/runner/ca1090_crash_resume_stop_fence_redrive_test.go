package runner

// CA-1090 — live run-3362 (vibe-tasks, PrivateVault): after a runner shutdown
// wrote the durable run_stop fence (gen 1) + cancel_requested on recoverable
// dispatch records, every resume/re-drive minted a hub turn that died at the
// send CAS with stop_outcome="stopped_before_send" — zero provider bytes, no
// terminal event, loop stuck "running" but forever quiet. Observed dispatch
// trace: prepared → send_claimed → terminal_cancelled(stopped_before_send).
//
// The stop tombstone belongs to the dead process — a legitimately resumed run
// (chat-open redriveQuietFlowLoop, agent-loop/resume pending intents) must
// release it before re-driving, exactly like the BUG-308 sealed follow-up
// path does for plain chat.

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// redriveQuietFlowLoop is the post-crash re-drive seam: loop durable-running,
// run status cancelled by shutdown, no live work. It already heals the in-RAM
// status — it must also release the durable stop fence or the minted turn is
// fenced before send.
func TestCA1090_CrashResumedRedriveReleasesStopFence(t *testing.T) {
	svc, store, parentID, sends, _ := setupPostStopHub(t, ProviderKeyCodex)

	// Post-restart reconstruct shape: durable loop reads running, run cancelled,
	// mounted flow nodes present, nothing live — exactly run-3362 at reload.
	svc.mu.Lock()
	rs := svc.runs[parentID]
	rs.activeFlowNodes = []agentpack.FlowNode{{ID: "synthesis", Behavior: "hub.inline"}}
	svc.mu.Unlock()
	svc.agentOrchestrator.mutateLoop(parentID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		return st
	})

	sendsBefore := atomic.LoadInt32(sends)
	svc.redriveQuietFlowLoop(parentID)

	waitLoop(t, "redriven hub turn reaches adapter", 3*time.Second, func() bool {
		return atomic.LoadInt32(sends) > sendsBefore
	})
	st, err := store.GetRunStopState(context.Background(), parentID)
	if err != nil {
		t.Fatalf("GetRunStopState: %v", err)
	}
	if st.Stopped {
		t.Fatal("durable run_stop fence still armed after legitimate resume redrive — every minted turn dies stopped_before_send")
	}
}

// Same fence kills the durable-restart drain: resumePendingLoopWork fires
// deliverPendingRestart for a survived stall-retry intent — the minted retry
// turn must not be fenced either.
func TestCA1090_PendingRestartDrainReleasesStopFence(t *testing.T) {
	svc, store, parentID, sends, _ := setupPostStopHub(t, ProviderKeyCodex)

	svc.mu.Lock()
	rs := svc.runs[parentID]
	rs.activeFlowNodes = []agentpack.FlowNode{{ID: "synthesis", Behavior: "hub.inline"}}
	rs.status = RunStatusRunning
	rs.agentStatus = string(RunStatusRunning)
	rs.pendingRestartRunID = parentID
	rs.pendingRestartPrompt = "retry the stalled hub turn"
	rs.pendingRestartGen = 1
	svc.mu.Unlock()
	svc.agentOrchestrator.resume(parentID)

	sendsBefore := atomic.LoadInt32(sends)
	svc.resumePendingLoopWork(parentID)

	waitLoop(t, "pending restart turn reaches adapter", 3*time.Second, func() bool {
		return atomic.LoadInt32(sends) > sendsBefore
	})
	st, err := store.GetRunStopState(context.Background(), parentID)
	if err != nil {
		t.Fatalf("GetRunStopState: %v", err)
	}
	if st.Stopped {
		t.Fatal("durable run_stop fence still armed after agent-loop resume — pending restart turn dies stopped_before_send")
	}
}

// Guard: an operator Stop that was NOT followed by a resume must keep fencing.
// Re-driving nothing, no release — a bare stopped loop stays fenced.
func TestCA1090_NoResumeNoFenceRelease(t *testing.T) {
	svc, store, parentID, sends, _ := setupPostStopHub(t, ProviderKeyCodex)

	// Nobody resumed — the fence must still be armed and a bare turn send fenced.
	st, err := store.GetRunStopState(context.Background(), parentID)
	if err != nil {
		t.Fatalf("GetRunStopState: %v", err)
	}
	if !st.Stopped {
		t.Fatal("stop fence unexpectedly released without a resume")
	}
	svc.mu.Lock()
	rs := svc.runs[parentID]
	rs.status = RunStatusRunning
	rs.agentStatus = string(RunStatusRunning)
	svc.mu.Unlock()
	svc.agentOrchestrator.mutateLoop(parentID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		return st
	})
	sendsBefore := atomic.LoadInt32(sends)
	if _, apiErr := svc.startTurn(parentID, TurnInput{StepID: "chat-" + parentID, Prompt: "sneaky"}, "", ""); apiErr != nil {
		// Refused admission is also acceptable — the fence held.
		return
	}
	waitLoop(t, "turn settles", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[parentID].turnInFlight
	})
	if atomic.LoadInt32(sends) > sendsBefore {
		t.Fatal("turn reached adapter while durable stop fence still armed")
	}
}
