package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func vibeLockBlockedModel(pk string) *AppModel {
	m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.asciiMode = true
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, FlowRef: "vibe-ingest", Label: "vibe-ingest"}
	m.runHandle = &client.RunHandle{RunID: "run-213333", Status: "running"}
	m.flowLoopStatus = "blocked"
	m.flowBlockReason = "vibe_lock"
	m.flowGateReason = "SS Preview & Lock\nEmpty Continue locks the current draft."
	m.agentRuns = []client.AgentRunSummary{{RunID: "run-213333", AgentName: "main", Role: "main", Status: "completed"}}
	return m
}

func TestVibeLock_RendersLockChipNotRetry(t *testing.T) {
	view := stripANSI(vibeLockBlockedModel("codex").View())
	if !strings.Contains(view, "[Lock]") {
		t.Fatalf("vibe_lock must render [Lock]:\n%s", view)
	}
	if strings.Contains(view, "[Retry]") {
		t.Fatalf("vibe_lock must not render [Retry]:\n%s", view)
	}
	if !strings.Contains(view, "lock current draft") {
		t.Fatalf("Lock copy missing:\n%s", view)
	}
	items := vibeLockBlockedModel("codex").actionRingItems()
	if len(items) < 1 || items[0].target != "retry" || items[0].label != "[Lock]" {
		t.Fatalf("ring[0]=%+v want {retry [Lock]}", items[0])
	}
}
