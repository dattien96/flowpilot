package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCompilerGate_SuccessCommand(t *testing.T) {
	dir := t.TempDir()
	gate := NewCompilerGate(dir, "echo \"success\"", 30*time.Second)

	result, err := gate.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if result == nil {
		t.Fatal("Run() returned nil result")
	}
	if !result.Passed {
		t.Fatalf("Passed = false, want true (exit=%d envError=%q raw=%q)", result.ExitCode, result.EnvError, result.RawOutput)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", result.ExitCode)
	}
	if !strings.Contains(result.RawOutput, "success") {
		t.Fatalf("RawOutput = %q, want to contain the command stdout", result.RawOutput)
	}
	if len(result.ParsedErrors) != 0 {
		t.Fatalf("ParsedErrors = %v, want empty on a passing gate", result.ParsedErrors)
	}
	if result.TimedOut || result.EnvError != "" {
		t.Fatalf("TimedOut=%v EnvError=%q, want both clear", result.TimedOut, result.EnvError)
	}
}

func TestCompilerGate_FailingCommand(t *testing.T) {
	dir := t.TempDir()
	// Mirrors the real recipe shape (`pnpm install && pnpm tsc --noEmit`): a
	// shell chain whose diagnostic line must survive into ParsedErrors.
	command := `echo "noise: installing" && echo "src/app.ts(4,10): error TS2307: Cannot find module '@flowpilot/core-ui'" >&2 && exit 1`
	gate := NewCompilerGate(dir, command, 30*time.Second)

	result, err := gate.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v, want nil (a failing compiler is a result, not an error)", err)
	}
	if result.Passed {
		t.Fatalf("Passed = true, want false; raw=%q", result.RawOutput)
	}
	if result.ExitCode != 1 {
		t.Fatalf("ExitCode = %d, want 1", result.ExitCode)
	}
	if !strings.Contains(result.RawOutput, "TS2307") {
		t.Fatalf("RawOutput = %q, want the raw compiler log preserved", result.RawOutput)
	}
	joined := strings.Join(result.ParsedErrors, "\n")
	if !strings.Contains(joined, "TS2307") {
		t.Fatalf("ParsedErrors = %v, want the file:line diagnostic extracted", result.ParsedErrors)
	}
	if !strings.Contains(joined, "src/app.ts(4,10)") {
		t.Fatalf("ParsedErrors = %v, want the file:line prefix kept", result.ParsedErrors)
	}
	if strings.Contains(joined, "installing") {
		t.Fatalf("ParsedErrors = %v, want noise lines dropped", result.ParsedErrors)
	}
}

func TestCompilerGate_Timeout(t *testing.T) {
	dir := t.TempDir()
	gate := NewCompilerGate(dir, "sleep 5", 60*time.Millisecond)

	started := time.Now()
	result, err := gate.Run(context.Background())
	elapsed := time.Since(started)

	if err != nil {
		t.Fatalf("Run() error = %v, want nil (timeout is reported in the result)", err)
	}
	if !result.TimedOut {
		t.Fatalf("TimedOut = false, want true (exit=%d)", result.ExitCode)
	}
	if result.Passed {
		t.Fatal("Passed = true, want false on timeout")
	}
	if elapsed > 3*time.Second {
		t.Fatalf("gate took %s, want it cancelled near its 60ms timeout", elapsed)
	}
	if len(result.ParsedErrors) == 0 || !strings.Contains(result.ParsedErrors[0], "timed out") {
		t.Fatalf("ParsedErrors = %v, want a leading timeout diagnosis", result.ParsedErrors)
	}
}

func TestCompilerGate_WorkingDirectoryAndEnvInheritance(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLOWPILOT_GATE_INHERIT", "yes")

	// Fails if cmd.Dir is not the workspace, fails if the parent env is dropped.
	gate := NewCompilerGate(dir, `test -f marker.txt && test "$FLOWPILOT_GATE_INHERIT" = "yes"`, 30*time.Second)
	result, err := gate.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.Passed {
		t.Fatalf("Passed = false, want true (workdir/env not honoured): %+v", result)
	}
}
func TestCompilerGate_EmptyCommandIsConfigurationError(t *testing.T) {
	gate := NewCompilerGate(t.TempDir(), "   ", 30*time.Second)
	result, err := gate.Run(context.Background())
	if err == nil {
		t.Fatal("Run() error = nil, want a configuration error for an empty command")
	}
	if result != nil {
		t.Fatalf("result = %+v, want nil on configuration error", result)
	}
	if gate.Timeout() != 30*time.Second {
		t.Fatalf("timeout = %s, want the explicit 30s", gate.Timeout())
	}
	if got := NewCompilerGate(t.TempDir(), "echo x", 0).Timeout(); got != defaultCompilerGateTimeout {
		t.Fatalf("timeout = %s, want the %s default when the recipe omits one", got, defaultCompilerGateTimeout)
	}
	if got := NewCompilerGate(t.TempDir(), "echo recipe-default", 0).Command(); got != "echo recipe-default" {
		t.Fatalf("Command() = %q", got)
	}
}

func TestParseCompilerErrors_FiltersAndDedupes(t *testing.T) {
	raw := strings.Join([]string{
		"Packages: +154",
		"Progress: resolved 154, reused 154",
		"src/a.ts(1,2): error TS2307: Cannot find module '@flowpilot/core-ui'",
		"src/a.ts(1,2): error TS2307: Cannot find module '@flowpilot/core-ui'",
		"src/b.ts(9,9): error TS2322: Type 'string' is not assignable to type 'number'.",
		"✖ Build failed",
		"Done in 3.2s",
	}, "\n")

	parsed := parseCompilerErrors(raw)
	if len(parsed) != 3 {
		t.Fatalf("parsed %d lines (%v), want 3 (dedupe + noise filter)", len(parsed), parsed)
	}
	joined := strings.Join(parsed, "\n")
	for _, want := range []string{"TS2307", "TS2322", "Build failed"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("parsed = %v, want %q", parsed, want)
		}
	}
	if parsed[0] != "src/a.ts(1,2): error TS2307: Cannot find module '@flowpilot/core-ui'" {
		t.Fatalf("parsed[0] = %q, want the first unique diagnostic", parsed[0])
	}

	if got := parseCompilerErrors(""); got != nil {
		t.Fatalf("parseCompilerErrors(\"\") = %v, want nil", got)
	}
	if got := parseCompilerErrors("all good\nnothing to see"); got != nil {
		t.Fatalf("parseCompilerErrors(clean log) = %v, want nil", got)
	}
}

func TestParseCompilerErrors_CapsLineLengthWithoutBreakingRunes(t *testing.T) {
	long := "error: " + strings.Repeat("✖", maxCompilerErrorLineLen*2)
	parsed := parseCompilerErrors(long)
	if len(parsed) != 1 {
		t.Fatalf("parsed = %v, want exactly 1 line", parsed)
	}
	if runes := []rune(parsed[0]); len(runes) != maxCompilerErrorLineLen+1 {
		t.Fatalf("truncated line has %d runes, want %d (cap + ellipsis)", len(runes), maxCompilerErrorLineLen+1)
	}
	if !strings.HasSuffix(parsed[0], "…") {
		t.Fatalf("truncated line = %q, want an ellipsis suffix", parsed[0])
	}
}
