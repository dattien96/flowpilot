package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/flowgate"
)

// BUG-632 (live run-174243): a baseline captured red during the TDD-stub
// phase survives the entire dirty sprint — RefreshBaselineIfStale keeps
// "prior truth" while the tree is dirty, which a sprint is by construction
// from scaffold until the audit commit. Meanwhile the gate's own oracle
// RUNS the suite every turn; when that run passes, the green evidence is
// discarded and red_at_capture warns on every code-touching turn, feeding
// drift scoring. Direct suite evidence must supersede a red baseline.

func bug632Repo(t *testing.T, exitCode string) (repoDir, dotFP string) {
	t.Helper()
	repoDir = t.TempDir()
	dotFP = filepath.Join(repoDir, ".flowpilot")
	if err := os.MkdirAll(filepath.Join(dotFP, "settings"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dotFP, "settings", "test-config.json"),
		[]byte(`{"test_command":"./suite.sh"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	bug632WriteSuite(t, repoDir, exitCode)
	return repoDir, dotFP
}

func bug632WriteSuite(t *testing.T, repoDir, exitCode string) {
	t.Helper()
	script := "#!/bin/sh\necho '[       OK ] Fake.Green'\nexit " + exitCode + "\n"
	if err := os.WriteFile(filepath.Join(repoDir, "suite.sh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func bug632Svc() (*InteractiveService, *interactiveRun) {
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	rs := &interactiveRun{id: "run-b632", gateEpoch: 1, flowEngineDriven: true}
	svc.runs = map[string]*interactiveRun{"run-b632": rs}
	return svc, rs
}

// Core defect: red baseline + oracle pass this turn → the baseline must be
// superseded by the verified green result, not warn forever.
func TestBug632_OracleGreenSupersedesRedBaseline(t *testing.T) {
	repoDir, dotFP := bug632Repo(t, "1")
	bl, err := flowgate.CaptureBaselineContext(context.Background(), repoDir, dotFP)
	if err != nil {
		t.Fatalf("capture red baseline: %v", err)
	}
	if bl.SuitePassed {
		t.Fatal("baseline must be red at capture (suite.sh exits 1)")
	}

	// Suite goes green (implementation landed); tree is still "dirty" from
	// the sprint's perspective — RefreshBaselineIfStale would keep the red
	// truth, but the oracle's own suite run is direct evidence.
	bug632WriteSuite(t, repoDir, "0")
	diff := []flowgate.ChangedFile{{Path: "src/main.go", Status: "M"}}
	oracle := flowgate.RunOracleContext(context.Background(), repoDir, bl, diff, nil)
	if !oracle.SuitePassed || oracle.EnvError != "" {
		t.Fatalf("oracle must pass on the now-green suite: %+v", oracle)
	}

	svc, rs := bug632Svc()
	rs.workspaceCwd = repoDir
	blocked := svc.gateBlindBlocksTurn(context.Background(), "run-b632", "turn-1", 1, rs, repoDir, dotFP, bl, oracle, diff)
	if blocked {
		t.Fatal("verified-green suite must not emit gate_blind — the red baseline is superseded, not fail-closed truth")
	}
	fresh, err := flowgate.LoadBaseline(dotFP)
	if err != nil || fresh == nil {
		t.Fatalf("LoadBaseline after supersede: %v", err)
	}
	if !fresh.SuitePassed {
		t.Fatal("superseded baseline must be persisted green (suite_passed=true)")
	}
}

// Fail-closed stays fail-closed: a still-red suite leaves the red baseline
// in place and the blind classification keeps warning/blocking.
func TestBug632_OracleRedKeepsRedBaseline(t *testing.T) {
	repoDir, dotFP := bug632Repo(t, "1")
	bl, err := flowgate.CaptureBaselineContext(context.Background(), repoDir, dotFP)
	if err != nil {
		t.Fatalf("capture red baseline: %v", err)
	}

	diff := []flowgate.ChangedFile{{Path: "src/main.go", Status: "M"}}
	oracle := flowgate.RunOracleContext(context.Background(), repoDir, bl, diff, nil)
	if oracle.SuitePassed {
		t.Fatal("oracle must fail on the still-red suite")
	}

	svc, rs := bug632Svc()
	rs.workspaceCwd = repoDir
	blocked := svc.gateBlindBlocksTurn(context.Background(), "run-b632", "turn-1", 1, rs, repoDir, dotFP, bl, oracle, diff)
	if !blocked {
		t.Fatal("still-red suite + red baseline must keep the enforce blind block")
	}
	fresh, _ := flowgate.LoadBaseline(dotFP)
	if fresh == nil || fresh.SuitePassed {
		t.Fatal("baseline must stay red — no evidence to supersede")
	}
}

// A green baseline is never rewritten by the supersede path (frozen against
// accidental broken commits, V9-30).
func TestBug632_GreenBaselineNeverTouched(t *testing.T) {
	repoDir, dotFP := bug632Repo(t, "0")
	bl, err := flowgate.CaptureBaselineContext(context.Background(), repoDir, dotFP)
	if err != nil {
		t.Fatalf("capture green baseline: %v", err)
	}
	diff := []flowgate.ChangedFile{{Path: "src/main.go", Status: "M"}}
	oracle := flowgate.RunOracleContext(context.Background(), repoDir, bl, diff, nil)

	svc, rs := bug632Svc()
	rs.workspaceCwd = repoDir
	if blocked := svc.gateBlindBlocksTurn(context.Background(), "run-b632", "turn-1", 1, rs, repoDir, dotFP, bl, oracle, diff); blocked {
		t.Fatal("green baseline + green oracle must never block")
	}
	fresh, _ := flowgate.LoadBaseline(dotFP)
	if fresh == nil || !fresh.SuitePassed || fresh.CapturedAt != bl.CapturedAt {
		t.Fatal("green baseline must be returned untouched, never recaptured")
	}
}
