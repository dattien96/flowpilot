package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"flowpilot-runner/internal/flowgate"
)

// BUG-387 (live run-19151): oracle.Tampered was never propagated into
// ValidationResult — a marker injected into a read-only test file survived to
// run completion and validate passed. A tampered pre-existing test file must
// fail the validate gate.
func TestBug387_ValidateFailsOnTamperedTestFile(t *testing.T) {
	dir := t.TempDir()
	dotFP := filepath.Join(dir, ".flowpilot")
	if err := os.MkdirAll(dotFP, 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "suite.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	bl := flowgate.Baseline{
		CapturedAt:  time.Now().UTC().Format(time.RFC3339),
		TestCmd:     script,
		SuitePassed: true,
	}
	if err := flowgate.SaveBaselineFile(dotFP, "test_baseline.json", &bl); err != nil {
		t.Fatal(err)
	}
	// ObserveGitDiffSince on a non-git temp dir returns empty -> the
	// changedFiles fallback feeds the oracle's tamper detection.
	changed := []string{"pkg/x_test.go"}
	res := runValidateWithOracleIfPossible(context.Background(), "", dir, dir, "sha", changed)
	if res.Passed() {
		t.Fatalf("tampered pre-existing test file must not pass validation: %#v", res)
	}
	if res.ExitCode == 0 && res.EnvError == "" {
		t.Fatal("tamper must surface as a validation failure")
	}
}
