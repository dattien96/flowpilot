package app

import (
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestModeSetupBareShowsPosture(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.inputValue = "/mode-setup"
	m.inputCursor = -1
	items := m.collectSuggestions()
	for _, it := range items {
		if it.kind == "mode-setup-posture" || it.kind == "mode-setup-field" || it.kind == "mode-setup-value" {
			t.Fatalf("TAB picker for bare /mode-setup must be disabled (modal on Enter), got %+v", items)
		}
	}
	// Underlying filter still works for typed power-user path
	filtered := filterModeSetupSuggestions(m.inputValue, m.providers, m.providerAccounts, m.provider, m.model, m.modeSetupDraft)
	found := map[string]bool{}
	for _, it := range filtered {
		if it.kind == "mode-setup-posture" {
			found[it.value] = true
		}
	}
	if !found["scan"] {
		t.Fatalf("filterModeSetupSuggestions must still show scan, got %+v", filtered)
	}
}

func TestModeSetupWizardStageAndSave(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.providers = []client.Provider{
		{Key: "claude", Models: []client.ProviderModel{{ID: "opus"}}},
		{Key: "codex", Models: []client.ProviderModel{{ID: "o3"}}},
	}
	m.provider = "codex"
	m.sessionDefaultsLoaded = true
	m.chatPostureCfg = client.ChatPostureConfig{Active: "code", Profiles: map[string]client.ChatPostureProfile{"plan": {}, "code": {}}}

	// TAB picker for /mode-setup is now disabled (modal on Enter)
	m.inputValue = "/mode-setup plan model "
	m.inputCursor = -1
	items := m.collectSuggestions()
	for _, it := range items {
		if it.kind == "mode-setup-value" || it.kind == "mode-setup-field" || it.kind == "mode-setup-posture" {
			t.Fatalf("TAB picker for /mode-setup must be disabled, got %+v", items)
		}
	}
	// Underlying filter still provides values for typed power-user path
	filtered := filterModeSetupSuggestions(m.inputValue, m.providers, m.providerAccounts, m.provider, m.model, m.modeSetupDraft)
	foundOpus := false
	for _, it := range filtered {
		if it.value == "plan model opus" {
			foundOpus = true
			break
		}
	}
	if !foundOpus {
		t.Fatalf("filter must still find plan model opus, got %+v", filtered)
	}
	// Typed power-user path still stages via editChatPostureProfile (no TAB)
	cfg := client.ChatPostureConfig{Active: "code", Profiles: map[string]client.ChatPostureProfile{"plan": {}, "code": {}}}
	m.editChatPostureProfile(cfg, "plan", "model", "opus")
	if m.chatPostureCfg.Profiles["plan"].Model != "opus" {
		t.Fatalf("staged model = %q want opus", m.chatPostureCfg.Profiles["plan"].Model)
	}
	if m.chatPostureCfg.Profiles["plan"].Provider != "claude" {
		t.Fatalf("staged provider inferred = %q want claude", m.chatPostureCfg.Profiles["plan"].Provider)
	}
	// Modal flow is the new interactive path (tested in modal_scan_additive_test.go)
	// Verify modal can still stage reasoning after model via direct draft
	m2 := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m2.providers = m.providers
	m2.provider = "codex"
	m2.openModeSetupModal(client.ChatPostureConfig{Active: "code", Profiles: map[string]client.ChatPostureProfile{"plan": {Model: "opus", Provider: "claude"}}}, "plan")
	if !m2.isReasoningEnabled() {
		t.Fatal("reasoning must be enabled after model via modal")
	}
}

func TestModeSetupWizardBack(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.inputValue = "/mode-setup plan model "
	m.inputCursor = -1
	items := m.collectSuggestions()
	for _, it := range items {
		if it.kind == "mode-setup-back" || it.kind == "mode-setup-value" || it.kind == "mode-setup-field" {
			t.Fatalf("TAB picker for /mode-setup must be disabled (modal), got back row %+v", items)
		}
	}
	m.inputValue = "/mode-setup plan "
	m.inputCursor = -1
	items = m.collectSuggestions()
	for _, it := range items {
		if it.kind == "mode-setup-back" {
			t.Fatalf("field picker back row must be disabled, got %+v", items)
		}
	}
}
