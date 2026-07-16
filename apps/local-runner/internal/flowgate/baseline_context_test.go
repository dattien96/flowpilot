package flowgate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCaptureBaselineContextHonorsCancellation is the focused regression test
// for BUG-288 P2-04 (Vòng 12): the first baseline capture used to always run
// its suite command under context.Background(), so Stop could not cut it
// short. This locks in that CaptureBaselineContext threads the caller's
// context into the suite run — an already-cancelled context must abort the
// suite immediately (envError reflecting cancellation) instead of running it
// to completion/timeout.
func TestCaptureBaselineContextHonorsCancellation(t *testing.T) {
	dir := t.TempDir()
	dotFP := filepath.Join(dir, ".flowpilot")
	if err := os.MkdirAll(filepath.Join(dotFP, "settings"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Explicit command wins over auto-detection (LoadTestConfig). "go version"
	// would normally succeed quickly; with an already-cancelled context it
	// must not be allowed to run to a real result.
	cfg := `{"test_command":"go version"}`
	if err := os.WriteFile(filepath.Join(dotFP, "settings", "test-config.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	bl, err := CaptureBaselineContext(ctx, dir, dotFP)
	// BUG-288 R13-03: cancellation must abort WITHOUT writing a poisoned baseline.
	if err == nil {
		t.Fatal("CaptureBaselineContext with cancelled ctx must return an error (no poisoned baseline)")
	}
	if bl != nil {
		t.Fatal("cancelled capture must not return a baseline value")
	}
	if _, statErr := os.Stat(filepath.Join(dotFP, "guard", "test_baseline.json")); !os.IsNotExist(statErr) {
		t.Fatalf("cancelled capture must not leave test_baseline.json on disk; stat err=%v", statErr)
	}
}

// TestCaptureBaselineContextNilFallsBackToBackground locks in the documented
// nil-safety fallback (mirrors flowInlineContext-style nil guards elsewhere).
func TestCaptureBaselineContextNilFallsBackToBackground(t *testing.T) {
	dir := t.TempDir()
	dotFP := filepath.Join(dir, ".flowpilot")
	if err := os.MkdirAll(filepath.Join(dotFP, "settings"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `{"test_command":"go version"}`
	if err := os.WriteFile(filepath.Join(dotFP, "settings", "test-config.json"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	//lint:ignore SA1012 intentionally passing nil to exercise the fallback guard
	bl, err := CaptureBaselineContext(nil, dir, dotFP)
	if err != nil {
		t.Fatalf("CaptureBaselineContext(nil, ...): %v", err)
	}
	if !strings.Contains(bl.TestCmd, "go version") {
		t.Fatalf("TestCmd = %q, want it to reflect the configured command", bl.TestCmd)
	}
}
