package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
	"flowpilot-runner/internal/workingmode"
)

func TestCA800_StatusLabelStripsPackPrefix(t *testing.T) {
	arm := LaunchArm{
		Mode:    ModeFlow,
		FlowRef: workingmode.PackPrefix + "vibe-sprint",
		Label:   workingmode.PackPrefix + "vibe-sprint",
	}
	if got := arm.StatusLabel(); got != "vibe-sprint" {
		t.Fatalf("StatusLabel=%q want vibe-sprint", got)
	}
}

func TestCA800_StatusLabelKeepsHumanCatalogName(t *testing.T) {
	arm := LaunchArm{Mode: ModeFlow, WorkflowID: "wf", Label: "Task Harness"}
	if got := arm.StatusLabel(); got != "Task Harness" {
		t.Fatalf("StatusLabel=%q want Task Harness", got)
	}
}

func TestCA800_ChatFrameTitleShowsBareVibeSprint(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 120, 24
	m.asciiMode = true
	m.mode = ModeFlow
	m.launch = LaunchArm{
		Mode:    ModeFlow,
		FlowRef: workingmode.PackPrefix + "vibe-sprint",
		Label:   workingmode.PackPrefix + "vibe-sprint",
	}
	m.runHandle = &client.RunHandle{RunID: "run-220036", FlowRef: workingmode.PackPrefix + "vibe-sprint", Status: "running"}
	title := stripANSI(m.chatFrameTitle())
	if !strings.Contains(title, "vibe-sprint") {
		t.Fatalf("title missing vibe-sprint: %q", title)
	}
	if strings.Contains(title, "flowpilot-core-flow-p") {
		t.Fatalf("title still truncated pack prefix: %q", title)
	}
}
