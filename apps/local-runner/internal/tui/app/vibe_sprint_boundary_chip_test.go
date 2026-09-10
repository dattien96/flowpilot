package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func sprintBoundaryBlockedModel(pk, reason string) *AppModel {
	m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.asciiMode = true
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, FlowRef: "vibe-sprint", Label: "vibe-sprint"}
	m.runHandle = &client.RunHandle{RunID: "run-223416", Status: "running"}
	m.flowLoopStatus = "blocked"
	m.flowBlockReason = reason
	m.flowGateReason = "Sprint 1/3 done (Task-904-sprint1-grid-snake-food-collision.md). Continue to sprint 2/3 (Task-905-sprint2-tick-loop-wasd-input.md)?"
	m.agentRuns = []client.AgentRunSummary{{RunID: "run-223416", AgentName: "main", Role: "main", Status: "completed"}}
	return m
}

func TestSprintBoundary_RendersContinueChipNotRetry(t *testing.T) {
	for _, pk := range []string{"codex", "claude", "grok"} {
		t.Run(pk, func(t *testing.T) {
			view := stripANSI(sprintBoundaryBlockedModel(pk, "vibe_sprint_boundary").View())
			if !strings.Contains(view, "[Continue]") {
				t.Fatalf("vibe_sprint_boundary must render [Continue]:\n%s", view)
			}
			if strings.Contains(view, "[Retry]") {
				t.Fatalf("vibe_sprint_boundary must not render [Retry]:\n%s", view)
			}
			if !strings.Contains(view, "[Stop]") {
				t.Fatalf("vibe_sprint_boundary must keep [Stop]:\n%s", view)
			}
			if !strings.Contains(view, "starts the next sprint") {
				t.Fatalf("Continue copy missing:\n%s", view)
			}
			items := sprintBoundaryBlockedModel(pk, "vibe_sprint_boundary").actionRingItems()
			if len(items) < 1 || items[0].target != "retry" || items[0].label != "[Continue]" {
				t.Fatalf("ring[0]=%+v want {retry [Continue]}", items[0])
			}
		})
	}
	// Negative control: any other reason keeps the stock [Retry] chip, so the
	// boundary label cannot leak onto unrelated blocked cards.
	control := stripANSI(sprintBoundaryBlockedModel("codex", "escalate").View())
	if !strings.Contains(control, "[Retry]") {
		t.Fatalf("non-boundary block must still render [Retry]:\n%s", control)
	}
	if strings.Contains(control, "[Continue]") {
		t.Fatalf("non-boundary block must not render [Continue]:\n%s", control)
	}
}
