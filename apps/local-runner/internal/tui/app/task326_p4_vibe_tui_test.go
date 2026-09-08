package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/config"
	"flowpilot-runner/internal/workingmode"
)

func TestTUIVibe_ToggleOnDoesNotArm(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m2, _ := m.handleSlashCommand("/vibe on")
	am := m2.(*AppModel)
	if am.workingMode != workingmode.Vibe {
		t.Fatalf("workingMode=%q", am.workingMode)
	}
	if am.launch.IsArmed() {
		t.Fatal("/vibe on must not arm ingest")
	}
}

func TestTUIVibe_RequirementArmsIngest(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m2, _ := m.handleSlashCommand("/vibe fix login timeout")
	am := m2.(*AppModel)
	if am.workingMode != workingmode.Vibe {
		t.Fatalf("workingMode=%q", am.workingMode)
	}
	if am.launch.FlowRef != "vibe-ingest" {
		t.Fatalf("flowRef=%q", am.launch.FlowRef)
	}
	if am.launch.SourceDocID != "fix login timeout" {
		t.Fatalf("source=%q", am.launch.SourceDocID)
	}
	if !strings.Contains(am.View(), "vibe-ingest") {
		t.Fatalf("view:\n%s", am.View())
	}
}
