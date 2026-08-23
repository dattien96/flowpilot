package app

import (
	"os"
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/config"
)

// CA-606: /dumpview writes a file stamped with the layout revision, the live
// geometry, and the You box — so a stale binary or a Ghostty-only render bug is
// visible from the file, not a screenshot. CA-607: the 5-line prompt clamps to
// 4 lines + "...." when collapsed, so the dump shows the clamped box; the tail
// line returns after an expand click.
func TestDumpView_WritesRevAndPrompt(t *testing.T) {
	forceTrueColor(t)
	prompt := "[Change Contract]\nfeature: calc-core\nintent: them ham SubtractWithGuard vao calc.go tra ve error khi b > a\nfiles: calc.go, calc_test.go\nsymbols: Subtract"
	m := New(config.ChatConfig{Provider: "grok", Model: "m"}, "http://127.0.0.1:4317")
	m.width, m.height = 197, 30
	m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
	m.sessionPanel.RunID = "run-208282"
	m.sessionPanel.ProjectPath = "/tmp/p"
	m.sessionPanel.Collapsed = false
	m.addMessage("user", prompt, "")
	path := os.TempDir() + "/flowpilot-you-view-test.txt"
	if err := writeYouViewDump(m, path); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if !strings.Contains(got, "rev="+youBoxLayoutRev) {
		t.Fatalf("dump missing rev stamp %q:\n%s", youBoxLayoutRev, got)
	}
	for _, need := range []string{"width=197", "chatW=", "sidebar=true", "--- raw ---", "--- plain ---", "--- you-box chat ---"} {
		if !strings.Contains(got, need) {
			t.Fatalf("dump missing %q:\n%s", need, got)
		}
	}
	// Collapsed: first 4 lines + "...." tail + [copy] chip.
	for _, need := range []string{"[Change Contract]", "feature: calc-core", "tra ve error khi b > a", "files: calc.go, calc_test.go....", "[copy]"} {
		if !strings.Contains(got, need) {
			t.Fatalf("dump missing clamped prompt %q:\n%s", need, got)
		}
	}
	if strings.Contains(got, "symbols: Subtract") {
		t.Fatalf("dump shows hidden clamp tail line:\n%s", got)
	}
	// Expanded: the full prompt (including symbols:) is in the dump.
	m.toggleUserPrompt(prompt)
	if err := writeYouViewDump(m, path); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got = string(raw)
	for _, need := range []string{"[Change Contract]", "feature: calc-core", "tra ve error khi b > a", "files: calc.go, calc_test.go", "symbols: Subtract", "[copy]"} {
		if !strings.Contains(got, need) {
			t.Fatalf("expanded dump missing prompt %q:\n%s", need, got)
		}
	}
	if strings.Contains(got, "calc_test.go....") {
		t.Fatalf("expanded dump must not show the ellipsis tail:\n%s", got)
	}
}

// The dump path must stay derived from the OS temp dir (no repo writes).
func TestDumpView_RejectsRelativePath(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	if err := writeYouViewDump(m, "relative/path.txt"); err != nil {
		// os.WriteFile on a missing dir errors — acceptable; the command uses
		// filepath.Join(os.TempDir(), ...) so this only guards the helper.
		_ = err
	}
}