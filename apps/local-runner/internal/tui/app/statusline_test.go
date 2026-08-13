package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "flowpilot-tui-test-*")
	if err == nil {
		os.Setenv("FLOWPILOT_TUI_SESSION_FILE", filepath.Join(tmpDir, "tui-session.json"))
	}
	code := m.Run()
	if tmpDir != "" {
		os.RemoveAll(tmpDir)
	}
	os.Exit(code)
}

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

func TestStatusLine_ShowsReasoningEffortAndDefaultsToMedium(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.5"}, "http://127.0.0.1:4317")
	m.reasoningEffort = "medium"
	line := m.renderStatusLine()
	if !strings.Contains(line, "reasoning: medium") {
		t.Fatalf("statusline missing 'reasoning: medium': %q", line)
	}
}

func TestWindowSizeMsg_ResponsiveLayout(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.5"}, "http://127.0.0.1:4317")
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 15})
	am := m2.(*AppModel)
	if am.width != 40 || am.height != 15 {
		t.Fatalf("expected width 40, height 15, got w=%d h=%d", am.width, am.height)
	}
	view := am.View()
	lines := strings.Split(view, "\n")
	for i, l := range lines {
		if len([]rune(l)) > 40 {
			t.Fatalf("line %d exceeds terminal width 40 (len=%d): %q", i, len([]rune(l)), l)
		}
	}
}

func TestShortID_PreservesFullRunID(t *testing.T) {
	if got := shortID("run-105935"); got != "run-105935" {
		t.Fatalf("shortID(run-105935) = %q, want run-105935", got)
	}
	if got := shortID("392355a6-5573-44f1-9aa5-4313502a5816"); got != "392355a6" {
		t.Fatalf("shortID(uuid) = %q, want 392355a6", got)
	}
}
