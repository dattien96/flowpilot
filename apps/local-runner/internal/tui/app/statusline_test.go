package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestStatusLine_AlwaysShowsYoloAndProjectRow(t *testing.T) {
	m := New(config.ChatConfig{ProjectPath: `C:\working\gate-sandbox`}, "http://127.0.0.1:4317")
	m.yolo = false
	m.projectPath = `C:\working\gate-sandbox`
	m.projectBranch = "main"
	m.project = &client.Project{Name: "gate-sandbox", Path: `C:\working\gate-sandbox`}
	got := m.renderStatusLine()
	lines := strings.Split(got, "\n")
	if len(lines) < 2 {
		t.Fatalf("want 2 status lines, got %q", got)
	}
	if !strings.Contains(lines[0], "YOLO:OFF") {
		t.Fatalf("line1 missing YOLO:OFF: %q", lines[0])
	}
	if !strings.Contains(lines[1], "gate-sandbox") || !strings.Contains(lines[1], "main") {
		t.Fatalf("line2 missing project/branch: %q", lines[1])
	}
}

func TestYoloStatus_FlowModeAutoOn(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.yolo = false
	m.mode = ModeFlow
	m.launch = LaunchArm{FlowRef: "pack/review-loop", Label: "review-loop"}
	if !m.effectiveYolo() {
		t.Fatal("flow mode should force YOLO on")
	}
	if got := m.yoloStatusLabel(); got != "YOLO:ON(auto)" {
		t.Fatalf("label=%q", got)
	}
}

func TestYoloToggle_BlockedInFlowMode(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.yolo = false
	m.mode = ModeFlow
	m.launch = LaunchArm{WorkflowID: "wf-1", Label: "Ship"}
	m2, _ := m.handleSlashCommand("/yolo")
	am := m2.(*AppModel)
	if am.yolo {
		t.Fatal("yolo flag should not toggle in flow mode")
	}
	if !strings.Contains(am.View(), "auto-on in flow mode") {
		t.Fatalf("expected blocked message:\n%s", am.View())
	}
}
