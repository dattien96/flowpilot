package app

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/config"
	"flowpilot-runner/internal/tui/prefs"
)

// CA-685 (operator decision): chat postures gain a fourth value "non" — the
// no-mode posture. Tab cycles Plan → Code → Non → Plan; in "non" NO pins are
// applied, so the session keeps the user's last provider/model choice across
// restarts. Scan/plan/code keep the strict pin semantics (CA-685: the pin
// always re-applies on restore). Chat-only — flow mode never touches postures.
// Additive tests; the rewritten CA-679-era file now encodes the same spec.

func nonPostureServer(t *testing.T, active string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/client/chat-posture" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"active":"` + active + `","profiles":{"scan":{},"plan":{},"code":{}}}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestTabCycleIncludesNon(t *testing.T) {
	srv := nonPostureServer(t, "code")
	m := New(config.ChatConfig{}, srv.URL)
	m.mode = ModeChat
	m.sessionDefaultsLoaded = true
	m.chatPosture = "code"
	m.providers = nil

	// code → non
	m.chatPosturePending = "apply:code"
	if next := tabPostureNext(m.activePosture()); next != "non" {
		t.Fatalf("Tab from code must reach non, got %q", next)
	}
	// non → plan
	if next := tabPostureNext("non"); next != "plan" {
		t.Fatalf("Tab from non must reach plan, got %q", next)
	}
	// plan → code
	if next := tabPostureNext("plan"); next != "code" {
		t.Fatalf("Tab from plan must reach code, got %q", next)
	}
	// scan is NOT in the Tab cycle (opt-in only, unchanged)
	if next := tabPostureNext("scan"); next != "plan" {
		t.Fatalf("Tab from scan (out of cycle) must default to plan, got %q", next)
	}
}

// tabPostureNext mirrors the inline Tab-handler lookup (app.go) so the cycle
// stays covered by tests.
func tabPostureNext(current string) string {
	for i, p := range tabPostureOrder {
		if p == current {
			return tabPostureOrder[(i+1)%len(tabPostureOrder)]
		}
	}
	return tabPostureOrder[0]
}

func TestApplyNonPostureAppliesNoPins(t *testing.T) {
	srv := nonPostureServer(t, "code")
	m := New(config.ChatConfig{Provider: "opencode"}, srv.URL)
	m.mode = ModeChat
	m.provider = "opencode"
	m.model = "opencode-go/muse-spark-1.2-contributor"
	m.reasoningEffort = "low"

	cmd := m.cmdLoadChatPosture()
	cp := cmd().(chatPostureMsg)
	m.chatPosturePending = "apply:non"
	m.chatPostureCmdFromPending(cp.Cfg)

	if m.chatPosture != "non" {
		t.Fatalf("posture = %q, want non", m.chatPosture)
	}
	if m.provider != "opencode" || m.model != "opencode-go/muse-spark-1.2-contributor" || m.reasoningEffort != "low" {
		t.Fatalf("non must apply no pins, got provider=%q model=%q reasoning=%q", m.provider, m.model, m.reasoningEffort)
	}
	if !m.chatPostureDirty {
		t.Fatal("apply must persist the new active posture back to the runner")
	}
}

func TestRestoreNonKeepsUserModelAcrossRestart(t *testing.T) {
	srv := nonPostureServer(t, "non")

	prefFile := filepath.Join(t.TempDir(), "tui-session.json")
	t.Setenv("FLOWPILOT_TUI_SESSION_FILE", prefFile)

	m := New(config.ChatConfig{Provider: "opencode"}, srv.URL)
	m.mode = ModeChat
	m.provider = "opencode"
	m.model = "opencode-go/muse-spark-1.2-contributor"
	m.persistSessionPrefs()

	m.chatPosturePending = "restore"
	cmd := m.cmdLoadChatPosture()
	cp := cmd().(chatPostureMsg)
	m.chatPostureCmdFromPending(cp.Cfg)

	if m.chatPosture != "non" {
		t.Fatalf("posture after restore = %q, want non", m.chatPosture)
	}
	if m.model != "opencode-go/muse-spark-1.2-contributor" {
		t.Fatalf("non restore must keep the user's model, got %q", m.model)
	}
	saved, _, err := prefs.Load()
	if err != nil {
		t.Fatalf("load prefs: %v", err)
	}
	if saved.Model != "opencode-go/muse-spark-1.2-contributor" {
		t.Fatalf("non restore must keep the persisted model, got %q", saved.Model)
	}
}

func TestRestoreNonIgnoresStaleProfilePins(t *testing.T) {
	// Even when the doc still carries pins for scan/plan/code, active=non
	// applies nothing.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/client/chat-posture" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"active":"non","profiles":{"scan":{"model":"grok-4.5"},"plan":{"model":"grok-4.5"},"code":{"model":"grok-4.5"}}}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)

	m := New(config.ChatConfig{Provider: "opencode"}, srv.URL)
	m.mode = ModeChat
	m.provider = "opencode"
	m.model = "opencode-go/deepseek-v4-flash"

	m.chatPosturePending = "restore"
	cmd := m.cmdLoadChatPosture()
	cp := cmd().(chatPostureMsg)
	m.chatPostureCmdFromPending(cp.Cfg)

	if m.model != "opencode-go/deepseek-v4-flash" {
		t.Fatalf("active=non must ignore stale pins, got model %q", m.model)
	}
}

func TestChatFrameTitleNonShowsBareChat(t *testing.T) {
	// CA-685: the no-mode default renders just "Chat:" — no "Chat: Non".
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.mode = ModeChat
	m.chatPosture = "non"
	nonTitle := stripANSI(m.chatFrameTitle())
	if !strings.HasPrefix(nonTitle, "Chat:") || strings.Contains(nonTitle, "Non") {
		t.Fatalf("non posture title = %q, want prefix %q without Non", nonTitle, "Chat:")
	}
	m.chatPosture = "code"
	if got := stripANSI(m.chatFrameTitle()); !strings.HasPrefix(got, "Chat: Code") {
		t.Fatalf("code posture title = %q, want prefix %q", got, "Chat: Code")
	}
	m.chatPosture = "scan"
	if got := stripANSI(m.chatFrameTitle()); !strings.HasPrefix(got, "Chat: Scan") {
		t.Fatalf("scan posture title = %q, want prefix %q", got, "Chat: Scan")
	}
}

func TestDefaultPostureIsNon(t *testing.T) {
	// CA-685: an unset posture resolves to "non" (the no-mode default).
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	if m.activePosture() != "non" {
		t.Fatalf("default activePosture = %q, want non", m.activePosture())
	}
}
