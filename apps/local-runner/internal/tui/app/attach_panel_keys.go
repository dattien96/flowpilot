package app

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *AppModel) clampAttachPanelSel() {
	if len(m.pendingAttach) == 0 {
		m.attachPanelSel = 0
		return
	}
	if m.attachPanelSel < 0 {
		m.attachPanelSel = 0
	}
	if m.attachPanelSel >= len(m.pendingAttach) {
		m.attachPanelSel = len(m.pendingAttach) - 1
	}
}

func (m *AppModel) handleAttachPanelKey(msg tea.KeyMsg) (handled bool, model tea.Model, cmd tea.Cmd) {
	if !m.attachPanelOpen || len(m.pendingAttach) == 0 {
		return false, m, nil
	}
	m.clampAttachPanelSel()
	switch msg.Type {
	case tea.KeyUp:
		m.attachPanelSel = (m.attachPanelSel - 1 + len(m.pendingAttach)) % len(m.pendingAttach)
		return true, m, nil
	case tea.KeyDown:
		m.attachPanelSel = (m.attachPanelSel + 1) % len(m.pendingAttach)
		return true, m, nil
	case tea.KeyEnter:
		return true, m, m.cmdOpenPendingAttachment(m.attachPanelSel + 1)
	case tea.KeyRunes:
		if len(msg.Runes) != 1 {
			return false, m, nil
		}
		switch msg.Runes[0] {
		case 'x', 'X', 'd', 'D':
			idx := m.attachPanelSel + 1
			if name, ok := m.removePendingAttachment(idx); ok {
				m.clampAttachPanelSel()
				m.addMessage("system", fmt.Sprintf("Removed pending image: %s (%d left)", name, len(m.pendingAttach)), "")
			}
			return true, m, nil
		}
	}
	return false, m, nil
}
