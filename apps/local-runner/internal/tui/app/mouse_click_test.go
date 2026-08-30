package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// Task-311: session/skills/status-details chrome clicks are no-ops — the
// sidebar is width-reactive and details live there; no toggle state exists.

func TestMouseClick_SessionChipIsNoOp(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
	m2, _ := m.activateClickTarget("session")
	am := m2.(*AppModel)
	if len(am.messages) != 0 {
		t.Fatal("session chip click must not toggle or spam")
	}
}

func TestMouseClick_SkillsChipIsNoOp(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.selectedSkills = []client.SkillSelection{{Name: "coding"}, {Name: "review"}}
	m2, _ := m.activateClickTarget("skills")
	if len(m2.(*AppModel).messages) != 0 {
		t.Fatal("skills chip click must be a no-op")
	}
}

func TestMouseClick_StatusDetailsChipIsNoOp(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m2, _ := m.activateClickTarget("status-details")
	if len(m2.(*AppModel).messages) != 0 {
		t.Fatal("status-details click must be a no-op")
	}
}

func TestMouseClick_ReleaseIgnored(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.selectedSkills = []client.SkillSelection{{Name: "coding"}}
	m2, _ := m.Update(tea.MouseMsg{X: 10, Y: 10, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft})
	if len(m2.(*AppModel).messages) != 0 {
		t.Fatal("mouse release must not trigger anything")
	}
}