package runner

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// CP-70 follow-up: the official Devin CLI installers do not put the binary on
// PATH (Windows setup.ps1 → %LOCALAPPDATA%\devin\cli\bin\devin.exe; unix
// install.sh → ~/.local/bin/devin symlink). Detection/spawn must resolve the
// well-known install location or a healthy install reports NOT_INSTALLED and
// Detect models falls back to the 2-entry static list.

func TestResolveDevinBinaryPath_AbsoluteOverride(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "devin-custom.exe")
	if err := os.WriteFile(bin, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FLOWPILOT_DEVIN_BIN", bin)
	if got := resolveDevinBinaryPath(); got != bin {
		t.Fatalf("resolveDevinBinaryPath = %q, want override %q", got, bin)
	}
}

func TestResolveDevinBinaryPath_PathLookupWins(t *testing.T) {
	t.Setenv("FLOWPILOT_DEVIN_BIN", "")
	oldLookPath := lookPathFn
	lookPathFn = func(name string) (string, error) {
		if name == "devin" {
			return filepath.Join(t.TempDir(), "devin-on-path"), nil
		}
		return "", os.ErrNotExist
	}
	t.Cleanup(func() { lookPathFn = oldLookPath })
	oldBin := devinBinaryName
	devinBinaryName = func() string { return "devin" }
	t.Cleanup(func() { devinBinaryName = oldBin })

	got := resolveDevinBinaryPath()
	if !strings.Contains(got, "devin-on-path") {
		t.Fatalf("resolveDevinBinaryPath = %q, want PATH hit", got)
	}
}

func TestResolveDevinBinaryPath_WellKnownFallback(t *testing.T) {
	t.Setenv("FLOWPILOT_DEVIN_BIN", "")
	oldLookPath := lookPathFn
	lookPathFn = func(string) (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { lookPathFn = oldLookPath })
	oldBin := devinBinaryName
	devinBinaryName = func() string { return "devin" }
	t.Cleanup(func() { devinBinaryName = oldBin })

	var want string
	if runtime.GOOS == "windows" {
		fakeLocal := t.TempDir()
		t.Setenv("LOCALAPPDATA", fakeLocal)
		t.Setenv("USERPROFILE", t.TempDir()) // no binary under the profile root
		want = filepath.Join(fakeLocal, "devin", "cli", "bin", "devin.exe")
	} else {
		home := t.TempDir()
		t.Setenv("HOME", home)
		want = filepath.Join(home, ".local", "bin", "devin")
	}
	if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(want, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}

	if got := resolveDevinBinaryPath(); got != want {
		t.Fatalf("resolveDevinBinaryPath = %q, want well-known %q", got, want)
	}
	if got := devinSpawnBinary(); got != want {
		t.Fatalf("devinSpawnBinary = %q, want %q", got, want)
	}
}

// Detect must reach the well-known install when PATH misses it — the exact
// shape of the reported "2 static models + auth UNKNOWN" failure: the early
// not-found return never ran model resolution or auth detection.
func TestDetectProvider_DevinWellKnownInstall(t *testing.T) {
	t.Setenv("FLOWPILOT_DEVIN_BIN", "")
	oldLookPath := lookPathFn
	lookPathFn = func(string) (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { lookPathFn = oldLookPath })
	oldBin := devinBinaryName
	devinBinaryName = func() string { return "devin" }
	t.Cleanup(func() { devinBinaryName = oldBin })

	var want string
	if runtime.GOOS == "windows" {
		fakeLocal := t.TempDir()
		t.Setenv("LOCALAPPDATA", fakeLocal)
		t.Setenv("USERPROFILE", t.TempDir())
		want = filepath.Join(fakeLocal, "devin", "cli", "bin", "devin.exe")
	} else {
		home := t.TempDir()
		t.Setenv("HOME", home)
		want = filepath.Join(home, ".local", "bin", "devin")
	}
	if err := os.MkdirAll(filepath.Dir(want), 0o755); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if err := os.WriteFile(want, []byte("MZ"), 0o755); err != nil {
			t.Fatal(err)
		}
	} else {
		if err := os.WriteFile(want, []byte("#!/bin/sh\necho devin 3000.10.31\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	provider := detectProvider(t.Context(), providerSpec{
		Key: "devin", Label: "Devin", BinaryName: "devin",
		Models: defaultDevinProviderModels(),
	})
	if !provider.Installed {
		t.Fatalf("Installed = false, LastError = %v — well-known install not resolved", provider.LastError)
	}
	if provider.BinaryPath != want {
		t.Fatalf("BinaryPath = %q, want %q", provider.BinaryPath, want)
	}
	if provider.LastError != nil && strings.Contains(*provider.LastError, "not found on PATH") {
		t.Fatalf("LastError = %q, PATH miss must not surface when install exists", *provider.LastError)
	}
}

func TestResolveDevinBinaryPath_NotFound(t *testing.T) {
	t.Setenv("FLOWPILOT_DEVIN_BIN", "")
	oldLookPath := lookPathFn
	lookPathFn = func(string) (string, error) { return "", os.ErrNotExist }
	t.Cleanup(func() { lookPathFn = oldLookPath })
	oldBin := devinBinaryName
	devinBinaryName = func() string { return "devin" }
	t.Cleanup(func() { devinBinaryName = oldBin })
	if runtime.GOOS == "windows" {
		t.Setenv("LOCALAPPDATA", t.TempDir())
		t.Setenv("USERPROFILE", t.TempDir())
	} else {
		t.Setenv("HOME", t.TempDir())
	}

	if got := resolveDevinBinaryPath(); got != "" {
		t.Fatalf("resolveDevinBinaryPath = %q, want empty", got)
	}
	// Spawn falls back to the bare name so exec reports the standard error.
	if got := devinSpawnBinary(); got != "devin" {
		t.Fatalf("devinSpawnBinary = %q, want bare name fallback", got)
	}
}
