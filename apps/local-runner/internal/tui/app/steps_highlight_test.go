package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestStyleForStepBannerLine_HighlightsRunning(t *testing.T) {
	got := styleForStepBannerLine("  >  2. [RUNNING] grok-context")
	if got.GetBold() != styleStepRunning.GetBold() {
		t.Fatalf("expected running style bold")
	}
	idle := styleForStepBannerLine("Flow steps (3):")
	if idle.GetBold() {
		t.Fatalf("header should not use running bold style")
	}
}

func TestFlowStepsPanelLines_ColorMarksRunning(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.flowSteps = []client.WorkflowStepRuntime{
		{StepID: "a", NodeID: "coder", Status: "DONE"},
		{StepID: "b", NodeID: "grok-context", Status: "RUNNING"},
	}
	m.flowStepsActive = "grok-context"
	lines := m.flowStepsPanelLines()
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "grok-context") || !strings.Contains(joined, "RUNNING") {
		t.Fatalf("panel missing running step: %q", joined)
	}
	if !strings.Contains(joined, "Now: grok-context") {
		t.Fatalf("panel missing Now line: %q", joined)
	}
}

func TestStepsRuntimeMsg_SetsStatusToActiveStep(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.runHandle = &client.RunHandle{RunID: "run-1"}
	m.launch = LaunchArm{WorkflowID: "wf-1", Label: "grok-flow", Mode: ModeFlow}
	m.mode = ModeFlow
	m.connStatus = ConnRunning
	m2, _ := m.Update(StepsRuntimeMsg{
		RunID: "run-1",
		Steps: []client.WorkflowStepRuntime{
			{StepID: "a", NodeID: "grok-context", Status: "RUNNING"},
		},
	})
	am := m2.(*AppModel)
	if !strings.Contains(am.statusMsg, "grok-context") {
		t.Fatalf("statusMsg=%q", am.statusMsg)
	}
	if !strings.Contains(am.View(), "grok-context") {
		t.Fatalf("view missing highlighted step:\n%s", am.View())
	}
}

func TestImagePasteSlash_DispatchesClipboardCmd(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.provider = "codex"
	m2, cmd := m.handleSlashCommand("/image paste")
	if cmd == nil {
		t.Fatal("expected clipboard paste cmd")
	}
	_ = m2
}
