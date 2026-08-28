package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestBug328_ImagePanel_UpDownSelect(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.authPhase = AuthNone
	m.pendingAttach = []client.PromptAttachment{
		{ID: "a1", OriginalName: "one.png"},
		{ID: "a2", OriginalName: "two.png"},
	}
	m.openAttachPanel()
	if m.attachPanelSel != 0 {
		t.Fatalf("panel must start at row 0, got %d", m.attachPanelSel)
	}
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	if m2.(*AppModel).attachPanelSel != 1 {
		t.Fatalf("Down must select row 1, got %d", m2.(*AppModel).attachPanelSel)
	}
}

func TestBug328_ImagePanel_DoesNotStealHistoryWhenClosed(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.authPhase = AuthNone
	m.promptHistory = []string{"hello"}
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyUp})
	am := m2.(*AppModel)
	if am.promptHistIdx < 0 {
		t.Fatal("Up with closed panel must still recall prompt history")
	}
}

func TestBug328_ImageCommandOpensPanel(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.pendingAttach = []client.PromptAttachment{{ID: "a1", OriginalName: "x.png"}}
	m2, _ := m.dispatchImageCommand(nil)
	am := m2.(*AppModel)
	if !am.attachPanelOpen {
		t.Fatal("/image with pending attachments must open the manage panel")
	}
}
