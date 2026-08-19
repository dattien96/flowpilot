package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/config"
)

func promptModel(pk string) *AppModel {
	m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.asciiMode = true
	return m
}

func longPrompt() string {
	return strings.Join([]string{
		"prompt line one",
		"prompt line two",
		"prompt line three",
		"prompt line four",
		"prompt line five",
		"prompt line six TAIL",
	}, "\n")
}

func findAnyTarget(m *AppModel, prefix string) (x, y int, target string, ok bool) {
	w, h := m.width, m.height
	for yy := 0; yy < h; yy++ {
		for xx := 0; xx < w; xx++ {
			t := m.clickTargetAt(xx, yy)
			if strings.HasPrefix(t, prefix) {
				return xx, yy, t, true
			}
		}
	}
	return 0, 0, "", false
}

// TestUserPrompt_TruncatedToFourLines: a user prompt that wraps to more than 4
// lines renders a 4-line box with a "...." tail by default; the full tail line is
// hidden until the bubble is expanded.
func TestUserPrompt_TruncatedToFourLines(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := promptModel(pk)
			prompt := longPrompt()
			m.addMessage("user", prompt, "")
			m.addMessage("assistant", "ok", "")
			view := stripANSI(m.View())
			if !strings.Contains(view, "....") {
				t.Fatalf("%s: truncated prompt must show '....':\n%s", pk, view)
			}
			if strings.Contains(view, "TAIL") {
				t.Fatalf("%s: collapsed prompt must hide the tail line:\n%s", pk, view)
			}
		})
	}
}

// TestUserPrompt_ExpandCollapseClick: clicking any row of a truncated user box
// expands the full prompt; a second click collapses it back to the clamp.
func TestUserPrompt_ExpandCollapseClick(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := promptModel(pk)
			prompt := longPrompt()
			m.addMessage("user", prompt, "")
			m.addMessage("assistant", "ok", "")
			x, y, ok := findClickTarget(m, "user-prompt-expand:"+prompt)
			if !ok {
				t.Fatalf("%s: truncated user box must be clickable", pk)
			}
			m2, _ := m.Update(clickLeft(x, y))
			am := m2.(*AppModel)
			view := stripANSI(am.View())
			if !strings.Contains(view, "TAIL") {
				t.Fatalf("%s: expanded prompt must show the tail line:\n%s", pk, view)
			}
			if strings.Contains(view, "....") {
				t.Fatalf("%s: expanded prompt must not show '....':\n%s", pk, view)
			}
			m3, _ := am.Update(clickLeft(x, y))
			view = stripANSI(m3.(*AppModel).View())
			if strings.Contains(view, "TAIL") {
				t.Fatalf("%s: second click must collapse the prompt:\n%s", pk, view)
			}
			if !strings.Contains(view, "....") {
				t.Fatalf("%s: collapsed prompt must show '....' again:\n%s", pk, view)
			}
		})
	}
}

// TestUserPrompt_CopyWinsOverExpand: clicking the [copy] chip on the last boxed
// row returns the copy target (hit-tested before the expand target) and does not
// toggle the expansion.
func TestUserPrompt_CopyWinsOverExpand(t *testing.T) {
	m := promptModel("claude")
	prompt := longPrompt()
	m.addMessage("user", prompt, "")
	m.addMessage("assistant", "ok", "")
	x, y, target, ok := findAnyTarget(m, "copy:")
	if !ok {
		t.Fatal("truncated user box must still expose a copy chip")
	}
	if !strings.HasPrefix(target, "copy:") {
		t.Fatalf("expected copy target, got %q", target)
	}
	m2, _ := m.Update(clickLeft(x, y))
	view := stripANSI(m2.(*AppModel).View())
	if !strings.Contains(view, "....") {
		t.Fatalf("copy click must not expand the prompt:\n%s", view)
	}
	if strings.Contains(view, "TAIL") {
		t.Fatalf("copy click must not expand the prompt (tail visible):\n%s", view)
	}
}

// TestUserPrompt_ShortPromptUntouched: a prompt that fits within 4 lines renders
// in full with no "...." tail and no clickable expand target.
func TestUserPrompt_ShortPromptUntouched(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := promptModel(pk)
			short := "short prompt"
			m.addMessage("user", short, "")
			m.addMessage("assistant", "ok", "")
			view := stripANSI(m.View())
			if strings.Contains(view, "....") {
				t.Fatalf("%s: short prompt must not show '....':\n%s", pk, view)
			}
			if !strings.Contains(view, short) {
				t.Fatalf("%s: short prompt must render in full:\n%s", pk, view)
			}
			if _, _, ok := findClickTarget(m, "user-prompt-expand:"+short); ok {
				t.Fatalf("%s: short prompt must not be expandable:\n%s", pk, view)
			}
		})
	}
}
