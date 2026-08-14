package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func isPromptNewlineKey(msg tea.KeyMsg) bool {
	switch strings.ToLower(msg.String()) {
	case "shift+enter", "ctrl+enter", "alt+enter", "cmd+enter", "ctrl+j":
		return true
	}
	if msg.Type == tea.KeyCtrlJ {
		return true
	}
	if msg.Type == tea.KeyEnter && msg.Alt {
		return true
	}
	if msg.Type == tea.KeyRunes && len(msg.Runes) == 1 && msg.Runes[0] == '\n' {
		return true
	}
	return false
}

func isApprovalDecisionInput(input string) bool {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "approve", "/approve", "deny", "/deny":
		return true
	default:
		return false
	}
}

func approvalDecisionOf(input string) string {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "deny", "/deny":
		return "deny"
	default:
		return "approve"
	}
}
