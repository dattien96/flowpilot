package app

import (
	"testing"

	"flowpilot-runner/internal/tui/config"
	tea "github.com/charmbracelet/bubbletea"
)

// Wheel-only: AltScreen+Filter, wheel via 1000h (no CellMotion hover flood).
func TestWheelOnly_ProgramOptsIsAltScreenFilter(t *testing.T) {
	opts := tuiProgramOpts()
	if len(opts) != 2 {
		t.Fatalf("wheel-only opts must be AltScreen+Filter, got %d", len(opts))
	}
}

// Wheel scroll must scroll transcript, not switch prompt history.
func TestWheelOnly_WheelScrollsTranscriptNotHistory(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.asciiMode = true
	// Seed history so Up would navigate if wheel were mis-routed.
	m.promptHistory = []string{"first", "second"}
	m.promptHistIdx = -1
	m.inputValue = "draft"
	m.promptDraft = "draft"
	// Long transcript to allow scroll.
	for i := 0; i < 20; i++ {
		m.addMessage("assistant", "line "+string(rune('a'+i%26)), "")
	}
	_ = m.View()
	prevDraft := m.promptDraft
	m2, _ := m.handleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	am := m2.(*AppModel)
	if am.promptHistIdx != -1 || am.inputValue != prevDraft {
		t.Fatalf("wheel must not change prompt history, idx=%d input=%q", am.promptHistIdx, am.inputValue)
	}
	if am.viewport.offset == 0 {
		t.Fatal("wheel up must increase viewport offset (scroll transcript)")
	}
}

// Wheel burst within 40ms coalesces to one View() to avoid 20s hang (BUG-328).
func TestWheelOnly_BurstCoalesces(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.asciiMode = true
	for i := 0; i < 20; i++ {
		m.addMessage("assistant", "line", "")
	}
	_ = m.View()
	m2, _ := m.handleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	am := m2.(*AppModel)
	if am.viewport.offset != 3 {
		t.Fatalf("first wheel offset=3, got %d", am.viewport.offset)
	}
	// Second wheel immediately — coalesced, not yet flushed.
	m3, cmd := am.handleMouse(tea.MouseMsg{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress})
	am3 := m3.(*AppModel)
	if am3.viewport.offset != 3 {
		t.Fatalf("burst wheel must coalesce, offset still 3, got %d", am3.viewport.offset)
	}
	if am3.pendingWheelDelta != 3 {
		t.Fatalf("pending delta=3, got %d", am3.pendingWheelDelta)
	}
	if cmd == nil {
		t.Fatal("burst must schedule flush tick")
	}
	msg := cmd()
	if _, ok := msg.(wheelFlushMsg); !ok {
		t.Fatalf("flush msg type wheelFlushMsg, got %T", msg)
	}
	m4, _ := am3.Update(msg)
	if m4.(*AppModel).viewport.offset != 6 {
		t.Fatalf("after flush offset=6, got %d", m4.(*AppModel).viewport.offset)
	}
}

// Up/Down when not over viewport still navigates history (keyboard).
func TestWheelOnly_UpDownStillHistory(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.authPhase = AuthNone
	m.promptHistory = []string{"a", "b"}
	m.promptHistIdx = -1
	m.inputValue = ""
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyUp})
	am := m2.(*AppModel)
	if am.promptHistIdx == -1 {
		t.Fatal("Up must navigate history when wheel-only")
	}
}
