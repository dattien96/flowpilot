package lsp

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

// lspEchoLines renders body as bourne-shell commands that print it verbatim
// (one printf per line, single-quote escaped). Shared by the fake-gradlew
// harnesses: raw output lines must never execute as shell commands.
func lspEchoLines(body string) string {
	var b strings.Builder
	for _, l := range strings.Split(body, "\n") {
		b.WriteString("printf '%s\\n' '")
		b.WriteString(strings.ReplaceAll(l, "'", `'\''`))
		b.WriteString("'\n")
	}
	return b.String()
}

// TestFakeGradlewRoundTripsSpecialLines locks the helper contract behind
// CA-885: every body line (gradle markers, redirect-looking "> Task",
// quotes, $/backtick metacharacters) must reach stdout verbatim, and no
// stray file may appear from shell redirection.
func TestFakeGradlewRoundTripsSpecialLines(t *testing.T) {
	dir := t.TempDir()
	body := "e: app/src/main/MainActivity.kt:12:5 Unresolved reference: R\n" +
		"> Task :app:compileDebugKotlin FAILED\n" +
		"it's $HOME & `back` \"quoted\" | pipe\n" +
		"w: app/src/main/Bar.kt:1:1 this is a warning"
	lspFakeGradlew(t, dir, body, "1")
	name := "gradlew"
	if runtime.GOOS == "windows" {
		name = "gradlew.bat"
	}
	out, err := exec.Command(filepath.Join(dir, name)).CombinedOutput()
	if err == nil {
		t.Fatalf("fake must exit non-zero, got nil error (out %q)", out)
	}
	for _, want := range []string{
		"Unresolved reference: R",
		"> Task :app:compileDebugKotlin FAILED",
		"quoted",
		"this is a warning",
	} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("stdout missing %q (out %q)", want, out)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "Task")); !os.IsNotExist(err) {
		t.Fatalf("stray redirect file 'Task' created: %v", err)
	}
}

// TestFakeGradlewEndToEndIgnoresGarbage covers degraded compiler output:
// blank + garbage + task-header lines around one real error.
func TestFakeGradlewEndToEndIgnoresGarbage(t *testing.T) {
	dir := t.TempDir()
	lspFakeGradlew(t, dir,
		"\n"+
			"Picked up JAVA_TOOL_OPTIONS: -Xmx2g\n"+
			"> Task :app:compileDebugKotlin FAILED\n"+
			"e: app/src/main/Broken.kt:7:3 Expecting an element\n"+
			"some unstructured tail line",
		"1")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	errs, err := RunGradleValidation(ctx, dir)
	if err != nil {
		t.Fatalf("RunGradleValidation: %v", err)
	}
	if len(errs) != 1 {
		t.Fatalf("errors = %+v, want exactly 1", errs)
	}
	if !strings.HasSuffix(errs[0].File, "Broken.kt") || errs[0].Line != 7 {
		t.Fatalf("error = %+v, want Broken.kt:7", errs[0])
	}
}

// TestFakeGradlewWarningOnlyYieldsNoErrors covers the all-warning shape
// end to end: warnings print but parse to zero errors.
func TestFakeGradlewWarningOnlyYieldsNoErrors(t *testing.T) {
	dir := t.TempDir()
	lspFakeGradlew(t, dir,
		"w: file:///proj/app/src/main/Bar.kt:1:1 this is a warning\n"+
			"w: file:///proj/app/src/main/Baz.kt:2:2 another warning",
		"0")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	errs, err := RunGradleValidation(ctx, dir)
	if err != nil {
		t.Fatalf("RunGradleValidation: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("errors = %+v, want none", errs)
	}
}
