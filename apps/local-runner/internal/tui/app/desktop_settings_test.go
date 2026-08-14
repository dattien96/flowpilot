package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestSettingsBridgeBlurb_MentionsDesktop(t *testing.T) {
	got := settingsBridgeBlurb("http://127.0.0.1:5173")
	if !strings.Contains(got, "Desktop") || !strings.Contains(got, "5173") {
		t.Fatalf("blurb=%q", got)
	}
}

func TestSlashSettings_QueuesEnsureDesktop(t *testing.T) {
	m := New(config.ChatConfig{
		DesktopHost:     "127.0.0.1",
		DesktopPort:     5173,
		RunnerWorkspace: t.TempDir(),
	}, "http://127.0.0.1:4317")
	m2, cmd := m.handleSlashCommand("/settings")
	am := m2.(*AppModel)
	if cmd == nil {
		t.Fatal("expected ensure-desktop cmd")
	}
	if !strings.Contains(am.View(), "Settings live in the Desktop app") {
		t.Fatalf("missing blurb:\n%s", am.View())
	}
	// Simulate reuse path (no spawn).
	m3, _ := am.Update(DesktopEnsureMsg{URL: "http://127.0.0.1:5173", Reused: true, AuthSync: `C:\x\session.json`})
	view := m3.(*AppModel).View()
	if !strings.Contains(view, "already running") {
		t.Fatalf("expected reuse message:\n%s", view)
	}
	if !strings.Contains(view, "Auth session synced") {
		t.Fatalf("expected auth sync line:\n%s", view)
	}
}

func TestProviderConnectMsg_SurfacesRunnerError(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m2, _ := m.Update(ProviderConnectMsg{ProviderKey: "grok", Err: "not installed"})
	if !strings.Contains(m2.(*AppModel).View(), "Provider connect failed") {
		t.Fatalf("view:\n%s", m2.(*AppModel).View())
	}
}

func TestCmdConnectProvider_PostsDesktopParityEndpoint(t *testing.T) {
	var gotKey string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/provider-accounts/connect" || r.Method != http.MethodPost {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotKey = body["providerKey"]
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	m := New(config.ChatConfig{}, srv.URL)
	cmd := m.cmdConnectProvider("claude")
	msg := cmd()
	pcm, ok := msg.(ProviderConnectMsg)
	if !ok || pcm.Err != "" || pcm.ProviderKey != "claude" {
		t.Fatalf("msg=%#v", msg)
	}
	if gotKey != "claude" {
		t.Fatalf("providerKey=%q", gotKey)
	}
}

func TestCmdInstallProvider_PostsProvidersInstall(t *testing.T) {
	var gotName string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/providers/install" || r.Method != http.MethodPost {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		gotName = body["providerName"]
		_ = json.NewEncoder(w).Encode(map[string]any{
			"providers": []map[string]any{
				{"key": "grok", "installed": true, "detected_version": "grok 1.0"},
			},
		})
	}))
	defer srv.Close()

	m := New(config.ChatConfig{}, srv.URL)
	msg := m.cmdInstallProvider("grok")()
	pim, ok := msg.(ProviderInstallMsg)
	if !ok || pim.Err != "" || pim.ProviderKey != "grok" {
		t.Fatalf("msg=%#v", msg)
	}
	if gotName != "grok" {
		t.Fatalf("providerName=%q", gotName)
	}
	if len(pim.Providers) != 1 || !pim.Providers[0].Installed {
		t.Fatalf("providers=%+v", pim.Providers)
	}
}

func TestSlashProvider_ListShowsReadinessAndBlocksConnectWhenNotInstalled(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.providers = []client.Provider{
		{Key: "grok", Installed: false},
		{Key: "codex", Installed: true},
	}
	m.providerAccounts = []client.ProviderAccountSummary{
		{ProviderKey: "codex", AuthStatus: "connected", IsActive: true, DisplayLabel: "main"},
	}
	m2, _ := m.handleSlashCommand("/provider")
	view := m2.(*AppModel).View()
	if !strings.Contains(view, "not installed") || !strings.Contains(view, "ready") {
		t.Fatalf("list missing readiness:\n%s", view)
	}
	m3, cmd := m2.(*AppModel).handleSlashCommand("/provider connect grok")
	if cmd != nil {
		t.Fatal("expected no connect cmd when not installed")
	}
	if !strings.Contains(m3.(*AppModel).View(), "not installed") {
		t.Fatalf("expected not-installed error:\n%s", m3.(*AppModel).View())
	}
}
