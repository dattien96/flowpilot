package app

import (
	"testing"

	"flowpilot-runner/internal/tui/config"
	"flowpilot-runner/internal/tui/prefs"
)

func TestApplySavedModeAndFlow_VibeIngestOmitsBugSubMode(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	applySavedModeAndFlow(m, prefs.Session{
		Mode:    "flow",
		FlowRef: "vibe-ingest",
	})
	if m.launch.FlowRef != "vibe-ingest" || !m.firstTurnPending {
		t.Fatalf("arm=%+v firstTurn=%v", m.launch, m.firstTurnPending)
	}
	if m.launch.SubMode == "bug" {
		t.Fatal("restored vibe-ingest must not send subMode=bug")
	}
}
