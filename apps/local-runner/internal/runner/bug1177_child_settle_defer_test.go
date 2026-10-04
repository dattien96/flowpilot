package runner

import (
	"testing"
	"time"
)

// BUG-1177 (live run-183756): resumePendingFlowGate permanently dropped a
// child's pendingFlowGateSettle whenever the parent loop was blocked — the
// 30s wedge sweep re-fired it into the same drop, so a member whose turn
// completed while the parent awaited a decision lost its owed terminal
// settle forever: status stayed running (markPendingFlowGateSettleLocked
// pins it), the verdict never joined the cohort, and no driver remained
// (dispatch record terminal+settle-final or the settle gone with it).
// Root cause: P1-04's drop was designed for ROOT runs whose own blocked
// loop would send a reprompt startTurn into the flow_awaiting_user wipe —
// for a CHILD every eval outcome is safe (pass→join, park→card, reprompt→
// durable fenced park), so the settle must DEFER until the parent unblocks.

// Child of a merely-blocked parent keeps its armed settle; the wedge sweep
// re-drives it until the parent loop unblocks and the eval can run.
func TestBug1177ChildSettleDefersWhileParentBlocked(t *testing.T) {
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
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Status: "blocked", BlockReason: "cap", Cap: 3, Round: 3, RoundCap: 3,
	})
	svc.mu.Lock()
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[parent.RunID].activeFlowNodes = reviewLoopTestNodes()
	crs := svc.runs[child.RunID]
	crs.parentRunID = parent.RunID
	crs.label = "reviewer"
	crs.workspaceCwd = dir
	crs.pendingFlowGateSettle = true
	crs.pendingFlowGateFinalMsg = "verdict emitted"
	crs.pendingFlowGateOccurredAt = time.Now().UTC().Format(time.RFC3339Nano)
	crs.lastTurnID = "turn-1177"
	crs.status = RunStatusRunning
	crs.pendingGateRepromptPrompt = "stale reprompt"
	crs.pendingGateRepromptStepID = "reviewer"
	crs.pendingGateRepromptGen = 4
	svc.mu.Unlock()

	svc.resumePendingFlowGate(child.RunID)

	svc.mu.Lock()
	crs = svc.runs[child.RunID]
	defer svc.mu.Unlock()
	if !crs.pendingFlowGateSettle {
		t.Fatal("child settle was dropped while parent blocked — owed terminal disposition lost (zombie)")
	}
	if crs.pendingFlowGateFinalMsg != "verdict emitted" || crs.pendingFlowGateTurnID == "" && crs.lastTurnID != "turn-1177" {
		t.Fatal("child settle payload must survive the defer")
	}
	if crs.gateClaimID != "" || crs.postTurnGateCancel != nil {
		t.Fatal("deferred child must not run a gate eval while parent blocked")
	}
	// Spawn intents still die — the deferred eval re-derives them when it
	// finally runs (P1-04's no-spawn property is preserved).
	if crs.pendingGateRepromptPrompt != "" || crs.pendingGateRepromptStepID != "" {
		t.Fatal("reprompt spawn intent must still drop while parent blocked")
	}
	if crs.pendingGateRepromptGen != 4 {
		t.Fatalf("reprompt gen high-water = %d, want 4", crs.pendingGateRepromptGen)
	}
}

// Converge: once the parent unblocks, the still-armed settle drives the eval
// to completion — the child reaches a terminal status and the flag clears.
func TestBug1177DeferredChildSettleConvergesOnUnblock(t *testing.T) {
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
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Status: "blocked", BlockReason: "cap", Cap: 3, Round: 3, RoundCap: 3,
	})
	svc.mu.Lock()
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[parent.RunID].activeFlowNodes = reviewLoopTestNodes()
	crs := svc.runs[child.RunID]
	crs.parentRunID = parent.RunID
	crs.label = "reviewer"
	crs.workspaceCwd = dir
	crs.pendingFlowGateSettle = true
	crs.pendingFlowGateFinalMsg = "verdict emitted"
	crs.pendingFlowGateOccurredAt = time.Now().UTC().Format(time.RFC3339Nano)
	crs.lastTurnID = "turn-1177b"
	crs.status = RunStatusRunning
	svc.mu.Unlock()

	// Blocked: deferred (armed, no eval).
	svc.resumePendingFlowGate(child.RunID)
	svc.mu.Lock()
	if !svc.runs[child.RunID].pendingFlowGateSettle {
		svc.mu.Unlock()
		t.Fatal("settle dropped under blocked parent before unblock")
	}
	svc.mu.Unlock()

	// Parent unblocks — the wedge sweep's next tick re-fires this same call.
	svc.agentOrchestrator.mutateLoop(parent.RunID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		st.BlockReason = ""
		st.GateReason = ""
		return st
	})
	svc.resumePendingFlowGate(child.RunID)

	svc.mu.Lock()
	crs = svc.runs[child.RunID]
	defer svc.mu.Unlock()
	if crs.status != RunStatusCompleted {
		t.Fatalf("status = %q after unblock eval, want Completed", crs.status)
	}
	if crs.pendingFlowGateSettle {
		t.Fatal("settle still armed after the deferred eval passed")
	}
}

// Terminal parents still own the drop: a stopped/done loop's children are
// settled by reconcile, so re-arming the settle would only reschedule every
// restart (safe-fix residual). The clear behavior is preserved.
func TestBug1177ChildSettleStillDropsForTerminalParent(t *testing.T) {
	for _, st := range []string{"stopped", "done"} {
		t.Run(st, func(t *testing.T) {
			svc, _ := newTestServer(t)
			parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
			if err != nil {
				t.Fatal(err)
			}
			child, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
			if err != nil {
				t.Fatal(err)
			}
			svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: st})
			svc.mu.Lock()
			svc.runs[parent.RunID].flowEngineDriven = true
			crs := svc.runs[child.RunID]
			crs.parentRunID = parent.RunID
			crs.pendingFlowGateSettle = true
			crs.pendingFlowGateFinalMsg = "orphaned"
			crs.lastTurnID = "turn-1177t"
			crs.status = RunStatusRunning
			svc.mu.Unlock()

			svc.resumePendingFlowGate(child.RunID)

			svc.mu.Lock()
			defer svc.mu.Unlock()
			if svc.runs[child.RunID].pendingFlowGateSettle {
				t.Fatalf("settle must still drop when parent loop is %q", st)
			}
		})
	}
}
