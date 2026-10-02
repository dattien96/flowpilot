package runner

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// BUG-592 (live run-139670): after the tournament child settled, the loop
// parked blocked/paused with gateReason "Resume from reviewer?" and the
// synthesis step stamped WAITING_USER_APPROVAL — yet the desktop rendered no
// card: /questions and /approvals were empty AND no decision frame ever hit
// the mux lane. maybeParkVibeResumeConfirm arms rs.vibeResumeConfirm but
// never emits the agent graph nor marks the lane dirty — the fingerprint is
// unchanged, so the resume-confirm decision is projected yet never flushed.
// Mirrors the BUG-365 requirement-park fix: arming a gate MUST emit.

func bug592SeedResumeRun(t *testing.T) (*InteractiveService, string) {
	t.Helper()
	store := newFakeWorkflowStore()
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)
	runID := "run-resume-confirm"
	rs := &interactiveRun{
		id:          runID,
		projectID:   "proj",
		providerKey: ProviderKeyDevin,
		status:      RunStatusRunning,
		agentStatus: string(RunStatusRunning),
		workingMode: workingmode.Vibe,
		chatFlowRef: workingmode.PackPrefix + "vibe-tasks",
		activeFlowNodes: []agentpack.FlowNode{
			{ID: "reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md"},
			{ID: "synthesis", Behavior: "hub.inline"},
		},
		activeFlowEdges: []agentpack.FlowEdge{
			{From: "reviewer", To: "synthesis", When: "done", Kind: "forward"},
		},
	}
	svc.mu.Lock()
	svc.runs[runID] = rs
	svc.mu.Unlock()
	store.seed(runID, []RuntimeWorkflowStep{
		{NodeID: "reviewer", Status: StepStatusDone},
		{NodeID: "synthesis", Status: StepStatusPending},
	})
	return svc, runID
}

func TestBUG592ResumeConfirmEmitsGateDecision(t *testing.T) {
	svc, runID := bug592SeedResumeRun(t)

	subID, _, _ := svc.subscribeRunUpdates()
	defer svc.unsubscribeRunUpdates(subID)

	svc.maybeParkVibeResumeConfirm(runID)

	svc.mu.Lock()
	armed := svc.runs[runID].vibeResumeConfirm
	dirty := svc.runUpdateSubs[subID].dirty[runID]
	svc.mu.Unlock()
	if !armed {
		t.Fatal("vibeResumeConfirm was never armed — fixture wrong, not the fix")
	}
	if !dirty {
		t.Fatal("resume-confirm gate armed but the lane was never marked dirty — the decision frame never reaches the desktop")
	}

	frames := svc.drainRunUpdates(subID)
	for _, f := range frames {
		if f.RunID != runID || f.Run == nil {
			continue
		}
		for _, d := range f.Run.Decisions {
			if strings.HasPrefix(d.ID, "vibe-gate-resume:") {
				return // gate card reaches the wire
			}
		}
	}
	t.Fatal("dirty frame drained but carried no vibe-gate-resume decision")
}
