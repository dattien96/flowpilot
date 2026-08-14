package prefs_test

import (
	"os"
	"path/filepath"
	"testing"

	"flowpilot-runner/internal/tui/prefs"
)

func TestSaveLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	// Force Paths() to use our temp APPDATA (windows layout).
	if len(prefs.Paths()) == 0 {
		t.Skip("no prefs paths on this OS")
	}
	// Write via Save into APPDATA/FlowPilot when on windows; on other OS use HOME.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	path, err := prefs.Save(prefs.Session{Provider: "grok", Model: "grok-4.5", ReasoningEffort: "high"})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	if path == "" {
		t.Fatal("empty save path")
	}
	got, loadedFrom, err := prefs.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Provider != "grok" || got.Model != "grok-4.5" {
		t.Fatalf("got=%+v from %s", got, loadedFrom)
	}
	// Ensure file exists under a known root.
	if _, err := os.Stat(filepath.Clean(loadedFrom)); err != nil {
		t.Fatalf("missing prefs file: %v", err)
	}
}
