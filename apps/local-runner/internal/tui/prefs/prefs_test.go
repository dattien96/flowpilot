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

func TestSaveLoad_ModeAndFlowRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("APPDATA", home)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if len(prefs.Paths()) == 0 {
		t.Skip("no prefs paths on this OS")
	}

	_, err := prefs.Save(prefs.Session{
		Provider:   "grok",
		Model:      "grok-4.5",
		Mode:       "flow",
		FlowRef:    "pack/fix-bug",
		FlowLabel:  "Fix Bug",
		WorkflowID: "",
	})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, _, err := prefs.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Mode != "flow" || got.FlowRef != "pack/fix-bug" || got.FlowLabel != "Fix Bug" {
		t.Fatalf("got=%+v", got)
	}

	// Chat mode clears flow identity on save.
	_, err = prefs.Save(prefs.Session{
		Provider: "grok",
		Model:    "grok-4.5",
		Mode:     "chat",
		FlowRef:  "should-not-persist",
	})
	if err != nil {
		t.Fatalf("Save chat: %v", err)
	}
	got, _, err = prefs.Load()
	if err != nil {
		t.Fatalf("Load chat: %v", err)
	}
	if got.Mode != "chat" || got.FlowRef != "" || got.WorkflowID != "" {
		t.Fatalf("chat should clear flow: %+v", got)
	}
}

func TestLoad_ModeOnlyWithoutProvider(t *testing.T) {
	home := t.TempDir()
	t.Setenv("APPDATA", home)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if len(prefs.Paths()) == 0 {
		t.Skip("no prefs paths on this OS")
	}
	_, err := prefs.Save(prefs.Session{Mode: "flow", WorkflowID: "wf-1", FlowLabel: "My Flow"})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, _, err := prefs.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Mode != "flow" || got.WorkflowID != "wf-1" {
		t.Fatalf("got=%+v", got)
	}
}

func TestSaveLoad_YoloTrueAndFalse(t *testing.T) {
	home := t.TempDir()
	t.Setenv("APPDATA", home)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	if len(prefs.Paths()) == 0 {
		t.Skip("no prefs paths on this OS")
	}
	on := true
	if _, err := prefs.Save(prefs.Session{Provider: "grok", Mode: "chat", Yolo: &on}); err != nil {
		t.Fatalf("Save on: %v", err)
	}
	got, _, err := prefs.Load()
	if err != nil {
		t.Fatalf("Load on: %v", err)
	}
	if got.Yolo == nil || !*got.Yolo {
		t.Fatalf("want yolo=true, got %+v", got.Yolo)
	}
	off := false
	if _, err := prefs.Save(prefs.Session{Provider: "grok", Mode: "chat", Yolo: &off}); err != nil {
		t.Fatalf("Save off: %v", err)
	}
	got, _, err = prefs.Load()
	if err != nil {
		t.Fatalf("Load off: %v", err)
	}
	if got.Yolo == nil || *got.Yolo {
		t.Fatalf("want yolo=false, got %+v", got.Yolo)
	}
}
