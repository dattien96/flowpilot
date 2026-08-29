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

// CA-679: resume-flavored posture applications (restart restore, /new
// re-applying the already-active posture) must keep the user's persisted
// /model choice instead of re-pinning the posture profile's model — the
// CA-638 analog for model. Operator report: an opencode model selected in
// chat reverted to the posture's grok-4.5 pin on every TUI restart, and the
// clobbered value was re-persisted to session prefs, destroying the choice.
// Additive — the chat_posture_restore_test.go / ca638 / ca641 suites are
// untouched.

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

func TestRestorePostureKeepsUserModelSameProvider(t *testing.T) {
	srv := postureModelRestoreServer(t,
		`{"active":"code","profiles":{"scan":{},"plan":{},"code":{"model":"grok-4.5"}}}`)

	m := New(config.ChatConfig{Provider: "grok"}, srv.URL)
	m.mode = ModeChat
	m.chatPosture = "code"
	m.provider = "grok"
	m.model = "opencode/muse-spark-1.2-contributor-free" // user's /model choice
	m.sessionDefaultsLoaded = false

	runRestore(t, m)
	if m.model != "opencode/muse-spark-1.2-contributor-free" {
		t.Fatalf("restore must keep user model, got %q", m.model)
	}
	if m.chatPosture != "code" {
		t.Fatalf("posture after restore = %q, want code", m.chatPosture)
	}
	if m.chatPostureDirty {
		t.Fatal("restore must NOT mark dirty")
	}
}

func TestRestorePostureDoesNotClobberPersistedPrefs(t *testing.T) {
	srv := postureModelRestoreServer(t,
		`{"active":"scan","profiles":{"scan":{"model":"grok-4.5","yolo":true},"plan":{},"code":{}}}`)

	prefFile := filepath.Join(t.TempDir(), "tui-session.json")
	t.Setenv("FLOWPILOT_TUI_SESSION_FILE", prefFile)

	m := New(config.ChatConfig{Provider: "grok"}, srv.URL)
	m.mode = ModeChat
	m.chatPosture = "scan"
	m.provider = "opencode"
	m.model = "opencode/muse-spark-1.2-contributor-free"
	m.persistSessionPrefs() // simulates the /model save from the previous run

	runRestore(t, m) // restart restore must not overwrite the saved model

	saved, _, err := prefs.Load()
	if err != nil {
		t.Fatalf("load persisted prefs: %v", err)
	}
	if saved.Model != "opencode/muse-spark-1.2-contributor-free" {
		t.Fatalf("persisted model was clobbered by posture restore, got %q", saved.Model)
	}
	if m.model != "opencode/muse-spark-1.2-contributor-free" {
		t.Fatalf("session model changed by restore, got %q", m.model)
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

func TestNewReapplyKeepsUserModelSameProvider(t *testing.T) {
	srv := postureModelRestoreServer(t,
		`{"active":"code","profiles":{"scan":{},"plan":{},"code":{"model":"grok-4.5"}}}`)

	m := New(config.ChatConfig{Provider: "grok"}, srv.URL)
	m.mode = ModeChat
	m.chatPosture = "code"
	m.provider = "grok"
	m.model = "opencode/muse-spark-1.2-contributor-free"

	// /new arms "apply:<activePosture>" — keepReasoning=true resume semantics.
	cmd := m.cmdLoadChatPosture()
	cp := cmd().(chatPostureMsg)
	m.chatPosturePending = "apply:" + m.activePosture()
	m.chatPostureCmdFromPending(cp.Cfg)
	if m.model != "opencode/muse-spark-1.2-contributor-free" {
		t.Fatalf("/new re-apply must keep user model, got %q", m.model)
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
