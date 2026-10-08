package runner

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

// BUG-657 (live run-523131, leg run-584652): Interrupt writes the durable
// run_stop fence keyed on the interrupted run's own id, while every
// releaseHubStopFenceForFollowUp call site keyed parentRunID — a leg-scoped
// stop had no release path. The adjudication's reprompt redrive minted a
// turn that died at the send CAS with "turn cancelled before send (run stop
// fence)"; the leg stayed dead weight forever.
func TestBug657_InterruptedLegRedrivesAfterFollowUp(t *testing.T) {
	var sendCount int32
	var cap atomic.Value
	svc := newInteractiveService(bug308FullAFRegistry(ProviderKeyCodex, &cap, &sendCount), newInteractiveCatalog(), newFakeWorkflowStore())
	svc.SetDispatchStore(NewMemoryDispatchStore())

	parent, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun parent: %v", err)
	}
	leg, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun leg: %v", err)
	}
	svc.mu.Lock()
	lr := svc.runs[leg.RunID]
	lr.parentRunID = parent.RunID
	lr.label = "tdd"
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(parent.RunID, leg.RunID)

	// Activate dispatch V2 on the leg like a live turn did.
	if _, apiErr := svc.startTurn(leg.RunID, TurnInput{StepID: "step-1", Prompt: "first leg turn"}, "", ""); apiErr != nil {
		t.Fatalf("seed startTurn: %v", apiErr)
	}
	waitLoop(t, "seed leg turn done", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[leg.RunID].turnInFlight
	})

	// Operator interrupt on the leg — fence lands on the LEG's own id.
	if apiErr := svc.Interrupt(leg.RunID); apiErr != nil {
		t.Fatalf("Interrupt leg: %s", apiErr.msg)
	}
	st, gerr := svc.dispatchStore.GetRunStopState(context.Background(), leg.RunID)
	if gerr != nil {
		t.Fatalf("GetRunStopState leg: %v", gerr)
	}
	if !st.Stopped {
		t.Fatal("leg-scoped run_stop fence not armed after Interrupt")
	}

	// Authorized re-drive: adjudication reprompt mints a new turn on the leg
	// through the child-dispatch seam — the leg's own fence must release.
	sendsBefore := atomic.LoadInt32(&sendCount)
	svc.scheduleChildTurn(leg.RunID, "step-2", "adjudication reprompt: fix the verdict shape")

	waitLoop(t, "redriven leg turn reaches adapter", 3*time.Second, func() bool {
		return atomic.LoadInt32(&sendCount) > sendsBefore
	})
	st, gerr = svc.dispatchStore.GetRunStopState(context.Background(), leg.RunID)
	if gerr != nil {
		t.Fatalf("GetRunStopState leg: %v", gerr)
	}
	if st.Stopped {
		t.Fatal("leg run_stop fence still armed after authorized redrive — minted turn dies stopped_before_send")
	}
}

// The fence is a linearization device, not a ban: with no authorized
// follow-up it stays armed and a bare startTurn still dies at the CAS —
// only the sanctioned dispatch seams release it.
func TestBug657_LegFenceBlocksUnauthorizedTurn(t *testing.T) {
	var sendCount int32
	svc := newInteractiveService(bug308FullAFRegistry(ProviderKeyCodex, nil, &sendCount), newInteractiveCatalog(), newFakeWorkflowStore())
	svc.SetDispatchStore(NewMemoryDispatchStore())

	leg, err := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun leg: %v", err)
	}
	if _, apiErr := svc.startTurn(leg.RunID, TurnInput{StepID: "step-1", Prompt: "first leg turn"}, "", ""); apiErr != nil {
		t.Fatalf("seed startTurn: %v", apiErr)
	}
	waitLoop(t, "seed leg turn done", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[leg.RunID].turnInFlight
	})
	if apiErr := svc.Interrupt(leg.RunID); apiErr != nil {
		t.Fatalf("Interrupt leg: %s", apiErr.msg)
	}

	sendsBefore := atomic.LoadInt32(&sendCount)
	if _, apiErr := svc.startTurn(leg.RunID, TurnInput{StepID: "step-2", Prompt: "bare follow-up without release"}, "", ""); apiErr == nil {
		waitLoop(t, "bare turn settles", 3*time.Second, func() bool {
			svc.mu.Lock()
			defer svc.mu.Unlock()
			return !svc.runs[leg.RunID].turnInFlight
		})
	}
	if atomic.LoadInt32(&sendCount) > sendsBefore {
		t.Fatal("bare startTurn reached the adapter while the leg stop fence is still armed — fence is not honored")
	}
}
