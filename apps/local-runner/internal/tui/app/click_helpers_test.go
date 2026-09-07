package app

import (
	tea "github.com/charmbracelet/bubbletea"
)

func clickLeft(x, y int) tea.MouseMsg {
	return tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft}
}

// clickLeftFull simulates a real terminal click: press + release on the same
// cell. BUG-359: an expanded prompt box toggles on release-click, not on
// press-down (press arms drag-select so long prompts stay copyable) —
// collapsing one therefore needs both halves, while expanding a collapsed
// box still works press-only.
func clickLeftFull(m *AppModel, x, y int) *AppModel {
	m2, _ := m.Update(clickLeft(x, y))
	am := m2.(*AppModel)
	m3, _ := am.Update(tea.MouseMsg{X: x, Y: y, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft})
	return m3.(*AppModel)
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