package runner

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func mustDecode(t *testing.T, body []byte, out any) {
	t.Helper()
	if err := json.Unmarshal(body, out); err != nil {
		t.Fatalf("decode response: %v (body=%s)", err, body)
	}
}

// BUG-165: a workflow/step run's model must resolve Step > Flow > Project >
// default (SS-05 §2.2/§3, SD-06 §6) even though the desktop client sends no
// explicit model for workflow-mode starts (Req 2 — Flow Mode has no
// chat-controller model picker to source one from). These tests drive
// createRun through a custom fake catalog and observe the resolved model
// indirectly via RunHandle.ProviderKey (derived from whichever model wins).

func newTestServerWithCatalog(t *testing.T, catalog *interactiveCatalog) (*InteractiveService, *httptest.Server) {
	t.Helper()
	svc := NewInteractiveServiceWith(DefaultProviderRegistry(), catalog)
	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return svc, srv
}

func baseTestCatalog() *interactiveCatalog {
	return &interactiveCatalog{
		projects: []Project{{ID: "proj-1", Name: "Proj", Path: "/tmp/proj"}},
		workflows: map[string][]Workflow{
			"proj-1": {{ID: "wf-1", ProjectID: "proj-1", Name: "WF"}},
		},
		steps: map[string][]Step{
			"wf-1": {{ID: "step-1", WorkflowID: "wf-1", Name: "Step", Order: 1}},
		},
	}
}

// resolvedRunModel starts a workflow/step run and returns the model the run
// actually resolved to (interactiveRun.modelName), read directly off the
// in-process run state rather than through ProviderKey — the default test
// registry (DefaultProviderRegistry) only marks Codex as selectable, so every
// case here deliberately uses codex-prefixed models ("gpt-*") to stay
// selectable while still exercising which tier wins.
func resolvedRunModel(t *testing.T, catalog *interactiveCatalog) string {
	t.Helper()
	svc, srv := newTestServerWithCatalog(t, catalog)
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{ProjectID: "proj-1", WorkflowID: "wf-1"}, nil)
	if status != http.StatusOK {
		t.Fatalf("start run status=%d body=%s", status, body)
	}
	var h RunHandle
	mustDecode(t, body, &h)
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs, ok := svc.runs[h.RunID]
	if !ok {
		t.Fatalf("run %q not tracked", h.RunID)
	}
	return rs.modelName
}

func TestCreateRunResolvesModelFromStepWhenSet(t *testing.T) {
	catalog := baseTestCatalog()
	catalog.steps["wf-1"][0].Model = "gpt-5.5-step"
	catalog.workflows["proj-1"][0].Model = "gpt-5.5-flow" // must lose to Step
	catalog.projects[0].Model = "gpt-5.5-project"         // must lose to Step

	if got := resolvedRunModel(t, catalog); got != "gpt-5.5-step" {
		t.Fatalf("resolved model = %q, want gpt-5.5-step (Step tier)", got)
	}
}

func TestCreateRunFallsBackToFlowWhenStepModelEmpty(t *testing.T) {
	catalog := baseTestCatalog()
	// Step has no model.
	catalog.workflows["proj-1"][0].Model = "gpt-5.5-flow"
	catalog.projects[0].Model = "gpt-5.5-project" // must lose to Flow

	if got := resolvedRunModel(t, catalog); got != "gpt-5.5-flow" {
		t.Fatalf("resolved model = %q, want gpt-5.5-flow (Flow tier)", got)
	}
}

func TestCreateRunFallsBackToProjectWhenStepAndFlowModelEmpty(t *testing.T) {
	catalog := baseTestCatalog()
	// Step and Flow have no model.
	catalog.projects[0].Model = "gpt-5.5-project"

	if got := resolvedRunModel(t, catalog); got != "gpt-5.5-project" {
		t.Fatalf("resolved model = %q, want gpt-5.5-project (Project tier)", got)
	}
}

func TestCreateRunFallsBackToHardDefaultWhenNothingConfigured(t *testing.T) {
	catalog := baseTestCatalog()
	// Step, Flow, and Project all have no model.
	if got := resolvedRunModel(t, catalog); got != "gpt-5.4" {
		t.Fatalf("resolved model = %q, want gpt-5.4 (hard default)", got)
	}
}

// TestCreateRunNeverInjectsResolvedModelForChat: Req 1 — normal_chat must keep
// using exactly whatever model the client explicitly selected/sent, never the
// Step > Flow > Project > default resolution (which only applies to workflow/
// step-mode runs with a WorkflowID).
func TestCreateRunNeverInjectsResolvedModelForChat(t *testing.T) {
	catalog := baseTestCatalog()
	catalog.steps["wf-1"][0].Model = "claude-haiku"
	catalog.workflows["proj-1"][0].Model = "claude-haiku"
	catalog.projects[0].Model = "claude-haiku"

	_, srv := newTestServerWithCatalog(t, catalog)
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID: "proj-1",
		ChatMode:  "normal_chat",
		Model:     "gpt-5.5",
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("start run status=%d body=%s", status, body)
	}
	var h RunHandle
	mustDecode(t, body, &h)
	if h.ProviderKey != ProviderKeyCodex {
		t.Fatalf("ProviderKey = %q, want codex (the explicitly-sent chat model, not catalog-resolved)", h.ProviderKey)
	}
}
