package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// User request: typing `/provider` + TAB must show the refresh/reload action
// (CA-687 model re-detection without restart) — previously the picker listed
// only account/connect/install/config while the handler supported refresh.
// New file; the pre-existing count assertion in
// picker_model_reasoning_test.go was updated 7→8 as the direct consequence
// of this requested row (disclosed edit).

func TestProviderRefreshActionListedInPicker(t *testing.T) {
	providers := []client.Provider{
		{Key: "codex", Label: "Codex", Installed: true, Models: []client.ProviderModel{{ID: "o3"}}},
	}
	all := filterProviderSuggestions("/provider ", providers, nil, "")
	var refresh *suggestItem
	for i := range all {
		if all[i].value == "refresh" {
			refresh = &all[i]
			break
		}
	}
	if refresh == nil {
		t.Fatalf("refresh action missing from /provider picker: %+v", all)
	}
	if refresh.kind != "provider-action" {
		t.Fatalf("refresh kind = %q, want provider-action", refresh.kind)
	}
	if !strings.Contains(refresh.detail, "reload") {
		t.Fatalf("refresh detail must mention the reload alias, got %q", refresh.detail)
	}
	// "re" prefix must surface it as you type.
	typed := filterProviderSuggestions("/provider re", providers, nil, "")
	found := false
	for _, it := range typed {
		if it.value == "refresh" {
			found = true
		}
	}
	if !found {
		t.Fatalf("refresh row must survive the 're' filter: %+v", typed)
	}
}

func TestProviderImmediateAction(t *testing.T) {
	for _, v := range []string{"refresh", "reload", "Refresh", " reload "} {
		if !providerImmediateAction(v) {
			t.Fatalf("%q must be an immediate action", v)
		}
	}
	for _, v := range []string{"connect", "install", "account", "config", ""} {
		if providerImmediateAction(v) {
			t.Fatalf("%q must expand the next picker, not execute", v)
		}
	}
}

// TestEnter_ExecutesProviderRefreshImmediately locks the Enter contract: the
// refresh row runs /provider refresh right away instead of expanding a next
// provider picker (the generic provider-action behavior).
func TestEnter_ExecutesProviderRefreshImmediately(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.providers = []client.Provider{
		{Key: "codex", Label: "Codex", Installed: true, Models: []client.ProviderModel{{ID: "o3"}}},
	}
	m.inputValue = "/provider "
	items := m.collectSuggestions()
	refreshIdx := -1
	for i, it := range items {
		if it.kind == "provider-action" && it.value == "refresh" {
			refreshIdx = i
			break
		}
	}
	if refreshIdx < 0 {
		t.Fatalf("refresh not in suggestions: %+v", items)
	}
	m.suggIdx = refreshIdx
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	am := m2.(*AppModel)
	if !strings.Contains(am.View(), "Refreshing provider catalog") {
		t.Fatalf("expected refresh execution message:\n%s", am.View())
	}
}
