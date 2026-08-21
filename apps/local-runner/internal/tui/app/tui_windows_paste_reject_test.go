package app

import (
	"runtime"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Windows production must reject WT Ctrl+V raw flood (char-by-char) and guide
// to Alt+V. Alt+V and macOS Cmd+V (bracketed Paste=true) must stay live.
func TestWindowsReject_RawFlood_BlockedAndHints(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only reject")
	}
	advance := clockAt(t)
	m := newPasteModel()
	m.rejectWindowsRawPaste = true
	// Simulate a long Ctrl+V flood: 20 rapid runes (no Paste flag).
	for _, r := range strings.Repeat("a", 20) {
		advance(5 * time.Millisecond)
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	// Input must stay empty — flood rejected.
	if m.inputValue != "" {
		t.Fatalf("raw flood on Windows must be rejected, got input=%q", m.inputValue)
	}
	if !m.pasteBurst.active {
		t.Fatal("burst should still be tracked even when rejected")
	}
	if len(m.messages) == 0 || !strings.Contains(m.messages[len(m.messages)-1].Content, "Use Alt+V for paste") {
		t.Fatalf("must show Alt+V hint after flood, got %+v", m.messages)
	}
	if m.messages[len(m.messages)-1].FormatHint != "gate" {
		t.Fatalf("hint must be highlight (gate), got %q", m.messages[len(m.messages)-1].FormatHint)
	}
	// Subsequent runes in same flood must stay blocked, not re-hint per rune.
	before := len(m.messages)
	advance(5 * time.Millisecond)
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = m2.(*AppModel)
	if m.inputValue != "" {
		t.Fatalf("subsequent flood rune must stay blocked, got %q", m.inputValue)
	}
	if len(m.messages) != before {
		t.Fatalf("hint should not spam per rune, before=%d after=%d", before, len(m.messages))
	}
	// After settle, burst resets and composer stays clean (no [Pasted]).
	advance(200 * time.Millisecond)
	m2, _ = m.Update(pasteBurstSettleMsg{})
	m = m2.(*AppModel)
	if m.pasteBurst.active {
		t.Fatal("burst must reset after settle even when rejected")
	}
	if strings.Contains(m.inputValue, "Pasted") || strings.Contains(m.inputValue, "Pasting") {
		t.Fatalf("rejected flood must not leave token, got %q", m.inputValue)
	}
	// Slow typing after settle must not be blocked (user: "k chat thêm được").
	advance(200 * time.Millisecond)
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = m2.(*AppModel)
	if m.inputValue != "h" {
		t.Fatalf("typing after rejected flood must insert, got %q", m.inputValue)
	}
	advance(200 * time.Millisecond)
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = m2.(*AppModel)
	if m.inputValue != "hi" {
		t.Fatalf("second slow char must append, got %q", m.inputValue)
	}
	// Enter after typing must send
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	sent := m2.(*AppModel)
	if len(sent.messages) == 0 {
		t.Fatal("Enter after typing after rejected flood must send")
	}
}

func TestWindowsReject_AltVStillWorks(t *testing.T) {
	m := newPasteModel()
	m.rejectWindowsRawPaste = true
	// Alt+V must still dispatch clipboard (not blocked).
	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}, Alt: true})
	if cmd == nil {
		t.Fatal("Alt+V must stay live even when raw flood is rejected")
	}
}

func TestWindowsReject_CtrlVShowsHint(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only")
	}
	m := newPasteModel()
	m.rejectWindowsRawPaste = true
	m2, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlV})
	am := m2.(*AppModel)
	if cmd == nil {
		t.Fatal("Ctrl+V on Windows must show hint toast")
	}
	if len(am.messages) == 0 || am.messages[len(am.messages)-1].FormatHint != "gate" {
		t.Fatalf("Ctrl+V hint must be gate highlight, got %+v", am.messages)
	}
	if !strings.Contains(am.messages[len(am.messages)-1].Content, "Use Alt+V for paste") {
		t.Fatalf("hint text must be short 'Use Alt+V for paste (text + image)', got %q", am.messages[len(am.messages)-1].Content)
	}
}

// Log 18700: hijack ClipboardPasteMsg inserted [Pasted] then next flood
// runes reverted it to empty. Subsequent runes after hijack must be
// swallowed without reverting the token.
func TestWindowsReject_HijackNotWipedByTrailingFlood(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only")
	}
	advance := clockAt(t)
	m := newPasteModel()
	m.rejectWindowsRawPaste = true
	// Start flood: 2 runes to arm + hijack.
	for _, r := range "ab" {
		advance(5 * time.Millisecond)
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	if m.inputValue != "" {
		t.Fatalf("after hijack arm input must be empty, got %q", m.inputValue)
	}
	// ClipboardPaste arrives (hijack result) → [Pasted].
	clampText := "Them ham ClampChecked(n, lo, hi int) (int, error) vao format.go: tra error khi lo > hi"
	m2, _ := m.Update(ClipboardPasteMsg{Text: clampText})
	m = m2.(*AppModel)
	if !strings.HasPrefix(m.inputValue, "[Pasted ") {
		t.Fatalf("clipboard hijack must insert [Pasted], got %q", m.inputValue)
	}
	token := m.inputValue
	// More flood runes (the tail of the original Ctrl+V) must be swallowed
	// without reverting the token.
	for _, r := range "tail" {
		advance(5 * time.Millisecond)
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	if m.inputValue != token {
		t.Fatalf("trailing flood must not wipe [Pasted], got %q want %q", m.inputValue, token)
	}
	// Enter after hijacked paste must send the full text (after burst settles).
	advance(200 * time.Millisecond)
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	sent := m2.(*AppModel)
	found := false
	for _, msg := range sent.messages {
		if msg.Role == "user" && msg.Content == clampText {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Enter after hijacked paste must send full text, got %+v", sent.messages)
	}
}
