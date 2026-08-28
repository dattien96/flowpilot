package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// openableStepRunIDs lists child run IDs for F2 steps that expose [open].
func (m *AppModel) openableStepRunIDs() []string {
	limit := len(m.flowSteps)
	if limit > 8 {
		limit = 8
	}
	var out []string
	for i := 0; i < limit; i++ {
		if child, ok := m.childRunForStep(m.flowSteps[i]); ok {
			out = append(out, child.RunID)
		}
	}
	return out
}

func (m *AppModel) f2StepPickerActive() bool {
	return m.authPhase == AuthNone && !m.sessionPanel.Collapsed && len(m.collectSuggestions()) == 0
}

func (m *AppModel) clampF2StepPickIdx() {
	runs := m.openableStepRunIDs()
	if len(runs) == 0 {
		m.f2StepPickIdx = 0
		return
	}
	if m.f2StepPickIdx < 0 {
		m.f2StepPickIdx = 0
	}
	if m.f2StepPickIdx >= len(runs) {
		m.f2StepPickIdx = len(runs) - 1
	}
}

func (m *AppModel) handleF2StepPickerKey(msg tea.KeyMsg) (handled bool, model tea.Model, cmd tea.Cmd) {
	if !m.f2StepPickerActive() {
		return false, m, nil
	}
	runs := m.openableStepRunIDs()
	if len(runs) == 0 {
		return false, m, nil
	}
	m.clampF2StepPickIdx()
	if msg.Type != tea.KeyRunes || len(msg.Runes) != 1 {
		return false, m, nil
	}
	switch msg.Runes[0] {
	case '[':
		m.f2StepPickIdx = (m.f2StepPickIdx - 1 + len(runs)) % len(runs)
		return true, m, nil
	case ']':
		m.f2StepPickIdx = (m.f2StepPickIdx + 1) % len(runs)
		return true, m, nil
	case 'o', 'O':
		runID := strings.TrimSpace(runs[m.f2StepPickIdx])
		if runID == "" {
			return true, m, nil
		}
		return true, m, m.cmdFocusAgent(runID)
	}
	return false, m, nil
}
