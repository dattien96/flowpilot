package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestProviderSuggestionHidesCannotFindModuleDump(t *testing.T) {
	p := client.Provider{
		Key:             "codex",
		Label:           "Codex",
		Installed:       true,
		InstallStatus:   "INSTALLED",
		DetectedVersion: "Error: Cannot find module '@openai/codex'\nRequire stack:\n- C:\\npm\\codex",
	}
	code, _ := providerReadiness(p, nil)
	if code != "not_installed" {
		t.Fatalf("broken CLI dump must be not_installed, got %s", code)
	}
	detail := providerSuggestionDetail(p, nil, "grok", "select")
	if strings.Contains(strings.ToLower(detail), "cannot find module") {
		t.Fatalf("picker must not dump node module error, got %q", detail)
	}
	if !strings.Contains(detail, "not installed") {
		t.Fatalf("picker must show not installed, got %q", detail)
	}
}

func TestProviderSuggestionKeepsGrokNotInstalledSubstring(t *testing.T) {
	p := client.Provider{Key: "grok", Label: "Grok", Installed: false, InstallHint: "install grok"}
	detail := providerSuggestionDetail(p, nil, "codex", "select")
	if !strings.Contains(detail, "not installed") {
		t.Fatalf("Installed=false must keep 'not installed' substring, got %q", detail)
	}
}

func TestProviderSuggestionFailedInstallStatusIsNotInstalled(t *testing.T) {
	p := client.Provider{Key: "codex", Label: "Codex", Installed: true, InstallStatus: "FAILED"}
	code, _ := providerReadiness(p, nil)
	if code != "not_installed" {
		t.Fatalf("FAILED install_status must be not_installed, got %s", code)
	}
}

func TestSlashProviderInstall_AllowsBrokenCLIReinstall(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.providers = []client.Provider{
		{
			Key:             "codex",
			Label:           "Codex",
			Installed:       true,
			InstallStatus:   "FAILED",
			DetectedVersion: "Error: Cannot find module '@openai/codex'",
		},
	}
	_, cmd := m.handleSlashCommand("/provider install codex")
	if cmd == nil {
		t.Fatal("broken Codex shim must not be treated as already-installed; install cmd required")
	}
}

func TestSlashProviderInstall_SkipsHealthyInstalled(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.providers = []client.Provider{
		{Key: "codex", Label: "Codex", Installed: true, InstallStatus: "INSTALLED", DetectedVersion: "0.140.0"},
	}
	next, cmd := m.handleSlashCommand("/provider install codex")
	if cmd != nil {
		t.Fatal("healthy Codex must stay already-installed")
	}
	blob := ""
	for _, msg := range next.(*AppModel).messages {
		blob += msg.Content + "\n"
	}
	if !strings.Contains(blob, "already installed") {
		t.Fatalf("expected already-installed notice, got:\n%s", blob)
	}
}

func TestLooksLikeProviderProbeError(t *testing.T) {
	if !looksLikeProviderProbeError("Error: Cannot find module 'foo'") {
		t.Fatal("expected cannot-find-module")
	}
	if looksLikeProviderProbeError("0.140.0") {
		t.Fatal("version must pass")
	}
}
