package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/config"
)

// CA-601: boxed user prompts render line by line — each original prompt line
// gets its own box row (wrapped only when it exceeds the box inner width) and
// the [copy] chip rides the last content row, flush before the right border.
func TestRegression_YouBoxRendersPromptLinesPlainly(t *testing.T) {
	prompt := "[Change Contract]\nfeature: calc-core\nintent: them ham SubtractWithGuard vao calc.go tra ve error khi b > a\nsymbols: Subtract"
	lines := strings.Split(prompt, "\n")
	cases := []struct {
		w      int
		sideOn bool
	}{
		{197, true},
		{160, true},
		{80, false},
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
			view := stripANSI(m.View())
			chatW := m.chatWidth()
			// Every original line must appear, and the four header/intent lines
			// must sit at the start of their own box row (no mid-line wrap on a
			// wide pane).
			for _, ln := range lines {
				if !strings.Contains(view, ln) {
					t.Fatalf("[%s w=%d] missing prompt line %q", pk, tc.w, ln)
				}
			}
			for _, row := range strings.Split(view, "\n") {
				plain := row
				if len([]rune(plain)) > chatW {
					plain = string([]rune(plain)[:chatW])
				}
				if strings.Contains(plain, "feature: calc-core") || strings.Contains(plain, "symbols: Subtract") {
					if !strings.HasPrefix(plain, "│") {
						t.Fatalf("[%s w=%d] line not at box row start: %q", pk, tc.w, plain)
					}
				}
				// The [copy] chip must share the last content row, not a line of
				// its own, and sit before the right border.
				if strings.Contains(plain, "[copy]") && strings.Contains(plain, "symbols: Subtract") {
					if !strings.HasSuffix(strings.TrimRight(plain, " "), "│") {
						t.Fatalf("[%s w=%d] copy row missing right border: %q", pk, tc.w, plain)
					}
				}
			}
			// Wrap boundary: the intent line must never break mid-word at the
			// box border (tra│ / │ve error) even when it wraps (narrow pane).
			if strings.Contains(view, "tra│") {
				t.Fatalf("[%s w=%d] mid-word break tra│", pk, tc.w)
			}
			if !strings.Contains(view, "tra ve error") {
				t.Fatalf("[%s w=%d] intent words lost: %q", pk, tc.w, view)
			}
		}
	}
}

// CA-604: the [copy] chip rides its own row (after the content rows, before the
// bottom border) — never on a content row, so prompt text is never cut for it.
// User request: [copy] at end of prompt removed, so this test now verifies no [copy].
func TestRegression_YouBoxCopyChipOnLastRow(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Model: "m"}, "http://127.0.0.1:4317")
	m.width, m.height = 120, 24
	m.addMessage("user", "intent: them ham SubtractWithGuard vao calc.go tra ve error khi b > a", "")
	view := stripANSI(strings.Join(m.renderMessages(), "\n"))
	if strings.Contains(view, "[copy]") {
		t.Fatalf("[copy] should be removed (user request), found in:\n%s", view)
	}
	rows := strings.Split(view, "\n")
	if len(rows) == 0 || !strings.Contains(rows[len(rows)-1], "└") {
		t.Fatalf("bottom border must be last row:\n%s", view)
	}
}

// CA-603/CA-605 → CA-607: the exact prompt the operator pasted (5 lines) clamps
// to 4 lines + "...." collapsed on the REAL View() path — [copy] on its own row,
// hidden tail line, right borders aligned — with and without the F2 sidebar, at
// any viewport height. After an expand click all 5 lines render in order.
func TestRegression_YouBoxUserPastedPromptAllLines(t *testing.T) {
	prompt := "[Change Contract]\nfeature: calc-core\nintent: them ham SubtractWithGuard vao calc.go tra ve error khi b > a\nfiles: calc.go, calc_test.go\nsymbols: Subtract"
	clampedOrder := []string{
		"[Change Contract]",
		"feature: calc-core",
		"intent: them ham SubtractWithGuard vao calc.go tra ve error khi b > a",
		"files: calc.go, calc_test.go....",
	}
	expandedOrder := []string{
		"[Change Contract]",
		"feature: calc-core",
		"intent: them ham SubtractWithGuard vao calc.go tra ve error khi b > a",
		"files: calc.go, calc_test.go",
		"symbols: Subtract",
	}
	for _, w := range []int{197, 80} {
		for _, h := range []int{24, 30} {
			for _, pk := range []string{"claude", "codex", "grok"} {
				m := New(config.ChatConfig{Provider: pk, Model: "m"}, "http://127.0.0.1:4317")
				m.width, m.height = w, h
				if w >= 100 {
					m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
					m.sessionPanel.RunID = "run-208282"
					m.sessionPanel.ProjectPath = "/tmp/p"
					enableSidebarForTest(m)
				}
				m.addMessage("user", prompt, "")
				checkOrder := func(label string, order []string, view string) {
					t.Helper()
					last := -1
					for _, need := range order {
						idx := strings.Index(view, need)
						if idx < 0 {
							t.Fatalf("[%s w=%d h=%d %s] missing %q:\n%s", pk, w, h, label, need, view)
						}
						if idx < last {
							t.Fatalf("[%s w=%d h=%d %s] lines out of order for %q", pk, w, h, label, need)
						}
						last = idx
					}
				}
				// Collapsed: 4 lines + "...." tail, symbols: hidden.
				view := stripANSI(m.View())
				checkOrder("collapsed", clampedOrder, view)
				if strings.Contains(view, "symbols: Subtract") {
					t.Fatalf("[%s w=%d h=%d] collapsed box must hide symbols line:\n%s", pk, w, h, view)
				}
				// The intent line must never break mid-word at the box edge.
				if strings.Contains(view, "error│") {
					t.Fatalf("[%s w=%d h=%d] intent broken at box edge:\n%s", pk, w, h, view)
				}
				chipOnSymbols := false
				for _, row := range strings.Split(view, "\n") {
					if strings.Contains(row, "symbols: Subtract") && strings.Contains(row, "[copy]") {
						chipOnSymbols = true
					}
				}
				if chipOnSymbols {
					t.Fatalf("[%s w=%d h=%d] [copy] must NOT ride the symbols line (own row, CA-604):\n%s", pk, w, h, view)
				}
				// Expand: all 5 lines.
				x, y, ok := findClickTarget(m, "user-prompt-expand:"+prompt)
				if !ok {
					t.Fatalf("[%s w=%d h=%d] collapsed box must be clickable:\n%s", pk, w, h, view)
				}
				m2, _ := m.Update(clickLeft(x, y))
				view2 := stripANSI(m2.(*AppModel).View())
				checkOrder("expanded", expandedOrder, view2)
				if strings.Contains(view2, "....") {
					t.Fatalf("[%s w=%d h=%d] expanded box must not show ellipsis:\n%s", pk, w, h, view2)
				}
				// Collapse again.
				m3, _ := m2.(*AppModel).Update(clickLeft(x, y))
				view3 := stripANSI(m3.(*AppModel).View())
				if !strings.Contains(view3, "calc_test.go....") {
					t.Fatalf("[%s w=%d h=%d] second click must collapse:\n%s", pk, w, h, view3)
				}
			}
		}
	}
}