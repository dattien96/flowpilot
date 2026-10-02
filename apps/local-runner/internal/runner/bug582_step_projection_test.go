package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// BUG-582 (live run-100368): the step-runtime projection leaked transient
// overlay rows (debate_trigger/owner_1/owner_2/debate_synthesis seeded while
// the owner-debate graph was mounted) into the sprint timeline forever —
// restore reseeds sprint nodes but BUG-562 merge keeps overlay rows as
// "historical", so the desktop rendered ~14-15 steps for a 9-node sprint.
// The timeline must project only the run's CURRENT topology: active flow
// nodes, plus parked (suspended) sprint nodes while an overlay is mounted.

func bug582SprintNodes() []agentpack.FlowNode {
	ids := []string{"preflight_contract_plan", "preflight_contract_freeze", "context", "tdd", "coder", "validate", "reviewer", "synthesis", "audit"}
	out := make([]agentpack.FlowNode, 0, len(ids))
	for _, id := range ids {
		out = append(out, agentpack.FlowNode{ID: id})
	}
	return out
}

func bug582DebateNodes() []agentpack.FlowNode {
	ids := []string{"debate_trigger", "owner_1", "owner_2", "debate_synthesis"}
	out := make([]agentpack.FlowNode, 0, len(ids))
	for _, id := range ids {
		out = append(out, agentpack.FlowNode{ID: id})
	}
	return out
}

func bug582StepIDs(t *testing.T, snap workflowStepsRuntimeSnapshot) map[string]bool {
	t.Helper()
	ids := map[string]bool{}
	for _, st := range snap.Steps {
		ids[st.NodeID] = true
	}
	return ids
}

func TestBug582_StepsRuntimeDropsSettledOverlayRows(t *testing.T) {
	svc := bug289Service(t)
	svc.mu.Lock()
	rs := &interactiveRun{
		id:               "run-582a",
		status:           RunStatusRunning,
		flowEngineDriven: true,
		activeFlowNodes:  bug582SprintNodes(), // post-restore: sprint topology again
		subs:             map[int64]chan ProviderEvent{},
	}
	svc.runs[rs.id] = rs
	svc.mu.Unlock()

	// Durable store carries BOTH sprint rows and the leftover debate overlay
	// rows (merge kept them as historical on restore reseed).
	rows := svc.flowStepRowsFromNodes(context.Background(), rs.id,
		append(bug582SprintNodes(), bug582DebateNodes()...), StepStatusDone, "")
	svc.workflowStore.(*fakeWorkflowStore).seed(rs.id, rows)

	snap, apiErr := svc.workflowStepsRuntime(context.Background(), rs.id)
	if apiErr != nil {
		t.Fatalf("workflowStepsRuntime: %v", apiErr)
	}
	ids := bug582StepIDs(t, snap)
	if len(snap.Steps) != len(bug582SprintNodes()) {
		t.Fatalf("expected %d sprint steps, got %d: %v", len(bug582SprintNodes()), len(snap.Steps), snap.Steps)
	}
	for _, overlayID := range []string{"debate_trigger", "owner_1", "owner_2", "debate_synthesis"} {
		if ids[overlayID] {
			t.Fatalf("settled overlay row %q must not project into the sprint timeline", overlayID)
		}
	}
	if !ids["audit"] {
		t.Fatal("sprint node audit must still project")
	}
}

// While the overlay is actually mounted, its nodes ARE the active topology —
// they stay visible (with the parked sprint rows) and disappear on restore.
func TestBug582_MountedOverlayProjectsUnion(t *testing.T) {
	svc := bug289Service(t)
	svc.mu.Lock()
	rs := &interactiveRun{
		id:               "run-582b",
		status:           RunStatusRunning,
		flowEngineDriven: true,
		activeFlowNodes:  bug582DebateNodes(),  // debate mounted
		vibeParkedNodes:  bug582SprintNodes(),  // sprint suspended underneath
		subs:             map[int64]chan ProviderEvent{},
	}
	svc.runs[rs.id] = rs
	svc.mu.Unlock()

	rows := svc.flowStepRowsFromNodes(context.Background(), rs.id,
		append(bug582SprintNodes(), bug582DebateNodes()...), StepStatusDone, "")
	svc.workflowStore.(*fakeWorkflowStore).seed(rs.id, rows)

	snap, apiErr := svc.workflowStepsRuntime(context.Background(), rs.id)
	if apiErr != nil {
		t.Fatalf("workflowStepsRuntime: %v", apiErr)
	}
	ids := bug582StepIDs(t, snap)
	want := len(bug582SprintNodes()) + len(bug582DebateNodes())
	if len(snap.Steps) != want {
		t.Fatalf("mounted overlay must project union parked+active: want %d, got %d", want, len(snap.Steps))
	}
	if !ids["owner_1"] || !ids["audit"] {
		t.Fatal("union must include both debate and parked sprint nodes")
	}
}

// A run with no tracked topology (restarted, durable-only rows) keeps every
// row — the projection must fail open for replayed history.
func TestBug582_NoTrackedTopologyKeepsAllRows(t *testing.T) {
	svc := bug289Service(t)
	svc.mu.Lock()
	rs := &interactiveRun{
		id:               "run-582c",
		status:           RunStatusCompleted,
		flowEngineDriven: true,
		subs:             map[int64]chan ProviderEvent{},
	}
	svc.runs[rs.id] = rs
	svc.mu.Unlock()

	rows := svc.flowStepRowsFromNodes(context.Background(), rs.id,
		append(bug582SprintNodes(), bug582DebateNodes()...), StepStatusDone, "")
	svc.workflowStore.(*fakeWorkflowStore).seed(rs.id, rows)

	snap, apiErr := svc.workflowStepsRuntime(context.Background(), rs.id)
	if apiErr != nil {
		t.Fatalf("workflowStepsRuntime: %v", apiErr)
	}
	if len(snap.Steps) != len(rows) {
		t.Fatalf("no tracked topology must fail open: want %d rows, got %d", len(rows), len(snap.Steps))
	}
}
