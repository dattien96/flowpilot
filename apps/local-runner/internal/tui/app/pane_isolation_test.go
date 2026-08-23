package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"flowpilot-runner/internal/tui/config"
)

func TestRegression_ChatPaneDoesNotContainSidebar(t *testing.T) {
	prompt := "intent: them ham\nfiles:\nsymbols: Subtract"
	for _, w := range []int{197, 120} {
		for _, pk := range []string{"claude", "codex", "grok"} {
			m := New(config.ChatConfig{Provider: pk, Model: "m"}, "http://127.0.0.1:4317")
			m.width, m.height = w, 30
			m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
			m.sessionPanel.ProjectPath = "/tmp/p"
			m.sessionPanel.Collapsed = false
			m.addMessage("user", prompt, "")
			if !m.useRightSidebar() {
				t.Fatal("sidebar")
			}
			chatW := m.chatWidth()
			view := m.View()
			for i, line := range strings.Split(view, "\n") {
				plain := stripANSI(line)
				if lipgloss.Width(plain) >= w {
					t.Fatalf("[%s w=%d line %d] vw>=term", pk, w, i)
				}
				left := plain
				if lipgloss.Width(plain) > chatW {
					left = truncateVisual(plain, chatW)
				}
				if strings.Contains(left, "Runner:") || strings.Contains(left, "session") {
					t.Fatalf("[%s w=%d] chat pane leaked sidebar: %q", pk, w, left)
				}
			}
			all := stripANSI(view)
			if !strings.Contains(all, "Runner:") || !strings.Contains(all, "session") {
				t.Fatal("sidebar pane missing")
			}
		}
	}
}
