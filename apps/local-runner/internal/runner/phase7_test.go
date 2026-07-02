package runner

import (
	"encoding/json"
	"net/http"
	"testing"
)

// ---- T-19: provider placeholders + runner-side capability enforcement ------

// Claude/Gemini are registered placeholders: visible in the registry with disabled
// capabilities, but not selectable for a controlled run.
func TestRegistrySelectableAndDefault(t *testing.T) {
	reg := DefaultProviderRegistry()

	if _, err := reg.Selectable(ProviderKeyCodex); err != nil {
		t.Fatalf("codex should be selectable: %v", err)
	}
	for _, k := range []ProviderKey{ProviderKeyClaude, ProviderKeyGemini} {
		_, err := reg.Selectable(k)
		if err == nil {
			t.Fatalf("%s should not be selectable (placeholder)", k)
		}
		if _, ok := err.(*UnsupportedProviderRuntimeError); !ok {
			t.Fatalf("%s rejection should be typed, got %T", k, err)
		}
	}

	// the default is the first available provider — never a placeholder
	def, ok := reg.DefaultProviderKey()
	if !ok || def != ProviderKeyCodex {
		t.Fatalf("default provider = %q ok=%v, want codex", def, ok)
	}
}

// Starting a run against a disabled provider is rejected runner-side with the typed
// error envelope — the UI gate is a convenience, the runner is the boundary.
func TestStartRunRejectsDisabledProvider(t *testing.T) {
	_, srv := newTestServer(t)

	st, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs",
		map[string]any{"projectId": "proj-web", "workflowId": "wf-feature", "stepId": "step-plan", "providerKey": "claude", "model": "claude-haiku"}, nil)
	if st != http.StatusUnprocessableEntity {
		t.Fatalf("start run on claude status=%d, want 422; body=%s", st, body)
	}
	var env struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &env)
	if env.Error.Code != "provider_unavailable" {
		t.Fatalf("error code = %q, want provider_unavailable; body=%s", env.Error.Code, body)
	}
}

// An empty providerKey defaults to the available provider (codex) and runs normally.
func TestStartRunDefaultsToAvailableProvider(t *testing.T) {
	_, srv := newTestServer(t)
	st, body := doJSON(t, "POST", srv.URL+"/client/workflow-runs",
		map[string]any{"projectId": "proj-web", "workflowId": "wf-feature", "stepId": "step-plan"}, nil)
	if st != http.StatusOK {
		t.Fatalf("default-provider run status=%d body=%s", st, body)
	}
	var h RunHandle
	_ = json.Unmarshal(body, &h)
	if h.ProviderKey != ProviderKeyCodex {
		t.Fatalf("default provider = %q, want codex", h.ProviderKey)
	}
}

// The admin providers endpoint surfaces placeholders with their (disabled)
// capability flags so the UI can show them clearly.
func TestAdminProvidersSurfacesCapabilities(t *testing.T) {
	_, srv := newTestServer(t)
	st, body := doJSON(t, "GET", srv.URL+"/admin/providers", nil, nil)
	if st != http.StatusOK {
		t.Fatalf("providers status=%d", st)
	}
	var regs []ProviderRegistration
	if err := json.Unmarshal(body, &regs); err != nil {
		t.Fatalf("decode providers: %v", err)
	}
	byKey := map[ProviderKey]ProviderRegistration{}
	for _, r := range regs {
		byKey[r.Key] = r
	}
	if byKey[ProviderKeyCodex].Status != ProviderStatusAvailable || !byKey[ProviderKeyCodex].Capabilities.Streaming {
		t.Fatalf("codex should be available + streaming: %+v", byKey[ProviderKeyCodex])
	}
	if byKey[ProviderKeyClaude].Status != ProviderStatusPlaceholder {
		t.Fatalf("claude should be placeholder: %+v", byKey[ProviderKeyClaude])
	}
	if byKey[ProviderKeyClaude].Capabilities.Streaming {
		t.Fatalf("claude (placeholder) should advertise disabled capabilities: %+v", byKey[ProviderKeyClaude].Capabilities)
	}
}
