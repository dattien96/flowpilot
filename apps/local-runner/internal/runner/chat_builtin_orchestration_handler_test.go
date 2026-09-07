package runner

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleListBuiltinOrchestrationOptionsBugMode(t *testing.T) {
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())

	req := httptest.NewRequest(http.MethodGet, "/client/chat/builtin-orchestration-options?subMode=bug", nil)
	rec := httptest.NewRecorder()
	svc.handleListBuiltinOrchestrationOptions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var opts []BuiltinFlowOption
	if err := json.Unmarshal(rec.Body.Bytes(), &opts); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	// review-loop is hidden (selectableIn []) — Bug mode offers nothing.
	for _, opt := range opts {
		if opt.FlowRef == "flowpilot-core-flow-pack/review-loop" {
			t.Fatalf("review-loop must stay hidden, got %#v", opts)
		}
	}
}

func TestHandleListBuiltinOrchestrationOptionsNormalModeIsEmpty(t *testing.T) {
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())

	req := httptest.NewRequest(http.MethodGet, "/client/chat/builtin-orchestration-options?subMode=normal", nil)
	rec := httptest.NewRecorder()
	svc.handleListBuiltinOrchestrationOptions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var opts []BuiltinFlowOption
	if err := json.Unmarshal(rec.Body.Bytes(), &opts); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(opts) != 0 {
		t.Fatalf("expected no options for normal sub-mode, got %#v", opts)
	}
}

func TestHandleListBuiltinOrchestrationOptionsMissingSubModeIsEmpty(t *testing.T) {
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())

	req := httptest.NewRequest(http.MethodGet, "/client/chat/builtin-orchestration-options", nil)
	rec := httptest.NewRecorder()
	svc.handleListBuiltinOrchestrationOptions(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	var opts []BuiltinFlowOption
	if err := json.Unmarshal(rec.Body.Bytes(), &opts); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(opts) != 0 {
		t.Fatalf("expected no options for missing subMode, got %#v", opts)
	}
}
