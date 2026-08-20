package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestFilterInitSuggestions_BareAndWithArgs(t *testing.T) {
	sugg := filterInitSuggestions("/init")
	if len(sugg) != 2 {
		t.Fatalf("bare /init want 2, got %d: %+v", len(sugg), sugg)
	}
	names := map[string]bool{}
	for _, s := range sugg {
		names[s.value] = true
	}
	if !names["skill"] || !names["all"] {
		t.Fatalf("want skill+all, got %+v", sugg)
	}
	// With space
	sugg = filterInitSuggestions("/init ")
	if len(sugg) != 2 {
		t.Fatalf("/init space want 2, got %d", len(sugg))
	}
	sugg = filterInitSuggestions("/init s")
	if len(sugg) != 1 || sugg[0].value != "skill" {
		t.Fatalf("/init s want skill, got %+v", sugg)
	}
	sugg = filterInitSuggestions("/init all")
	if len(sugg) != 1 || sugg[0].value != "all" {
		t.Fatalf("/init all want all, got %+v", sugg)
	}
	if filterInitSuggestions("/help") != nil {
		t.Fatal("non-init should be nil")
	}
}

func TestInit_TabCompletesAndEnterDispatches(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	// Tab on "/init" should show picker (collectSuggestions is what Tab uses)
	m.inputValue = "/init"
	sugg := m.collectSuggestions()
	foundSkill := false
	for _, s := range sugg {
		if s.kind == "init" && s.value == "skill" {
			foundSkill = true
		}
	}
	if !foundSkill {
		t.Fatalf("collectSuggestions /init want init/skill, got %+v", sugg)
	}
	// Unknown kind
	m = New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.project = &client.Project{ID: "p1", Name: "proj", Path: "/tmp/ws", Platform: "android"}
	m2, _ := m.handleSlashCommand("/init unknown")
	view := m2.(*AppModel).messages[len(m2.(*AppModel).messages)-1].Content
	if !strings.Contains(view, "Unknown /init kind") {
		t.Fatalf("want Unknown kind error, got %q", view)
	}
	// No args
	m = New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m3, _ := m.handleSlashCommand("/init")
	view = m3.(*AppModel).messages[len(m3.(*AppModel).messages)-1].Content
	if !strings.Contains(view, "Usage: /init") {
		t.Fatalf("want Usage, got %q", view)
	}
	// No project
	m = New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.project = nil
	m4, cmd := m.handleSlashCommand("/init skill")
	if cmd != nil {
		t.Fatal("no project should not return cmd")
	}
	view = m4.(*AppModel).messages[len(m4.(*AppModel).messages)-1].Content
	if !strings.Contains(view, "No project bound") {
		t.Fatalf("want No project bound, got %q", view)
	}
}
