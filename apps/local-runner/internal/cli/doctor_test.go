package cli

import (
	"errors"
	"strings"
	"testing"
)

// TestDoctorCommand_RegistersOnRoot verifies flowpilot doctor is on the real
// root command.
func TestDoctorCommand_RegistersOnRoot(t *testing.T) {
	root := NewRootCommand()
	doctorCmd, _, err := root.Find([]string{"doctor"})
	if err != nil {
		t.Fatalf("Find('doctor'): %v", err)
	}
	if doctorCmd == nil || doctorCmd.Use != "doctor" {
		t.Fatal("doctor command not found")
	}
}

func TestDoctorCheck_AllPresent(t *testing.T) {
	rows := doctorCheck(func(name string) (string, error) { return "/bin/" + name, nil })
	if len(rows) == 0 {
		t.Fatal("expected one row per registered platform")
	}
	if n := doctorMissing(rows); n != 0 {
		t.Fatalf("missing = %d, want 0", n)
	}
	for _, r := range rows {
		if !r.Installed || r.FoundPath == "" || r.InstallHint == "" {
			t.Fatalf("row = %+v", r)
		}
	}
	// Sorted by platform for stable output.
	for i := 1; i < len(rows); i++ {
		if rows[i-1].Platform > rows[i].Platform {
			t.Fatalf("rows not sorted: %+v", rows)
		}
	}
}

func TestDoctorCheck_SomeMissing(t *testing.T) {
	rows := doctorCheck(func(name string) (string, error) {
		if name == "gopls" {
			return "/usr/bin/gopls", nil
		}
		return "", errDoctorTestMissing
	})
	if n := doctorMissing(rows); n != len(rows)-1 {
		t.Fatalf("missing = %d, want all-but-gopls", n)
	}
	for _, r := range rows {
		if r.Binary == "gopls" && !r.Installed {
			t.Fatalf("gopls should be installed: %+v", r)
		}
	}
}

func TestFormatDoctorShowsMissingAndHints(t *testing.T) {
	rows := []doctorRow{
		{Platform: "golang", Binary: "gopls", FoundPath: "/usr/bin/gopls", Installed: true, InstallHint: "go install x/gopls@latest"},
		{Platform: "python", Binary: "pyright-langserver", InstallHint: "npm i -g pyright"},
	}
	out := formatDoctor(rows)
	if !strings.Contains(out, "gopls") || !strings.Contains(out, "/usr/bin/gopls") {
		t.Fatalf("output missing installed row:\n%s", out)
	}
	if !strings.Contains(out, "MISSING") || !strings.Contains(out, "npm i -g pyright") {
		t.Fatalf("output missing MISSING marker or hint:\n%s", out)
	}
	if !strings.Contains(out, "1 server(s) missing") {
		t.Fatalf("output missing summary:\n%s", out)
	}
	out = formatDoctor(rows[:1])
	if !strings.Contains(out, "All language servers present") {
		t.Fatalf("all-present summary missing:\n%s", out)
	}
}

// errDoctorTestMissing simulates exec.LookPath failure.
var errDoctorTestMissing = errors.New("executable file not found in PATH")
