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

// lspAndroidHarness builds a ServerSet whose android server is the helper
// binary (publishing diag) plus a fake gradlew emitting gradleOut.
func lspAndroidHarness(t *testing.T, diag, gradleOut string) (*ServerSet, string) {
	t.Helper()
	t.Setenv("GO_WANT_LSP_HELPER", "1")
	t.Setenv("GO_LSP_HELPER_MODE", "diagnostics")
	t.Setenv("GO_LSP_HELPER_DIAG", diag)
	helper := os.Args[0]
	helperArgs := []string{"-test.run=TestLSPHelperProcess"}
	reg := Registry{
		"android": {Platform: "android", Binary: helper, Args: helperArgs, FileExtensions: []string{".kt", ".kts"}},
		"golang":  {Platform: "golang", Binary: helper, Args: helperArgs, FileExtensions: []string{".go"}},
	}
	set := NewServerSet(reg)
	ws := t.TempDir()
	// Register Close after TempDir so helpers die before removal.
	t.Cleanup(set.Close)
	if err := os.WriteFile(filepath.Join(ws, "build.gradle.kts"), []byte("plugins {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "Main.kt"), []byte("fun main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	name := "gradlew"
	// Unix branch echoes each line quoted (raw output lines would execute
	// as shell commands; "> Task ..." would redirect stdout away).
	content := "#!/bin/sh\n" + lspEchoLines(gradleOut) + "\nexit 1\n"
	if runtime.GOOS == "windows" {
		name = "gradlew.bat"
		lines := "@echo off\r\n"
		for _, l := range strings.Split(gradleOut, "\n") {
			l = strings.ReplaceAll(l, ">", "^>")
			lines += "echo " + l + "\r\n"
		}
		content = lines + "exit /b 1\r\n"
	}
	if err := os.WriteFile(filepath.Join(ws, name), []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return set, ws
}

func TestGradleFallbackRunsWhenAndroidLSPClean(t *testing.T) {
	set, ws := lspAndroidHarness(t, "clean",
		"e: app/src/main/MainActivity.kt:12:5 Unresolved reference: R")
	defer set.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	got := set.CheckFiles(ctx, ws, []string{"Main.kt"})
	if !strings.Contains(got, "Unresolved reference") {
		t.Fatalf("output = %q, want the gradle R-class error", got)
	}
}

func TestGradleFallbackSkippedWhenLSPErrors(t *testing.T) {
	set, ws := lspAndroidHarness(t, "lsp says no",
		"e: app/src/main/MARKER_SHOULD_NOT_APPEAR.kt:1:1 boom")
	defer set.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	got := set.CheckFiles(ctx, ws, []string{"Main.kt"})
	if !strings.Contains(got, "lsp says no") {
		t.Fatalf("output = %q, want the LSP error", got)
	}
	if strings.Contains(got, "MARKER_SHOULD_NOT_APPEAR") {
		t.Fatalf("output = %q, gradle must not run when LSP reports errors", got)
	}
}

func TestGradleFallbackSkippedForNonAndroid(t *testing.T) {
	set, ws := lspAndroidHarness(t, "clean",
		"e: MARKER_SHOULD_NOT_APPEAR.kt:1:1 boom")
	defer set.Close()
	// Turn the workspace into a golang one (gradle files stay but go.mod wins).
	if err := os.WriteFile(filepath.Join(ws, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "a.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	got := set.CheckFiles(ctx, ws, []string{"a.go"})
	if strings.Contains(got, "MARKER_SHOULD_NOT_APPEAR") {
		t.Fatalf("output = %q, gradle must not run for non-android", got)
	}
}
