package runner

import (
	"context"
	"sync/atomic"
	"testing"
)

// run-203966 (CP-58 S2 task-harness): the audit node's escalate parked the
// flow (parkFlowForAwaitingUser) while the hub synthesis turn was still in
// flight. finishTurn mapped that engine park-cancel to the generic
// "interrupted by user" Cancelled stamp, flowRunTerminalLocked went true, and
// every later advance after Retry was silently skipped ("run terminal/
// stopped") until the watchdog parked hub_stalled. The park cancel must keep
// the parent non-terminal, Stop must still win, and Retry must heal a
// poisoned Cancelled parent.
//
// New file; no pre-existing test is modified. The engine path takes no
// providerKey (park/finishTurn/resume never branch on it), so the matrix
// guards future provider drift.

func run203966Service(t *testing.T, pk ProviderKey) (*InteractiveService, string) {
	t.Helper()
	ch := make(chan TurnRequest, 4)
	reg := newProviderRegistry()
	registerKeyedCapture(reg, pk, ch)
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: pk})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	return svc, parent.RunID
}

func run203966Providers() []ProviderKey {
	return []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
}

// TestRun203966AuditEscalateParkKeepsParentNonterminal locks the reported
// repro: audit tier-3 escalate (missing CA) while the hub turn is in flight
// parks the flow but must NOT terminalize the parent — the cancelled turn
// must land non-terminal so flowRunTerminalLocked stays false and Retry can
// advance.
func TestRun203966AuditEscalateParkKeepsParentNonterminal(t *testing.T) {
	for _, pk := range run203966Providers() {
		t.Run(string(pk), func(t *testing.T) {
			svc, runID, edges, nodes, auditNode := run202550AuditFixture(t, pk)
			var cancelled atomic.Bool
			svc.mu.Lock()
			rs := svc.runs[runID]
			rs.status = RunStatusRunning
			rs.turnInFlight = true
			rs.currentTurnID = "turn-hub"
			rs.turnCancel = func() { cancelled.Store(true) }
			svc.mu.Unlock()

			if !svc.runAuditNode(context.Background(), runID, edges, nodes, auditNode, "implemented GCD") {
				t.Fatalf("%s: runAuditNode must handle (escalate) the missing-CA tier-3 block", pk)
			}
			if !cancelled.Load() {
				t.Fatalf("%s: park must cancel the in-flight hub turn", pk)
			}
			st := svc.agentOrchestrator.loopStateFor(runID)
			if st.Status != "blocked" || st.BlockReason != "escalate" || !isMissingChangeAuditNoteReason(st.GateReason) {
				t.Fatalf("%s: loop = %q/%q/%q, want blocked/escalate missing-CA", pk, st.Status, st.BlockReason, st.GateReason)
			}
			svc.mu.Lock()
			status := svc.runs[runID].status
			cause := svc.runs[runID].parkCancelCause
			suppress := svc.runs[runID].parkCancelSuppress
			svc.mu.Unlock()
			if status != RunStatusRunning {
				t.Fatalf("%s: parent status = %v, want Running after park cancel (run-203966)", pk, status)
			}
			if !cause || !suppress {
				t.Fatalf("%s: park-cancel flags must be armed (cause=%v suppress=%v)", pk, cause, suppress)
			}
			if svc.flowRunTerminalLocked(runID) {
				t.Fatalf("%s: parent must stay non-terminal so later advances are not skipped", pk)
			}
		})
	}
}

// TestRun203966FinishTurnParkCancelDoesNotStampCancelled locks the
// finishTurn contract: a context.Canceled err carrying parkCancelCause keeps
// the run Running (no "interrupted by user" Cancelled stamp) and consumes
// the one-shot flag.
func TestRun203966FinishTurnParkCancelDoesNotStampCancelled(t *testing.T) {
	for _, pk := range run203966Providers() {
		t.Run(string(pk), func(t *testing.T) {
			svc, runID := run203966Service(t, pk)
			svc.mu.Lock()
			rs := svc.runs[runID]
			rs.status = RunStatusRunning
			rs.parkCancelCause = true
			svc.mu.Unlock()

			completed, _ := svc.finishTurn(svc.runs[runID], "turn-1", context.Canceled)
			if completed {
				t.Fatalf("%s: park-cancel finish must not report completed", pk)
			}
			svc.mu.Lock()
			defer svc.mu.Unlock()
			rs = svc.runs[runID]
			if rs.status != RunStatusRunning {
				t.Fatalf("%s: status = %v, want Running (no Cancelled stamp)", pk, rs.status)
			}
			if rs.parkCancelCause {
				t.Fatalf("%s: parkCancelCause must be consumed one-shot", pk)
			}
		})
	}
}

// TestRun203966StopWinsOverParkCancelCause locks the Stop contract: when
// stopAgentLoop already sealed the loop "stopped" and stamped the run
// Cancelled, the parked turn's later context.Canceled finishTurn must NOT
// revive it as Running.
func TestRun203966StopWinsOverParkCancelCause(t *testing.T) {
	for _, pk := range run203966Providers() {
		t.Run(string(pk), func(t *testing.T) {
			svc, runID := run203966Service(t, pk)
			svc.mu.Lock()
			rs := svc.runs[runID]
			rs.status = RunStatusRunning
			rs.turnInFlight = true
			rs.turnCancel = func() {}
			rs.parkCancelCause = true
			svc.mu.Unlock()

			if _, apiErr := svc.stopAgentLoop(runID); apiErr != nil {
				t.Fatalf("%s: stopAgentLoop: %v", pk, apiErr)
			}
			svc.mu.Lock()
			rs = svc.runs[runID]
			if rs.status != RunStatusCancelled {
				svc.mu.Unlock()
				t.Fatalf("%s: stop must stamp Cancelled, got %v", pk, rs.status)
			}
			svc.mu.Unlock()
			if st := svc.agentOrchestrator.loopStateFor(runID); st.Status != "stopped" {
				t.Fatalf("%s: loop = %q, want stopped", pk, st.Status)
			}

			svc.finishTurn(svc.runs[runID], "turn-1", context.Canceled)
			svc.mu.Lock()
			defer svc.mu.Unlock()
			rs = svc.runs[runID]
			if rs.status != RunStatusCancelled {
				t.Fatalf("%s: finishTurn must not revive a stopped run, got %v", pk, rs.status)
			}
			if rs.parkCancelCause || rs.parkCancelSuppress {
				t.Fatalf("%s: park flags must be cleared after the Stop-guard fallthrough", pk)
			}
		})
	}
}

// TestRun203966ParkCancelCauseClearedOnCleanFinish locks the flag hygiene: a
// non-context.Canceled finish (including nil) clears both park flags so a
// stale cause cannot misclassify the NEXT real user interrupt.
func TestRun203966ParkCancelCauseClearedOnCleanFinish(t *testing.T) {
	for _, pk := range run203966Providers() {
		t.Run(string(pk), func(t *testing.T) {
			svc, runID := run203966Service(t, pk)
			svc.mu.Lock()
			rs := svc.runs[runID]
			rs.status = RunStatusRunning
			rs.parkCancelCause = true
			rs.parkCancelSuppress = true
			svc.mu.Unlock()

			completed, _ := svc.finishTurn(svc.runs[runID], "turn-1", nil)
			if !completed {
				t.Fatalf("%s: clean finish must report completed", pk)
			}
			svc.mu.Lock()
			defer svc.mu.Unlock()
			rs = svc.runs[runID]
			if rs.parkCancelCause || rs.parkCancelSuppress {
				t.Fatalf("%s: park flags must clear on non-cancel finish (cause=%v suppress=%v)", pk, rs.parkCancelCause, rs.parkCancelSuppress)
			}
		})
	}
}

// TestRun203966ResumeHealsCancelledBlockedNotStopped locks the Retry heal:
// a blocked loop resuming a park-poisoned Cancelled parent restores Running,
// while a stopped loop never heals (Stop stays terminal) and a Failed root
// is left alone (real failure, not park poison).
func TestRun203966ResumeHealsCancelledBlockedNotStopped(t *testing.T) {
	for _, pk := range run203966Providers() {
		t.Run(string(pk), func(t *testing.T) {
			svc, runID := run203966Service(t, pk)

			svc.agentOrchestrator.setLoop(runID, AgentLoopState{
				Status: "blocked", BlockReason: "escalate", GateReason: "escalated",
				Cap: 3, RoundCap: 3, Mode: "explicit",
			})
			svc.mu.Lock()
			svc.runs[runID].status = RunStatusCancelled
			svc.mu.Unlock()
			snap, err := svc.resumeFlowWithFeedback(runID, "")
			if err != nil {
				t.Fatalf("%s: resumeFlowWithFeedback: %v", pk, err)
			}
			if snap.LoopState.Status != "running" {
				t.Fatalf("%s: loop = %q, want running after resume", pk, snap.LoopState.Status)
			}
			svc.mu.Lock()
			healed := svc.runs[runID].status
			svc.mu.Unlock()
			if healed != RunStatusRunning {
				t.Fatalf("%s: blocked+Cancelled parent must heal to Running, got %v", pk, healed)
			}

			// A genuinely stopped loop must NOT heal its Cancelled stamp.
			svc.agentOrchestrator.setLoop(runID, AgentLoopState{
				Status: "stopped", BlockReason: "", GateReason: "",
				Cap: 3, RoundCap: 3, Mode: "explicit",
			})
			svc.mu.Lock()
			svc.runs[runID].status = RunStatusCancelled
			svc.mu.Unlock()
			if _, err := svc.resumeFlowWithFeedback(runID, ""); err != nil {
				t.Fatalf("%s: resume on stopped loop: %v", pk, err)
			}
			svc.mu.Lock()
			healed = svc.runs[runID].status
			svc.mu.Unlock()
			if healed != RunStatusCancelled {
				t.Fatalf("%s: stopped loop must stay Cancelled, got %v", pk, healed)
			}

			// A Failed root is a real failure — never healed by a park resume.
			svc.agentOrchestrator.setLoop(runID, AgentLoopState{
				Status: "blocked", BlockReason: "escalate", GateReason: "escalated",
				Cap: 3, RoundCap: 3, Mode: "explicit",
			})
			svc.mu.Lock()
			svc.runs[runID].status = RunStatusFailed
			svc.mu.Unlock()
			if _, err := svc.resumeFlowWithFeedback(runID, ""); err != nil {
				t.Fatalf("%s: resume on failed root: %v", pk, err)
			}
			svc.mu.Lock()
			healed = svc.runs[runID].status
			svc.mu.Unlock()
			if healed != RunStatusFailed {
				t.Fatalf("%s: Failed root must not heal, got %v", pk, healed)
			}
		})
	}
}

// TestRun203966CapPreserveDoesNotSetParkCancelCause locks the CA-403
// contract against the new flags: a preserveParentTurnID park (cap on the
// submitting hub turn) must neither cancel the turn nor arm the park-cancel
// flags; a default park still cancels and DOES arm them.
func TestRun203966CapPreserveDoesNotSetParkCancelCause(t *testing.T) {
	for _, pk := range run203966Providers() {
		t.Run(string(pk), func(t *testing.T) {
			svc, runID := run203966Service(t, pk)
			var cancelled atomic.Bool
			svc.mu.Lock()
			rs := svc.runs[runID]
			rs.status = RunStatusRunning
			rs.currentTurnID = "turn-hub"
			rs.turnInFlight = true
			rs.turnCancel = func() { cancelled.Store(true) }
			svc.mu.Unlock()

			// Matching preserve id (cap on the submitting turn): no cancel, no flags.
			svc.parkFlowForAwaitingUser(runID, parkFlowForAwaitingUserOptions{preserveParentTurnID: "turn-hub"})
			if cancelled.Load() {
				t.Fatalf("%s: preserveParent park must not cancel the submitting turn (CA-403)", pk)
			}
			svc.mu.Lock()
			rs = svc.runs[runID]
			if rs.parkCancelCause || rs.parkCancelSuppress {
				svc.mu.Unlock()
				t.Fatalf("%s: preserveParent park must not arm park-cancel flags", pk)
			}
			svc.mu.Unlock()

			// Mismatched preserve id: default park fires and flags ARE armed.
			svc.parkFlowForAwaitingUser(runID, parkFlowForAwaitingUserOptions{preserveParentTurnID: "turn-other"})
			if !cancelled.Load() {
				t.Fatalf("%s: mismatched preserve id must not suppress the parent cancel", pk)
			}
			svc.mu.Lock()
			defer svc.mu.Unlock()
			rs = svc.runs[runID]
			if !rs.parkCancelCause || !rs.parkCancelSuppress {
				t.Fatalf("%s: default park must arm park-cancel flags (cause=%v suppress=%v)", pk, rs.parkCancelCause, rs.parkCancelSuppress)
			}
		})
	}
}
