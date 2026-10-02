package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// BUG-587 (live run-139670): the parent hub's post-turn gate ran the suite
// while the vibe-sprint tdd leg (agent.scaffold) was mid-flight — the suite
// was red BY CONTRACT (stubs not yet filled) and r-tests/r-reg escalated to
// owner debate over an expected state. The writing phase (scaffold → coder)
// owns the suite's color; parent turns in that window must suppress the
// tier-2 test rules.
func TestBUG587_ExpectedRedSuiteWindow(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	nodes := []agentpack.FlowNode{
		{ID: "context", Behavior: "context.produce"},
		{ID: "tdd", Behavior: "agent.scaffold"},
		{ID: "coder", Behavior: "agent.code"},
		{ID: "reviewer", Behavior: "agent.delegate"},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	svc.mu.Unlock()
	svc.markFlowEngineDriven(runID)
	svc.reseedFlowStepRuntime(runID, nodes)

	check := func(want bool, because string) {
		t.Helper()
		svc.mu.Lock()
		got := svc.expectedRedSuiteWindowLocked(svc.runs[runID])
		svc.mu.Unlock()
		if got != want {
			t.Fatalf("%s: expectedRedSuiteWindow=%v, want %v", because, got, want)
		}
	}

	check(false, "all pending: no writer in flight")
	svc.setFlowStepStatus(context.Background(), runID, "tdd", StepStatusRunning)
	check(true, "scaffold step RUNNING — suite red is contractual")
	svc.setFlowStepStatus(context.Background(), runID, "tdd", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), runID, "coder", StepStatusRunning)
	check(true, "coder filling stubs — suite may still be red mid-fill")
	svc.setFlowStepStatus(context.Background(), runID, "coder", StepStatusDone)
	check(false, "writing phase done — parent gate owns the suite again")
}
