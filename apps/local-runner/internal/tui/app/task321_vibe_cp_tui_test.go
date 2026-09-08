package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/config"
	"flowpilot-runner/internal/workingmode"
)

func TestTUIVibeCp_RejectsNonCP(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m2, _ := m.handleSlashCommand("/vibe-cp README.md")
	view := m2.(*AppModel).View()
	if !strings.Contains(view, "vibe-cp requires") {
		t.Fatalf("want reject:\n%s", view)
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
	if am.workingMode != workingmode.Vibe {
		t.Fatalf("workingMode=%q", am.workingMode)
	}
	if am.launch.FlowRef != "vibe-cp-ingest" {
		t.Fatalf("flowRef=%q", am.launch.FlowRef)
	}
	if am.launch.SourceDocID != path {
		t.Fatalf("source=%q", am.launch.SourceDocID)
	}
	if !strings.Contains(am.View(), "vibe-cp-ingest") {
		t.Fatalf("view:\n%s", am.View())
	}
}
