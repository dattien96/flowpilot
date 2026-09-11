package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestCA801_PausedGateMessageOKCancelOnly(t *testing.T) {
	msg := buildGateMessage("block", "", []string{"ok", "cancel"}, nil)
	for _, want := range []string{"[GATE] Run paused.", "Continue?", "[OK]", "[Cancel]"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("missing %q in:\n%s", want, msg)
		}
	}
	if strings.Contains(msg, "[Retry]") || strings.Contains(msg, "[Stop]") {
		t.Fatalf("must not show Retry/Stop:\n%s", msg)
	}
}

func TestCA801_OpenSnapshotArmsPausedGate(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 120, 40
	m.asciiMode = true
	m.runHandle = &client.RunHandle{RunID: "run-220036"}
	m.applyPendingFromSnapshot(client.RunSnapshot{
		RunID:       "run-220036",
		Status:      "blocked",
		PendingGate: &client.GateInfo{Options: []string{"ok", "cancel"}},
	})
	if m.gate == nil || len(m.gate.Options) != 2 {
		t.Fatalf("gate=%#v", m.gate)
	}
	view := strings.Join([]string{stripANSI(m.View())}, "")
	joined := view
	for _, msg := range m.messages {
		joined += msg.Content
	}
	if !strings.Contains(joined, "[OK]") || !strings.Contains(joined, "[Cancel]") {
		t.Fatalf("open must show OK/Cancel, got:\n%s", joined)
	}
	if m.flowLoopBlocked() {
		t.Fatal("armed gate must hide Continue/Stop bar")
	}
}
