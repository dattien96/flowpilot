package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestModeSetupBareShowsPosture(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.inputValue = "/mode-setup"
	m.inputCursor = -1
	items := m.collectSuggestions()
	found := map[string]bool{}
	for _, it := range items {
		if it.kind == "mode-setup-posture" {
			found[it.value] = true
		}
	}
	if !found["scan"] {
		t.Fatalf("bare /mode-setup must show scan, got %+v", items)
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

	// Simulate picking "plan" then "model" -> "opus" via wizard staging
	m.inputValue = "/mode-setup plan model "
	m.inputCursor = -1
	items := m.collectSuggestions()
	foundOpus := false
	for i, it := range items {
		if it.value == "plan model opus" {
			m.suggIdx = i
			foundOpus = true
			break
		}
	}
	if !foundOpus {
		t.Fatalf("must find plan model opus in %+v", items)
	}
	// Enter on opus should stage, not PUT immediately
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	am := m2.(*AppModel)
	if !am.modeSetupDraftDirty {
		t.Fatal("must be dirty after staging model")
	}
	if am.modeSetupDraft == nil {
		t.Fatal("draft must exist")
	}
	if am.modeSetupDraft.Profiles["plan"].Model != "opus" {
		t.Fatalf("staged model = %q want opus", am.modeSetupDraft.Profiles["plan"].Model)
	}
	if am.modeSetupDraft.Profiles["plan"].Provider != "claude" {
		t.Fatalf("staged provider inferred = %q want claude", am.modeSetupDraft.Profiles["plan"].Provider)
	}
	if !strings.Contains(am.View(), "Staged plan model") {
		t.Fatalf("must show staged message, got %s", am.View())
	}
	if strings.TrimSpace(am.inputValue) != "/mode-setup plan" {
		t.Fatalf("after staging must return to field picker, got %q", am.inputValue)
	}

	// Stage reasoning as well
	am.inputValue = "/mode-setup plan reasoning "
	am.inputCursor = -1
	items = am.collectSuggestions()
	foundHigh := false
	for i, it := range items {
		if it.value == "plan reasoning high" {
			am.suggIdx = i
			foundHigh = true
			break
		}
	}
	if !foundHigh {
		t.Fatalf("must find reasoning high in %+v", items)
	}
	m3, _ := am.Update(tea.KeyMsg{Type: tea.KeyEnter})
	am2 := m3.(*AppModel)
	if am2.modeSetupDraft.Profiles["plan"].ReasoningEffort != "high" {
		t.Fatalf("staged reasoning = %q want high", am2.modeSetupDraft.Profiles["plan"].ReasoningEffort)
	}
	// Still dirty, now go to save
	am2.inputValue = "/mode-setup"
	am2.inputCursor = -1
	items = am2.collectSuggestions()
	foundSave := false
	for i, it := range items {
		if it.kind == "mode-setup-save" {
			am2.suggIdx = i
			foundSave = true
			break
		}
	}
	if !foundSave {
		t.Fatalf("must show save row when draft dirty, got %+v", items)
	}
	// Enter save should trigger PUT (saving flag)
	m4, cmd := am2.Update(tea.KeyMsg{Type: tea.KeyEnter})
	am3 := m4.(*AppModel)
	if am3.modeSetupDraft != nil {
		t.Fatal("draft must clear after save")
	}
	if !am3.chatPostureSaving {
		t.Fatal("must be saving after save")
	}
	if cmd == nil {
		t.Fatal("save must return PUT cmd")
	}
}

func TestModeSetupWizardBack(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.inputValue = "/mode-setup plan model "
	m.inputCursor = -1
	items := m.collectSuggestions()
	hasBack := false
	for _, it := range items {
		if it.kind == "mode-setup-back" {
			hasBack = true
		}
	}
	if !hasBack {
		t.Fatalf("value picker must have back row, got %+v", items)
	}
	m.inputValue = "/mode-setup plan "
	m.inputCursor = -1
	items = m.collectSuggestions()
	hasBack = false
	for _, it := range items {
		if it.kind == "mode-setup-back" {
			hasBack = true
		}
	}
	if !hasBack {
		t.Fatalf("field picker must have back row, got %+v", items)
	}
}
