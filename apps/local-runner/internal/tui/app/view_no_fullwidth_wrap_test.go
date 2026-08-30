package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"flowpilot-runner/internal/tui/config"
)

// macOS Terminal.app / Ghostty / iTerm autowrap a line that fills the last
// column — the 197-wide "tràn" screenshot (sidebar bleed into chat) was this
// desync, not real text wrapping in the You box. Windows Terminal is fine at
// full width. Every View line must stay strictly < terminalWidth so no terminal
// ever wraps.
func TestRegression_ViewNeverFillsLastColumn(t *testing.T) {
	prompt := "intent: them ham SubtractWithGuard vao calc.go tra ve |Path: /Users/tiendat/Desktop/BE/gate-sandbox |symbols: Subtract |Project: Gate-sandbox (db51ec26) calc.go tra ve error khi b > a\nSubtract"
	cases := []struct {
		w    int
		side bool
	}{
		{w: 197, side: true},
		{w: 120, side: true},
		{w: 80, side: false},
	}
	for _, tc := range cases {
		t.Run("", func(t *testing.T) {
			for _, pk := range []string{"claude", "codex", "grok"} {
				m := New(config.ChatConfig{Provider: pk, Model: "m"}, "http://127.0.0.1:4317")
				m.width, m.height = tc.w, 30
				m.fullWidth = tc.w
				m.asciiMode = false
				if tc.side {
					m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
					m.sessionPanel.ProjectPath = "/tmp/p"
					enableSidebarForTest(m)
					m.width, m.fullWidth = tc.w, tc.w
				} else {
					m.width, m.fullWidth = tc.w, tc.w
				}
				m.addMessage("user", prompt, "")
				m.addMessage("assistant", "ok", "")
				if tc.side && !m.useRightSidebar() {
					t.Fatalf("[%s w=%d] must engage sidebar", pk, tc.w)
				}
				view := m.View()
				for i, line := range strings.Split(view, "\n") {
					plain := stripANSI(line)
					vw := lipgloss.Width(plain)
					// Sidebar case must be strictly < (macOS autowrap). No-sidebar legacy keeps rule at full width.
					if tc.side {
						if vw >= tc.w {
							t.Fatalf("[%s w=%d line %d] View line must be < terminalWidth (macOS autowrap): vw=%d w=%d line=%q", pk, tc.w, i, vw, tc.w, plain)
						}
					} else {
						if vw > tc.w {
							t.Fatalf("[%s w=%d line %d] View line must be <= terminalWidth: vw=%d w=%d line=%q", pk, tc.w, i, vw, tc.w, plain)
						}
					}
				}
			}
		})
	}
}
