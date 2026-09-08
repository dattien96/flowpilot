package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/config"
)

func TestFilterVibeArgSuggestions_ExactVibeOffersOnOffNotCp(t *testing.T) {
	sugg := filterVibeArgSuggestions("/vibe")
	if len(sugg) != 2 || sugg[0].value != "on" || sugg[1].value != "off" {
		t.Fatalf("got %+v want on, off", sugg)
	}
	for _, it := range sugg {
		if it.kind != "vibe" {
			t.Fatalf("kind=%q", it.kind)
		}
	}
	if filterVibeArgSuggestions("/vibe-cp") != nil {
		t.Fatal("/vibe-cp must not use the on/off picker")
	}
	if filterVibeArgSuggestions("/vibe-") != nil {
		t.Fatal("hyphen prefix belongs to /vibe-cp, not on/off")
	}
}
func TestFilterVibeArgSuggestions_PartialOff(t *testing.T) {
	sugg := filterVibeArgSuggestions("/vibe of")
	if len(sugg) != 1 || sugg[0].value != "off" {
		t.Fatalf("got %+v want off", sugg)
	}
	both := filterVibeArgSuggestions("/vibe o")
	if len(both) != 2 {
		t.Fatalf("/vibe o should still list on and off, got %+v", both)
	}
}

func TestCollectSuggestions_VibeHidesVibeCp(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.inputValue = "/vibe"
	items := m.collectSuggestions()
	if len(items) != 2 {
		t.Fatalf("len=%d %+v", len(items), items)
	}
	for _, it := range items {
		if strings.Contains(it.value, "vibe-cp") || it.value == "/vibe-cp" {
			t.Fatalf("vibe-cp leaked: %+v", items)
		}
	}
}

func TestSlashTabOnVibeFillsOnNotVibeCp(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.inputValue = "/vibe"
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	got := m2.(*AppModel).inputValue
	if got != "/vibe on" && got != "/vibe on " {
		t.Fatalf("first Tab inputValue=%q want /vibe on", got)
	}
	if strings.Contains(got, "vibe-cp") {
		t.Fatalf("Tab jumped to vibe-cp: %q", got)
	}
	m3, _ := m2.(*AppModel).handleKey(tea.KeyMsg{Type: tea.KeyTab})
	got2 := m3.(*AppModel).inputValue
	if !strings.HasPrefix(got2, "/vibe off") {
		t.Fatalf("second Tab inputValue=%q want /vibe off", got2)
	}
}

func TestSlashSuggestions_VibeHyphenStillListsCp(t *testing.T) {
	sugg := filterSlashSuggestions("/vibe-")
	found := false
	for _, sc := range sugg {
		if sc.name == "/vibe-cp" {
			found = true
		}
		if sc.name == "/vibe" {
			t.Fatal("/vibe must not match /vibe- prefix")
		}
	}
	if !found {
		t.Fatalf("want /vibe-cp in %+v", sugg)
	}
}
