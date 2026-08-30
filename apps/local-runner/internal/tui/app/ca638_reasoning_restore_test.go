package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-638 history: restart-resume used to keep the user's /reasoning choice.
// SUPERSEDED 2026-08-29 by the CA-685 operator decision — scan/plan/code are
// pinned postures: the FULL profile (reasoning included) reloads on restart,
// and the "non" posture is where a personal reasoning choice survives. The
// tests below encode the CA-685 spec (same-day rewrite, flagged per R1).

func TestRestorePostureAppliesReasoningPin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/client/chat-posture" {
			w.Header().Set("Content-Type", "application/json")
			// code profile pins reasoning=medium — the pin reloads (CA-685).
			_, _ = w.Write([]byte(`{"active":"code","profiles":{"scan":{},"plan":{},"code":{"reasoningEffort":"medium"}}}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	m := New(config.ChatConfig{Provider: "grok"}, srv.URL)
	m.mode = ModeChat
	m.chatPosture = "code"
	m.reasoningEffort = "low" // user's persisted /reasoning choice
	m.sessionDefaultsLoaded = false

	m2, _ := m.Update(SessionDefaultsMsg{Provider: "grok", Model: "grok-4.5", Providers: []client.Provider{{Key: "grok", Models: []client.ProviderModel{{ID: "grok-4.5"}}}}})
	am := m2.(*AppModel)
	cmd := am.cmdLoadChatPosture()
	cp, ok := cmd().(chatPostureMsg)
	if !ok || cp.Err != nil {
		t.Fatalf("load chat posture failed: %+v", cp)
	}
	am.chatPostureCmdFromPending(cp.Cfg)
	if am.reasoningEffort != "medium" {
		t.Fatalf("restore must apply the code posture reasoning pin, got %q", am.reasoningEffort)
	}
	if am.chatPosture != "code" {
		t.Fatalf("posture after restore = %q, want code", am.chatPosture)
	}
	if am.chatPostureDirty {
		t.Fatal("restore must NOT mark dirty")
	}
}

func TestRestorePostureAppliesReasoningPinOtherProfile(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/client/chat-posture" {
			w.Header().Set("Content-Type", "application/json")
			// plan profile pins high — the pin reloads on restore (CA-685).
			_, _ = w.Write([]byte(`{"active":"plan","profiles":{"scan":{},"plan":{"provider":"grok","reasoningEffort":"high"},"code":{}}}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	m := New(config.ChatConfig{Provider: "grok"}, srv.URL)
	m.mode = ModeChat
	m.chatPosture = "code"
	m.reasoningEffort = "medium"
	m.providers = []client.Provider{{Key: "grok", Models: []client.ProviderModel{{ID: "grok-4.5"}}}}
	m.sessionDefaultsLoaded = false

	m2, _ := m.Update(SessionDefaultsMsg{Provider: "grok", Model: "grok-4.5", Providers: m.providers})
	am := m2.(*AppModel)
	cmd := am.cmdLoadChatPosture()
	cp := cmd().(chatPostureMsg)
	am.chatPostureCmdFromPending(cp.Cfg)
	if am.chatPosture != "plan" {
		t.Fatalf("posture after restore = %q, want plan", am.chatPosture)
	}
	if am.reasoningEffort != "high" {
		t.Fatalf("restore must apply the plan reasoning pin, got %q", am.reasoningEffort)
	}
}

func TestApplyPostureStillPinsReasoning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/client/chat-posture" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"active":"code","profiles":{"scan":{},"plan":{"reasoningEffort":"high"},"code":{}}}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	m := New(config.ChatConfig{Provider: "grok"}, srv.URL)
	m.mode = ModeChat
	m.reasoningEffort = "low"
	cmd := m.cmdLoadChatPosture()
	cp := cmd().(chatPostureMsg)
	m.chatPosturePending = "apply:plan"
	m.chatPostureCmdFromPending(cp.Cfg)
	if m.reasoningEffort != "high" {
		t.Fatalf("explicit apply must pin plan reasoning=high, got %q", m.reasoningEffort)
	}
}
