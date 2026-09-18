package lsp

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// lspFakeGradlew drops a fake wrapper that prints body and exits code.
func lspFakeGradlew(t *testing.T, dir, body string, code string) {
	t.Helper()
	name := "gradlew"
	content := "#!/bin/sh\n" + body + "\nexit " + code + "\n"
	if runtime.GOOS == "windows" {
		name = "gradlew.bat"
		lines := "@echo off\r\n"
		for _, l := range strings.Split(body, "\n") {
			l = strings.ReplaceAll(l, ">", "^>")
			l = strings.ReplaceAll(l, "|", "^|")
			l = strings.ReplaceAll(l, "&", "^&")
			lines += "echo " + l + "\r\n"
		}
		content = lines + "exit /b " + code + "\r\n"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o755); err != nil {
		t.Fatalf("write fake gradlew: %v", err)
	}
}

func TestShouldRunGradleFallbackTrueForAndroidCleanDiagnostics(t *testing.T) {
	if !ShouldRunGradleFallback(nil, "android") {
		t.Fatal("android + nil diagnostics must run fallback")
	}
	if !ShouldRunGradleFallback([]FileDiagnostic{}, "android") {
		t.Fatal("android + empty diagnostics must run fallback")
	}
}

func TestShouldRunGradleFallbackFalseForNonAndroid(t *testing.T) {
	for _, p := range []string{"golang", "node", "python", "", "general"} {
		if ShouldRunGradleFallback(nil, p) {
			t.Fatalf("platform %q must not run gradle fallback", p)
		}
	}
}

func TestShouldRunGradleFallbackFalseWhenDiagnosticsExist(t *testing.T) {
	errs := []FileDiagnostic{{URI: "file:///a.kt", Diagnostic: lspDiag(0, 0, DiagnosticSeverityError, "x")}}
	if ShouldRunGradleFallback(errs, "android") {
		t.Fatal("existing LSP errors must skip the fallback")
	}
}

func TestRunGradleValidationParsesCompilerOutput(t *testing.T) {
	dir := t.TempDir()
	lspFakeGradlew(t, dir,
		"e: file:///proj/app/src/main/MainActivity.kt:12:5 Unresolved reference: R\n"+
			"app/src/main/Foo.kt:30: error: too many arguments\n"+
			"> Task :app:compileDebugKotlin FAILED\n"+
			"w: file:///proj/app/src/main/Bar.kt:1:1 this is a warning",
		"1")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	errs, err := RunGradleValidation(ctx, dir)
	if err != nil {
		t.Fatalf("RunGradleValidation: %v", err)
	}
	if len(errs) != 2 {
		t.Fatalf("errors = %+v", errs)
	}
	if !strings.HasSuffix(errs[0].File, "MainActivity.kt") || errs[0].Line != 12 ||
		!strings.Contains(errs[0].Message, "Unresolved reference") {
		t.Fatalf("first error = %+v", errs[0])
	}
	if !strings.HasSuffix(errs[1].File, "Foo.kt") || errs[1].Line != 30 {
		t.Fatalf("second error = %+v", errs[1])
	}
}

func TestRunGradleValidationHandlesMissingGradlew(t *testing.T) {
	dir := t.TempDir() // no wrapper
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := RunGradleValidation(ctx, dir)
	if err == nil {
		t.Fatal("expected error when gradlew is missing")
	}
	if !strings.Contains(err.Error(), "gradlew") {
		t.Fatalf("error = %v, want gradlew mention", err)
	}
}

func TestFormatGradleErrorsForAgentOutput(t *testing.T) {
	got := FormatGradleErrorsForAgent([]GradleError{
		{File: "app/Main.kt", Line: 12, Message: "unresolved reference: R"},
		{File: "app/Util.kt", Message: "module broken"},
		{Message: "daemon disappeared"},
	})
	want := "app/Main.kt:12: error: unresolved reference: R\n" +
		"app/Util.kt: error: module broken\n" +
		"error: daemon disappeared"
	if got != want {
		t.Fatalf("formatted = %q, want %q", got, want)
	}
	if got := FormatGradleErrorsForAgent(nil); got != "" {
		t.Fatalf("empty input must format empty, got %q", got)
	}
}

func TestGradleFallbackTimesOut(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\nsleep 2\nexit 0\n"
	name := "gradlew"
	if runtime.GOOS == "windows" {
		name = "gradlew.bat"
		script = "@echo off\r\nping -n 3 127.0.0.1 >nul\r\nexit /b 0\r\n"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := RunGradleValidation(ctx, dir)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("error = %v, want deadline", err)
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Fatalf("kill took %v", elapsed)
	}
}
