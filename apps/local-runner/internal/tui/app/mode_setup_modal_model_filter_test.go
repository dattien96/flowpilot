package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// User request: the mode-setup modal's model picker must support the same
// type-to-filter UX as the /model input picker — typing narrows the list,
// Backspace deletes, Esc closes, "(inherit)" stays first, and the narrowed
// selection applies to the draft profile.

func modeSetupFilterTestModel(t *testing.T) *AppModel {
	t.Helper()
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.providers = []client.Provider{
		{Key: "opencode", Label: "Opencode", Installed: true, Models: []client.ProviderModel{
			{ID: "opencode-go/longcat-2.0"},
			{ID: "opencode-go/muse-spark-1.2"},
		}},
		{Key: "grok", Label: "Grok", Installed: true, Models: []client.ProviderModel{
			{ID: "grok-4.5"},
			{ID: "grok-4.6"},
		}},
	}
	m.openModeSetupModal(client.ChatPostureConfig{Active: "code"}, "code")
	return m
}

func TestModeSetupModelPicker_TypeToFilterAndSelect(t *testing.T) {
	m := modeSetupFilterTestModel(t)
	m2, _ := m.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEnter})
	am := m2.(*AppModel)
	if !am.modeSetupModalPickerOpen || am.modeSetupModalPickerKind != "model" {
		t.Fatal("model picker must open")
	}
	// Type "long" → only longcat matches; "(inherit)" is hidden while a
	// filter is active (same as /model); cursor resets.
	m2, _ = am.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("long")})
	am = m2.(*AppModel)
	visible := am.modalPickerVisibleOptions()
	if len(visible) != 1 || visible[0] != "opencode-go/longcat-2.0" {
		t.Fatalf("filtered=%v", visible)
	}
	if am.modeSetupModalPickerIdx != 0 {
		t.Fatalf("filter change must reset cursor, idx=%d", am.modeSetupModalPickerIdx)
	}
	if !strings.Contains(am.View(), "Search: long") {
		t.Fatalf("search echo missing:\n%s", am.View())
	}
	// Enter selects the filtered model and infers the provider.
	m2, _ = am.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEnter})
	am = m2.(*AppModel)
	prof := am.modeSetupModalDraft.Profiles[am.modeSetupModalTab]
	if prof.Model != "opencode-go/longcat-2.0" {
		t.Fatalf("model=%q", prof.Model)
	}
	if prof.Provider != "opencode" {
		t.Fatalf("provider=%q", prof.Provider)
	}
	if am.modeSetupModalPickerOpen || am.modeSetupModalPickerFilter != "" {
		t.Fatal("selection must close the picker and clear the filter")
	}
}

func TestModeSetupModelPicker_BackspaceAndEsc(t *testing.T) {
	m := modeSetupFilterTestModel(t)
	m2, _ := m.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEnter})
	am := m2.(*AppModel)
	m2, _ = am.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("grok")})
	am = m2.(*AppModel)
	if got := len(am.modalPickerVisibleOptions()); got != 2 {
		t.Fatalf("grok filter → %d rows, want 2 (inherit hidden while filtering)", got)
	}
	// Backspace removes one rune; "gro" still matches both grok models.
	m2, _ = am.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyBackspace})
	am = m2.(*AppModel)
	if am.modeSetupModalPickerFilter != "gro" {
		t.Fatalf("filter=%q", am.modeSetupModalPickerFilter)
	}
	if got := len(am.modalPickerVisibleOptions()); got != 2 {
		t.Fatalf("gro filter → %d rows, want 2", got)
	}
	// Esc closes the picker and clears the filter.
	m2, _ = am.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEsc})
	am = m2.(*AppModel)
	if am.modeSetupModalPickerOpen || am.modeSetupModalPickerFilter != "" {
		t.Fatal("esc must close the picker and clear the filter")
	}
}

func TestModeSetupModelPicker_NoMatchEmptyStateAndEnterCloses(t *testing.T) {
	m := modeSetupFilterTestModel(t)
	m2, _ := m.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEnter})
	am := m2.(*AppModel)
	m2, _ = am.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("zzz")})
	am = m2.(*AppModel)
	if got := am.modalPickerVisibleOptions(); len(got) != 0 {
		t.Fatalf("zzz must match nothing, got %v", got)
	}
	if !strings.Contains(am.View(), "(no matching models)") {
		t.Fatalf("empty state missing:\n%s", am.View())
	}
	// Enter with no options closes the picker without applying anything.
	m2, _ = am.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEnter})
	am = m2.(*AppModel)
	if am.modeSetupModalPickerOpen {
		t.Fatal("Enter on an empty picker must close it")
	}
	prof := am.modeSetupModalDraft.Profiles[am.modeSetupModalTab]
	if prof.Model != "" {
		t.Fatalf("no selection must not set a model, got %q", prof.Model)
	}
}

func TestModeSetupModelPicker_NonModelKindsDoNotFilter(t *testing.T) {
	m := modeSetupFilterTestModel(t)
	prof := m.modeSetupModalDraft.Profiles["code"]
	prof.Model = "grok-4.5"
	m.modeSetupModalDraft.Profiles["code"] = prof
	m.modeSetupModalFocus = modalFocusYolo
	m2, _ := m.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyEnter})
	am := m2.(*AppModel)
	if !am.modeSetupModalPickerOpen || am.modeSetupModalPickerKind != "yolo" {
		t.Fatal("yolo picker must open")
	}
	// Typing in the yolo picker must not create a filter (scope: model only).
	m2, _ = am.handleModeSetupModalKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("on")})
	am = m2.(*AppModel)
	if am.modeSetupModalPickerFilter != "" {
		t.Fatalf("yolo picker must not filter, got %q", am.modeSetupModalPickerFilter)
	}
	if got := len(am.modalPickerOptions()); got != len(modalYoloOptions) {
		t.Fatalf("yolo options=%d, want %d", got, len(modalYoloOptions))
	}
}
