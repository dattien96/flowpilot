package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/config"
)

func TestBug328_F2SetsVisibleStatus(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.authPhase = AuthNone
	m.sessionPanel.Collapsed = true

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyF2})
	am := got.(*AppModel)
	if am.sessionPanel.Collapsed {
		t.Fatal("F2 must expand a collapsed steps panel")
	}
	if !strings.Contains(am.statusMsg, "F2") {
		t.Fatalf("F2 must set a visible status, got %q", am.statusMsg)
	}

	got, _ = am.handleKey(tea.KeyMsg{Type: tea.KeyF2})
	am = got.(*AppModel)
	if !am.sessionPanel.Collapsed {
		t.Fatal("second F2 must collapse again")
	}
	if !strings.Contains(am.statusMsg, "F2") {
		t.Fatalf("second F2 must still set status, got %q", am.statusMsg)
	}
}

func TestBug328_F4SetsVisibleStatus(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.authPhase = AuthNone
	m.statusDetailsCollapsed = false

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyF4})
	am := got.(*AppModel)
	if !am.statusDetailsCollapsed {
		t.Fatal("F4 must collapse details")
	}
	if !strings.Contains(am.statusMsg, "F4") {
		t.Fatalf("F4 must set a visible status, got %q", am.statusMsg)
	}
}
