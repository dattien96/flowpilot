package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/config"
)

func TestInit_DisablesAutoWrapFirst(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init must return Sequence(disable-autowrap, ...)")
	}
}

func TestWindowSize_ClearsScreen(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if cmd == nil {
		t.Fatal("WindowSize must ClearScreen (macOS desync)")
	}
}

func TestPaintRow_RawWidthNeverExceeds(t *testing.T) {
	in := styleUser.Render("intent: them ham") + strings.Repeat("─", 80)
	out := paintRow(in, 40, styleCanvas)
	if w := lipgloss.Width(out); w > 40 {
		t.Fatalf("paintRow raw width %d > 40", w)
	}
}
