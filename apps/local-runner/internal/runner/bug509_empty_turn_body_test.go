package runner

// BUG-509 (live run-22241 predecessor probe, 2026-09-26): POST /turns
// silently accepted a body with a misspelled/unknown field — `{"text":"…"}`
// decoded into turnBody leaving Prompt empty, and the turn dispatched a
// no-op provider call (devin answered an empty prompt; the post-turn gate
// then fired on zero output). The operator only discovered the mistake from
// the persisted turns.ndjson row.
//
// Fix: fail closed when the body carries no actionable content at all —
// empty prompt AND no attachments AND no flow-launch selectors. A bare
// {"text": "…"} (or any all-unknown-field body) now gets
// `prompt is required` instead of silently dispatching an empty turn.
// Flow-launch turns (flowRef/subMode), attachment-only turns (Task-052),
// and field-carrying turns stay legal — clients send runId/idempotencyKey
// fields the server doesn't model, so strict DisallowUnknownFields is not
// viable; the content check is the fail-closed seam.

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestBug509_UnknownFieldBodyRejected(t *testing.T) {
	svc, srv := newTestServer(t)
	handle, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui", Cwd: t.TempDir(),
	})
	if apiErr != nil {
		t.Fatalf("createRun: %v", apiErr)
	}

	st, body := doJSON(t, http.MethodPost, srv.URL+"/client/workflow-runs/"+handle.RunID+"/turns", map[string]any{
		"stepId": handle.StepID,
		"text":   "the whole prompt lives in a field the server does not model",
	}, nil)
	if st != http.StatusBadRequest {
		t.Fatalf("unknown-field body must be rejected, got %d body=%s", st, body)
	}
	var resp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if resp.Error.Code != "invalid_request" {
		t.Fatalf("error code got %q want invalid_request", resp.Error.Code)
	}
}

func TestBug509_EmptyPromptWithNoSelectorsRejected(t *testing.T) {
	svc, srv := newTestServer(t)
	handle, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui", Cwd: t.TempDir(),
	})
	if apiErr != nil {
		t.Fatalf("createRun: %v", apiErr)
	}

	// An explicit-but-empty prompt with no attachments/flow selectors is the
	// same no-op footgun — nothing for the provider to do.
	st, body := doJSON(t, http.MethodPost, srv.URL+"/client/workflow-runs/"+handle.RunID+"/turns", map[string]any{
		"stepId": handle.StepID,
		"prompt": "   ",
	}, nil)
	if st != http.StatusBadRequest {
		t.Fatalf("empty-prompt body must be rejected, got %d body=%s", st, body)
	}
}
