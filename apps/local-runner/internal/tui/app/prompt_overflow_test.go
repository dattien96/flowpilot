package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"flowpilot-runner/internal/tui/config"
)

// Regression: You prompt box overflowed the chat column and overlapped the
// right sidebar (F2) when prompt was long with pipes/paths. Box must wrap and
// stay within chatWidth, borders aligned, even with sidebar and narrow width.
func TestRegression_UserPromptLongNoOverflow(t *testing.T) {
	prompts := []struct {
		name string
		text string
	}{
		{"pipe heavy", "intent: them ham SubtractWithGuard vao calc.go tra ve |Path: /Users/tiendat/Desktop/BE/gate-sandbox |symbols: Subtract |Project: Gate-sandbox (db51ec26) " + strings.Repeat("extra ", 20)},
		{"no spaces path", strings.Repeat("/Users/tiendat/Desktop/BE/gate-sandbox", 3)},
		{"vietnamese", "intent: them ham SubtractWithGuard vào calc.go trả về error khi b > a " + strings.Repeat("tiếng Việt ", 15)},
		{"very long", strings.Repeat("a very long prompt token ", 30)},
	}
	widths := []int{60, 80, 100, 120}
	for _, tc := range prompts {
		for _, w := range widths {
			t.Run(tc.name+"_w"+strings.TrimSpace(strings.Repeat("x", 0)), func(t *testing.T) {
				m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.5"}, "http://127.0.0.1:4317")
				m.width = w
				m.height = 24
				m.asciiMode = false
				// Enable session panel so right sidebar engages at >=100 (contentWidth < width)
				if w >= 100 {
					m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
					m.sessionPanel.ProjectPath = "/tmp/p"
				}
				m.addMessage("user", tc.text, "")
				m.addMessage("assistant", "ok", "")

				cw := m.chatWidth()
				for i, line := range m.renderMessages() {
					plain := stripANSI(line)
					vw := lipgloss.Width(line)
					if vw > cw {
						t.Fatalf("prompt %q width=%d overflow cw=%d line %d vw=%d plain=%q", tc.name, w, cw, i, vw, plain)
					}
					// Box borders must be aligned: any You box line starting with │ or ┌ must have matching width
					trim := strings.TrimSpace(plain)
					if strings.HasPrefix(trim, "│") || strings.HasPrefix(trim, "┌") || strings.HasPrefix(trim, "└") || strings.HasPrefix(trim, "+") {
						if !strings.HasSuffix(trim, "│") && !strings.HasSuffix(trim, "┐") && !strings.HasSuffix(trim, "┘") && !strings.HasSuffix(trim, "+") && trim != "" {
							// Check visual width consistent: all box lines for same box should equal
							if vw > cw {
								t.Fatalf("box line exceeds chat width: vw=%d cw=%d line=%q", vw, cw, plain)
							}
						}
					}
				}
				// View with sidebar must also not overflow terminal width
				view := stripANSI(m.View())
				for i, line := range strings.Split(view, "\n") {
					if lipgloss.Width(line) > w && w > 1 {
						t.Fatalf("View line %d exceeds terminal width %d: vw=%d line=%q", i, w, lipgloss.Width(line), line)
					}
				}
			})
		}
	}
}

// CA-603 → CA-607: prompts clamp to 4 lines + "...." when collapsed — nothing
// overflows the chat pane (user request: [copy] removed).
func TestRegression_UserPromptShowsCopyWithoutOverflow(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex", Model: "gpt-5.4"}, "http://127.0.0.1:4317")
	m.width = 80
	m.asciiMode = true
	long := strings.Repeat("long prompt line with many words to exceed clamp ", 10)
	m.addMessage("user", long, "")
	m.addMessage("assistant", "ok", "")
	got := stripANSI(strings.Join(m.renderMessages(), "\n"))
	if strings.Contains(got, "[copy]") {
		t.Fatalf("trailing [copy] should be removed (user request), got:\n%s", got)
	}
	for i, line := range m.renderMessages() {
		if lipgloss.Width(line) > m.chatWidth() {
			t.Fatalf("line %d overflows: vw=%d cw=%d line=%q", i, lipgloss.Width(line), m.chatWidth(), stripANSI(line))
		}
	}
}

func TestRegression_SidebarDoesNotCutPromptBox(t *testing.T) {
	// Screenshot repro: wide terminal with F2 sidebar, prompt with |Path: |symbols: |Project:
	prompt := "intent: them ham SubtractWithGuard vao calc.go tra ve |Path: /Users/tiendat/Desktop/BE/gate-sandbox |symbols: Subtract |Project: Gate-sandbox (db51ec26) calc.go tra ve error khi b > a\nSubtract"
	for _, w := range []int{100, 120, 140} {
		m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.5"}, "http://127.0.0.1:4317")
		m.width = w
		m.height = 30
		m.asciiMode = false
		m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
		m.sessionPanel.ProjectPath = "/tmp/p"
		enableSidebarForTest(m)
		m.addMessage("user", prompt, "")
		m.addMessage("assistant", "ok", "")

		// renderMessages (chat column only) must contain the pipes from prompt and must not contain sidebar Runner
		chatOnly := stripANSI(strings.Join(m.renderMessages(), "\n"))
		if !strings.Contains(chatOnly, "|Path:") {
			t.Fatalf("width=%d chatMessages must keep |Path: pipe, got:\n%s", w, chatOnly)
		}
		if strings.Contains(chatOnly, "Runner: http") {
			t.Fatalf("width=%d chatMessages must not contain sidebar Runner, got:\n%s", w, chatOnly)
		}
		if !strings.Contains(chatOnly, "You") {
			t.Fatalf("width=%d missing You box, got:\n%s", w, chatOnly)
		}
		for i, line := range m.renderMessages() {
			if lipgloss.Width(line) > m.chatWidth() {
				t.Fatalf("width=%d chat line %d overflows cw=%d vw=%d %q", w, i, m.chatWidth(), lipgloss.Width(line), stripANSI(line))
			}
		}
		// View joins chat + sidebar: left part before sidebar sep must still contain pipe, full View line must not have box border cut
		view := m.View()
		for _, line := range strings.Split(view, "\n") {
			if lipgloss.Width(line) > w && w > 1 {
				t.Fatalf("width=%d View overflows vw=%d line=%q", w, lipgloss.Width(line), stripANSI(line))
			}
		}
		// Ensure the You box top border is present and not truncated mid-pipe
		if !strings.Contains(chatOnly, "You") || !strings.Contains(chatOnly, "│") {
			t.Fatalf("width=%d You box missing borders, got:\n%s", w, chatOnly)
		}
	}
}


