package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"flowpilot-runner/internal/tui/config"
)

// CA-599: chat and sidebar are painted into one cell buffer at fixed rects —
// chat cells x<chatW, sidebar cells x>=chatW. A styled You row can never drag
// the sidebar into the chat column because placement is per-cell, not string
// concatenation.
func TestRegression_CellBufColumnsIsolated(t *testing.T) {
	prompt := "[Change Contract]\nfeature: calc-core\nintent: them ham SubtractWithGuard vao calc.go tra ve error khi b > a\nsymbols: Subtract"
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
				plain := stripANSI(line)
				if lipgloss.Width(plain) > w {
					t.Fatalf("[%s w=%d line %d] line wider than terminal: %d > %d", pk, w, i, lipgloss.Width(plain), w)
				}
				rs := []rune(plain)
				if len(rs) > chatW+sideW {
					t.Fatalf("[%s w=%d line %d] line wider than chat+side %d: %d", pk, w, i, chatW+sideW, len(rs))
				}
				left := string(rs[:minInt(len(rs), chatW)])
				// Chat cells must never contain sidebar text.
				for _, banned := range []string{"Runner: http", "Run: run-", "session", "Path: /tmp"} {
					if strings.Contains(left, banned) {
						t.Fatalf("[%s w=%d line %d] sidebar text %q in chat cells (x<chatW=%d): %q", pk, w, i, banned, chatW, left)
					}
				}
				// Full line must contain the You box content on its rows.
				if strings.Contains(plain, "intent:") && !strings.Contains(plain, "tra ve error") {
					t.Fatalf("[%s w=%d line %d] intent row missing text: %q", pk, w, i, plain)
				}
				// Every You box row must have its right border at the same column
				// (>= chatW-2), never mid-screen.
				if strings.Contains(left, "You") || strings.Contains(left, "intent:") || strings.Contains(left, "symbols:") {
					bar := lastBarCol([]rune(left))
					if bar >= 0 && bar < chatW-2 {
						t.Fatalf("[%s w=%d line %d] box border at %d (chatW=%d): %q", pk, w, i, bar, chatW, left)
					}
				}
			}
			// Sidebar content present in right cells.
			all := stripANSI(view)
			for _, need := range []string{"session", "Runner: http", "Path: /tmp"} {
				if !strings.Contains(all, need) {
					t.Fatalf("[%s w=%d] sidebar missing %q", pk, w, need)
				}
			}
		}
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}