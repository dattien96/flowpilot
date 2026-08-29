package app

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"flowpilot-runner/internal/tui/prefs"
	"flowpilot-runner/internal/tui/config"
)

// CA-641 history: /new used to preserve the user's /reasoning choice.
// SUPERSEDED 2026-08-29 by the CA-685 operator decision — /new re-applying
// the active scan/plan/code posture reloads the FULL profile (reasoning
// included); "non" is where a personal reasoning choice survives. The tests
// below encode the CA-685 spec (same-day rewrite, flagged per R1).

func TestNewReapplyAppliesReasoningPin(t *testing.T) {
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
	m.reasoningEffort = "high" // user's mid-session /reasoning change

	cmd := m.cmdLoadChatPosture()
	cp := cmd().(chatPostureMsg)
	if cp.Err != nil {
		t.Fatalf("load chat posture failed: %+v", cp)
	}
	// /new arms "apply:<activePosture>" — posture is unchanged, pin reloads.
	m.chatPosturePending = "apply:" + m.activePosture()
	m.chatPostureCmdFromPending(cp.Cfg)
	if m.reasoningEffort != "medium" {
		t.Fatalf("/new re-apply must reload the reasoning pin, got %q", m.reasoningEffort)
	}
}

func TestNewReapplyAppliesReasoningPinActiveDiffersFromRunner(t *testing.T) {
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
	if m.reasoningEffort != "high" {
		t.Fatalf("re-apply of active plan must reload the reasoning pin, got %q", m.reasoningEffort)
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

func TestNewReapplyPersistsReloadedReasoningPin(t *testing.T) {
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
	if saved.ReasoningEffort != "medium" {
		t.Fatalf("session prefs must persist the reloaded pin after /new, got %q", saved.ReasoningEffort)
	}
}