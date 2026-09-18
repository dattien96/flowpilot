package lsp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// lspHookHarness starts a diagnostics-speaking helper and returns a hook
// wired to it. diag controls the published payload (see lspHelperDiagnostics).
func lspHookHarness(t *testing.T, diag string) (*PostWriteDiagnosticsHook, string) {
	t.Helper()
	dir := t.TempDir()
	m := &ServerManager{}
	m.Env = []string{"GO_WANT_LSP_HELPER=1", "GO_LSP_HELPER_MODE=diagnostics", "GO_LSP_HELPER_DIAG=" + diag}
	if err := m.Start(context.Background(), os.Args[0], []string{"-test.run=TestLSPHelperProcess"}, dir); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Registered after TempDir so Stop precedes TempDir removal (Windows).
	t.Cleanup(func() { _ = m.Stop() })
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := m.WaitReady(ctx, 0); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}
	return &PostWriteDiagnosticsHook{
		Manager:       m,
		DocSync:       NewDocumentSyncManager(m.Client()),
		Collector:     NewDiagnosticsCollector(m.Client()),
		WorkspaceRoot: dir,
	}, dir
}

func TestPostWriteHookReturnsNilWhenClean(t *testing.T) {
	hook, _ := lspHookHarness(t, "clean")
	got, err := hook.AfterFileWrite("a.go", "package main\n")
	if err != nil {
		t.Fatalf("AfterFileWrite: %v", err)
	}
	if got != "" {
		t.Fatalf("clean file must yield empty diagnostics, got %q", got)
	}
	// Unsupported extensions are skipped silently.
	if got, err := hook.AfterFileWrite("notes.md", "hi"); got != "" || err != nil {
		t.Fatalf("markdown: got %q, err %v", got, err)
	}
}

func TestPostWriteHookReturnsDiagnosticsOnError(t *testing.T) {
	hook, dir := lspHookHarness(t, "undefined: Foo")
	got, err := hook.AfterFileWrite("a.go", "package main\n\nfunc F() { Foo() }\n")
	if err != nil {
		t.Fatalf("AfterFileWrite: %v", err)
	}
	if !strings.Contains(got, "undefined: Foo") {
		t.Fatalf("diagnostics = %q, want the server message", got)
	}
	if !strings.Contains(got, "error:") || !strings.Contains(got, filepath.Base(dir)) && !strings.Contains(got, "a.go") {
		t.Fatalf("diagnostics = %q, want file:line:col error format", got)
	}
}

func TestPostWriteHookSkipsWhenLSPNotRunning(t *testing.T) {
	hook := &PostWriteDiagnosticsHook{Manager: &ServerManager{}, WorkspaceRoot: t.TempDir()}
	if hook.ShouldRun("a.go") {
		t.Fatal("ShouldRun must be false when the server never started")
	}
	if got, err := hook.AfterFileWrite("a.go", "package a"); got != "" || err != nil {
		t.Fatalf("got %q, err %v", got, err)
	}
	var nilHook *PostWriteDiagnosticsHook
	if nilHook.ShouldRun("a.go") {
		t.Fatal("nil hook must not run")
	}
	if got, err := nilHook.AfterFileWrite("a.go", "x"); got != "" || err != nil {
		t.Fatalf("nil hook: got %q, err %v", got, err)
	}
}

func TestPostWriteHookTimesOutGracefully(t *testing.T) {
	dir := t.TempDir()
	m := &ServerManager{}
	m.Env = []string{"GO_WANT_LSP_HELPER=1", "GO_LSP_HELPER_MODE=hang"}
	if err := m.Start(context.Background(), os.Args[0], []string{"-test.run=TestLSPHelperProcess"}, dir); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = m.Stop() })
	hook := &PostWriteDiagnosticsHook{
		Manager: m, DocSync: NewDocumentSyncManager(m.Client()),
		Collector: NewDiagnosticsCollector(m.Client()), WorkspaceRoot: dir,
		DiagnosticsTimeout: 300 * time.Millisecond,
	}
	start := time.Now()
	got, err := hook.AfterFileWrite("a.go", "package main\n")
	if err != nil {
		t.Fatalf("timeout must degrade to empty, got err %v", err)
	}
	if got != "" {
		t.Fatalf("timeout must degrade to empty, got %q", got)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("hook took %v, expected fast degradation", elapsed)
	}
}

func TestPostWriteHookFormatsMultipleErrors(t *testing.T) {
	hook, _ := lspHookHarness(t, "first boom|second boom")
	got, err := hook.AfterFileWrite("a.go", "package main\n")
	if err != nil {
		t.Fatalf("AfterFileWrite: %v", err)
	}
	if !strings.Contains(got, "first boom") || !strings.Contains(got, "second boom") {
		t.Fatalf("diagnostics = %q, want both errors", got)
	}
	if strings.Count(got, "\n") != 1 {
		t.Fatalf("two errors must render as two lines, got %q", got)
	}
}

func TestServerSetRoutesByWorkspaceAndLanguage(t *testing.T) {
	t.Setenv("GO_WANT_LSP_HELPER", "1")
	t.Setenv("GO_LSP_HELPER_MODE", "diagnostics")
	t.Setenv("GO_LSP_HELPER_DIAG", "route boom")
	helper := os.Args[0]
	helperArgs := []string{"-test.run=TestLSPHelperProcess"}
	reg := Registry{
		"golang": {Platform: "golang", Binary: helper, Args: helperArgs, FileExtensions: []string{".go"}},
		"python": {Platform: "python", Binary: helper, Args: helperArgs, FileExtensions: []string{".py"}},
	}
	set := NewServerSet(reg)
	goWS := t.TempDir()
	pyWS := t.TempDir()
	// Registered after the TempDirs so helpers die before TempDir removal.
	t.Cleanup(set.Close)
	if err := os.WriteFile(filepath.Join(goWS, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pyWS, "requirements.txt"), []byte("fastapi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(goWS, "a.go"), []byte("package x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pyWS, "b.py"), []byte("x = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if got := set.CheckFiles(ctx, goWS, []string{"a.go"}); !strings.Contains(got, "route boom") {
		t.Fatalf("go workspace diagnostics = %q", got)
	}
	if got := set.CheckFiles(ctx, pyWS, []string{"b.py"}); !strings.Contains(got, "route boom") {
		t.Fatalf("python workspace diagnostics = %q", got)
	}
	if n := set.ServerCount(); n != 2 {
		t.Fatalf("servers = %d, want 2 (one per workspace+language)", n)
	}
	// Unknown extensions never start a server.
	if got := set.CheckFiles(ctx, goWS, []string{"notes.md"}); got != "" {
		t.Fatalf("markdown check = %q, want empty", got)
	}
}

func TestServerSetSkipsUnknownPlatform(t *testing.T) {
	set := NewServerSet(nil)
	ws := t.TempDir() // no markers -> "general", unregistered
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if got := set.CheckFiles(ctx, ws, []string{"a.go"}); got != "" {
		t.Fatalf("unregistered platform check = %q, want empty", got)
	}
	if n := set.ServerCount(); n != 0 {
		t.Fatalf("servers = %d, want 0", n)
	}
}

func TestServerSetMissingBinary(t *testing.T) {
	reg := Registry{
		"golang": {Platform: "golang", Binary: "definitely-not-a-real-lsp-binary-xyz", FileExtensions: []string{".go"}},
	}
	set := NewServerSet(reg)
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if got := set.CheckFiles(ctx, ws, []string{"a.go"}); got != "" {
		t.Fatalf("missing binary check = %q, want empty", got)
	}
	if n := set.ServerCount(); n != 0 {
		t.Fatalf("servers = %d, want 0", n)
	}
}
