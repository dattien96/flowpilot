package runner

import (
	"encoding/json"
	"net/http"
	"testing"
)

// BUG-153: the desktop Flow-mode sidebar needs a client-facing read path over
// the same RuntimeWorkflowStep list the Go orchestrator already tracks, so it
// can show step-by-step runtime state without inferring it from chat text.

func getStepsRuntime(t *testing.T, base, runID string) (int, workflowStepsRuntimeSnapshot) {
	t.Helper()
	status, body := doJSON(t, "GET", base+"/client/workflow-runs/"+runID+"/steps-runtime", nil, nil)
	var v workflowStepsRuntimeSnapshot
	if status == http.StatusOK {
		if err := json.Unmarshal(body, &v); err != nil {
			t.Fatalf("decode steps-runtime: %v", err)
		}
	}
	return status, v
}

func TestWorkflowStepsRuntimeReturnsOrderedStepsForWorkflowRun(t *testing.T) {
	_, srv := newTestServer(t)
	runID := startRun(t, srv.URL)

	status, snap := getStepsRuntime(t, srv.URL, runID)
	if status != http.StatusOK {
		t.Fatalf("steps-runtime status=%d", status)
	}
	if snap.RunID != runID {
		t.Fatalf("runId = %q, want %q", snap.RunID, runID)
	}
	if len(snap.Steps) == 0 {
		t.Fatalf("expected at least one seeded step, got none")
	}
	if snap.Steps[0].Status != StepStatusPending {
		t.Fatalf("first step status = %q, want PENDING", snap.Steps[0].Status)
	}
}

func TestWorkflowStepsRuntimeReflectsTransitionsAndRetry(t *testing.T) {
	svc, srv := newTestServer(t)
	runID := startRun(t, srv.URL)

	_, snap := getStepsRuntime(t, srv.URL, runID)
	stepID := snap.Steps[0].StepID

	if err := svc.workflowStore.ApplyStepTransition(t.Context(), runID, WorkflowStepTransition{
		StepID: stepID,
		Patch: WorkflowStepPatch{
			Status:        StepStatusFailed,
			RejectionNote: strptr("validation failed: exit 1"),
			RetryCount:    intptr(1),
		},
	}); err != nil {
		t.Fatalf("apply transition: %v", err)
	}

	status, snap2 := getStepsRuntime(t, srv.URL, runID)
	if status != http.StatusOK {
		t.Fatalf("steps-runtime status=%d", status)
	}
	got := snap2.Steps[0]
	if got.Status != StepStatusFailed {
		t.Fatalf("status = %q, want FAILED", got.Status)
	}
	if got.RetryCount != 1 {
		t.Fatalf("retryCount = %d, want 1", got.RetryCount)
	}
	if got.RejectionNote != "validation failed: exit 1" {
		t.Fatalf("rejectionNote = %q", got.RejectionNote)
	}
	// The step row evolves in place rather than duplicating (8.4): still one row.
	if len(snap2.Steps) != len(snap.Steps) {
		t.Fatalf("step count changed after retry transition: %d -> %d", len(snap.Steps), len(snap2.Steps))
	}
}

func TestWorkflowStepsRuntimeUnknownRunReturns404(t *testing.T) {
	_, srv := newTestServer(t)
	status, _ := getStepsRuntime(t, srv.URL, "no-such-run")
	if status != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", status)
	}
}

// BUG-158: yolo is a run-wide toggle (not a per-step-type default), and a
// built-in flow node has no provider/model override of its own — its agent
// definition inherits the run's — so the snapshot surfaces the run's own
// provider/model/yolo once at the top level rather than per step.
func TestWorkflowStepsRuntimeIncludesRunLevelProviderModelYolo(t *testing.T) {
	_, srv := newTestServer(t)
	runID := startRunYolo(t, srv.URL)

	status, snap := getStepsRuntime(t, srv.URL, runID)
	if status != http.StatusOK {
		t.Fatalf("steps-runtime status=%d", status)
	}
	if !snap.YoloMode {
		t.Fatalf("YoloMode = %v, want true", snap.YoloMode)
	}
	if snap.Provider == "" {
		t.Fatalf("Provider = %q, want non-empty", snap.Provider)
	}
}
