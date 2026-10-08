package runner

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// BUG-650 (live run-523131): a resume-with-feedback turn answering a parked
// routing decision was allowed to complete prose-only — no
// submit_review_outcome / flow_control call, no edge traversal, no record —
// while the ledger read the resume as "accepted". The fix arms an
// owed-outcome marker on the run when a resume consumes an escalate park so
// the turn-completion fallback validates the routing call (reprompt once,
// then re-park) instead of silently accepting the turn.
func TestBug650_ResumeOnDecisionParkArmsOutcomeMarker(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = "vibe"
	rs.flowEngineDriven = true
	rs.status = RunStatusRunning
	// No continue back-edge — the park cannot be traversed, so resume falls
	// through to the hub dispatch path that owes the outcome.
	rs.activeFlowNodes = []agentpack.FlowNode{{ID: "synthesis", Behavior: "hub.inline"}}
	rs.activeFlowEdges = nil
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Status: "blocked", BlockReason: "escalate",
		GateReason: "writer gate violation — needs human decision",
		Cap: 5, RoundCap: 5, Round: 1,
	})

	if _, err := svc.resumeFlowWithFeedback(parent.RunID, "continue past the card"); err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if !svc.runs[parent.RunID].resumeOwesOutcome {
		t.Fatal("resume-into-decision did not arm the owed-outcome marker — a prose-only turn will be accepted silently")
	}
	if svc.runs[parent.RunID].resumeOutcomeReprompted {
		t.Fatal("reprompt budget spent before the turn even ran")
	}
}

// The reprompt bound is exactly one retry: the first prose-only completion
// re-invokes the hub with the constrained routing face; the second falls
// through to the escalate re-park.
func TestBug650_RepromptBoundedToOne(t *testing.T) {
	svc := bug289Service(t)
	svc.dispatchStore = NewMemoryDispatchStore()
	runID := "run-650b"
	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:                runID,
		status:            RunStatusRunning,
		flowEngineDriven:  true,
		resumeOwesOutcome: true,
		subs:              map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()

	if !svc.repromptResumeOutcomeTurn(runID) {
		t.Fatal("first prose-only resume-decision completion refused the reprompt")
	}
	if svc.repromptResumeOutcomeTurn(runID) {
		t.Fatal("second prose-only completion got another reprompt — unbounded loop")
	}
}
