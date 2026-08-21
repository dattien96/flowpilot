package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// Regression: Mode was "code" model A. User opens posture setup, edits code -> B, Enter (save).
// Expect live m.model == B immediately, not only after TAB /mode.
func TestModeSetupModalSave_ActiveTabUpdatesLive(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.chatPosture = "code"
	m.model = "model-a"
	m.provider = "claude"
	m.providers = []client.Provider{
		{Key: "claude", Models: []client.ProviderModel{{ID: "model-a"}, {ID: "model-b"}}},
	}
	m.chatPostureCfg = client.ChatPostureConfig{
		Active: "code",
		Profiles: map[string]client.ChatPostureProfile{
			"code": {Model: "model-a", Provider: "claude"},
			"plan": {Model: "model-a"},
			"scan": {},
		},
	}
	cfg := m.chatPostureCfg
	m.openModeSetupModal(cfg, "code")
	// Simulate picking model-b in the draft.
	prof := m.modeSetupModalDraft.Profiles["code"]
	prof.Model = "model-b"
	prof.Provider = "claude"
	m.modeSetupModalDraft.Profiles["code"] = prof
	m.modeSetupModalTab = "code"
	m.modeSetupModalFocus = modalFocusSave
	_, cmd := m.handleModeSetupModalKey(mkKeyEnter())
	if cmd == nil {
		t.Fatal("Save must return cmd")
	}
	if m.model != "model-b" {
		t.Fatalf("live model must be B after Save, got %q", m.model)
	}
	if m.provider != "claude" {
		t.Fatalf("provider=%q", m.provider)
	}
}

// Editing inactive tab (scan) while active is code must not affect live session.
func TestModeSetupModalSave_InactiveTabDoesNotAffectLive(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.chatPosture = "code"
	m.model = "model-a"
	m.provider = "claude"
	m.providers = []client.Provider{
		{Key: "claude", Models: []client.ProviderModel{{ID: "model-a"}, {ID: "model-b"}}},
	}
	m.chatPostureCfg = client.ChatPostureConfig{
		Active: "code",
		Profiles: map[string]client.ChatPostureProfile{
			"code": {Model: "model-a", Provider: "claude"},
			"scan": {Model: "model-a"},
		},
	}
	cfg := m.chatPostureCfg
	m.openModeSetupModal(cfg, "scan")
	prof := m.modeSetupModalDraft.Profiles["scan"]
	prof.Model = "model-b"
	prof.Provider = "claude"
	m.modeSetupModalDraft.Profiles["scan"] = prof
	m.modeSetupModalTab = "scan"
	m.modeSetupModalFocus = modalFocusSave
	_, _ = m.handleModeSetupModalKey(mkKeyEnter())
	if m.model != "model-a" {
		t.Fatalf("inactive tab save must not change live model, got %q", m.model)
	}
}

func mkKeyEnter() tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyEnter}
}
