package runner

import (
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/flowgate"
)

// Task-455: chat-mode runs must gate as "warn" regardless of the persisted
// gate_mode — enforce/warn is a flow-mode choice.

func writeGateConfigForTest(t *testing.T, dot, mode string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dot, "settings"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dot, "settings", "gate-config.json"),
		[]byte(`{"gate_mode":"`+mode+`"}`), 0o644); err != nil {
		t.Fatalf("write gate config: %v", err)
	}
}

func TestTask455_ChatRunGatesWarnUnderEnforceConfig(t *testing.T) {
	dot := filepath.Join(t.TempDir(), ".flowpilot")
	writeGateConfigForTest(t, dot, "enforce")
	rs := &interactiveRun{id: "run-chat", runKind: "chat"}
	if got := effectiveGateMode(dot, rs); got != "warn" {
		t.Fatalf("chat-mode run must auto-warn under enforce config, got %q", got)
	}
}

func TestTask455_FlowContextRunsKeepConfiguredMode(t *testing.T) {
	dot := filepath.Join(t.TempDir(), ".flowpilot")
	writeGateConfigForTest(t, dot, "enforce")
	cases := map[string]*interactiveRun{
		"workflow root":      {id: "r1", runKind: "workflow"},
		"empty-kind root":    {id: "r2", runKind: ""},
		"workflowID pin":     {id: "r3", runKind: "chat", workflowID: "wf-1"},
		"flow engine live":   {id: "r4", runKind: "chat", flowEngineDriven: true},
		"spawned child":      {id: "r5", runKind: "chat", parentRunID: "r4"},
		"nil run":            nil,
	}
	for name, rs := range cases {
		if got := effectiveGateMode(dot, rs); got != "enforce" {
			t.Fatalf("%s must keep the configured enforce mode, got %q", name, got)
		}
	}
}

func TestTask455_FlowContextRunsKeepWarnConfig(t *testing.T) {
	dot := filepath.Join(t.TempDir(), ".flowpilot")
	writeGateConfigForTest(t, dot, "warn")
	rs := &interactiveRun{id: "r1", runKind: "workflow"}
	if got := effectiveGateMode(dot, rs); got != "warn" {
		t.Fatalf("workflow run must keep the configured warn mode, got %q", got)
	}
}

// A blind-gate trigger (env error / missing baseline) on a chat run must
// emit a warn and NOT block the turn, even with gate_mode:"enforce".
func TestTask455_GateBlindChatRunWarnsNotBlocks(t *testing.T) {
	dot := filepath.Join(t.TempDir(), ".flowpilot")
	writeGateConfigForTest(t, dot, "enforce")
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	rs := &interactiveRun{id: "run-chat-blind", runKind: "chat", gateEpoch: 1}
	svc.runs = map[string]*interactiveRun{"run-chat-blind": rs}
	bl := &flowgate.Baseline{SuitePassed: true}
	diff := []flowgate.ChangedFile{{Path: "pkg/foo.go", Status: "M"}}
	blocked := svc.gateBlindBlocksTurn("run-chat-blind", "turn-1", 1, rs, dot, bl, flowgate.OracleResult{EnvError: "cannot start tests"}, diff)
	if blocked {
		t.Fatal("chat-mode run must not block on gate_blind under enforce config")
	}
}
