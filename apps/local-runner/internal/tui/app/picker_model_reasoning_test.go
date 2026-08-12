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
		{Key: "codex", Label: "Codex", Models: []client.ProviderModel{{ID: "o3"}}},
		{Key: "claude", Label: "Claude", Models: []client.ProviderModel{{ID: "opus"}, {ID: "sonnet"}}},
		{Key: "grok", Name: "Grok"},
	}
	all := filterProviderSuggestions("/provider ", providers, "codex")
	if len(all) != 3 {
		t.Fatalf("expected 3, got %d", len(all))
	}
	filtered := filterProviderSuggestions("/provider cl", providers, "codex")
	if len(filtered) != 1 || filtered[0].value != "claude" {
		t.Fatalf("filtered=%+v", filtered)
	}
	if filterProviderSuggestions("/provider", providers, "codex") != nil {
		t.Fatal("bare /provider should not open picker")
	}
}

func TestEnter_AcceptsHighlightedProviderSuggestion(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.providers = []client.Provider{
		{Key: "codex", Models: []client.ProviderModel{{ID: "o3"}}},
		{Key: "claude", Models: []client.ProviderModel{{ID: "opus"}}},
	}
	m.inputValue = "/provider "
	m.suggIdx = 1
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
