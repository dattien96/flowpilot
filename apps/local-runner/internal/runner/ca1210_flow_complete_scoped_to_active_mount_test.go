package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// CA-1210 (live run-262417): markFlowRunComplete iterated EVERY step row of
// the run — including rows seeded by other flow mounts sharing the same
// runID. When sprint-042's audit emitted flow_control:done, the terminal
// cascade stamped the parent vibe-tasks graph's `task_plan_reader` row DONE,
// even though that node belongs to a different mounted flow whose lifecycle
// is not over. The seal must be scoped to the flow that actually declared
// done — the run's currently active flow node set.
func TestCA1210_FlowCompleteLeavesForeignMountRowsUntouched(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	parentID := parent.RunID
	svc.mu.Lock()
	rs := svc.runs[parentID]
	rs.flowEngineDriven = true
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "tdd"}, {ID: "coder"}, {ID: "synthesis"}, {ID: "audit"},
	}
	svc.mu.Unlock()

	store, ok := svc.workflowStore.(*fakeWorkflowStore)
	if !ok {
		t.Fatalf("expected fakeWorkflowStore, got %T", svc.workflowStore)
	}
	store.seed(parentID, []RuntimeWorkflowStep{
		// Active sprint-graph rows.
		{ID: "tdd", NodeID: "tdd", Status: StepStatusRunning},
		{ID: "coder", NodeID: "coder", Status: StepStatusPending},
		{ID: "synthesis", NodeID: "synthesis", Status: StepStatusDone},
		{ID: "audit", NodeID: "audit", Status: StepStatusDone},
		// Foreign row seeded by the parent vibe-tasks mount on the same run —
		// the sprint's done-declaration has no authority over it.
		{ID: "task_plan_reader", NodeID: "task_plan_reader", Status: StepStatusPending},
	})

	svc.markFlowRunComplete(context.Background(), parentID)

	if st := svc.lookupFlowStepStatus(parentID, "tdd"); st != StepStatusDone {
		t.Fatalf("running in-scope node must settle DONE on flow complete, got %q", st)
	}
	if st := svc.lookupFlowStepStatus(parentID, "coder"); st != StepStatusSkipped {
		t.Fatalf("pending in-scope node must settle SKIPPED on flow complete, got %q", st)
	}
	if st := svc.lookupFlowStepStatus(parentID, "task_plan_reader"); st != StepStatusPending {
		t.Fatalf("foreign-mount row must be left untouched by another flow's done, got %q", st)
	}
}
