package app

import (
	"path/filepath"
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/config"
	"flowpilot-runner/internal/tui/prefs"
)

func TestNew_RestoresProviderFromPrefs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	if _, err := prefs.Save(prefs.Session{Provider: "grok", Model: "grok-4.5"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	if m.provider != "grok" || m.model != "grok-4.5" {
		t.Fatalf("provider=%q model=%q", m.provider, m.model)
	}
}

func TestNew_FlagOverridesPrefs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	if _, err := prefs.Save(prefs.Session{Provider: "grok", Model: "grok-4.5"}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	m := New(config.ChatConfig{Provider: "claude", Model: "opus"}, "http://127.0.0.1:4317")
	if m.provider != "claude" || m.model != "opus" {
		t.Fatalf("provider=%q model=%q", m.provider, m.model)
	}
}

func TestSlashNew_KeepsProviderAndPersists(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("APPDATA", dir)
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.provider = "grok"
	m.model = "grok-4.5"
	m2, _ := m.handleSlashCommand("/new")
	am := m2.(*AppModel)
	if am.provider != "grok" {
		t.Fatalf("provider reset to %q", am.provider)
	}
	if !strings.Contains(am.messages[len(am.messages)-1].Content, "grok") {
		t.Fatalf("new chat message missing provider: %q", am.messages[len(am.messages)-1].Content)
	}
	got, _, err := prefs.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Provider != "grok" {
		t.Fatalf("prefs not saved: %+v (paths under %s)", got, filepath.Join(dir))
	}
}
