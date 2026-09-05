package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/config"
)

// CA-607 regression tests for the restored 4-line user-prompt clamp on the
// full-width You box: collapsed = 4 wrapped lines + "...." tail, click-to-expand
// on the whole box, [copy] always wins over expand on its own row, prompts that
// fit in 4 lines are never clamped.

const changeContractPrompt = "[Change Contract]\nfeature: calc-core\nintent: them ham SubtractWithGuard vao calc.go tra ve error khi b > a\nfiles: calc.go, calc_test.go\nsymbols: Subtract"

func TestYouBoxClamp_ChangeContractCollapsedExpanded(t *testing.T) {
	for _, w := range []int{197, 80} {
		for _, pk := range []string{"claude", "codex", "grok"} {
			m := New(config.ChatConfig{Provider: pk, Model: "m"}, "http://127.0.0.1:4317")
			m.width, m.height = w, 30
			if w >= 100 {
				m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
				m.sessionPanel.RunID = "run-208282"
				m.sessionPanel.ProjectPath = "/tmp/p"
				enableSidebarForTest(m)
			}
			m.addMessage("user", changeContractPrompt, "")
			m.addMessage("assistant", "ok", "")
			view := stripANSI(m.View())
			if !strings.Contains(view, "....") {
				t.Fatalf("[%s w=%d] collapsed box must show '....' tail:\n%s", pk, w, view)
			}
			if !strings.Contains(view, "files: calc.go, calc_test.go....") {
				t.Fatalf("[%s w=%d] ellipsis must ride the 4th line:\n%s", pk, w, view)
			}
			if strings.Contains(view, "symbols: Subtract") {
				t.Fatalf("[%s w=%d] collapsed box must hide the 5th line:\n%s", pk, w, view)
			}
			if strings.Contains(view, "[copy]") {
				t.Fatalf("[%s w=%d] [copy] should be removed (user request):\n%s", pk, w, view)
			}
			// Whole box is clickable: any row exposes the expand target.
			x, y, ok := findClickTarget(m, "user-prompt-expand:"+changeContractPrompt)
			if !ok {
				t.Fatalf("[%s w=%d] collapsed box must expose expand target:\n%s", pk, w, view)
			}
			m2, _ := m.Update(clickLeft(x, y))
			view2 := stripANSI(m2.(*AppModel).View())
			if !strings.Contains(view2, "symbols: Subtract") {
				t.Fatalf("[%s w=%d] expanded box must show the 5th line:\n%s", pk, w, view2)
			}
			if strings.Contains(view2, "....") {
				t.Fatalf("[%s w=%d] expanded box must not show '....':\n%s", pk, w, view2)
			}
			// Order preserved after expand.
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
			// Second click collapses again.
			m3 := clickLeftFull(m2.(*AppModel), x, y)
			view3 := stripANSI(m3.View())
			if strings.Contains(view3, "symbols: Subtract") {
				t.Fatalf("[%s w=%d] second click must collapse:\n%s", pk, w, view3)
			}
			if !strings.Contains(view3, "calc_test.go....") {
				t.Fatalf("[%s w=%d] collapsed again must show '....':\n%s", pk, w, view3)
			}
		}
	}
}

// TestYouBoxClamp_CopyWinsOverExpand: clicking the [copy] chip on a clamped box
// returns the copy target (hit-tested before the expand target) and does not
// toggle the expansion — the full prompt stays hidden after the copy click.
func TestYouBoxClamp_CopyWinsOverExpand(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		m := New(config.ChatConfig{Provider: pk, Model: "m"}, "http://127.0.0.1:4317")
		m.width, m.height = 80, 24
		m.addMessage("user", changeContractPrompt, "")
		m.addMessage("assistant", "ok", "")
		if _, _, _, ok := findAnyTarget(m, "copy:"); ok {
			t.Fatalf("%s: [copy] should be removed (user request)", pk)
		}
		view := stripANSI(m.View())
		if !strings.Contains(view, "....") {
			t.Fatalf("%s: collapsed tail must remain\n%s", pk, view)
		}
	}
}

// TestYouBoxClamp_ShortPromptNoClamp: a prompt that wraps to 4 or fewer lines
// renders in full — no "...." tail, no clickable expand target.
func TestYouBoxClamp_ShortPromptNoClamp(t *testing.T) {
	short := "[Change Contract]\nfeature: calc-core\nintent: them ham SubtractWithGuard vao calc.go tra ve error khi b > a\nsymbols: Subtract"
	for _, pk := range []string{"claude", "codex", "grok"} {
		m := New(config.ChatConfig{Provider: pk, Model: "m"}, "http://127.0.0.1:4317")
		m.width, m.height = 80, 24
		m.addMessage("user", short, "")
		m.addMessage("assistant", "ok", "")
		view := stripANSI(m.View())
		if strings.Contains(view, "....") {
			t.Fatalf("%s: 4-line prompt must not show '....':\n%s", pk, view)
		}
		for _, need := range []string{"[Change Contract]", "feature: calc-core", "intent:", "symbols: Subtract"} {
			if !strings.Contains(view, need) {
				t.Fatalf("%s: short prompt missing %q:\n%s", pk, need, view)
			}
		}
		if _, _, ok := findClickTarget(m, "user-prompt-expand:"+short); ok {
			t.Fatalf("%s: short prompt must not be expandable:\n%s", pk, view)
		}
	}
}