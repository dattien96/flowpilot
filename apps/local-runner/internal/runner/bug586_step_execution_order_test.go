package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// BUG-586 (live run-139670): when the sprint topology reseeds over the
// already-executed ingest chain, mergeReseedSteps appended the historical
// ingest rows AFTER the fresh sprint rows — the desktop timeline showed ten
// sprint steps above the four cp_reader/cp_validator/cp_lock/task_plan_reader
// rows even though ingest ran first. Execution order (first-seen position)
// must win over the new topology's index order.
func TestBUG586_ReseedKeepsExecutionOrder(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	ingest := []agentpack.FlowNode{
		{ID: "cp_reader", Behavior: "agent.delegate"},
		{ID: "cp_validator", Behavior: "agent.delegate"},
		{ID: "cp_lock", Behavior: "hub.inline"},
		{ID: "task_plan_reader", Behavior: "agent.delegate"},
	}
	svc.reseedFlowStepRuntime(runID, ingest)
	for _, n := range ingest {
		svc.setFlowStepStatus(context.Background(), runID, n.ID, StepStatusDone)
	}

	// Sprint mounts later — the live overlay that reordered the timeline.
	sprint := []agentpack.FlowNode{
		{ID: "preflight_contract_plan", Behavior: "agent.delegate"},
		{ID: "context", Behavior: "agent.delegate"},
		{ID: "tdd", Behavior: "agent.code"},
		{ID: "coder", Behavior: "agent.code"},
		{ID: "reviewer", Behavior: "agent.delegate"},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	svc.reseedFlowStepRuntime(runID, sprint)

	steps, err := svc.workflowStore.LoadRunSteps(context.Background(), runID)
	if err != nil {
		t.Fatalf("LoadRunSteps: %v", err)
	}
	pos := map[string]int{}
	for i, st := range steps {
		pos[st.NodeID] = i
	}
	// Ingest rows must stay ahead of every sprint row — they executed first.
	for _, ing := range ingest {
		for _, sp := range sprint {
			if pos[ing.ID] > pos[sp.ID] {
				t.Fatalf("ingest node %q (pos %d) rendered after sprint node %q (pos %d) — execution order lost",
					ing.ID, pos[ing.ID], sp.ID, pos[sp.ID])
			}
		}
	}
	// Statuses survive the merge (BUG-562 contract unchanged).
	st, _ := stepByID(steps, "cp_reader")
	if st.Status != StepStatusDone {
		t.Fatalf("cp_reader status = %q, want DONE preserved through reseed", st.Status)
	}
}
