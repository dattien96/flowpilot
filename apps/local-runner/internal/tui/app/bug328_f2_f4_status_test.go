package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/config"
)

// Task-311: F2/F4 no longer toggle anything. F2 is the /info alias and must
// print the session dump; F3/F4 must be no-ops.
func TestBug328_F2PrintsInfoDump(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.authPhase = AuthNone
	m.sessionDefaultsLoaded = true

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyF2})
	am := got.(*AppModel)
	if len(am.messages) == 0 {
		t.Fatal("F2 must print the info dump")
	}
	if !strings.Contains(am.messages[len(am.messages)-1].Content, "Status:") {
		t.Fatalf("F2 info dump must contain Status line, got %q", am.messages[len(am.messages)-1].Content)
	}
}

func TestBug328_F4IsNoOp(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.authPhase = AuthNone
	m.sessionDefaultsLoaded = true
	m.statusMsg = ""

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyF4})
	am := got.(*AppModel)
	if len(am.messages) != 0 {
		t.Fatal("F4 must not add messages")
	}
	if am.statusMsg != "" {
		t.Fatalf("F4 must not set status, got %q", am.statusMsg)
	}
}
