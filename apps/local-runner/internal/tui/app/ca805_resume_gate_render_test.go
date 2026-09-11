package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestCA805_ResumeGateRendersNode(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 120, 40
	m.asciiMode = true
	m.runHandle = &client.RunHandle{RunID: "run-220036"}
	m.applyPendingFromSnapshot(client.RunSnapshot{
		RunID:       "run-220036",
		Status:      "blocked",
		PendingGate: &client.GateInfo{Options: []string{"ok", "cancel"}, ResumeFrom: "coder"},
	})
	if m.gate == nil || m.gate.ResumeFrom != "coder" {
		t.Fatalf("gate=%#v want ResumeFrom coder", m.gate)
	}
	joined := ""
	for _, msg := range m.messages {
		joined += msg.Content
	}
	if !strings.Contains(joined, "Resume from coder?") {
		t.Fatalf("gate card must name resume node, got:\n%s", joined)
	}
}

func TestCA805_StaleVibeGateDismissed(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 120, 40
	m.runHandle = &client.RunHandle{RunID: "run-220036"}
	m.applyPendingFromSnapshot(client.RunSnapshot{
		RunID:       "run-220036",
		Status:      "blocked",
		PendingGate: &client.GateInfo{Options: []string{"ok", "cancel"}},
	})
	if m.gate == nil {
		t.Fatal("gate must arm first")
	}
	m.applyPendingFromSnapshot(client.RunSnapshot{RunID: "run-220036", Status: "running"})
	if m.gate != nil {
		t.Fatalf("stale ok/cancel gate must dismiss, got %#v", m.gate)
	}
}

func TestCA805_NonVibeGateKept(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 120, 40
	m.runHandle = &client.RunHandle{RunID: "run-1"}
	m.gate = &GateState{Options: []string{"keep-test-fix-code", "suggest-requirement-change", "custom"}, RunID: "run-1"}
	m.applyPendingFromSnapshot(client.RunSnapshot{RunID: "run-1", Status: "running"})
	if m.gate == nil {
		t.Fatal("non-vibe gate must not be auto-dismissed")
	}
}
