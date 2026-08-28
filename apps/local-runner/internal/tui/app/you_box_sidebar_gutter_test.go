package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"flowpilot-runner/internal/tui/config"
)

// Regression 197x57: You bubble right-align sát cột F2 produced "││Runner"
// / "┐│session" (box border glued to sidebar sep). Right-aligned user boxes
// must keep a gutter before the F2 separator so the chat column visually
// separates from the sidebar. This tests the actual screenshot width plus a few
// others.
func TestRegression_YouBoxKeepsGutterBeforeSidebar(t *testing.T) {
	prompt := "intent: them ham SubtractWithGuard vao calc.go tra ve |Path: /Users/tiendat/Desktop/BE/gate-sandbox |symbols: Subtract |Project: Gate-sandbox (db51ec26) calc.go tra ve error khi b > a\nSubtract"
	for _, w := range []int{197, 160, 140, 120} {
		t.Run("", func(t *testing.T) {
			for _, pk := range []string{"claude", "codex", "grok"} {
				m := New(config.ChatConfig{Provider: pk, Model: "m"}, "http://127.0.0.1:4317")
				m.width, m.height = w, 30
				m.asciiMode = false
				m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
				m.sessionPanel.ProjectPath = "/tmp/p"
				enableSidebarForTest(m)
				m.addMessage("user", prompt, "")
				m.addMessage("assistant", "ok", "")
				if !m.useRightSidebar() {
					t.Fatalf("[%s w=%d] must engage right sidebar", pk, w)
				}
				view := m.View()
				sideX := m.terminalWidth() - m.sideWidth()
				contentW := sideX - 1
				for i, line := range strings.Split(view, "\n") {
					plain := stripANSI(line)
					if lipgloss.Width(line) > w {
						t.Fatalf("[%s w=%d line %d] View overflows vw=%d: %q", pk, w, i, lipgloss.Width(line), plain)
					}
					// Only inspect lines that carry a user You box.
					hasBox := strings.Contains(plain, "You") || (strings.Contains(plain, "Subtract") && strings.Contains(plain, "│"))
					if !hasBox {
						continue
					}
					// Split into chat column vs sidebar+sep at the join.
					leftPlain := plain
					if len([]rune(plain)) > contentW+1 {
						leftPlain = string([]rune(plain)[:contentW])
					} else if lipgloss.Width(plain) > contentW {
						// Fallback: truncate by visual width (rare with ANSI, but View lines are styled).
						leftPlain = truncateVisual(plain, contentW)
					}
					// 1) Chat column alone must still hold box borders (not truncated by joinRightSidebar).
					if !(strings.Contains(leftPlain, "┌") && strings.Contains(leftPlain, "┐")) &&
						!(strings.Contains(leftPlain, "│")) &&
						!(strings.Contains(leftPlain, "└") && strings.Contains(leftPlain, "┘")) {
						// Not a box row (e.g. wrapped inner without corners) — still must not contain sidebar text.
					}
					if strings.Contains(leftPlain, "Runner:") || strings.Contains(leftPlain, "session") {
						t.Fatalf("[%s w=%d line %d] chat column must not contain sidebar text: left=%q", pk, w, i, leftPlain)
					}
					// 2) Gutter: chat column must end with space — forbids "┐│" / "││" glue at the join.
					if lipgloss.Width(leftPlain) >= 1 && !strings.HasSuffix(leftPlain, " ") {
						t.Fatalf("[%s w=%d line %d] chat column before sep must end with gutter space, got left=%q full=%q", pk, w, i, leftPlain, plain)
					}
					// 3) Box right edge must not be cut: any You-box line in the chat column must
					// still end with its box glyph before the gutter.
					trim := strings.TrimRight(leftPlain, " ")
					if strings.Contains(trim, "You") || strings.Contains(trim, "Subtract") || strings.HasPrefix(strings.TrimSpace(trim), "│") || strings.HasPrefix(strings.TrimSpace(trim), "┌") || strings.HasPrefix(strings.TrimSpace(trim), "└") {
						if !(strings.HasSuffix(trim, "│") || strings.HasSuffix(trim, "┐") || strings.HasSuffix(trim, "┘")) {
							t.Fatalf("[%s w=%d line %d] You box line missing right border before gutter: left=%q", pk, w, i, leftPlain)
						}
					}
				}
			}
		})
	}
}
