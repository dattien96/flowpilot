package app

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"flowpilot-runner/internal/tui/config"
)

// CA-531 — code fences render as a solid card: the full block (top stroke, body,
// bottom stroke, border glyphs) shares colorCodeBg so the panel reads as a lifted
// gray card instead of a white-outlined box over the dark terminal.

// forceTrueColor pins the lipgloss default renderer so styled output carries
// truecolor sequences in the test environment (lipgloss otherwise auto-detects
// Ascii under a non-TTY and strips colors).
func forceTrueColor(t *testing.T) {
	t.Helper()
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(prev) })
}

// bgANSI returns the truecolor background escape marker lipgloss emits for hex.
func bgANSI(hex string) string {
	r, _ := strconv.ParseInt(hex[1:3], 16, 64)
	g, _ := strconv.ParseInt(hex[3:5], 16, 64)
	b, _ := strconv.ParseInt(hex[5:7], 16, 64)
	return fmt.Sprintf("48;2;%d;%d;%d", r, g, b)
}

func TestCodeBoxSolidFill_AllRowsOnCodeBg(t *testing.T) {
	forceTrueColor(t)
	// Distinct src so the md cache (keyed by width+hash) cannot serve a row
	// rendered under a different color profile.
	src := "Intro\n\n```go\nfunc SolidAdd(a, b int) int {\n\treturn a + b\n}\n```\n"
	lines := renderMarkdown(src, 48)
	bg := string(styleMdCodeBox.GetBackground().(lipgloss.Color))
	bgSeq := bgANSI(bg)
	got := 0
	for _, line := range lines {
		row := stripANSI(line)
		if !strings.Contains(row, "SolidAdd") {
			continue
		}
		// Body rows must carry the code-bg ANSI.
		if !strings.Contains(line, bgSeq) {
			t.Fatalf("code body row missing bg %s: %q", bg, line)
		}
		got++
	}
	if got == 0 {
		t.Fatalf("no code body rows found:\n%s", strings.Join(lines, "\n"))
	}
}

func TestCodeBoxSolidFill_HeaderAndFooterOnCodeBg(t *testing.T) {
	forceTrueColor(t)
	src := "Intro\n\n```go\nfunc SolidHead(a, b int) int {\n\treturn a + b\n}\n```\n"
	lines := renderMarkdown(src, 49)
	bg := string(styleMdCodeBox.GetBackground().(lipgloss.Color))
	bgSeq := bgANSI(bg)
	top, bot := -1, -1
	for i, line := range lines {
		plain := stripANSI(line)
		if strings.Contains(plain, "go") && (strings.Contains(plain, "┌") || strings.Contains(plain, "+")) {
			top = i
		}
		if strings.HasPrefix(strings.TrimSpace(plain), "└") || strings.HasPrefix(strings.TrimSpace(plain), "+") {
			bot = i
		}
	}
	if top < 0 || bot <= top {
		t.Fatalf("missing header/footer:\n%s", strings.Join(lines, "\n"))
	}
	if !strings.Contains(lines[top], bgSeq) {
		t.Fatalf("code header missing bg %s: %q", bg, lines[top])
	}
	if !strings.Contains(lines[bot], bgSeq) {
		t.Fatalf("code footer missing bg %s: %q", bg, lines[bot])
	}
}

func TestCodeBoxSolidFill_BorderGlyphsDimmed(t *testing.T) {
	bg := string(styleMdCodeBox.GetBackground().(lipgloss.Color))
	if barBg := string(styleMdCodeBar.GetBackground().(lipgloss.Color)); barBg != bg {
		t.Fatalf("border bar bg=%q want %q (solid card)", barBg, bg)
	}
	if barFg := string(styleMdCodeBar.GetForeground().(lipgloss.Color)); barFg == colorText {
		t.Fatalf("border glyphs must be dimmer than body text, got %q", barFg)
	}
	if colorCodeBg == colorBg3 {
		t.Fatalf("code bg must stay distinct from --bg-3")
	}
}

func TestCodeBoxSolidFill_RendersAcrossProviders(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.width = 60
			m.addMessage("assistant", "Intro\n\n```go\nfunc Add(a, b int) int {\n\treturn a + b\n}\n```\n", "")
			joined := stripANSI(strings.Join(m.renderMessages(), "\n"))
			if !strings.Contains(joined, "go") || !strings.Contains(joined, "func Add") {
				t.Fatalf("%s: missing labeled code box:\n%s", pk, joined)
			}
			if strings.Contains(joined, "```") {
				t.Fatalf("%s: raw fence leaked:\n%s", pk, joined)
			}
		})
	}
}
