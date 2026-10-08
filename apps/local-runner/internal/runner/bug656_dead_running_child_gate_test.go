package runner

import (
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// BUG-656 (live run-523131): maybeParkVibeResumeConfirm treated ANY child
// stamped `running` as live work — a leg whose turn died at provider-limit
// kept the stamp (BUG-647 class) and silently vetoed the resume-confirm
// gate forever, leaving the parent in an unarmed cancelled dead end with no
// card. The liveness test must be a real driver (in-flight turn or fresh
// provider traffic), not the status string.

func bug656Parent(t *testing.T, svc *InteractiveService) string {
	t.Helper()
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = workingmode.Vibe
	rs.status = RunStatusCancelled
	rs.flowEngineDriven = true
	rs.activeFlowNodes = []agentpack.FlowNode{{ID: "tdd"}, {ID: "coder"}}
	rs.activeFlowEdges = []agentpack.FlowEdge{{From: "tdd", To: "coder", When: "done", Kind: "forward"}}
	svc.mu.Unlock()
	// tdd DONE, coder unfinished → pendingVibeResumeFromNode returns "tdd".
	fake, _ := svc.workflowStore.(*fakeWorkflowStore)
	fake.seed(parent.RunID, []RuntimeWorkflowStep{
		{ID: "tdd", NodeID: "tdd", Status: StepStatusDone},
		{ID: "coder", NodeID: "coder", Status: StepStatusPending},
	})
	return parent.RunID
}

func bug656GateArmed(svc *InteractiveService, runID string) bool {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	return rs != nil && rs.vibeResumeConfirm
}

func TestBug656_DeadRunningChildDoesNotBlockResumeConfirm(t *testing.T) {
	svc, _ := newTestServer(t)
	parentID := bug656Parent(t, svc)

	// Dead leg: stamped running, no turn in flight, last provider traffic
	// long past the wedge bound (provider-limit kill residue).
	svc.mu.Lock()
	svc.runs["run-656-dead"] = &interactiveRun{
		id:                  "run-656-dead",
		parentRunID:         parentID,
		label:               "tdd",
		status:              RunStatusRunning,
		createdAt:           time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339Nano),
		lastProviderEventAt: time.Now().UTC().Add(-10 * time.Minute),
		subs:                map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()

	svc.maybeParkVibeResumeConfirm(parentID)

	if !bug656GateArmed(svc, parentID) {
		t.Fatal("dead running child stamp vetoed the resume-confirm gate")
	}
}

func TestBug656_LiveRunningChildStillDefers(t *testing.T) {
	svc, _ := newTestServer(t)
	parentID := bug656Parent(t, svc)

	// Live leg: running with a turn actually in flight.
	svc.mu.Lock()
	svc.runs["run-656-live"] = &interactiveRun{
		id:            "run-656-live",
		parentRunID:   parentID,
		label:         "coder",
		status:        RunStatusRunning,
		turnInFlight:  true,
		createdAt:     time.Now().UTC().Format(time.RFC3339Nano),
		subs:          map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()

	svc.maybeParkVibeResumeConfirm(parentID)

	if bug656GateArmed(svc, parentID) {
		t.Fatal("resume-confirm gate armed over a child with a live in-flight turn")
	}
}
