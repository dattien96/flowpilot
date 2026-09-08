package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
	"flowpilot-runner/internal/tui/prefs"
	"flowpilot-runner/internal/workingmode"
)

func mixedFlowBuiltins() []client.BuiltinFlowOption {
	opts := []client.BuiltinFlowOption{
		{FlowRef: "vibe-ingest", Label: "vibe-ingest"},
		{FlowRef: "vibe-sprint", Label: "vibe-sprint"},
		{FlowRef: "vibe-owner-debate", Label: "vibe-owner-debate"},
	}
	for _, id := range workingmode.DevHarnessFive {
		opts = append(opts, client.BuiltinFlowOption{FlowRef: id, Label: id})
	}
	return opts
}

func suggestionIDs(items []suggestItem) map[string]struct{} {
	out := map[string]struct{}{}
	for _, it := range items {
		if it.value != "" {
			out[workingmode.BareFlowID(it.value)] = struct{}{}
		}
	}
	return out
}

func tuiModelWithFlows(t *testing.T) *AppModel {
	t.Helper()
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.flowBuiltins = mixedFlowBuiltins()
	return m
}

func collectFlowLine(m *AppModel) []suggestItem {
	m.inputValue = "/flow "
	return m.collectSuggestions()
}

// Scenario: /vibe then /flow suggestions are only vibe-ingest.
func TestTUIFlowSuggest_VibeOnlyIngest(t *testing.T) {
	m := tuiModelWithFlows(t)
	m2, _ := m.handleSlashCommand("/vibe")
	am := m2.(*AppModel)
	ids := suggestionIDs(collectFlowLine(am))
	if _, ok := ids["vibe-ingest"]; !ok {
		t.Fatalf("vibe /flow missing ingest: %v", ids)
	}
	if len(ids) != 1 {
		t.Fatalf("vibe /flow ids=%v, want only vibe-ingest", ids)
	}
}

// Scenario: /vibe off then /flow omits vibe-*.
func TestTUIFlowSuggest_DevOmitsVibe(t *testing.T) {
	m := tuiModelWithFlows(t)
	m2, _ := m.handleSlashCommand("/vibe")
	m3, _ := m2.(*AppModel).handleSlashCommand("/vibe off")
	am := m3.(*AppModel)
	ids := suggestionIDs(collectFlowLine(am))
	for _, id := range workingmode.DevHarnessFive {
		if _, ok := ids[id]; !ok {
			t.Fatalf("dev /flow missing %s: %v", id, ids)
		}
	}
	for _, id := range []string{"vibe-ingest", "vibe-sprint", "vibe-owner-debate"} {
		if _, ok := ids[id]; ok {
			t.Fatalf("dev /flow leaked %s", id)
		}
	}
}

// Scenario: /vibe persists as next-start default.
func TestTUIVibeCommand_OnOffPersistsDefault(t *testing.T) {
	isolateSessionFile(t)
	m := tuiModelWithFlows(t)
	m2, _ := m.handleSlashCommand("/vibe")
	saved, _, err := prefs.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.WorkingMode != workingmode.Vibe {
		t.Fatalf("saved=%q, want vibe", saved.WorkingMode)
	}
	if m2.(*AppModel).workingMode != workingmode.Vibe {
		t.Fatal("model workingMode not vibe")
	}
	m3, _ := m2.(*AppModel).handleSlashCommand("/vibe off")
	saved, _, err = prefs.Load()
	if err != nil {
		t.Fatal(err)
	}
	if saved.WorkingMode != workingmode.Dev {
		t.Fatalf("saved after off=%q, want dev", saved.WorkingMode)
	}
	if m3.(*AppModel).workingMode != workingmode.Dev {
		t.Fatal("model workingMode not dev")
	}
}

// Scenario: typing /flow vibe in dev does not arm ingest.
func TestTUIFlowSuggest_DevQueryVibeDoesNotArm(t *testing.T) {
	m := tuiModelWithFlows(t)
	m2, _ := m.handleSlashCommand("/vibe off")
	am := m2.(*AppModel)
	am.inputValue = "/flow vibe"
	items := am.collectSuggestions()
	for _, it := range items {
		if workingmode.BareFlowID(it.value) == "vibe-ingest" {
			t.Fatalf("dev query vibe armed ingest: %+v", items)
		}
	}
	m3, _ := am.handleSlashCommand("/flow vibe-ingest")
	view := m3.(*AppModel).View()
	if strings.Contains(view, "Flow armed") && strings.Contains(view, "vibe-ingest") {
		t.Fatalf("dev must not arm vibe-ingest:\n%s", view)
	}
}

// Scenario: /flow vibe-owner-debate in vibe is rejected with frozen code.
func TestTUIFlowStart_OwnerDebateRejected(t *testing.T) {
	m := tuiModelWithFlows(t)
	m2, _ := m.handleSlashCommand("/vibe")
	m3, _ := m2.(*AppModel).handleSlashCommand("/flow vibe-owner-debate")
	am := m3.(*AppModel)
	view := am.View()
	if !strings.Contains(view, workingmode.CodeFlowForbidden) {
		t.Fatalf("want %s in view:\n%s", workingmode.CodeFlowForbidden, view)
	}
	if am.launch.IsArmed() {
		t.Fatalf("launch armed: %+v", am.launch)
	}
}

// Scenario: /flow task-harness in vibe is rejected with frozen code.
func TestTUIFlowStart_HarnessRejectedInVibe(t *testing.T) {
	m := tuiModelWithFlows(t)
	m2, _ := m.handleSlashCommand("/vibe")
	m3, _ := m2.(*AppModel).handleSlashCommand("/flow task-harness")
	am := m3.(*AppModel)
	view := am.View()
	if !strings.Contains(view, workingmode.CodeFlowForbidden) {
		t.Fatalf("want %s in view:\n%s", workingmode.CodeFlowForbidden, view)
	}
	if am.launch.IsArmed() {
		t.Fatalf("launch armed: %+v", am.launch)
	}
}
