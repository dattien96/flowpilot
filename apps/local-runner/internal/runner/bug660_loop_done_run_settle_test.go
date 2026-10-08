package runner

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// BUG-660 (live run-523131): the sprint loop reached "done" but the parent
// run kept reporting status=running for ~7 minutes — nothing re-drove the
// terminal transition until an operator POSTed /resume, whose lazy reconcile
// (ensureTerminalTurnCompletedAfterResumeLocked) finally flipped it. The
// loop-done write and the run-status write are not one ledger update, so any
// completion consumed/lost upstream (gate divert, overlay, racy persist)
// strands the run in a live state forever. The wedge sweep must settle a
// done-loop run via the canonical completion path, durably.
func TestBug660_SweepSettlesDoneLoopRun(t *testing.T) {
	svc := bug289Service(t)
	svc.dispatchStore = NewMemoryDispatchStore()
	runID := "run-660a"

	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:               runID,
		status:           RunStatusRunning,
		agentStatus:      string(RunStatusRunning),
		flowEngineDriven: true,
		activeFlowNodes:  []agentpack.FlowNode{{ID: "audit", Behavior: "artifact.audit_draft"}},
		subs:             map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "done", Cap: 5, RoundCap: 5})

	svc.sweepWedgedFlowWork()

	svc.mu.Lock()
	st := svc.runs[runID].status
	svc.mu.Unlock()
	if st != RunStatusCompleted {
		t.Fatalf("done-loop run left status=%s — completion settle lost", st)
	}

	// The settle must be durable: the workflow store's run-status write, not
	// just the in-memory run, reports terminal — a restart must not resurrect
	// a running row.
	fake, _ := svc.workflowStore.(*fakeWorkflowStore)
	fake.mu.Lock()
	stored := fake.runStatus[runID]
	fake.mu.Unlock()
	if stored != RunStatusEngineDone {
		t.Fatalf("store run status %q after done-loop settle — want %s", stored, RunStatusEngineDone)
	}
}

// A live turn still owns the run — the sweep must not settle out from under
// it even if the loop row already reads done.
func TestBug660_SweepLeavesInFlightTurnAlone(t *testing.T) {
	svc := bug289Service(t)
	svc.dispatchStore = NewMemoryDispatchStore()
	runID := "run-660b"

	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:               runID,
		status:           RunStatusRunning,
		flowEngineDriven: true,
		turnInFlight:     true,
		subs:             map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "done", Cap: 5, RoundCap: 5})

	svc.sweepWedgedFlowWork()

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.runs[runID].status != RunStatusRunning {
		t.Fatalf("sweep settled a run with an in-flight turn: %s", svc.runs[runID].status)
	}
}

// A loop that is still advancing is not a settle target — only the done
// state proves no future producer will flip the run status.
func TestBug660_SweepLeavesRunningLoopAlone(t *testing.T) {
	svc := bug289Service(t)
	svc.dispatchStore = NewMemoryDispatchStore()
	runID := "run-660c"

	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:               runID,
		status:           RunStatusRunning,
		flowEngineDriven: true,
		subs:             map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running", Cap: 5, RoundCap: 5})

	svc.sweepWedgedFlowWork()

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.runs[runID].status != RunStatusRunning {
		t.Fatalf("sweep settled a run whose loop is still running: %s", svc.runs[runID].status)
	}
}
