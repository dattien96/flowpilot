// BUG-354 (run-540927): the post-turn gate oracle hung forever — its 5-minute
// deadline never produced a result because cmd.Wait blocked on output pipes
// held by grandchildren. These tests lock the hard-bound contract: executeSuite
// always returns within the deadline + kill grace, and the whole suite process
// group dies on cancel.
package flowgate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Test540927OracleGroupKillReturnsDespitePipeHoldingChild: the suite ignores
// TERM-style cancellation only via a grandchild that inherits the output pipes
// (the run-540927 shape). The oracle must still return with EnvError quickly,
// never report a regression, and never hang.
func Test540927OracleGroupKillReturnsDespitePipeHoldingChild(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "pipe_holder_suite.sh")
	// sh traps nothing here; the KILLed group must take `sleep` (pipe holder)
	// down too — without the group kill, cmd.Wait would block ~30s on the pipe.
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsh -c 'sleep 30' &\nsleep 30\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	bl := &Baseline{
		CapturedAt:  time.Now().UTC().Format(time.RFC3339),
		TestCmd:     script,
		SuitePassed: true,
		GreenTests:  []string{"TestFoo"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
	defer cancel()
	started := time.Now()
	result := RunOracleContext(ctx, dir, bl, nil, nil)
	elapsed := time.Since(started)
	if elapsed > 10*time.Second {
		t.Fatalf("oracle returned after %s; BUG-354 hang shape not bounded", elapsed)
	}
	if result.HasRegression {
		t.Fatalf("timeout must not be a regression; got %#v", result)
	}
	if result.EnvError == "" {
		t.Fatalf("want EnvError on timeout, got %#v", result)
	}
	if result.SuitePassed {
		t.Fatal("aborted suite is not a pass")
	}
}

// Test540927OracleSigtermIgnoringSuiteBounded: even a suite that traps TERM
// and refuses to die must yield an EnvError within the kill grace — the
// executor never blocks indefinitely on cmd.Wait.
func Test540927OracleSigtermIgnoringSuiteBounded(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "stubborn_suite.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntrap '' TERM\nsleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	bl := &Baseline{
		CapturedAt:  time.Now().UTC().Format(time.RFC3339),
		TestCmd:     script,
		SuitePassed: true,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	var result OracleResult
	go func() {
		result = RunOracleContext(ctx, dir, bl, nil, nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("executeSuite hung past deadline + kill grace (BUG-354)")
	}
	if result.EnvError == "" {
		t.Fatalf("want EnvError, got %#v", result)
	}
	if !strings.Contains(strings.ToLower(result.EnvError), "context deadline exceeded") {
		t.Fatalf("EnvError should reflect deadline, got %q", result.EnvError)
	}
	if result.HasRegression {
		t.Fatalf("timeout must not be a regression; got %#v", result)
	}
}

// Test540927OracleCleanSuiteStillPasses: the group-kill plumbing must not
// break the happy path — a green suite still reports SuitePassed with no
// EnvError and parsed test names.
func Test540927OracleCleanSuiteStillPasses(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "green_suite.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho '--- PASS: TestFoo'\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	bl := &Baseline{
		CapturedAt:  time.Now().UTC().Format(time.RFC3339),
		TestCmd:     script,
		SuitePassed: true,
		GreenTests:  []string{"TestFoo"},
	}
	result := RunOracleContext(context.Background(), dir, bl, nil, nil)
	if !result.SuitePassed || result.EnvError != "" || result.HasRegression {
		t.Fatalf("clean suite regressed by BUG-354 plumbing: %#v", result)
	}
}
