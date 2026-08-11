package runner

import (
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/flowgate"
)

func TestGateBlindBlocksTurnEnforceMissingBaseline(t *testing.T) {
	dir := t.TempDir()
	dot := filepath.Join(dir, ".flowpilot")
	_ = os.MkdirAll(filepath.Join(dot, "settings"), 0o755)
	if err := os.WriteFile(filepath.Join(dot, "settings", "gate-config.json"), []byte(`{"gate_mode":"enforce"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	rs := &interactiveRun{id: "run-blind", lastTurnStepID: "step-1", gateEpoch: 1}
	svc.runs = map[string]*interactiveRun{"run-blind": rs}
	diff := []flowgate.ChangedFile{{Path: "src/main.go", Status: "M"}}
	blocked := svc.gateBlindBlocksTurn("run-blind", "turn-1", 1, rs, dot, nil, flowgate.OracleResult{}, diff)
	if !blocked {
		t.Fatal("expected enforce blind block for missing baseline + code diff")
	}
}

func TestGateBlindDocsOnlyDoesNotBlock(t *testing.T) {
	dir := t.TempDir()
	dot := filepath.Join(dir, ".flowpilot")
	_ = os.MkdirAll(filepath.Join(dot, "settings"), 0o755)
	_ = os.WriteFile(filepath.Join(dot, "settings", "gate-config.json"), []byte(`{"gate_mode":"enforce"}`), 0o644)
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	rs := &interactiveRun{id: "run-docs", gateEpoch: 1}
	svc.runs = map[string]*interactiveRun{"run-docs": rs}
	diff := []flowgate.ChangedFile{{Path: "change-audit/CA-999.md", Status: "A"}}
	blocked := svc.gateBlindBlocksTurn("run-docs", "turn-1", 1, rs, dot, nil, flowgate.OracleResult{}, diff)
	if blocked {
		t.Fatal("docs-only must not trigger gate_blind block")
	}
}

func TestGateBlindEnvErrorBlocksEnforce(t *testing.T) {
	dir := t.TempDir()
	dot := filepath.Join(dir, ".flowpilot")
	_ = os.MkdirAll(filepath.Join(dot, "settings"), 0o755)
	_ = os.WriteFile(filepath.Join(dot, "settings", "gate-config.json"), []byte(`{"gate_mode":"enforce"}`), 0o644)
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	rs := &interactiveRun{id: "run-env", gateEpoch: 1}
	svc.runs = map[string]*interactiveRun{"run-env": rs}
	bl := &flowgate.Baseline{SuitePassed: true}
	diff := []flowgate.ChangedFile{{Path: "pkg/foo.go", Status: "M"}}
	blocked := svc.gateBlindBlocksTurn("run-env", "turn-1", 1, rs, dot, bl, flowgate.OracleResult{EnvError: "cannot start tests"}, diff)
	if !blocked {
		t.Fatal("expected env_error blind block")
	}
}
