package app

import (
	"testing"

	"flowpilot-runner/internal/tui/config"
	"flowpilot-runner/internal/workingmode"
)

func TestTUIVibe_AutoDetectsCPPath(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	path := "requirements/07-Coding-Plan/inprogress/CP-60-Vibe-Working-Mode.md"
	m2, _ := m.handleSlashCommand("/vibe " + path)
	am := m2.(*AppModel)
	if am.workingMode != workingmode.Vibe {
		t.Fatalf("workingMode=%q", am.workingMode)
	}
	if am.launch.FlowRef != "vibe-cp-ingest" {
		t.Fatalf("flowRef=%q, want vibe-cp-ingest", am.launch.FlowRef)
	}
	if am.launch.SourceDocID != path {
		t.Fatalf("source=%q", am.launch.SourceDocID)
	}
}
