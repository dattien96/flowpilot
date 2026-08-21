package app

import (
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Win conpty: after a raw paste burst settles, keys stay stuck until a mouse
// click re-enables cell-motion (log 22964 10:02:04.145 collapse -> 32s
// MouseMsg-only -> click -> Enter). The settle tick must pulse mouse tracking
// on Windows to unstick the keyboard without dropping the next KeyMsg.
func TestBurst_SettleTickPulsesMouseOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only mouse unstick")
	}
	advance := clockAt(t)
	m := newPasteModel()
	full := strings.Join([]string{"L1", "L2", "L3", "L4", "L5", "L6", "L7"}, "\n")
	lines := strings.Split(full, "\n")
	for i, line := range lines {
		for _, r := range line {
			advance(5 * time.Millisecond)
			m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = m2.(*AppModel)
		}
		if i < len(lines)-1 {
			advance(5 * time.Millisecond)
			m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
			m = m2.(*AppModel)
		}
	}
	advance(200 * time.Millisecond)
	_, cmd := m.Update(pasteBurstSettleMsg{})
	if cmd == nil {
		t.Fatal("settle tick after raw paste must return a pulse cmd on windows to unstick keys")
	}
}

// After the burst settles and collapses, a real Enter must still submit the
// full expanded prompt — pulse must be Enable-only (Disable+Enable drops it).
func TestBurst_EnterAfterSettleStillSubmitsAfterPulse(t *testing.T) {
	advance := clockAt(t)
	m := newPasteModel()
	full := strings.Join([]string{"L1", "L2", "L3", "L4", "L5", "L6", "L7"}, "\n")
	lines := strings.Split(full, "\n")
	for i, line := range lines {
		for _, r := range line {
			advance(5 * time.Millisecond)
			m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
			m = m2.(*AppModel)
		}
		if i < len(lines)-1 {
			advance(5 * time.Millisecond)
			m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
			m = m2.(*AppModel)
		}
	}
	advance(200 * time.Millisecond)
	m2, _ := m.Update(pasteBurstSettleMsg{})
	m = m2.(*AppModel)
	if m.inputValue != "[Pasted 20 chars]" {
		t.Fatalf("settle must collapse, got %q", m.inputValue)
	}
	// The next real Enter submits the full text even though settle pulsed.
	m3, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	sent := m3.(*AppModel)
	if len(sent.messages) == 0 || sent.messages[0].Content != full {
		t.Fatalf("Enter after settled burst must submit full text, got %+v", sent.messages)
	}
}

// Tiny single-paste settle should not require a pulse but must not break Enter.
func TestBurst_TinyPasteSettleStillAllowsEnter(t *testing.T) {
	advance := clockAt(t)
	m := newPasteModel()
	for _, r := range "hi" {
		advance(5 * time.Millisecond)
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	advance(200 * time.Millisecond)
	m2, _ := m.Update(pasteBurstSettleMsg{})
	m = m2.(*AppModel)
	m3, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	sent := m3.(*AppModel)
	if len(sent.messages) == 0 {
		t.Fatal("Enter after tiny burst settle must submit")
	}
}
