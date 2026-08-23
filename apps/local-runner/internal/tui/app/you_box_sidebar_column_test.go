package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"flowpilot-runner/internal/tui/config"
)

func TestRegression_YouBoxSidebarColumnFixed(t *testing.T) {
	prompt := "intent: them ham\nfiles:\nsymbols: Subtract"
	for _, w := range []int{197, 160, 120} {
		for _, pk := range []string{"claude", "codex", "grok"} {
			m := New(config.ChatConfig{Provider: pk, Model: "m"}, "http://127.0.0.1:4317")
			m.width, m.height = w, 30
			m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
			m.sessionPanel.RunID = "run-208282"
			m.sessionPanel.ProjectPath = "/tmp/p"
			m.sessionPanel.Collapsed = false
			m.addMessage("user", prompt, "")
			m.addMessage("assistant", "assistant reply that is long enough to fill chat width and should not bleed", "")
			if !m.useRightSidebar() {
				continue
			}
			chatW := m.chatWidth()
			view := m.View()
			for i, line := range strings.Split(view, "\n") {
				plain := stripANSI(line)
				// Sidebar must start at >= chatW, not in the middle of a narrow You box.
				if strings.Contains(plain, "Runner:") || strings.Contains(plain, "session") {
					idx := strings.Index(plain, "Runner:")
					if idx < 0 {
						idx = strings.Index(plain, "session")
					}
					if idx >= 0 && idx < chatW-1 {
						t.Fatalf("[%s w=%d line %d] sidebar leaked into chat column idx=%d chatW=%d line=%q", pk, w, i, idx, chatW, plain)
					}
				}
				// Any line that contains both You/intent and Runner/session on same row would be bleed.
				hasPrompt := strings.Contains(plain, "intent:") || strings.Contains(plain, "You") || strings.Contains(plain, "Subtract")
				hasSide := strings.Contains(plain, "Runner:") || strings.Contains(plain, "session")
				if hasPrompt && hasSide {
					// They must be separated by at least chatW distance
					pi := strings.Index(plain, "intent:")
					if pi < 0 {
						pi = strings.Index(plain, "You")
					}
					si := strings.Index(plain, "Runner:")
					if si < 0 {
						si = strings.Index(plain, "session")
					}
					if pi >= 0 && si >= 0 && si-pi < 20 {
						t.Fatalf("[%s w=%d line %d] You box and sidebar on same narrow row (gap %d) chatW=%d line=%q", pk, w, i, si-pi, chatW, plain)
					}
				}
				if vw := lipgloss.Width(plain); vw >= w {
					t.Fatalf("[%s w=%d line %d] View overflow vw=%d w=%d line=%q", pk, w, i, vw, w, plain)
				}
			}
		}
	}
}
