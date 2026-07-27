package runner

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
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

// BUG-158: yolo is a run-wide toggle (not a per-step-type default), so the
// snapshot surfaces the run's own yolo once at the top level rather than per
// step. Provider/Model are also surfaced at the top level as the run's own
// baseline — but see BUG-228 below: an individual flow-engine step ROW can
// still carry its own resolved provider/model, distinct from this baseline.
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

// TestWorkflowStepsRuntimeReflectsPerNodeModelOverride is the display-side
// regression test for BUG-228: a flow-engine node whose role has its own
// step_definitions row must show ITS OWN resolved provider/model in the
// step-timeline snapshot, not the run's own baseline posture. Without
// stampFlowNodePosture, flowStepRowsFromNodes never sets a step row's
// Provider/Model at all (unlike the classic planner's static seeding), so
// the desktop step-timeline sidebar had no way to learn a node ran on a
// different model than the run's baseline even after spawnChildRun/
// resolveFlowNodeModel correctly gave it one.
func TestWorkflowStepsRuntimeReflectsPerNodeModelOverride(t *testing.T) {
	reg := registryWithClaudeAvailable()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				if autoAnswerPreflightContractPlanTurn(req, b) {
					return nil
				}
				finalMsg := "ok"
				if strings.Contains(req.Prompt, "fix the crash") {
					finalMsg = "Fixed the null pointer at handler.go:42."
				}
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: finalMsg})
				return nil
			})
		},
	})

	catalog := newInteractiveCatalog()
	catalog.steps["wf-feature"] = append(catalog.steps["wf-feature"], Step{
		ID: "flow-agent-delegate-reviewer", Name: "Flow: Reviewer", Model: "claude-sonnet",
	})

	svc := newInteractiveService(reg, catalog, newFakeWorkflowStore())
	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	// markFlowEngineDriven mirrors what handleStartTurn does for a real
	// workflow-picker launch — without it, reseedFlowStepRuntime/
	// setFlowStepStatus/stampFlowNodePosture all no-op (isFlowEngineDriven
	// gate), and the step rows never get NodeID/Provider/Model populated.
	svc.markFlowEngineDriven(parent.RunID)

	svc.startResolvedFlow(context.Background(), parent.RunID, "flowpilot-core-flow-pack/review-loop", "fix the crash")

	// Assert the reviewer rows already carry their resolved posture right
	// after startResolvedFlow returns — flowStepRowsFromNodes resolves every
	// node's posture at seed time, before anything is spawned, so a user
	// watching the step timeline sees what a step WILL run on immediately,
	// not only once it actually starts. This must hold regardless of the
	// row's status at this exact instant (the fake adapter below completes
	// turns synchronously, so the flow may have already raced ahead to
	// RUNNING/DONE by the time control returns here — the status itself is
	// covered by the wait-loop assertion below; this checkpoint is purely
	// about posture correctness, at whatever status is observed).
	_, seeded := getStepsRuntime(t, srv.URL, parent.RunID)
	for _, st := range seeded.Steps {
		if st.NodeID != "reviewer_correctness" && st.NodeID != "reviewer_security" {
			continue
		}
		if st.Provider != "claude" || st.Model != "claude-sonnet" {
			t.Errorf("%s row (status=%q) = provider=%q model=%q, want claude/claude-sonnet resolved regardless of spawn timing", st.NodeID, st.Status, st.Provider, st.Model)
		}
	}

	// Wait for the cohort to actually spawn (status leaves PENDING) and
	// confirm the posture is still correct once stampFlowNodePosture re-stamps
	// it at RUNNING time — a regression there could only show up after spawn.
	waitLoop(t, "reviewer cohort auto-spawned", 3*time.Second, func() bool {
		_, snap := getStepsRuntime(t, srv.URL, parent.RunID)
		reviewerRows := 0
		for _, st := range snap.Steps {
			if (st.NodeID == "reviewer_correctness" || st.NodeID == "reviewer_security") && st.Status != StepStatusPending {
				reviewerRows++
			}
		}
		return reviewerRows == 2
	})

	status, snap := getStepsRuntime(t, srv.URL, parent.RunID)
	if status != http.StatusOK {
		t.Fatalf("steps-runtime status=%d", status)
	}
	for _, st := range snap.Steps {
		switch st.NodeID {
		case "coder":
			if st.Provider != "codex" || st.Model != "gpt-5.4-mini" {
				t.Errorf("coder step row = provider=%q model=%q, want codex/gpt-5.4-mini (flow's own baseline)", st.Provider, st.Model)
			}
		case "reviewer_correctness", "reviewer_security":
			if st.Provider != "claude" || st.Model != "claude-sonnet" {
				t.Errorf("%s step row = provider=%q model=%q, want claude/claude-sonnet (its own flow-agent-delegate-reviewer row)", st.NodeID, st.Provider, st.Model)
			}
		}
	}
}
