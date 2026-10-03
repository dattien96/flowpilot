package runner

import (
	"context"
	"testing"
)

// BUG-625 (live run-174243): while the owner-debate overlay is mounted the
// BUG-582 union projection shows debate rows inline with the sprint rows —
// the desktop numbers them 12-15 inside the task timeline — and parked
// sprint rows keep their last stamp, so the sprint's hub.inline node reads
// "running" even though the whole sprint is suspended. The projection must
// flag each row so the client can split overlay work out of the task chain
// and render parked rows as suspended rather than in-flight.

func bug625StepFlags(t *testing.T, snap workflowStepsRuntimeSnapshot) map[string]workflowStepRuntimeView {
	t.Helper()
	out := map[string]workflowStepRuntimeView{}
	for _, st := range snap.Steps {
		out[st.NodeID] = st
	}
	return out
}

func TestBUG625_MountedOverlayFlagsRows(t *testing.T) {
	svc := bug289Service(t)
	svc.mu.Lock()
	rs := &interactiveRun{
		id:               "run-625a",
		status:           RunStatusRunning,
		flowEngineDriven: true,
		activeFlowNodes:  bug582DebateNodes(), // debate mounted
		vibeParkedNodes:  bug582SprintNodes(), // sprint suspended underneath
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
	views := bug625StepFlags(t, snap)
	for _, sprintID := range []string{"tdd", "coder", "synthesis"} {
		v, ok := views[sprintID]
		if !ok {
			t.Fatalf("sprint row %q must still project", sprintID)
		}
		if !v.Parked {
			t.Fatalf("sprint row %q must be flagged parked while overlay mounted", sprintID)
		}
		if v.Overlay {
			t.Fatalf("sprint row %q must not be flagged overlay", sprintID)
		}
	}
	for _, debateID := range []string{"debate_trigger", "owner_1", "owner_2", "debate_synthesis"} {
		v, ok := views[debateID]
		if !ok {
			t.Fatalf("overlay row %q must still project", debateID)
		}
		if !v.Overlay {
			t.Fatalf("debate row %q must be flagged overlay", debateID)
		}
		if v.Parked {
			t.Fatalf("debate row %q must not be flagged parked", debateID)
		}
	}
}

// After restore the sprint owns the graph again — no parked, no overlay.
func TestBUG625_PostRestoreClearsFlags(t *testing.T) {
	svc := bug289Service(t)
	svc.mu.Lock()
	rs := &interactiveRun{
		id:               "run-625b",
		status:           RunStatusRunning,
		flowEngineDriven: true,
		activeFlowNodes:  bug582SprintNodes(),
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
	for _, st := range snap.Steps {
		if st.Parked || st.Overlay {
			t.Fatalf("post-restore row %q must carry no parked/overlay flag", st.NodeID)
		}
	}
}
