package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func clickLeft(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
}

func findClickTarget(m *AppModel, want string) (x, y int, ok bool) {
	w, h := m.width, m.height
	if w <= 0 {
		w = 80
	}
	if h <= 0 {
		h = 24
	}
	for yy := 0; yy < h; yy++ {
		for xx := 0; xx < w; xx++ {
			if m.clickTargetAt(xx, yy) == want {
				return xx, yy, true
			}
		}
	}
	return 0, 0, false
}

func TestMouseClick_TogglesSessionPanel(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.asciiMode = true
	m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
	m.sessionPanel.Collapsed = true

	x, y, ok := findClickTarget(m, "session")
	if !ok {
		t.Fatal("expected a clickable session panel chip")
	}
	m2, _ := m.Update(clickLeft(x, y))
	am := m2.(*AppModel)
	if am.sessionPanel.Collapsed {
		t.Fatalf("click at %d,%d should expand session panel", x, y)
	}
	m3, _ := am.Update(clickLeft(x, y))
	am = m3.(*AppModel)
	if !am.sessionPanel.Collapsed {
		t.Fatal("second click should collapse session panel")
	}
}

func TestMouseClick_TogglesSkillsChip(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.asciiMode = true
	m.selectedSkills = []client.SkillSelection{{Name: "coding"}, {Name: "review"}}

	x, y, ok := findClickTarget(m, "skills")
	if !ok {
		t.Fatal("expected a clickable skills chip")
	}
	m2, _ := m.Update(clickLeft(x, y))
	am := m2.(*AppModel)
	if !am.statusSkillsExpanded {
		t.Fatalf("click at %d,%d should expand skills", x, y)
	}
	m3, _ := am.Update(clickLeft(x, y))
	am = m3.(*AppModel)
	if am.statusSkillsExpanded {
		t.Fatal("second click should collapse skills")
	}
}

func TestMouseClick_MissDoesNothing(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.asciiMode = true
	m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
	m.sessionPanel.Collapsed = true
	m.selectedSkills = []client.SkillSelection{{Name: "coding"}}
	wasCollapsed := m.sessionPanel.Collapsed
	m2, _ := m.Update(clickLeft(0, 12))
	am := m2.(*AppModel)
	if am.sessionPanel.Collapsed != wasCollapsed || am.statusSkillsExpanded {
		t.Fatal("click in empty chat area must not toggle F2/F3 chrome")
	}
}

func TestMouseClick_ReleaseIgnored(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.asciiMode = true
	m.selectedSkills = []client.SkillSelection{{Name: "coding"}}
	x, y, ok := findClickTarget(m, "skills")
	if !ok {
		t.Fatal("expected skills chip")
	}
	m2, _ := m.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft})
	if m2.(*AppModel).statusSkillsExpanded {
		t.Fatal("mouse release must not toggle")
	}
}
