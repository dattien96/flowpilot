package app

import (
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Windows: first raw paste flood should hint once that WT steals Ctrl+V
// (use Alt+V). Mouse is off via tuiProgramOpts, so settle does not pulse
// mouse — it returns the hint toast on first collapse, nil afterwards.
func TestBurst_SettleTickNoPulseWhenMouseOffOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only hint toast")
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
	m2, cmd := m.Update(pasteBurstSettleMsg{})
	am := m2.(*AppModel)
	if cmd == nil {
		t.Fatal("first windows settle after raw paste must show Alt+V hint toast")
	}
	if !am.pasteCtrlVHintShown {
		t.Fatal("hint flag must be set after first raw paste")
	}
	// Hint must not repeat on second settle (pasteBurst inactive).
	_, cmd2 := am.Update(pasteBurstSettleMsg{})
	if cmd2 != nil {
		t.Fatalf("second settle must not re-hint, got cmd %v", cmd2)
	}
}

// After the burst settles and collapses, a real Enter must still submit the
// full expanded prompt — with mouse off on Windows no pulse is needed.
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
	// The next real Enter submits the full text even though settle showed hint.
	m3, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	sent := m3.(*AppModel)
	found := false
	for _, msg := range sent.messages {
		if msg.Role == "user" && msg.Content == full {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Enter after settled burst must submit full text, got %+v", sent.messages)
	}
}

// While a raw paste flood is active, the composer must hide the char-by-char
// flood and show [Pasting…] instead of the partial raw text.
func TestBurst_RenderShowsPastingDuringFlood(t *testing.T) {
	advance := clockAt(t)
	m := newPasteModel()
	for _, r := range "he" {
		advance(5 * time.Millisecond)
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	if !m.pasteBurst.active {
		t.Fatal("burst should be active after 2 rapid runes")
	}
	line := m.renderInputLine()
	// caret ("▌"/"_" or " " when cursorOff) splits the placeholder, so allow
	// both contiguous and caret-split forms.
	plain := stripANSI(line)
	if !(strings.Contains(plain, "Pasting") || strings.Contains(plain, "P asting") || strings.Contains(plain, "P▌asting") || strings.Contains(plain, "P_asting")) {
		t.Fatalf("render must show [Pasting…] during burst, got %q", line)
	}
	advance(200 * time.Millisecond)
	m2, _ := m.Update(pasteBurstSettleMsg{})
	m = m2.(*AppModel)
	plain2 := strings.ReplaceAll(strings.ReplaceAll(stripANSI(m.renderInputLine()), "▌", ""), "_", "")
	if strings.Contains(plain2, "Pasting") {
		t.Fatalf("after settle must show [Pasted …], not Pasting, got %q", m.renderInputLine())
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
