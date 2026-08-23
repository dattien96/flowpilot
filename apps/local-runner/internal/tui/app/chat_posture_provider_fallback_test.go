package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// Reported bug: /mode-setup scan provider showed "(no providers)" when the
// catalog had not loaded yet. Underlying filter still offers claude/codex/grok,
// but TAB picker for /mode-setup is now disabled (modal on Enter).
func TestModeSetupProviderFallbackWhenCatalogEmpty(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.providers = nil
	m.providerAccounts = nil
	m.provider = "codex"
	m.sessionDefaultsLoaded = true
	m.inputValue = "/mode-setup scan provider "
	m.inputCursor = -1

	// TAB flow disabled for /mode-setup — collectSuggestions must not return wizard picker
	if items := m.collectSuggestions(); len(items) > 0 {
		for _, it := range items {
			if it.kind == "mode-setup-value" || it.kind == "mode-setup-posture" || it.kind == "mode-setup-field" {
				t.Fatalf("TAB picker for /mode-setup must be disabled, got %+v", items)
			}
		}
	}
	// Underlying filter still provides fallback (used by typed power-user path)
	items := filterModeSetupSuggestions(m.inputValue, m.providers, m.providerAccounts, m.provider, m.model, m.modeSetupDraft)
	var filtered []string
	for _, it := range items {
		if it.kind == "mode-setup-value" {
			filtered = append(filtered, it.value)
		}
	}
	if len(filtered) != 3 {
		t.Fatalf("fallback must show 3 providers, got %d: %+v (filtered %v)", len(items), items, filtered)
	}
	seen := map[string]bool{}
	for _, v := range filtered {
		seen[v] = true
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

	items := filterModeSetupSuggestions(m.inputValue, m.providers, m.providerAccounts, m.provider, m.model, m.modeSetupDraft)
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

	items := filterModeSetupSuggestions(m.inputValue, m.providers, m.providerAccounts, m.provider, m.model, m.modeSetupDraft)
	var filtered []string
	for _, it := range items {
		if it.kind == "mode-setup-value" {
			filtered = append(filtered, it.value)
		}
	}
	if len(filtered) != 1 || filtered[0] != "scan provider claude" {
		t.Fatalf("filter cla must yield only claude, got %+v (filtered %v)", items, filtered)
	}
}

func TestModeSetupModelShowsAllProviders(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.providers = []client.Provider{
		{Key: "claude", Models: []client.ProviderModel{{ID: "opus"}, {ID: "sonnet"}}},
		{Key: "codex", Models: []client.ProviderModel{{ID: "o3"}}},
		{Key: "grok", Models: nil},
	}
	m.provider = "grok"
	m.model = ""
	m.sessionDefaultsLoaded = true
	m.inputValue = "/mode-setup scan model "
	m.inputCursor = -1

	items := filterModeSetupSuggestions(m.inputValue, m.providers, m.providerAccounts, m.provider, m.model, m.modeSetupDraft)
	if len(items) < 3 {
		t.Fatalf("must show at least 3 models across providers, got %d: %+v", len(items), items)
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
