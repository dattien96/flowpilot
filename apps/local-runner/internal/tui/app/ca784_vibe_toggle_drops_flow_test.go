package app

import (
	"testing"

	"flowpilot-runner/internal/tui/config"
	"flowpilot-runner/internal/workingmode"
)

func TestTUIVibeOn_DropsArmedHarnessToChat(t *testing.T) {
	isolateSessionFile(t)
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, FlowRef: "task-harness", Label: "Task Harness"}
	m.firstTurnPending = true
	m2, _ := m.handleSlashCommand("/vibe on")
	am := m2.(*AppModel)
	if am.workingMode != workingmode.Vibe {
		t.Fatalf("workingMode=%q", am.workingMode)
	}
	if am.mode != ModeChat {
		t.Fatalf("mode=%s want chat", am.mode)
	}
	if am.launch.IsArmed() {
		t.Fatalf("launch still armed: %+v", am.launch)
	}
}

func TestTUIVibeOff_DropsArmedVibeFlowToChat(t *testing.T) {
	isolateSessionFile(t)
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.workingMode = workingmode.Vibe
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, FlowRef: "vibe-ingest", Label: "vibe-ingest"}
	m.firstTurnPending = true
	m2, _ := m.handleSlashCommand("/vibe off")
	am := m2.(*AppModel)
	if am.workingMode != workingmode.Dev {
		t.Fatalf("workingMode=%q", am.workingMode)
	}
	if am.mode != ModeChat {
		t.Fatalf("mode=%s want chat", am.mode)
	}
	if am.launch.IsArmed() {
		t.Fatalf("launch still armed: %+v", am.launch)
	}
}

func TestTUIVibeRequirement_StillArmsIngest(t *testing.T) {
	isolateSessionFile(t)
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, FlowRef: "task-harness", Label: "Task Harness"}
	m2, _ := m.handleSlashCommand("/vibe snake game")
	am := m2.(*AppModel)
	if am.workingMode != workingmode.Vibe {
		t.Fatalf("workingMode=%q", am.workingMode)
	}
	if am.launch.FlowRef != "vibe-ingest" {
		t.Fatalf("flowRef=%q want vibe-ingest", am.launch.FlowRef)
	}
	if am.mode != ModeFlow {
		t.Fatalf("mode=%s want flow", am.mode)
	}
}
