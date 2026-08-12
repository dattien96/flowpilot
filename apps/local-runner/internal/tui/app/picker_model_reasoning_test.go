package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestFilterModelSuggestions_FiltersAsYouType(t *testing.T) {
	models := []string{"gpt-5.4", "o3", "o4-mini"}
	all := filterModelSuggestions("/model ", models, "o3")
	if len(all) != 3 {
		t.Fatalf("expected 3, got %d", len(all))
	}
	filtered := filterModelSuggestions("/model o4", models, "o3")
	if len(filtered) != 1 || filtered[0].value != "o4-mini" {
		t.Fatalf("filtered=%+v", filtered)
	}
	if filterModelSuggestions("/model", models, "o3") != nil {
		t.Fatal("bare /model should not open picker")
	}
}

func TestFilterReasoningSuggestions_FiltersAsYouType(t *testing.T) {
	all := filterReasoningSuggestions("/reasoning ", "medium")
	if len(all) != 3 {
		t.Fatalf("expected 3, got %d", len(all))
	}
	filtered := filterReasoningSuggestions("/reasoning hi", "medium")
	if len(filtered) != 1 || filtered[0].value != "high" {
		t.Fatalf("filtered=%+v", filtered)
	}
}

func TestEnter_AcceptsHighlightedModelSuggestion(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.provider = "codex"
	m.providers = []client.Provider{{
		Key: "codex",
		Models: []client.ProviderModel{
			{ID: "gpt-5.4"},
			{ID: "o3"},
		},
	}}
	m.inputValue = "/model "
	m.suggIdx = 1
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	am := m2.(*AppModel)
	if am.model != "o3" {
		t.Fatalf("model=%q want o3", am.model)
	}
	if !strings.Contains(am.View(), "Model set to: o3") {
		t.Fatalf("expected set message:\n%s", am.View())
	}
}

func TestEnter_AcceptsHighlightedReasoningSuggestion(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.inputValue = "/reasoning "
	m.suggIdx = 0 // high
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	am := m2.(*AppModel)
	if am.reasoningEffort != "high" {
		t.Fatalf("reasoningEffort=%q", am.reasoningEffort)
	}
	if !strings.Contains(am.View(), "Reasoning effort set to: high") {
		t.Fatalf("expected set message:\n%s", am.View())
	}
}

func TestFilterProviderSuggestions_FiltersAsYouType(t *testing.T) {
	providers := []client.Provider{
		{Key: "codex", Label: "Codex", Installed: true, Models: []client.ProviderModel{{ID: "o3"}}},
		{Key: "claude", Label: "Claude", Installed: true, Models: []client.ProviderModel{{ID: "opus"}, {ID: "sonnet"}}},
		{Key: "grok", Name: "Grok", Installed: false},
	}
	accounts := []client.ProviderAccountSummary{
		{ProviderKey: "codex", AuthStatus: "connected", IsActive: true, DisplayLabel: "codex-1"},
	}
	all := filterProviderSuggestions("/provider ", providers, accounts, "codex")
	// connect + install + config actions + 3 providers
	if len(all) != 6 {
		t.Fatalf("expected 6 (3 actions + 3 providers), got %d: %+v", len(all), all)
	}
	if all[0].value != "connect" || all[0].kind != "provider-action" {
		t.Fatalf("first row should be connect action, got %+v", all[0])
	}
	if all[1].value != "install" {
		t.Fatalf("second row should be install action, got %+v", all[1])
	}
	filtered := filterProviderSuggestions("/provider cl", providers, accounts, "codex")
	// "claude" provider + no connect/config/install (doesn't match "cl")
	if len(filtered) != 1 || filtered[0].value != "claude" {
		t.Fatalf("filtered=%+v", filtered)
	}
	connOnly := filterProviderSuggestions("/provider co", providers, accounts, "codex")
	if len(connOnly) < 1 || connOnly[0].value != "connect" {
		t.Fatalf("expected connect action for /provider co, got %+v", connOnly)
	}
	if filterProviderSuggestions("/provider", providers, accounts, "codex") != nil {
		t.Fatal("bare /provider should not open picker")
	}
}

func TestEnter_ProviderConnectActionOpensConnectPicker(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.providers = []client.Provider{
		{Key: "codex"},
		{Key: "claude"},
	}
	m.inputValue = "/provider "
	m.suggIdx = 0 // connect action
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	am := m2.(*AppModel)
	if am.inputValue != "/provider connect " {
		t.Fatalf("inputValue=%q want /provider connect ", am.inputValue)
	}
	items := am.collectSuggestions()
	if len(items) == 0 || items[0].kind != "provider-connect" {
		t.Fatalf("expected connect-mode provider picker, got %+v", items)
	}
}

func TestFilterProviderSuggestions_ConnectMode(t *testing.T) {
	providers := []client.Provider{
		{Key: "codex", Label: "Codex", Installed: true},
		{Key: "claude", Label: "Claude", Installed: true},
	}
	got := filterProviderSuggestions("/provider connect cl", providers, nil, "codex")
	if len(got) != 1 || got[0].value != "claude" || got[0].kind != "provider-connect" {
		t.Fatalf("got=%+v", got)
	}
	if suggestionAcceptValue(got[0]) != "/provider connect claude" {
		t.Fatalf("accept=%q", suggestionAcceptValue(got[0]))
	}
}

func TestFilterProviderSuggestions_InstallModeAndReadiness(t *testing.T) {
	providers := []client.Provider{
		{Key: "grok", Label: "Grok", Installed: false, InstallHint: "install grok"},
		{Key: "codex", Label: "Codex", Installed: true},
	}
	accounts := []client.ProviderAccountSummary{
		{ProviderKey: "codex", AuthStatus: "connected", IsActive: true, DisplayLabel: "main"},
	}
	inst := filterProviderSuggestions("/provider install ", providers, accounts, "")
	if len(inst) != 2 || inst[0].kind != "provider-install" {
		t.Fatalf("install picker=%+v", inst)
	}
	if suggestionAcceptValue(inst[0]) != "/provider install grok" {
		t.Fatalf("accept=%q", suggestionAcceptValue(inst[0]))
	}
	all := filterProviderSuggestions("/provider ", providers, accounts, "codex")
	var grok, codex *suggestItem
	for i := range all {
		switch all[i].value {
		case "grok":
			grok = &all[i]
		case "codex":
			codex = &all[i]
		}
	}
	if grok == nil || !strings.Contains(grok.detail, "not installed") {
		t.Fatalf("grok detail want not installed, got %+v", grok)
	}
	if codex == nil || !strings.Contains(codex.detail, "ready") {
		t.Fatalf("codex detail want ready, got %+v", codex)
	}
}

func TestProviderReadiness_NoActiveAccount(t *testing.T) {
	p := client.Provider{Key: "claude", Installed: true}
	code, detail := providerReadiness(p, []client.ProviderAccountSummary{
		{ProviderKey: "claude", AuthStatus: "connected", IsActive: false, DisplayLabel: "slot-2"},
	})
	if code != "no_active_account" || !strings.Contains(detail, "no active account") {
		t.Fatalf("code=%s detail=%s", code, detail)
	}
}

func TestEnter_AcceptsHighlightedProviderSuggestion(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.providers = []client.Provider{
		{Key: "codex", Models: []client.ProviderModel{{ID: "o3"}}},
		{Key: "claude", Models: []client.ProviderModel{{ID: "opus"}}},
	}
	m.inputValue = "/provider "
	items := m.collectSuggestions()
	claudeIdx := -1
	for i, it := range items {
		if it.kind == "provider" && it.value == "claude" {
			claudeIdx = i
			break
		}
	}
	if claudeIdx < 0 {
		t.Fatalf("claude not in suggestions: %+v", items)
	}
	m.suggIdx = claudeIdx
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	am := m2.(*AppModel)
	if am.provider != "claude" {
		t.Fatalf("provider=%q want claude", am.provider)
	}
	if am.model != "opus" {
		t.Fatalf("model should reset to first catalog model, got %q", am.model)
	}
	if !strings.Contains(am.View(), "Provider set to: claude") {
		t.Fatalf("expected set message:\n%s", am.View())
	}
}

func TestModelTabCompletesSelection(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.provider = "codex"
	m.providers = []client.Provider{{
		Key: "codex",
		Models: []client.ProviderModel{
			{ID: "gpt-5.4"},
			{ID: "o3"},
		},
	}}
	m.inputValue = "/model o"
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	if got := m2.(*AppModel).inputValue; got != "/model o3" {
		t.Fatalf("inputValue=%q", got)
	}
}
