package runner

import (
	"context"
	"testing"
	"time"
)

// Live run-45 (bug-harness, devin/swe-2-high): the audit node settled the flow
// to done INSIDE the hub's still-in-flight provider turn (inline audit behavior
// ran during the hub's own turn). The sequence wedged the run at status
// "running" forever:
//
//  1. applyFlowControl(done) mid-turn → markFlowRunComplete →
//     settleParentRunOnFlowDone → rs.status = completed, loop = done.
//  2. EventTurnCompleted for that same turn: !turnStartedAfterLoopDone &&
//     flowEngineDriven → markPendingFlowGateSettleLocked arms the deferred
//     gate settle — and force-flips rs.status back to running ("keep
//     non-terminal" guard), overwriting the just-settled completed row.
//  3. The armed post-turn gate then refuses evaluation: gateEpochStillValid
//     returns false because the loop is already done and the turn did NOT
//     start after it sealed — runFlowGateAtEpoch returns blocked with no
//     log/event; the blocked path clears the settle and leaves rs.status
//     running with no reprompt, no card, no further schedule.
//
// The durable session row persisted at step 2 recorded status=running +
// pending_flow_gate_settle=true + loop=done — only the CA-1030 loop backfill
// made a kill/restart recover to completed; a live process kept reporting a
// finished flow as "running".
//
// Pin: a hub turn that completes cleanly AFTER its own flow already sealed
// done mid-turn keeps the settled terminal status — the deferred gate settle
// is not armed on an already-terminal loop (the seal path owns the terminal).
func TestFlowDoneMidTurnKeepsSettledCompletedStatus(t *testing.T) {
	reg := newProviderRegistry()
	var svc *InteractiveService
	var runID string
	reg.register(ProviderRegistration{
		Key: ProviderKeyGrok, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				// Mid-turn seal: audit's inline behavior settles the flow while
				// this hub turn is still in flight (the run-45 shape).
				if _, err := svc.applyFlowControl(runID, FlowControlInput{
					Status:  "done",
					Summary: "audit accepted",
				}); err != nil {
					t.Errorf("mid-turn applyFlowControl(done): %v", err)
				}
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "audit accepted"})
				return nil
			})
		},
	})
	svc = newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	run, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyGrok})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	runID = run.RunID
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.flowEngineDriven = true
	// Non-empty cwd so the post-turn gate does not early-pass on the empty path;
	// the loop-done epoch guard fires before any diff observation, so a plain
	// temp dir suffices.
	rs.workspaceCwd = t.TempDir()
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	if _, apiErr := svc.startTurn(runID, TurnInput{StepID: "turn-1", Prompt: "synthesize"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn: %s", apiErr.msg)
	}
	waitLoop(t, "hub turn settled", 5*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		r := svc.runs[runID]
		return r != nil && !r.turnInFlight
	})

	svc.mu.Lock()
	defer svc.mu.Unlock()
	got := svc.runs[runID]
	if got.status != RunStatusCompleted {
		t.Fatalf("flow hub that sealed done mid-turn must stay completed, got %q", got.status)
	}
	if got.pendingFlowGateSettle {
		t.Fatal("deferred gate settle must not linger armed on an already-done loop")
	}
}
