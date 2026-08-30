package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/config"
)

// CA-600: the You prompt box is always full chat-pane width and left aligned —
// with the F2 sidebar on (chat pane narrower) and off (full terminal width) —
// for every provider. The [copy] chip must ride the last row flush before the
// right border, and every box border column must agree with the top border.
func TestRegression_YouBoxFullWidthWithAndWithoutSidebar(t *testing.T) {
	prompt := "[Change Contract]\nfeature: calc-core\nintent: them ham SubtractWithGuard vao calc.go tra ve error khi b > a\nsymbols: Subtract"
	cases := []struct {
		w      int
		sideOn bool
	}{
		{80, false},
		{197, true},
		{160, true},
	}
	for _, tc := range cases {
		for _, pk := range []string{"claude", "codex", "grok"} {
			m := New(config.ChatConfig{Provider: pk, Model: "m"}, "http://127.0.0.1:4317")
			m.width, m.height = tc.w, 30
			m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
			m.sessionPanel.RunID = "run-208282"
			m.sessionPanel.ProjectPath = "/tmp/p"
			enableSidebarForTest(m)
			m.addMessage("user", prompt, "")
			if m.useRightSidebar() != tc.sideOn {
				t.Fatalf("[%s w=%d] sidebar expected %v got %v", pk, tc.w, tc.sideOn, m.useRightSidebar())
			}
			view := m.View()
			chatW := m.chatWidth()
			width := safeTermWidth(chatW)
			var topCol = -1
			for _, line := range strings.Split(view, "\n") {
				plain := stripANSI(line)
				if len([]rune(plain)) > chatW {
					plain = string([]rune(plain)[:chatW])
				}
				if strings.Contains(plain, "┌") && strings.Contains(plain, "You") {
					topCol = lastBarCol([]rune(plain))
					if strings.HasPrefix(plain, " ") {
						t.Fatalf("[%s w=%d] You box has lead pad: %q", pk, tc.w, plain)
					}
					if topCol < width-2 {
						t.Fatalf("[%s w=%d] You top border at %d want >=%d: %q", pk, tc.w, topCol, width-2, plain)
					}
					break
				}
			}
			if topCol < 0 {
				t.Fatalf("[%s w=%d] missing You top border", pk, tc.w)
			}
			for _, line := range strings.Split(view, "\n") {
				plain := stripANSI(line)
				if len([]rune(plain)) > chatW {
					plain = string([]rune(plain)[:chatW])
				}
				if strings.Contains(plain, "intent:") || strings.Contains(plain, "symbols:") || strings.Contains(plain, "[copy]") {
					col := lastBarCol([]rune(plain))
					if col != topCol {
						t.Fatalf("[%s w=%d] inner bar col %d != top col %d line=%q", pk, tc.w, col, topCol, plain)
					}
				}
			}
		}
	}
}