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

func resolvedRunYolo(t *testing.T, catalog *interactiveCatalog, input StartRunInput) bool {
	t.Helper()
	svc, srv := newTestServerWithCatalog(t, catalog)
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", input, nil)
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
	return rs.yolo
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

func TestCreateRunDoesNotApplyEntryStepYoloToWorkflowLaunch(t *testing.T) {
	catalog := baseTestCatalog()
	catalog.steps["wf-1"][0].YoloMode = true

	if got := resolvedRunYolo(t, catalog, StartRunInput{ProjectID: "proj-1", WorkflowID: "wf-1"}); got {
		t.Fatal("workflow run yolo = true, want false when only the entry step enables yolo_mode")
	}
}

func TestCreateRunResolvesYoloFromWorkflowWhenEnabled(t *testing.T) {
	catalog := baseTestCatalog()
	catalog.workflows["proj-1"][0].YoloMode = true

	if got := resolvedRunYolo(t, catalog, StartRunInput{ProjectID: "proj-1", WorkflowID: "wf-1"}); !got {
		t.Fatal("run yolo = false, want true from workflow yolo_mode")
	}
}

func TestCreateRunResolvesWorkflowYoloEvenWhenEntryStepDefinesModel(t *testing.T) {
	catalog := baseTestCatalog()
	catalog.steps["wf-1"][0].Model = "gpt-5.4-mini"
	catalog.workflows["proj-1"][0].YoloMode = true

	if got := resolvedRunYolo(t, catalog, StartRunInput{ProjectID: "proj-1", WorkflowID: "wf-1"}); !got {
		t.Fatal("run yolo = false, want true from workflow yolo_mode even when the entry step resolves the model")
	}
}

func TestCreateRunResolvesYoloFromSingleStepWhenEnabled(t *testing.T) {
	catalog := baseTestCatalog()
	catalog.steps["wf-1"][0].YoloMode = true

	if got := resolvedRunYolo(t, catalog, StartRunInput{ProjectID: "proj-1", StepID: "step-1"}); !got {
		t.Fatal("single-step run yolo = false, want true from selected step yolo_mode")
	}
}

func TestCreateRunYoloRequestStillEnablesWhenEntryStepDefaultOff(t *testing.T) {
	catalog := baseTestCatalog()

	if got := resolvedRunYolo(t, catalog, StartRunInput{ProjectID: "proj-1", WorkflowID: "wf-1", YoloMode: true}); !got {
		t.Fatal("run yolo = false, want true from explicit start request")
	}
}

func TestCreateRunDoesNotApplyWorkflowYoloDefaultToNormalChat(t *testing.T) {
	catalog := baseTestCatalog()
	catalog.workflows["proj-1"][0].YoloMode = true

	if got := resolvedRunYolo(t, catalog, StartRunInput{ProjectID: "proj-1", ChatMode: "normal_chat"}); got {
		t.Fatal("normal chat run yolo = true, want false unless the chat request explicitly enables it")
	}
}

// registryWithClaudeAvailable returns the default registry but with Claude marked
// Available (backed by the fake adapter) so a run reconciled onto Claude is actually
// selectable in-test — DefaultProviderRegistry keeps Claude as a placeholder.
func registryWithClaudeAvailable() *ProviderRegistry {
	r := DefaultProviderRegistry()
	r.register(ProviderRegistration{
		Key:         ProviderKeyClaude,
		DisplayName: "Claude",
		Status:      ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{
			Streaming: true, Resume: true, ApprovalEvents: true, FileEvents: true,
			SkillSelection: true, Mcp: true, Interrupt: true,
		},
		newAdapter: func() ProviderRuntimeAdapter { return newFakeProviderAdapter(ProviderKeyClaude) },
	})
	return r
}

// TestCreateRunReconcilesProviderToResolvedModel reproduces BUG-171: a workflow/flow
// launch sends providerKey=codex (the desktop passes selectedProvider) but no model,
// and the Step>Flow>Project resolution lands on a Claude model (BUG-162 defaults
// coder/reviewer to claude-haiku). The run must be stamped with the provider the model
// requires (claude), not the mismatched codex it was launched with — otherwise the turn
// dies with "the 'claude-haiku' model is not supported when using Codex".
func TestCreateRunReconcilesProviderToResolvedModel(t *testing.T) {
	catalog := baseTestCatalog()
	catalog.projects[0].Model = "claude-haiku" // project default resolves to a Claude model

	svc := NewInteractiveServiceWith(registryWithClaudeAvailable(), catalog)
	mux := http.NewServeMux()
	svc.RegisterInteractiveRoutes(mux)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Explicit providerKey=codex, mirroring what the desktop's flow launch sends.
	status, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs", StartRunInput{
		ProjectID:   "proj-1",
		WorkflowID:  "wf-1",
		ProviderKey: ProviderKeyCodex,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("start run status=%d body=%s", status, body)
	}
	var h RunHandle
	mustDecode(t, body, &h)
	if h.ProviderKey != ProviderKeyClaude {
		t.Fatalf("ProviderKey = %q, want claude (reconciled from the resolved claude-haiku model, not the launched codex)", h.ProviderKey)
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[h.RunID]
	if rs == nil {
		t.Fatalf("run %q not tracked", h.RunID)
	}
	if rs.providerKey != ProviderKeyClaude || rs.modelName != "claude-haiku" {
		t.Fatalf("run stamped provider=%q model=%q, want claude + claude-haiku", rs.providerKey, rs.modelName)
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
