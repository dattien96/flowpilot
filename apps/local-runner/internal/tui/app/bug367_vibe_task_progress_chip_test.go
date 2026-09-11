package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestBUG367_StepsHeaderShowsTaskChip(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:0")
			m.asciiMode = true
			m.applyAgentGraph(&client.AgentGraphSnapshot{
				LoopState: client.AgentLoopState{
					Status:        "running",
					Round:         0,
					RoundCap:      3,
					VibeTaskIndex: 1,
					VibeTaskTotal: 3,
					VibeTaskName:  "Task-910-core.md",
				},
			})
			title := stripANSI(m.stepsSectionTitle())
			if !strings.Contains(title, "task 1/3") {
				t.Fatalf("%s: header missing task 1/3: %q", pk, title)
			}
			if !strings.Contains(title, "Task-910-core.md") {
				t.Fatalf("%s: header missing task name: %q", pk, title)
			}
			if !strings.Contains(title, "round 0/3") {
				t.Fatalf("%s: round chip must remain: %q", pk, title)
			}
		})
	}
}

func TestBUG367_ComposerShowsTaskXY(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:0")
			m.asciiMode = true
			m.mode = ModeFlow
			m.launch = LaunchArm{Mode: ModeFlow, FlowRef: "vibe-sprint", Label: "vibe-sprint"}
			m.vibeTaskIndex = 2
			m.vibeTaskTotal = 3
			title := stripANSI(m.chatFrameTitle())
			if !strings.Contains(title, "vibe-sprint") {
				t.Fatalf("%s: missing flow name: %q", pk, title)
			}
			if !strings.Contains(title, "task 2/3") {
				t.Fatalf("%s: composer missing task 2/3: %q", pk, title)
			}
		})
	}
}

func TestBUG367_NoTaskChipWithoutPlan(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:0")
	m.asciiMode = true
	m.applyAgentGraph(&client.AgentGraphSnapshot{
		LoopState: client.AgentLoopState{Status: "running", Round: 1, RoundCap: 3},
	})
	title := stripANSI(m.stepsSectionTitle())
	if title != "steps  round 1/3" {
		t.Fatalf("non-vibe header must stay round-only, got %q", title)
	}
}
