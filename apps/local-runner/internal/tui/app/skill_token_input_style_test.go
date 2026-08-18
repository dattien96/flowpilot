package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestStyleInputBodyWithSkillTokens_HighlightsAttachedOnly(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	sel := []client.SkillSelection{{Name: "coding"}, {Name: "review"}}
	got := styleInputBodyWithSkillTokens("abc [coding] and [review] def [other]", sel)
	plain := stripANSI(got)
	if plain != "abc [coding] and [review] def [other]" {
		t.Fatalf("plain=%q", plain)
	}
	if got == plain {
		t.Fatal("expected ANSI on skill tokens")
	}
	// [other] is not attached — should not get its own hi span alone if coding/review do.
	// Coding token must appear as a styled segment.
	if !strings.Contains(got, styleMention.Render("[coding]")) {
		t.Fatalf("missing hi [coding] in %q", got)
	}
	if !strings.Contains(got, styleMention.Render("[review]")) {
		t.Fatalf("missing hi [review]")
	}
	// Unattached [other] is rendered with input focus, not mention green.
	if strings.Contains(got, styleMention.Render("[other]")) {
		t.Fatal("[other] must not be highlighted")
	}
}

func TestRenderInputLine_SkillTokensHighlighted(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.5"}, "http://127.0.0.1:4317")
	m.width = 80
	m.selectedSkills = []client.SkillSelection{{Name: "coding"}}
	m.inputValue = "abc [coding] def"
	m.inputCursor = -1
	line := m.renderInputLine()
	plain := stripANSI(line)
	if !strings.Contains(plain, "abc [coding] def") {
		t.Fatalf("plain missing draft:\n%s", plain)
	}
	if !strings.Contains(line, styleMention.Render("[coding]")) {
		t.Fatalf("input should highlight [coding]:\n%q", line)
	}
}
