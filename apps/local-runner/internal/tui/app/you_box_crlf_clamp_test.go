package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/config"
)

// Tests for CRLF (\r\n) and bare CR (\r) prompt handling in the You-box:
// When prompts arrive with \r or \r\n (e.g. from NDJSON, clipboard paste, or Windows),
// wrapText normalizes them so the 4-line clamp, "...." ellipsis, click-to-expand,
// and copy chip work identically across Claude, Codex, and Grok.

func TestWrapText_NormalizesNewlines(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "bare LF",
			input:    "line1\nline2\nline3",
			expected: []string{"line1", "line2", "line3"},
		},
		{
			name:     "CRLF",
			input:    "line1\r\nline2\r\nline3",
			expected: []string{"line1", "line2", "line3"},
		},
		{
			name:     "bare CR",
			input:    "line1\rline2\rline3",
			expected: []string{"line1", "line2", "line3"},
		},
		{
			name:     "mixed LF CRLF CR",
			input:    "line1\r\nline2\rline3\nline4",
			expected: []string{"line1", "line2", "line3", "line4"},
		},
		{
			name:     "consecutive CR and CRLF empty lines",
			input:    "line1\r\rline2\r\n\r\nline3",
			expected: []string{"line1", "", "line2", "", "line3"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := wrapText(tc.input, 80)
			if len(got) != len(tc.expected) {
				t.Fatalf("expected %d lines, got %d (%v)", len(tc.expected), len(got), got)
			}
			for i, exp := range tc.expected {
				if got[i] != exp {
					t.Fatalf("line %d: expected %q, got %q", i, exp, got[i])
				}
			}
		})
	}
}

func TestYouBoxClamp_BareCRPromptCollapsedExpanded(t *testing.T) {
	// Repro from user: bare CR prompt with 5 lines
	bareCRPrompt := "[Change Contract]\rfeature: calc-core\rintent: them ham SubtractWithGuard vao calc.go tra ve error khi b > a\rfiles: calc.go, calc_test.go\rsymbols: Subtract"

	for _, w := range []int{197, 80} {
		for _, pk := range []string{"claude", "codex", "grok"} {
			m := New(config.ChatConfig{Provider: pk, Model: "m"}, "http://127.0.0.1:4317")
			m.width, m.height = w, 30
			if w >= 100 {
				m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
				m.sessionPanel.RunID = "run-208282"
				m.sessionPanel.ProjectPath = "/tmp/p"
				m.sessionPanel.Collapsed = false
			}
			m.addMessage("user", bareCRPrompt, "")
			m.addMessage("assistant", "ok", "")

			view := stripANSI(m.View())
			if !strings.Contains(view, "....") {
				t.Fatalf("[%s w=%d] bare CR prompt collapsed box must show '....' tail:\n%s", pk, w, view)
			}
			if !strings.Contains(view, "files: calc.go, calc_test.go....") {
				t.Fatalf("[%s w=%d] ellipsis must ride the 4th line:\n%s", pk, w, view)
			}
			if strings.Contains(view, "symbols: Subtract") {
				t.Fatalf("[%s w=%d] collapsed box must hide the 5th line:\n%s", pk, w, view)
			}

			// Ensure whole box is clickable with expand target
			x, y, ok := findClickTarget(m, "user-prompt-expand:"+bareCRPrompt)
			if !ok {
				t.Fatalf("[%s w=%d] collapsed box must expose expand target:\n%s", pk, w, view)
			}

			// Click to expand
			m2, _ := m.Update(clickLeft(x, y))
			view2 := stripANSI(m2.(*AppModel).View())
			if !strings.Contains(view2, "symbols: Subtract") {
				t.Fatalf("[%s w=%d] expanded box must show the 5th line:\n%s", pk, w, view2)
			}
			if strings.Contains(view2, "....") {
				t.Fatalf("[%s w=%d] expanded box must not show '....':\n%s", pk, w, view2)
			}

			// Verify ordering
			last := -1
			for _, need := range []string{"[Change Contract]", "feature: calc-core", "intent:", "files: calc.go, calc_test.go", "symbols: Subtract"} {
				idx := strings.Index(view2, need)
				if idx < 0 {
					t.Fatalf("[%s w=%d] expanded missing %q:\n%s", pk, w, need, view2)
				}
				if idx < last {
					t.Fatalf("[%s w=%d] %q out of order", pk, w, need)
				}
				last = idx
			}

			// Click to collapse again
			m3, _ := m2.(*AppModel).Update(clickLeft(x, y))
			view3 := stripANSI(m3.(*AppModel).View())
			if strings.Contains(view3, "symbols: Subtract") {
				t.Fatalf("[%s w=%d] second click must collapse:\n%s", pk, w, view3)
			}
			if !strings.Contains(view3, "calc_test.go....") {
				t.Fatalf("[%s w=%d] collapsed again must show '....':\n%s", pk, w, view3)
			}
		}
	}
}

func TestYouBoxClamp_CRLFPromptCollapsedExpanded(t *testing.T) {
	crlfPrompt := "[Change Contract]\r\nfeature: calc-core\r\nintent: them ham SubtractWithGuard vao calc.go tra ve error khi b > a\r\nfiles: calc.go, calc_test.go\r\nsymbols: Subtract"

	for _, pk := range []string{"claude", "codex", "grok"} {
		m := New(config.ChatConfig{Provider: pk, Model: "m"}, "http://127.0.0.1:4317")
		m.width, m.height = 80, 24
		m.addMessage("user", crlfPrompt, "")
		m.addMessage("assistant", "ok", "")

		view := stripANSI(m.View())
		if !strings.Contains(view, "....") {
			t.Fatalf("[%s] CRLF prompt collapsed box must show '....':\n%s", pk, view)
		}
		if strings.Contains(view, "symbols: Subtract") {
			t.Fatalf("[%s] collapsed box must hide 5th line:\n%s", pk, view)
		}

		x, y, ok := findClickTarget(m, "user-prompt-expand:"+crlfPrompt)
		if !ok {
			t.Fatalf("[%s] collapsed box must expose expand target", pk)
		}
		m2, _ := m.Update(clickLeft(x, y))
		view2 := stripANSI(m2.(*AppModel).View())
		if !strings.Contains(view2, "symbols: Subtract") {
			t.Fatalf("[%s] expanded box must show 5th line:\n%s", pk, view2)
		}
	}
}

func TestYouBoxClamp_MixedNewlinesPromptCollapsedExpanded(t *testing.T) {
	mixedPrompt := "[Change Contract]\rfeature: calc-core\r\nintent: them ham SubtractWithGuard vao calc.go tra ve error khi b > a\nfiles: calc.go, calc_test.go\rsymbols: Subtract"

	for _, pk := range []string{"claude", "codex", "grok"} {
		m := New(config.ChatConfig{Provider: pk, Model: "m"}, "http://127.0.0.1:4317")
		m.width, m.height = 80, 24
		m.addMessage("user", mixedPrompt, "")
		m.addMessage("assistant", "ok", "")

		view := stripANSI(m.View())
		if !strings.Contains(view, "....") {
			t.Fatalf("[%s] mixed newline prompt collapsed box must show '....':\n%s", pk, view)
		}
		if strings.Contains(view, "symbols: Subtract") {
			t.Fatalf("[%s] collapsed box must hide 5th line:\n%s", pk, view)
		}

		x, y, ok := findClickTarget(m, "user-prompt-expand:"+mixedPrompt)
		if !ok {
			t.Fatalf("[%s] collapsed box must expose expand target", pk)
		}
		m2, _ := m.Update(clickLeft(x, y))
		view2 := stripANSI(m2.(*AppModel).View())
		if !strings.Contains(view2, "symbols: Subtract") {
			t.Fatalf("[%s] expanded box must show 5th line:\n%s", pk, view2)
		}
	}
}

func TestYouBoxClamp_BareCRShortPromptNoClamp(t *testing.T) {
	shortBareCR := "[Change Contract]\rfeature: calc-core\rintent: simple fix\rsymbols: Subtract"

	for _, pk := range []string{"claude", "codex", "grok"} {
		m := New(config.ChatConfig{Provider: pk, Model: "m"}, "http://127.0.0.1:4317")
		m.width, m.height = 80, 24
		m.addMessage("user", shortBareCR, "")
		m.addMessage("assistant", "ok", "")

		view := stripANSI(m.View())
		if strings.Contains(view, "....") {
			t.Fatalf("%s: 4-line bare CR prompt must not show '....':\n%s", pk, view)
		}
		for _, need := range []string{"[Change Contract]", "feature: calc-core", "intent: simple fix", "symbols: Subtract"} {
			if !strings.Contains(view, need) {
				t.Fatalf("%s: short prompt missing %q:\n%s", pk, need, view)
			}
		}
		if _, _, ok := findClickTarget(m, "user-prompt-expand:"+shortBareCR); ok {
			t.Fatalf("%s: short bare CR prompt must not be expandable:\n%s", pk, view)
		}
	}
}

func TestYouBoxClamp_CRLFShortPromptNoClamp(t *testing.T) {
	shortCRLF := "[Change Contract]\r\nfeature: calc-core\r\nintent: simple fix\r\nsymbols: Subtract"

	for _, pk := range []string{"claude", "codex", "grok"} {
		m := New(config.ChatConfig{Provider: pk, Model: "m"}, "http://127.0.0.1:4317")
		m.width, m.height = 80, 24
		m.addMessage("user", shortCRLF, "")
		m.addMessage("assistant", "ok", "")

		view := stripANSI(m.View())
		if strings.Contains(view, "....") {
			t.Fatalf("%s: 4-line CRLF prompt must not show '....':\n%s", pk, view)
		}
		for _, need := range []string{"[Change Contract]", "feature: calc-core", "intent: simple fix", "symbols: Subtract"} {
			if !strings.Contains(view, need) {
				t.Fatalf("%s: short prompt missing %q:\n%s", pk, need, view)
			}
		}
		if _, _, ok := findClickTarget(m, "user-prompt-expand:"+shortCRLF); ok {
			t.Fatalf("%s: short CRLF prompt must not be expandable:\n%s", pk, view)
		}
	}
}

func TestYouBoxClamp_VietnameseWithCRLF(t *testing.T) {
	vnPrompt := "Dòng một: kiểm tra tiếng Việt\r\nDòng hai: tính năng SubtractWithGuard\r\nDòng ba: trả về lỗi khi b > a\r\nDòng bốn: kiểm tra file calc_test.go\r\nDòng năm: dòng đuôi cần được ẩn"

	for _, pk := range []string{"claude", "codex", "grok"} {
		m := New(config.ChatConfig{Provider: pk, Model: "m"}, "http://127.0.0.1:4317")
		m.width, m.height = 80, 24
		m.addMessage("user", vnPrompt, "")
		m.addMessage("assistant", "ok", "")

		view := stripANSI(m.View())
		if !strings.Contains(view, "....") {
			t.Fatalf("%s: VN prompt must clamp with '....':\n%s", pk, view)
		}
		if strings.Contains(view, "Dòng năm") {
			t.Fatalf("%s: VN prompt must hide 5th line when collapsed:\n%s", pk, view)
		}

		x, y, ok := findClickTarget(m, "user-prompt-expand:"+vnPrompt)
		if !ok {
			t.Fatalf("%s: VN prompt box must be clickable", pk)
		}
		m2, _ := m.Update(clickLeft(x, y))
		view2 := stripANSI(m2.(*AppModel).View())
		if !strings.Contains(view2, "Dòng năm") {
			t.Fatalf("%s: expanded VN prompt must show 5th line:\n%s", pk, view2)
		}
	}
}

func TestYouBoxClamp_BareCRCopyChip(t *testing.T) {
	bareCRPrompt := "[Change Contract]\rfeature: calc-core\rintent: them ham SubtractWithGuard vao calc.go tra ve error khi b > a\rfiles: calc.go, calc_test.go\rsymbols: Subtract"

	for _, pk := range []string{"claude", "codex", "grok"} {
		m := New(config.ChatConfig{Provider: pk, Model: "m"}, "http://127.0.0.1:4317")
		m.width, m.height = 80, 24
		m.addMessage("user", bareCRPrompt, "")
		m.addMessage("assistant", "ok", "")

		x, y, target, ok := findAnyTarget(m, "copy:")
		if !ok {
			t.Fatalf("%s: clamped box must expose copy chip", pk)
		}
		if !strings.HasPrefix(target, "copy:") {
			t.Fatalf("%s: expected copy target, got %q", pk, target)
		}
		m2, _ := m.Update(clickLeft(x, y))
		view := stripANSI(m2.(*AppModel).View())
		if strings.Contains(view, "symbols: Subtract") {
			t.Fatalf("%s: copy click must not expand prompt:\n%s", pk, view)
		}
		if !strings.Contains(view, "....") {
			t.Fatalf("%s: copy click must keep collapsed tail:\n%s", pk, view)
		}
	}
}
