package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-484: empty pending attach must not show a permanent [+img] chip.
func TestInputAttachChip_HiddenWhenEmpty(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:9")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.inputValue = "hello"
	if m.inputAttachChipPlain() != "" {
		t.Fatalf("empty pending: chip=%q", m.inputAttachChipPlain())
	}
	line := stripANSI(m.renderInputLine())
	if strings.Contains(line, "[+img]") || strings.Contains(line, " img]") {
		t.Fatalf("empty pending must not paint img chip:\n%s", line)
	}
}

func TestInputAttachChip_ShowsCountWhenPending(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:9")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.pendingAttach = []client.PromptAttachment{
		{ID: "1", OriginalName: "a.png", MimeType: "image/png"},
		{ID: "2", OriginalName: "b.png", MimeType: "image/png"},
	}
	if got := m.inputAttachChipPlain(); got != "[2 img] " {
		t.Fatalf("chip=%q", got)
	}
	line := stripANSI(m.renderInputLine())
	if !strings.Contains(line, "[2 img]") {
		t.Fatalf("pending count missing:\n%s", line)
	}
	if strings.Contains(line, "[+img]") {
		t.Fatalf("must not use empty-state [+img] when pending:\n%s", line)
	}
}
