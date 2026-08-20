package app

import (
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// Reported bug: /mode-setup scan provider showed "(no providers)" when the
// catalog had not loaded yet (m.providers empty). Pins are plain strings —
// the picker must offer claude/codex/grok even with no catalog.
func TestModeSetupProviderFallbackWhenCatalogEmpty(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.providers = nil
	m.providerAccounts = nil
	m.provider = "codex"
	m.sessionDefaultsLoaded = true
	m.inputValue = "/mode-setup scan provider "
	m.inputCursor = -1

	items := m.collectSuggestions()
	if len(items) != 3 {
		t.Fatalf("fallback must show 3 providers, got %d: %+v", len(items), items)
	}
	seen := map[string]bool{}
	for _, it := range items {
		seen[it.value] = true
	}
	for _, want := range []string{"scan provider claude", "scan provider codex", "scan provider grok"} {
		if !seen[want] {
			t.Fatalf("missing %q in %+v", want, items)
		}
	}
}

func TestModeSetupProviderUsesAccountsWhenProvidersEmpty(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.providers = nil
	m.providerAccounts = []client.ProviderAccountSummary{
		{ProviderKey: "myprov", IsActive: true},
		{ProviderKey: "codex", IsActive: false},
	}
	m.provider = "codex"
	m.sessionDefaultsLoaded = true
	m.inputValue = "/mode-setup scan provider "
	m.inputCursor = -1

	items := m.collectSuggestions()
	// Must contain myprov (from accounts) and codex, not fallback grok/claude when accounts present
	seen := map[string]bool{}
	for _, it := range items {
		seen[it.value] = true
	}
	if !seen["scan provider myprov"] {
		t.Fatalf("must include account provider myprov, got %+v", items)
	}
	if !seen["scan provider codex"] {
		t.Fatalf("must include codex from accounts, got %+v", items)
	}
}

func TestModeSetupProviderFiltersFallback(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.providers = nil
	m.providerAccounts = nil
	m.inputValue = "/mode-setup scan provider cla"
	m.inputCursor = -1

	items := m.collectSuggestions()
	if len(items) != 1 || items[0].value != "scan provider claude" {
		t.Fatalf("filter cla must yield only claude, got %+v", items)
	}
}
