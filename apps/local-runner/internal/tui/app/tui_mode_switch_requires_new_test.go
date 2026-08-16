package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-530 — switching modes (/chat, /flow <ref>) while a run is open must require
// /new first; arming a flow without a run keeps working (legacy parity).

func TestModeSwitch_ChatBlockedWhileRunOpen(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.mode = ModeFlow
			m.launch = LaunchArm{Mode: ModeFlow, FlowRef: "pack/x", Label: "X"}
			m.runHandle = &client.RunHandle{RunID: "run-1", Status: "running"}

			m2, _ := m.handleSlashCommand("/chat")
			am := m2.(*AppModel)
			if am.mode != ModeFlow {
				t.Fatalf("%s: mode=%v want ModeFlow while run open", pk, am.mode)
			}
			if !am.launch.IsArmed() {
				t.Fatalf("%s: launch must stay armed", pk)
			}
			if am.runHandle == nil {
				t.Fatalf("%s: run must stay open", pk)
			}
			last := am.messages[len(am.messages)-1].Content
			if !strings.Contains(last, "/new") {
				t.Fatalf("%s: expected /new hint, got %q", pk, last)
			}
		})
	}
}

func TestModeSwitch_FlowBlockedWhileRunOpen(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.mode = ModeChat
	m.runHandle = &client.RunHandle{RunID: "run-1", Status: "running"}

	m2, _ := m.handleSlashCommand("/flow pack/x")
	am := m2.(*AppModel)
	if am.mode != ModeChat {
		t.Fatalf("mode=%v want ModeChat while run open", am.mode)
	}
	if am.launch.IsArmed() {
		t.Fatalf("launch must stay unarmed")
	}
	last := am.messages[len(am.messages)-1].Content
	if !strings.Contains(last, "/new") {
		t.Fatalf("expected /new hint, got %q", last)
	}
}

func TestModeSwitch_ChatAllowedWhenNoRun(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, FlowRef: "pack/x", Label: "X"}

	m2, _ := m.handleSlashCommand("/chat")
	am := m2.(*AppModel)
	if am.mode != ModeChat {
		t.Fatalf("mode=%v want ModeChat when no run open", am.mode)
	}
	if am.launch.IsArmed() {
		t.Fatalf("launch must be cleared by /chat without a run")
	}
}
