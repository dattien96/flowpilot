package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/config"
)

func lastBoxGlyphCol(plain string) int {
	rs := []rune(plain)
	for i := len(rs) - 1; i >= 0; i-- {
		switch rs[i] {
		case '│', '┐', '┘', '|', '+':
			return i
		}
	}
	return -1
}

func TestRegression_YouBoxBarAlignsWithTopWhenSidebarOn(t *testing.T) {
	prompt := "intent: them ham SubtractWithGuard vao calc.go tra ve error\nsymbols: Subtract"
	for _, w := range []int{197, 160} {
		for _, pk := range []string{"claude", "codex", "grok"} {
			m := New(config.ChatConfig{Provider: pk, Model: "m"}, "http://127.0.0.1:4317")
			m.width, m.height = w, 30
			m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
			m.sessionPanel.RunID = "run-208282"
			m.sessionPanel.ProjectPath = "/tmp/p"
			m.sessionPanel.Collapsed = false
			m.addMessage("user", prompt, "")
			if !m.useRightSidebar() {
				t.Fatalf("[%s w=%d] need sidebar", pk, w)
			}
			view := m.View()
			chatW := m.chatWidth()
			var topCol = -1
			for _, line := range strings.Split(view, "\n") {
				plain := stripANSI(line)
				if strings.Contains(plain, "┌") && strings.Contains(plain, "You") {
					// Truncate to chat pane before measuring
					if len([]rune(plain)) > chatW {
						plain = string([]rune(plain)[:chatW])
					}
					topCol = lastBoxGlyphCol(plain)
					break
				}
			}
			if topCol < 0 {
				t.Fatalf("[%s w=%d] missing You top", pk, w)
			}
			for _, line := range strings.Split(view, "\n") {
				plain := stripANSI(line)
				if len([]rune(plain)) > chatW {
					plain = string([]rune(plain)[:chatW])
				}
				if strings.Contains(plain, "intent:") || strings.Contains(plain, "[copy]") {
					col := lastBoxGlyphCol(plain)
					if col < 0 {
						continue
					}
					if col != topCol {
						t.Fatalf("[%s w=%d] inner bar col %d != top col %d line=%q", pk, w, col, topCol, plain)
					}
					// Also ensure Runner: not inside the box (should be after topCol)
					if idx := strings.Index(plain, "Runner:"); idx >= 0 && idx < topCol {
						t.Fatalf("[%s w=%d] Runner leaked inside box idx=%d topCol=%d line=%q", pk, w, idx, topCol, plain)
					}
				}
			}
			// Full view: Runner must be at >= chatW, not mid
			for i, line := range strings.Split(view, "\n") {
				plain := stripANSI(line)
				if idx := strings.Index(plain, "Runner:"); idx >= 0 && idx < chatW-1 && strings.Contains(plain, "intent:") {
					t.Fatalf("[%s w=%d line %d] Runner mid-screen idx=%d chatW=%d line=%q", pk, w, i, idx, chatW, plain)
				}
			}
		}
	}
}
