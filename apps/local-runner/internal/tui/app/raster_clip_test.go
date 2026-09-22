package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/config"
)

func lastBarCol(rs []rune) int {
	for i := len(rs) - 1; i >= 0; i-- {
		switch rs[i] {
		case '│', '┐', '┘', '|', '+':
			return i
		}
	}
	return -1
}

func TestRegression_RasterClipIsolatesColumns(t *testing.T) {
	prompt := "intent: them ham SubtractWithGuard vao calc.go tra ve error\nsymbols: Subtract"
	for _, w := range []int{250, 197, 160, 120} {
		for _, pk := range []string{"claude", "codex", "grok"} {
			m := New(config.ChatConfig{Provider: pk, Model: "m"}, "http://127.0.0.1:4317")
			m.width, m.height = w, 30
			m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
			m.sessionPanel.RunID = "run-208282"
			m.sessionPanel.ProjectPath = "/tmp/p"
			enableSidebarForTest(m)
			m.addMessage("user", prompt, "")
			m.addMessage("assistant", "ok", "")
			if !m.useRightSidebar() {
				continue
			}
			chatW := m.chatWidth()
			sideW := m.sideWidth()
			view := m.View()
			for i, line := range strings.Split(view, "\n") {
				rs := []rune(stripANSI(line))
				if len(rs) > chatW+sideW {
					t.Fatalf("[%s w=%d line %d] view wider than chatW+sideW=%d got %d", pk, w, i, chatW+sideW, len(rs))
				}
				left := ""
				if len(rs) > chatW {
					left = string(rs[:chatW])
				} else {
					left = string(rs)
				}
				// Chat column must never contain sidebar text.
				if strings.Contains(left, "Runner:") || strings.Contains(left, "Run: run-") || strings.Contains(left, "session") {
					t.Fatalf("[%s w=%d line %d] sidebar text in chat column: %q", pk, w, i, left)
				}
				// You box right border must sit at chatW-2/chatW-1 (full-pane, safeTermWidth
				// leaves one free column), never mid-column.
				if strings.Contains(left, "┌") && strings.Contains(left, "You") {
					bar := lastBarCol([]rune(left))
					if bar < chatW-2 {
						t.Fatalf("[%s w=%d line %d] You top border at %d want >=%d: %q", pk, w, i, bar, chatW-2, left)
					}
				}
				if strings.Contains(left, "intent:") {
					bar := lastBarCol([]rune(left))
					if bar < chatW-2 {
						t.Fatalf("[%s w=%d line %d] intent border at %d want >=%d: %q", pk, w, i, bar, chatW-2, left)
					}
				}
			}
			// Sidebar content must exist in the right column.
			all := stripANSI(view)
			if !strings.Contains(all, "Runner:") || !strings.Contains(all, "session") {
				t.Fatalf("[%s w=%d] sidebar content missing", pk, w)
			}
		}
	}
}
