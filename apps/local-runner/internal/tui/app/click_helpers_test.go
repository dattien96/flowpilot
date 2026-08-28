package app

import (
	tea "github.com/charmbracelet/bubbletea"
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