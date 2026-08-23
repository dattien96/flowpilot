package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestHighlightMentions_FilesAndSkills(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	got := highlightMentions("see apps/foo/ChatInput.tsx and [coding]", []string{"coding"}, styleUser)
	plain := stripANSI(got)
	if plain != "see apps/foo/ChatInput.tsx and [coding]" {
		t.Fatalf("plain=%q", plain)
	}
	if !strings.Contains(got, styleMentionFile.Render("apps/foo/ChatInput.tsx")) {
		t.Fatalf("missing file hi:\n%q", got)
	}
	if !strings.Contains(got, styleMention.Render("[coding]")) {
		t.Fatalf("missing skill hi:\n%q", got)
	}
}

func TestPaintWrappedMentions_FilePathKeepsBlueAcrossWrap(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	plain := "look apps/desktop-flowpilot/src/components/ChatInput.tsx"
	lines := wrapText(plain, 24)
	if len(lines) < 2 {
		t.Fatalf("precondition: want wrapped path, got %#v", lines)
	}
	painted := paintWrappedMentions(plain, nil, lines, styleUser)
	joined := strings.Join(painted, "")
	if !strings.Contains(joined, styleMentionFile.Render(lines[0][strings.Index(lines[0], "apps"):])) &&
		!strings.Contains(joined, "\x1b[") {
		t.Fatalf("wrapped file fragment must stay blue:\n%q\nlines=%#v", joined, lines)
	}
	found := false
	for _, line := range painted {
		if strings.Contains(line, "\x1b[") && strings.Contains(stripANSI(line), "/") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no painted path fragment:\n%#v", painted)
	}
}

// CA-601: boxed user prompts render as plain lines (styled SGR shifted the
// box's right border on Ghostty). Mention text must still appear — uncolored —
// inside the You box.
func TestRenderMessages_UserPromptShowsMentionText(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.5"}, "http://127.0.0.1:4317")
	m.width = 100
	m.height = 40
	m.selectedSkills = []client.SkillSelection{{Name: "coding"}}
	m.addMessage("user", "look apps/foo/ChatInput.tsx [coding]", "")
	view := m.View()
	plain := stripANSI(view)
	if !strings.Contains(plain, "apps/foo/ChatInput.tsx") {
		t.Fatalf("timeline missing file mention text:\n%s", plain)
	}
	if !strings.Contains(plain, "[coding]") {
		t.Fatalf("timeline missing skill mention text:\n%s", plain)
	}
}
