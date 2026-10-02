package runner

import (
	"context"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// BUG-577 (live run-100368): crossing a vibe-sprint boundary used to leave the
// PREVIOUS sprint's terminal step rows standing — reseedFlowStepRuntime's
// BUG-562 merge preserves DONE rows, and startTakenVibeSprint /
// maybeStartNextVibeSprint never reset them. Sprint-2 then started with
// coder/validate/reviewer/synthesis/audit all reading DONE: the timeline lied,
// maybeReparkVibeSprintBoundary's "audit no longer DONE" guard was defeated,
// and vibeSprintEvidenceComplete could auto-finalize the new sprint's audit
// on stale evidence. A sprint start is a fresh round for the whole node set —
// every sprint node must reset to PENDING before the entry stamp RUNNING.
func TestBUG577BoundaryStartReseedsDownstreamSteps(t *testing.T) {
	svc, _ := newTestServer(t)
	runID := armBoundaryRun(t, svc, ProviderKeyCodex, workingmode.Vibe, boundaryTestPlan, 1)

	// Simulate sprint-1 having settled the full chain — the stale rows a
	// boundary start must not inherit.
	svc.reseedFlowStepRuntime(runID, []agentpack.FlowNode{
		{ID: "preflight_contract_plan", Behavior: "agent.delegate"},
		{ID: "tdd", Behavior: "agent.scaffold"},
		{ID: "coder", Behavior: "agent.code"},
		{ID: "validate", Behavior: "command.validate"},
		{ID: "reviewer", Behavior: "agent.delegate"},
		{ID: "synthesis", Behavior: "hub.inline"},
		{ID: "audit", Behavior: "artifact.audit_draft"},
	})
	for _, id := range []string{"preflight_contract_plan", "tdd", "coder", "validate", "reviewer", "synthesis", "audit"} {
		svc.setFlowStepStatus(context.Background(), runID, id, StepStatusDone)
	}

	if !svc.maybeParkVibeSprintBoundary(context.Background(), runID, "audit", false) {
		t.Fatal("must park first")
	}
	if apiErr := svc.SubmitGateDecision(runID, "ok", ""); apiErr != nil {
		t.Fatalf("ok: %v", apiErr)
	}

	waitLoop(t, "sprint-2 steps reseeded pending", 5*time.Second, func() bool {
		// Every downstream node must no longer read DONE. The entry node may
		// already be RUNNING — anything but a stale terminal/leaked DONE.
		for _, id := range []string{"tdd", "coder", "validate", "reviewer", "synthesis", "audit"} {
			if st := svc.lookupFlowStepStatus(runID, id); st == StepStatusDone {
				return false
			}
		}
		return true
	})

	// The new sprint's entry work must be genuinely in flight.
	svc.mu.Lock()
	rs := svc.runs[runID]
	idx := 0
	if rs != nil {
		idx = rs.vibeSprintIndex
	}
	svc.mu.Unlock()
	if idx != 2 {
		t.Fatalf("index=%d want 2 (sprint-2 taken)", idx)
	}
}
