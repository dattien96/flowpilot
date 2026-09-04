// BUG-354 (run-540927): the post-turn gate oracle hung forever — its 5-minute
// deadline never produced a result because cmd.Wait blocked on output pipes
// held by grandchildren. These tests lock the hard-bound contract: executeSuite
// always returns within the deadline + kill grace, and the whole suite process
// group dies on cancel.
package flowgate

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

// Test540927OracleGraceAbandonWhenKillMissesPipeHolder (review F6): a suite
// child that calls setsid escapes the killed process group while still holding
// the output pipes — cmd.Wait would block ~30s after the kill. The executor
// must abandon it via the 2s grace and return EnvError anyway.
func Test540927OracleGraceAbandonWhenKillMissesPipeHolder(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("setsid escape is unix-only")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available for setsid pipe-holder")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "setsid_escape_suite.sh")
	// The setsid'd python holds stdout/stderr open for 30s in a NEW process
	// group — the group kill cannot reach it; only the grace bound unblocks.
	if err := os.WriteFile(script, []byte(
		"#!/bin/sh\n"+python+" -c \"import os,sys,time; os.setsid(); print('escaped', flush=True); time.sleep(30)\" &\nwait\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	bl := &Baseline{
		CapturedAt:  time.Now().UTC().Format(time.RFC3339),
		TestCmd:     script,
		SuitePassed: true,
		GreenTests:  []string{"TestFoo"},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	started := time.Now()
	result := RunOracleContext(ctx, dir, bl, nil, nil)
	elapsed := time.Since(started)
	if elapsed > 15*time.Second {
		t.Fatalf("grace abandon failed: oracle returned after %s (BUG-354)", elapsed)
	}
	if elapsed < 300*time.Millisecond {
		// Must have waited at least the deadline; a fast return would mean the
		// pipe-holder never actually held the pipes.
		t.Logf("note: returned after %s", elapsed)
	}
	if result.EnvError == "" || result.HasRegression || result.SuitePassed {
		t.Fatalf("abandoned suite must be EnvError/non-pass, got %#v", result)
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
