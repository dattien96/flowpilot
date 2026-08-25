package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"flowpilot-runner/internal/tui/prefs"
	"flowpilot-runner/internal/tui/config"
)

// CA-641: /new re-applies the already-active posture and must preserve the
// user's /reasoning choice instead of re-pinning the posture profile default
// (operator report: /reasoning high in chat resets to the code profile's
// medium after /new and the clobbered value is persisted to session prefs).
// Additive — the existing ca638_reasoning_restore_test.go suite is untouched.

func TestNewReapplyKeepsUserReasoning(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/client/chat-posture" {
			w.Header().Set("Content-Type", "application/json")
			// code profile pins reasoning=medium — must NOT override the user's high.
			_, _ = w.Write([]byte(`{"active":"code","profiles":{"scan":{},"plan":{},"code":{"reasoningEffort":"medium"}}}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	m := New(config.ChatConfig{Provider: "grok"}, srv.URL)
	m.mode = ModeChat
	m.chatPosture = "code"
	m.reasoningEffort = "high" // user's /reasoning choice

	cmd := m.cmdLoadChatPosture()
	cp := cmd().(chatPostureMsg)
	if cp.Err != nil {
		t.Fatalf("load chat posture failed: %+v", cp)
	}
	// /new arms "apply:<activePosture>" — posture is unchanged.
	m.chatPosturePending = "apply:" + m.activePosture()
	m.chatPostureCmdFromPending(cp.Cfg)
	if m.reasoningEffort != "high" {
		t.Fatalf("/new re-apply must keep user reasoning=high, got %q", m.reasoningEffort)
	}
}

func TestNewReapplyKeepsUserReasoningEvenWhenActiveDiffersFromRunner(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/client/chat-posture" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"active":"plan","profiles":{"scan":{},"plan":{"reasoningEffort":"high"},"code":{}}}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	m := New(config.ChatConfig{Provider: "grok"}, srv.URL)
	m.mode = ModeChat
	m.chatPosture = "plan"
	m.reasoningEffort = "low" // user override on top of plan's pin

	cmd := m.cmdLoadChatPosture()
	cp := cmd().(chatPostureMsg)
	m.chatPosturePending = "apply:plan"
	m.chatPostureCmdFromPending(cp.Cfg)
	if m.reasoningEffort != "low" {
		t.Fatalf("re-apply of active plan must keep user reasoning=low, got %q", m.reasoningEffort)
	}
	if m.chatPosture != "plan" {
		t.Fatalf("posture = %q, want plan", m.chatPosture)
	}
}

func TestRealPostureSwitchStillPinsReasoning(t *testing.T) {
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
	m.chatPosture = "code"
	m.reasoningEffort = "low"

	cmd := m.cmdLoadChatPosture()
	cp := cmd().(chatPostureMsg)
	// Tab / /mode switch to a DIFFERENT posture must still apply the pin.
	m.chatPosturePending = "apply:plan"
	m.chatPostureCmdFromPending(cp.Cfg)
	if m.reasoningEffort != "high" {
		t.Fatalf("real switch to plan must pin reasoning=high, got %q", m.reasoningEffort)
	}
}

func TestNewReapplyKeepsUserReasoningAndPersistsIt(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("FLOWPILOT_TUI_SESSION_FILE", dir+"/tui-session.json")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/client/chat-posture" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"active":"code","profiles":{"scan":{},"plan":{},"code":{"reasoningEffort":"medium"}}}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	m := New(config.ChatConfig{Provider: "grok"}, srv.URL)
	m.mode = ModeChat
	m.chatPosture = "code"
	m.reasoningEffort = "low"

	cmd := m.cmdLoadChatPosture()
	cp := cmd().(chatPostureMsg)
	m.chatPosturePending = "apply:" + m.activePosture()
	m.chatPostureCmdFromPending(cp.Cfg)

	saved, _, err := prefs.Load()
	if err != nil {
		t.Fatalf("load session prefs: %v", err)
	}
	if saved.ReasoningEffort != "low" {
		t.Fatalf("session prefs must keep user reasoning=low after /new, got %q", saved.ReasoningEffort)
	}
}