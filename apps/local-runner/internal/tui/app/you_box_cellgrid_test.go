package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/config"
)

// CA-606: the real View() path must render the You box in the chat pane under
// TrueColor (the color profile Ghostty uses) — no mid-word break at the box
// edge, all right borders aligned, [copy] on its own row. CA-607: the 5-line
// pasted prompt clamps to 4 lines + "...." collapsed; after an expand click all
// 5 lines render in order. Plain-Ascii unit tests cannot see SGR width drift,
// so this test forces TrueColor and inspects the chat column.
func TestRegression_YouBoxTrueColorViewAllLines(t *testing.T) {
	forceTrueColor(t)
	prompt := "[Change Contract]\nfeature: calc-core\nintent: them ham SubtractWithGuard vao calc.go tra ve error khi b > a\nfiles: calc.go, calc_test.go\nsymbols: Subtract"
	clamped := []string{
		"[Change Contract]",
		"feature: calc-core",
		"intent: them ham SubtractWithGuard vao calc.go tra ve error khi b > a",
		"files: calc.go, calc_test.go....",
	}
	expanded := []string{
		"[Change Contract]",
		"feature: calc-core",
		"intent: them ham SubtractWithGuard vao calc.go tra ve error khi b > a",
		"files: calc.go, calc_test.go",
		"symbols: Subtract",
	}
	for _, h := range []int{24, 30} {
		for _, pk := range []string{"claude", "codex", "grok"} {
			m := New(config.ChatConfig{Provider: pk, Model: "m"}, "http://127.0.0.1:4317")
			m.width, m.height = 197, h
			m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
			m.sessionPanel.RunID = "run-208282"
			m.sessionPanel.ProjectPath = "/tmp/p"
			enableSidebarForTest(m)
			m.addMessage("user", prompt, "")
			if !m.useRightSidebar() {
				t.Fatalf("[%s h=%d] need sidebar", pk, h)
			}
			chatW := m.chatWidth()
			rows := func() []string {
				out := make([]string, 0)
				for _, line := range strings.Split(stripANSI(m.View()), "\n") {
					if len([]rune(line)) > chatW {
						line = string([]rune(line)[:chatW])
					}
					out = append(out, line)
				}
				return out
			}
			checkBars := func(label string, rr []string) {
				t.Helper()
				var topBar = -1
				for _, row := range rr {
					if strings.Contains(row, "┌") && strings.Contains(row, "You") {
						topBar = lastBarCol([]rune(row))
						break
					}
				}
				if topBar < 0 {
					t.Fatalf("[%s h=%d %s] missing You top border:\n%s", pk, h, label, strings.Join(rr, "\n"))
				}
				if topBar < chatW-2 {
					t.Fatalf("[%s h=%d %s] top bar %d want >=%d", pk, h, label, topBar, chatW-2)
				}
				for _, row := range rr {
					if strings.Contains(row, "intent:") || strings.Contains(row, "files:") || strings.Contains(row, "symbols: Subtract") || strings.Contains(row, "[copy]") || strings.Contains(row, "└") {
						bar := lastBarCol([]rune(row))
						if bar != topBar {
							t.Fatalf("[%s h=%d %s] bar %d != top %d row=%q", pk, h, label, bar, topBar, row)
						}
					}
				}
			}
			// Collapsed: first 4 lines + "...." tail, hidden symbols: line.
			rr := rows()
			prev := -1
			for _, need := range clamped {
				idx := -1
				for i, row := range rr {
					if strings.Contains(row, need) {
						idx = i
						break
					}
				}
				if idx < 0 {
					t.Fatalf("[%s h=%d] line %q missing in chat pane:\n%s", pk, h, need, strings.Join(rr, "\n"))
				}
				if idx < prev {
					t.Fatalf("[%s h=%d] %q out of order", pk, h, need)
				}
				prev = idx
			}
			if strings.Contains(stripANSI(m.View()), "symbols: Subtract") {
				t.Fatalf("[%s h=%d] collapsed box must hide symbols: line:\n%s", pk, h, stripANSI(m.View()))
			}
			if strings.Contains(stripANSI(m.View()), "error│") {
				t.Fatalf("[%s h=%d] intent broken at box edge:\n%s", pk, h, stripANSI(m.View()))
			}
			checkBars("collapsed", rr)
			for _, row := range rr {
				if strings.Contains(row, "symbols: Subtract") && strings.Contains(row, "[copy]") {
					t.Fatalf("[%s h=%d] [copy] must ride its own row:\n%s", pk, h, strings.Join(rr, "\n"))
				}
			}
			// Expand click → all 5 lines, no ellipsis.
			x, y, ok := findClickTarget(m, "user-prompt-expand:"+prompt)
			if !ok {
				t.Fatalf("[%s h=%d] collapsed box must be clickable:\n%s", pk, h, strings.Join(rr, "\n"))
			}
			m2, _ := m.Update(clickLeft(x, y))
			am := m2.(*AppModel)
			rr = rows()
			prev = -1
			for _, need := range expanded {
				idx := -1
				for i, row := range rr {
					if strings.Contains(row, need) {
						idx = i
						break
					}
				}
				if idx < 0 {
					t.Fatalf("[%s h=%d] expanded line %q missing:\n%s", pk, h, need, strings.Join(rr, "\n"))
				}
				if idx < prev {
					t.Fatalf("[%s h=%d] %q out of order (expanded)", pk, h, need)
				}
				prev = idx
			}
			if strings.Contains(stripANSI(am.View()), "calc_test.go....") {
				t.Fatalf("[%s h=%d] expanded box must not show ellipsis:\n%s", pk, h, stripANSI(am.View()))
			}
			checkBars("expanded", rr)
			// Second click collapses again.
			m3, _ := am.Update(clickLeft(x, y))
			view3 := stripANSI(m3.(*AppModel).View())
			if !strings.Contains(view3, "files: calc.go, calc_test.go....") {
				t.Fatalf("[%s h=%d] second click must collapse:\n%s", pk, h, view3)
			}
		}
	}
}