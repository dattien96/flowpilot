package app

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// CA-612: Ctrl+V raw flood must not stream into composer. After hint at 8,
// tail must stay swallowed even across >25ms gaps, and settle must wipe leaks.

// Replay log 11764: 8 rapid varied runes -> hint + empty, then tail with
// 3-5ms gaps then a 29-50ms gap (simulating handleKey slowdown) must still be swallowed.
func TestCA612_RejectStaysArmedAcrossGap(t *testing.T) {
	advance := clockAt(t)
	m := newPasteModel()
	m.rejectWindowsRawPaste = true

	// First 8 rapid varied to arm (past IME guard, varied).
	for _, r := range "Them ha" { // 7? need 8
		advance(3 * time.Millisecond)
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	// 8th char 'm' of "Them ham" at 39.552 triggers arm.
	advance(3 * time.Millisecond)
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = m2.(*AppModel)

	if m.inputValue != "" {
		t.Fatalf("after 8 should be wiped, got %q", m.inputValue)
	}
	if len(m.messages) == 0 || !strings.Contains(m.messages[len(m.messages)-1].Content, "Use Alt+V") {
		t.Fatalf("must hint at 8, got %+v", m.messages)
	}
	before := len(m.messages)

	// Tail: continue flood with small gaps (still swallowed by rejectArmed).
	for _, r := range "Cla" {
		advance(4 * time.Millisecond)
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	if m.inputValue != "" {
		t.Fatalf("tail with <25ms must stay empty, got %q", m.inputValue)
	}
	// Simulate View slowdown: next gap 29ms >= burstRuneGap.
	advance(29 * time.Millisecond)
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	m = m2.(*AppModel)
	if m.inputValue != "" {
		t.Fatalf("tail after 29ms gap must still be swallowed (rejectArmed), got %q", m.inputValue)
	}
	// Even larger gap (50-160ms) must still be swallowed until settle.
	advance(50 * time.Millisecond)
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = m2.(*AppModel)
	if m.inputValue != "" {
		t.Fatalf("tail after 50ms gap must still be swallowed, got %q", m.inputValue)
	}
	if len(m.messages) != before {
		t.Fatalf("hint must not spam, before=%d after=%d", before, len(m.messages))
	}
}

// Settle must wipe leaked residue (log 11764 inputLen 31 then 40) when rejectArmed.
func TestCA612_SettleWipesWhenRejectArmed(t *testing.T) {
	advance := clockAt(t)
	m := newPasteModel()
	m.rejectWindowsRawPaste = true
	for _, r := range "abcdefgh" {
		advance(3 * time.Millisecond)
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	// Simulate a leaked tail that slipped before fix: manually inject residue
	// (in real bug, 25ms gap lets tail insert). Here we inject to test settle wipe.
	m.inputValue = "leaked residue after 8"
	// Keep burst armed as it would be mid-paste.
	if !m.pasteBurst.rejectArmed {
		t.Fatal("rejectArmed must be true after 8 varied")
	}
	advance(200 * time.Millisecond)
	m2, _ := m.Update(pasteBurstSettleMsg{})
	m = m2.(*AppModel)
	if m.inputValue != "" {
		t.Fatalf("settle when rejectArmed must wipe to start, got %q", m.inputValue)
	}
	if m.pasteBurst.active || m.pasteBurst.rejectArmed {
		t.Fatal("burst must reset after settle")
	}
	// Slow typing after settle must insert.
	advance(200 * time.Millisecond)
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = m2.(*AppModel)
	if m.inputValue != "h" {
		t.Fatalf("typing after settle wipe must insert, got %q", m.inputValue)
	}
}

// KeyCtrlV on Windows arms reject so following flood inserts nothing.
func TestCA612_KeyCtrlVArmsReject(t *testing.T) {
	// Only meaningful on windows; gate still works in test via GOOS check in code.
	// We test the branch directly: calling KeyCtrlV with reject enabled arms.
	m := newPasteModel()
	m.rejectWindowsRawPaste = true
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlV})
	m = m2.(*AppModel)
	if !m.pasteBurst.rejectArmed {
		t.Fatal("KeyCtrlV must arm rejectArmed when reject enabled")
	}
	if len(m.messages) == 0 || !strings.Contains(m.messages[len(m.messages)-1].Content, "Use Alt+V") {
		t.Fatalf("KeyCtrlV must hint, got %+v", m.messages)
	}
	advance := clockAt(t)
	advance(5 * time.Millisecond)
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = m2.(*AppModel)
	if m.inputValue != "" {
		t.Fatalf("runes after KeyCtrlV must be swallowed, got %q", m.inputValue)
	}
}

// Settle dedup: rapid runes schedule only one pending Tick.
func TestCA612_SettleDedup(t *testing.T) {
	advance := clockAt(t)
	m := newPasteModel()
	m.rejectWindowsRawPaste = true
	for _, r := range "abcdefgh" {
		advance(2 * time.Millisecond)
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	if !m.pasteBurst.settlePending {
		t.Fatal("settle should be pending after flood")
	}
	// Next rapid rune must not create a second pending (Once returns nil).
	advance(2 * time.Millisecond)
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = m2.(*AppModel)
	if !m.pasteBurst.settlePending {
		t.Fatal("settle must remain pending (dedup)")
	}
	if !m.pasteBurst.rejectArmed {
		t.Fatal("rejectArmed must stay true")
	}
}

// Alt+V must still work after rejected flood.
func TestCA612_AltVAfterReject(t *testing.T) {
	advance := clockAt(t)
	m := newPasteModel()
	m.rejectWindowsRawPaste = true
	for _, r := range "abcdefgh" {
		advance(3 * time.Millisecond)
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	advance(200 * time.Millisecond)
	m2, _ := m.Update(pasteBurstSettleMsg{})
	m = m2.(*AppModel)
	// Alt+V after reject
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}, Alt: true})
	if m2 == nil {
		t.Fatal("Alt+V should dispatch")
	}
	m = m2.(*AppModel)
	// Simulate clipboard return
	m2, _ = m.Update(ClipboardPasteMsg{Text: "hello alt paste"})
	m = m2.(*AppModel)
	if !strings.Contains(m.inputValue, "hello") && !strings.Contains(m.inputValue, "Pasted") {
		t.Fatalf("Alt+V after reject must paste, got %q", m.inputValue)
	}
}
