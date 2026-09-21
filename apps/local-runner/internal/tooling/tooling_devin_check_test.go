package tooling

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

// CP-70 Task-402 gap closure: CheckTool("devin") (DV-18) mirror of the
// opencode bin-override probe. Additive.

func TestCheckToolDevinViaBinOverride(t *testing.T) {
	var script string
	if runtime.GOOS == "windows" {
		dir := t.TempDir()
		script = filepathJoin(dir, "fake-devin.bat")
		if err := os.WriteFile(script, []byte("@echo off\r\necho devin 3000.10.31\r\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	} else {
		dir := t.TempDir()
		script = filepathJoin(dir, "fake-devin")
		content := "#!/bin/sh\necho devin 3000.10.31\n"
		if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("FLOWPILOT_DEVIN_BIN", script)

	ts := CheckTool("devin", "")
	if ts.Status != "ok" {
		t.Fatalf("CheckTool(devin) status = %q (version=%q), want ok", ts.Status, ts.Version)
	}
	if !strings.Contains(ts.Version, "3000.10.31") {
		t.Fatalf("version = %q, want 3000.10.31", ts.Version)
	}
	if ts.Tool != "devin" {
		t.Fatalf("tool = %q, want devin", ts.Tool)
	}
}

func TestCheckToolDevinMissing(t *testing.T) {
	t.Setenv("FLOWPILOT_DEVIN_BIN", "flowpilot-no-such-devin-binary-xyz")
	ts := CheckTool("devin", "")
	if ts.Status != "missing" {
		t.Fatalf("missing binary must report missing, got %q", ts.Status)
	}
}
