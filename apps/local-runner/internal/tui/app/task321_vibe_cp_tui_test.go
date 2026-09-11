package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/config"
)

func TestTUIVibeCp_RejectsNonCP(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m2, _ := m.handleSlashCommand("/vibe-cp README.md")
	view := m2.(*AppModel).View()
	if !strings.Contains(view, "Unknown command: /vibe-cp") {
		t.Fatalf("want unknown command:\n%s", view)
	}
	if m2.(*AppModel).launch.IsArmed() {
		t.Fatal("must not arm")
	}
}

func TestTUIVibeCp_ArmsIngest(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	path := "requirements/07-Coding-Plan/inprogress/CP-60-Vibe-Working-Mode.md"
	m2, _ := m.handleSlashCommand("/vibe-cp " + path)
	am := m2.(*AppModel)
	if am.launch.IsArmed() {
		t.Fatal("/vibe-cp removed; arm via /flow vibe-cp-ingest")
	}
	if strings.Contains(am.View(), "CP locked entry armed") {
		t.Fatalf("must not arm via /vibe-cp:\n%s", am.View())
	}
}
