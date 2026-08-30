package tooling

import (
	"os"
	"runtime"
	"strings"
	"testing"
)

// CP-57 Task-302 gap closure: CheckTool("opencode") (OC-18) had wiring but no
// test. Uses the FLOWPILOT_OPENCODE_BIN override so the probe does not depend
// on a real opencode install. Additive.

func TestCheckToolOpencodeViaBinOverride(t *testing.T) {
	var script string
	if runtime.GOOS == "windows" {
		dir := t.TempDir()
		script = filepathJoin(dir, "fake-opencode.bat")
		if err := os.WriteFile(script, []byte("@echo off\r\necho 1.18.18\r\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	} else {
		dir := t.TempDir()
		script = filepathJoin(dir, "fake-opencode")
		content := "#!/bin/sh\necho 1.18.18\n"
		if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("FLOWPILOT_OPENCODE_BIN", script)

	ts := CheckTool("opencode", "")
	if ts.Status != "ok" {
		t.Fatalf("CheckTool(opencode) status = %q (version=%q), want ok", ts.Status, ts.Version)
	}
	if !strings.Contains(ts.Version, "1.18.18") {
		t.Fatalf("version = %q, want 1.18.18", ts.Version)
	}
	if ts.Tool != "opencode" {
		t.Fatalf("tool = %q, want opencode", ts.Tool)
	}
}

func TestCheckToolOpencodeMissing(t *testing.T) {
	t.Setenv("FLOWPILOT_OPENCODE_BIN", "flowpilot-no-such-opencode-binary-xyz")
	ts := CheckTool("opencode", "")
	if ts.Status != "missing" {
		t.Fatalf("missing binary must report missing, got %q", ts.Status)
	}
}

func filepathJoin(elem ...string) string {
	if len(elem) == 0 {
		return ""
	}
	out := elem[0]
	for _, e := range elem[1:] {
		out += string(os.PathSeparator) + e
	}
	return out
}
