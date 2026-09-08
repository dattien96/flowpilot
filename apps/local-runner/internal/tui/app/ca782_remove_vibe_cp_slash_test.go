package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/config"
)

func TestCA782_VibeCpNotInSlashCatalog(t *testing.T) {
	for _, sc := range knownSlashCommands {
		if sc.name == "/vibe-cp" {
			t.Fatal("/vibe-cp must not be in knownSlashCommands")
		}
	}
	if got := filterSlashSuggestions("/vibe-cp"); len(got) != 0 {
		t.Fatalf("slash picker still offers /vibe-cp: %+v", got)
	}
}

func TestCA782_VibeCpUnknownCommand(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m2, _ := m.handleSlashCommand("/vibe-cp README.md")
	am := m2.(*AppModel)
	if am.launch.IsArmed() {
		t.Fatal("must not arm")
	}
	if !strings.Contains(am.View(), "Unknown command: /vibe-cp") {
		t.Fatalf("view:\n%s", am.View())
	}
}
