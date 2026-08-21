package app

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"flowpilot-runner/internal/tui/config"
)

func TestRegression_YouBoxLeftAlignedWhenSidebarOn(t *testing.T) {
	prompt := "intent: them ham\nfiles:\nsymbols: Subtract"
	for _, pk := range []string{"claude", "codex", "grok"} {
		m := New(config.ChatConfig{Provider: pk, Model: "m"}, "http://127.0.0.1:4317")
		m.width, m.height = 197, 30
		m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
		m.sessionPanel.RunID = "run-208282"
		m.sessionPanel.ProjectPath = "/tmp/p"
		m.sessionPanel.Collapsed = false
		m.addMessage("user", prompt, "")
		if !m.useRightSidebar() {
			t.Fatal("need sidebar")
		}
		chatW := m.chatWidth()
		var sawYou bool
		for _, line := range strings.Split(m.View(), "\n") {
			plain := stripANSI(line)
			left := plain
			if lipgloss.Width(plain) > chatW {
				left = truncateVisual(plain, chatW)
			}
			if strings.Contains(left, "Runner:") {
				t.Fatalf("[%s] chat pane leaked Runner: %q", pk, left)
			}
			trim := strings.TrimLeft(left, " ")
			if strings.HasPrefix(trim, "┌") && strings.Contains(trim, "You") {
				sawYou = true
				lead := lipgloss.Width(left) - lipgloss.Width(strings.TrimLeft(left, " "))
				if lead > 8 {
					t.Fatalf("[%s] You box still right-aligned (lead=%d): %q", pk, lead, left)
				}
			}
		}
		if !sawYou {
			t.Fatal("missing You box in chat pane")
		}
	}
}

func TestRegression_YouBoxRightAlignedWhenSidebarOff(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Model: "m"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.addMessage("user", "hello right", "")
	joined := strings.Join(m.renderMessages(), "\n")
	plain := stripANSI(joined)
	if !strings.Contains(plain, "You") {
		t.Fatal("missing You")
	}
	foundLead := false
	for _, line := range strings.Split(plain, "\n") {
		if strings.Contains(line, "┌") && strings.Contains(line, "You") {
			if strings.HasPrefix(line, " ") {
				foundLead = true
			}
		}
	}
	if !foundLead {
		t.Fatalf("sidebar off must keep right-align:\n%s", plain)
	}
}
