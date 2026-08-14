package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"flowpilot-runner/internal/tui/config"
)

func TestRenderInputLine_FitsNarrowTerminal(t *testing.T) {
	for _, w := range []int{16, 20, 24, 28, 32, 40} {
		m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.6"}, "http://127.0.0.1:4317")
		m.width = w
		m.height = 40
		m.asciiMode = false
		m.sessionLoading = false
		m.sessionDefaultsLoaded = true
		m.model = "grok-4.6"
		m.inputValue = "jj"
		m.cursorOn = true
		got := m.renderInputLine()
		for i, line := range strings.Split(got, "\n") {
			if vw := lipgloss.Width(line); vw >= w && w > 1 {
				t.Errorf("width=%d line %d visual=%d (must keep last col free):\nplain=%q", w, i, vw, stripANSI(line))
			}
		}
	}
}

func TestView_NarrowWidthDoesNotOverflowInputRows(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.6"}, "http://127.0.0.1:4317")
	m.width = 28
	m.height = 24
	m.asciiMode = false
	m.sessionLoading = false
	m.sessionDefaultsLoaded = true
	m.accountLabel = "trashname899@gmail.com"
	m.inputValue = "jj"
	m.cursorOn = true
	view := m.View()
	var frameHits int
	for i, line := range strings.Split(view, "\n") {
		plain := stripANSI(line)
		vw := lipgloss.Width(line)
		isInput := strings.Contains(plain, "╭") || strings.Contains(plain, "╰") ||
			strings.Contains(plain, " img]") || strings.Contains(plain, " chat ")
		if isInput && vw >= m.width && m.width > 1 {
			t.Errorf("input line %d visual=%d >= %d\nplain=%q", i, vw, m.width, plain)
		}
		if strings.Contains(plain, "╭") || strings.Contains(plain, "╰") {
			frameHits++
		}
		// Empty pending attach must not paint a permanent [+img] affordance.
		if strings.Contains(plain, "[+img]") {
			t.Fatalf("unexpected empty-state [+img] chip:\n%s", stripANSI(view))
		}
	}
	if frameHits < 2 {
		t.Fatalf("expected input frame corners, got frameHits=%d\n%s", frameHits, stripANSI(view))
	}
}

func TestBuildChatRows_DoesNotAssume80WhenNarrow(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width = 16
	m.asciiMode = true
	m.messages = []ChatMessage{{Role: "assistant", Content: strings.Repeat("abcd ", 20)}}
	for i, line := range m.renderMessages() {
		if vw := lipgloss.Width(line); vw > m.width {
			t.Fatalf("line %d visual=%d > %d: %q", i, vw, m.width, stripANSI(line))
		}
	}
}
