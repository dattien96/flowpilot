package app

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
	"flowpilot-runner/internal/tui/prefs"
)

// Posture model semantics after CA-685 (operator decision, superseding the
// CA-679 interim rule): scan/plan/code ALWAYS come back with their pinned
// model on restart restore and /new re-apply — the pin only changes via
// /mode-setup or Desktop settings; mid-session /model changes do not survive
// a restart while a real posture is active. The "non" posture applies no
// pins at all (see ca685_posture_non_mode_test.go). The CA-679 tests below
// were rewritten to the CA-685 spec the same day they were authored; no
// legacy (pre-2026-08-29) test was edited.

func postureModelRestoreServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/client/chat-posture" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func runRestore(t *testing.T, m *AppModel) {
	t.Helper()
	m.chatPosturePending = "restore" // firstLoad arms this before cmdLoadChatPosture
	cmd := m.cmdLoadChatPosture()
	cp, ok := cmd().(chatPostureMsg)
	if !ok || cp.Err != nil {
		t.Fatalf("load chat posture failed: %+v", cp)
	}
	m.chatPostureCmdFromPending(cp.Cfg)
}

func TestRestorePostureAppliesPinnedModelSameProvider(t *testing.T) {
	srv := postureModelRestoreServer(t,
		`{"active":"code","profiles":{"scan":{},"plan":{},"code":{"model":"grok-4.5"}}}`)

	m := New(config.ChatConfig{Provider: "grok"}, srv.URL)
	m.mode = ModeChat
	m.chatPosture = "code"
	m.provider = "grok"
	m.model = "grok-4.6" // user's mid-session /model change
	m.sessionDefaultsLoaded = false

	runRestore(t, m)
	if m.model != "grok-4.5" {
		t.Fatalf("restore in a real posture must show the pinned model, got %q", m.model)
	}
	if m.chatPosture != "code" {
		t.Fatalf("posture after restore = %q, want code", m.chatPosture)
	}
	if m.chatPostureDirty {
		t.Fatal("restore must NOT mark dirty")
	}
}

func TestRestorePosturePinWinsOverUserModelPersistedPrefs(t *testing.T) {
	srv := postureModelRestoreServer(t,
		`{"active":"scan","profiles":{"scan":{"model":"grok-4.5","yolo":true},"plan":{},"code":{}}}`)

	prefFile := filepath.Join(t.TempDir(), "tui-session.json")
	t.Setenv("FLOWPILOT_TUI_SESSION_FILE", prefFile)

	m := New(config.ChatConfig{Provider: "grok"}, srv.URL)
	m.mode = ModeChat
	m.chatPosture = "scan"
	m.provider = "grok"
	m.model = "grok-4.6"
	m.persistSessionPrefs() // simulates the mid-session /model save

	runRestore(t, m) // restart restore in a real posture: the pin wins (CA-685)

	saved, _, err := prefs.Load()
	if err != nil {
		t.Fatalf("load persisted prefs: %v", err)
	}
	if saved.Model != "grok-4.5" {
		t.Fatalf("posture pin must be the persisted model after restore, got %q", saved.Model)
	}
}

func TestRestorePostureModelPinInfersProviderFromModel(t *testing.T) {
	// BUG-330 guard: a pinned model without an explicit provider pin must not
	// stamp a foreign provider — the provider is inferred from the model.
	srv := postureModelRestoreServer(t,
		`{"active":"plan","profiles":{"scan":{},"plan":{"model":"grok-4.5"},"code":{}}}`)

	m := New(config.ChatConfig{Provider: "opencode"}, srv.URL)
	m.mode = ModeChat
	m.chatPosture = "code"
	m.provider = "opencode"
	m.model = "opencode-go/muse-spark-1.2-contributor"
	m.providers = []client.Provider{
		{Key: "opencode", Models: []client.ProviderModel{{ID: "opencode-go/muse-spark-1.2-contributor"}}},
		{Key: "grok", Models: []client.ProviderModel{{ID: "grok-4.5"}}},
	}
	m.sessionDefaultsLoaded = false

	runRestore(t, m)
	if m.model != "grok-4.5" {
		t.Fatalf("pin model must apply, got %q", m.model)
	}
	if m.provider != "grok" {
		t.Fatalf("provider must follow the pinned model, got %q", m.provider)
	}
}

func TestRestorePostureAdoptsModelPinWhenSessionModelEmpty(t *testing.T) {
	srv := postureModelRestoreServer(t,
		`{"active":"plan","profiles":{"scan":{},"plan":{"model":"opus"},"code":{}}}`)

	m := New(config.ChatConfig{}, srv.URL)
	m.mode = ModeChat
	m.chatPosture = "code"
	m.provider = "claude"
	m.model = "" // user never picked a model — the pin is a sane default
	m.providers = []client.Provider{{Key: "claude", Models: []client.ProviderModel{{ID: "opus"}}}}
	m.sessionDefaultsLoaded = false

	runRestore(t, m)
	if m.model != "opus" {
		t.Fatalf("empty session model must adopt the posture pin, got %q", m.model)
	}
}

func TestRestorePosturePinsModelWhenProfileSwitchesProvider(t *testing.T) {
	srv := postureModelRestoreServer(t,
		`{"active":"plan","profiles":{"scan":{},"plan":{"provider":"claude","model":"opus"},"code":{}}}`)

	m := New(config.ChatConfig{Provider: "codex"}, srv.URL)
	m.mode = ModeChat
	m.chatPosture = "code"
	m.provider = "codex"
	m.model = "o3" // invalid for the pinned claude provider
	m.providers = []client.Provider{
		{Key: "codex", Models: []client.ProviderModel{{ID: "o3"}}},
		{Key: "claude", Models: []client.ProviderModel{{ID: "opus"}, {ID: "sonnet"}}},
	}
	m.sessionDefaultsLoaded = false

	runRestore(t, m)
	if m.provider != "claude" {
		t.Fatalf("provider must switch to the pinned claude, got %q", m.provider)
	}
	if m.model != "opus" {
		t.Fatalf("provider switch must adopt the pinned model, got %q", m.model)
	}
}

func TestNewReapplyPinsModelSameProvider(t *testing.T) {
	srv := postureModelRestoreServer(t,
		`{"active":"code","profiles":{"scan":{},"plan":{},"code":{"model":"grok-4.5"}}}`)

	m := New(config.ChatConfig{Provider: "grok"}, srv.URL)
	m.mode = ModeChat
	m.chatPosture = "code"
	m.provider = "grok"
	m.model = "grok-4.6"

	// /new re-applies the already-active posture — the pin wins (CA-685);
	// only reasoning is kept (CA-641).
	cmd := m.cmdLoadChatPosture()
	cp := cmd().(chatPostureMsg)
	m.chatPosturePending = "apply:" + m.activePosture()
	m.chatPostureCmdFromPending(cp.Cfg)
	if m.model != "grok-4.5" {
		t.Fatalf("/new re-apply must pin the posture model, got %q", m.model)
	}
}

func TestNewReapplyPinsModelWhenProfileSwitchesProvider(t *testing.T) {
	srv := postureModelRestoreServer(t,
		`{"active":"code","profiles":{"scan":{"provider":"claude","model":"sonnet"},"plan":{},"code":{}}}`)

	m := New(config.ChatConfig{Provider: "codex"}, srv.URL)
	m.mode = ModeChat
	m.chatPosture = "code"
	m.provider = "codex"
	m.model = "o3"
	m.providers = []client.Provider{
		{Key: "codex", Models: []client.ProviderModel{{ID: "o3"}}},
		{Key: "claude", Models: []client.ProviderModel{{ID: "sonnet"}, {ID: "opus"}}},
	}

	// /new re-apply where the (edited) profile now demands a different provider:
	// the user's model is invalid there, so the pin wins.
	cmd := m.cmdLoadChatPosture()
	cp := cmd().(chatPostureMsg)
	m.chatPosturePending = "apply:scan"
	m.chatPostureCmdFromPending(cp.Cfg)
	if m.provider != "claude" || m.model != "sonnet" {
		t.Fatalf("provider-switch re-apply must pin profile selection, got provider=%q model=%q", m.provider, m.model)
	}
}

func TestRealSwitchStillPinsModelOverUserChoice(t *testing.T) {
	// A real posture switch (/mode plan) keeps pinning the profile model even
	// when the provider is unchanged — explicit user action on the SSOT.
	srv := postureModelRestoreServer(t,
		`{"active":"code","profiles":{"scan":{},"plan":{"model":"grok-4.6"},"code":{}}}`)

	m := New(config.ChatConfig{Provider: "grok"}, srv.URL)
	m.mode = ModeChat
	m.provider = "grok"
	m.model = "grok-4.5"
	m.providers = []client.Provider{{Key: "grok", Models: []client.ProviderModel{{ID: "grok-4.5"}, {ID: "grok-4.6"}}}}

	cmd := m.cmdLoadChatPosture()
	cp := cmd().(chatPostureMsg)
	m.chatPosturePending = "apply:plan"
	m.chatPostureCmdFromPending(cp.Cfg)
	if m.model != "grok-4.6" {
		t.Fatalf("real switch must pin plan model, got %q", m.model)
	}
}
