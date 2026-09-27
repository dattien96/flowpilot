package runner

// BUG-510 (deep review B-6, 2026-09-26): GET
// /client/workflow-runs/{runId}/steps-runtime returns 404 for a run whose
// durable session row exists but is absent from s.runs — the same post-
// restart asymmetry BUG-508 fixed on the run snapshot endpoint. Steps are
// durable (workflowStore.LoadRunSteps), so the endpoint can serve them for
// any run the durable store proves exists; only the in-memory membership
// check was failing.
// Observed shape: post-restart, GET run → 200 (BUG-508 fallback) while GET
// steps-runtime → run_not_found on the same run.

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestBug510_StepsRuntimeFallsBackToDurableSession(t *testing.T) {
	store := newFakeWorkflowStore()
	svc, srv := newTestServerWith(t, DefaultProviderRegistry(), &stubCatalogStore{}, store)

	// A completed run that exists only in durable rows — the post-restart
	// shape BUG-508 covered for the snapshot endpoint.
	store.sessions["run-6010"] = ProviderSessionState{
		RunID:             "run-6010",
		ProjectID:         "proj-bed",
		ProviderKey:       ProviderKeyGrok,
		ProviderSessionID: "01a0d-SESSION",
		WorkingDirectory:  t.TempDir(),
		Status:            RunStatusCompleted,
	}
	store.seed("run-6010", []RuntimeWorkflowStep{
		{ID: "st-1", StepType: "plan_writer", Status: StepStatusDone, NodeID: "plan_writer"},
		{ID: "st-2", StepType: "coder", Status: StepStatusDone, NodeID: "coder"},
	})

	svc.mu.Lock()
	_, inMemory := svc.runs["run-6010"]
	svc.mu.Unlock()
	if inMemory {
		t.Fatal("precondition: run-6010 must not be in s.runs")
	}

	st, body := doJSON(t, http.MethodGet, srv.URL+"/client/workflow-runs/run-6010/steps-runtime", nil, nil)
	if st != http.StatusOK {
		t.Fatalf("durable-backed run's steps must resolve, got %d body=%s", st, body)
	}
	var view struct {
		RunID string `json:"runId"`
		Steps []struct {
			StepID   string `json:"stepId"`
			StepType string `json:"stepType"`
			Status   string `json:"status"`
			NodeID   string `json:"nodeId"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(body, &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(view.Steps) != 2 || view.Steps[0].NodeID != "plan_writer" || view.Steps[1].Status != "DONE" {
		t.Fatalf("steps must project the durable rows, got %+v", view.Steps)
	}
}

// Round 2: the durable fallback must hydrate the response ENVELOPE too —
// Model/YoloMode were left empty even though ProviderSessionState carries
// them, so a restarted runner reported a provider but no binding detail.
func TestBug510_StepsRuntimeHydratesModelAndYolo(t *testing.T) {
	store := newFakeWorkflowStore()
	svc, srv := newTestServerWith(t, DefaultProviderRegistry(), &stubCatalogStore{}, store)
	_ = svc

	store.sessions["run-6011"] = ProviderSessionState{
		RunID:            "run-6011",
		ProjectID:        "proj-bed",
		ProviderKey:      ProviderKeyDevin,
		ModelName:        "devin/swe-2",
		Yolo:             true,
		WorkingDirectory: t.TempDir(),
		Status:           RunStatusCompleted,
	}
	store.seed("run-6011", []RuntimeWorkflowStep{
		{ID: "st-1", StepType: "coder", Status: StepStatusDone, NodeID: "coder", Model: "devin/swe-2", YoloMode: true},
	})

	st, body := doJSON(t, http.MethodGet, srv.URL+"/client/workflow-runs/run-6011/steps-runtime", nil, nil)
	if st != http.StatusOK {
		t.Fatalf("durable-backed run must resolve, got %d body=%s", st, body)
	}
	var view struct {
		Provider string `json:"provider"`
		Model    string `json:"model"`
		YoloMode bool   `json:"yoloMode"`
		Steps    []struct {
			Model    string `json:"model"`
			YoloMode bool   `json:"yoloMode"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(body, &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.Model != "devin/swe-2" || !view.YoloMode {
		t.Fatalf("envelope must hydrate model/yolo from the durable session, got model=%q yolo=%v", view.Model, view.YoloMode)
	}
	if len(view.Steps) != 1 || view.Steps[0].Model != "devin/swe-2" {
		t.Fatalf("step-level model must keep the durable step row's value, got %+v", view.Steps)
	}
}

func TestBug510_StepsRuntimeUnknownRunStill404s(t *testing.T) {
	store := newFakeWorkflowStore()
	_, srv := newTestServerWith(t, DefaultProviderRegistry(), &stubCatalogStore{}, store)

	st, body := doJSON(t, http.MethodGet, srv.URL+"/client/workflow-runs/run-nothing/steps-runtime", nil, nil)
	if st != http.StatusNotFound {
		t.Fatalf("a run with no durable row must still 404, got %d body=%s", st, body)
	}
}
