package app

import (
	"strings"
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

func TestModeSetupModelShowsAllProviders(t *testing.T) {
	// Current provider grok has no models, but claude/codex do — picker must
	// still list all models with provider detail, and provider auto-inferred.
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.providers = []client.Provider{
		{Key: "claude", Models: []client.ProviderModel{{ID: "opus"}, {ID: "sonnet"}}},
		{Key: "codex", Models: []client.ProviderModel{{ID: "o3"}}},
		{Key: "grok", Models: nil},
	}
	m.provider = "grok"
	m.sessionDefaultsLoaded = true
	m.inputValue = "/mode-setup scan model "
	m.inputCursor = -1

	items := m.collectSuggestions()
	if len(items) != 3 {
		t.Fatalf("must show 3 models across providers, got %d: %+v", len(items), items)
	}
	seen := map[string]string{}
	for _, it := range items {
		seen[it.value] = it.detail
	}
	if seen["scan model opus"] != "claude" {
		t.Fatalf("opus detail must be claude, got %+v", items)
	}
	if seen["scan model o3"] != "codex" {
		t.Fatalf("o3 detail must be codex, got %+v", items)
	}
}

func TestModeSetupModelInfersProvider(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.providers = []client.Provider{
		{Key: "claude", Models: []client.ProviderModel{{ID: "opus"}}},
		{Key: "codex", Models: []client.ProviderModel{{ID: "o3"}}},
	}
	m.provider = "grok"
	// Simulate /mode-setup scan model opus — provider should auto-pin to claude
	cfg := client.ChatPostureConfig{Active: "code", Profiles: map[string]client.ChatPostureProfile{
		"scan": {}, "plan": {}, "code": {},
	}}
	m.editChatPostureProfile(cfg, "scan", "model", "opus")
	p := m.chatPostureCfg.Profiles["scan"]
	if p.Model != "opus" {
		t.Fatalf("model = %q want opus", p.Model)
	}
	if p.Provider != "claude" {
		t.Fatalf("provider inferred from model must be claude, got %q", p.Provider)
	}
	if !m.chatPostureSaving {
		t.Fatal("must mark saving after model edit")
	}
}

func TestModeSetupSavingReplacedWithSaved(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.chatPostureSaving = true
	m.chatPostureSavingPosture = "scan"
	m.chatPostureCfg = client.ChatPostureConfig{Active: "code", Profiles: map[string]client.ChatPostureProfile{"scan": {Provider: "codex"}}}
	// Simulate successful PUT result
	_, cmd := m.Update(chatPostureMsg{Cfg: client.ChatPostureConfig{Active: "code", Profiles: map[string]client.ChatPostureProfile{"scan": {Provider: "codex"}}}})
	if m.chatPostureSaving {
		t.Fatal("saving flag must clear after PUT success")
	}
	if cmd != nil {
		t.Fatal("save success should not dispatch further cmd")
	}
	view := m.View()
	if !strings.Contains(view, "Posture scan saved.") {
		t.Fatalf("must show saved banner, got view:\n%s", view)
	}
}
