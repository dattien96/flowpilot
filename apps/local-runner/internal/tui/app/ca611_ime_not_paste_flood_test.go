package app

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// CA-611: Vietnamese Telex IME (dd->đ) must not be mistaken for WT Ctrl+V flood.
// Real log 4332: IME sends Backspace + d + d in 3-5ms, then next char. That
// 2-rune chain previously hit chainLen>=2 and wiped the composer + toast Alt+V.

func TestIME_TelexDd_NotRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("IME timing")
	}
	advance := clockAt(t)
	m := newPasteModel()
	m.rejectWindowsRawPaste = true

	// Simulate: d, Backspace (IME rewrite), d, d, then next char 'a' quickly.
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = m2.(*AppModel)
	advance(5 * time.Millisecond)
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	m = m2.(*AppModel)
	advance(4 * time.Millisecond)
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = m2.(*AppModel)
	advance(3 * time.Millisecond)
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = m2.(*AppModel)

	if strings.Contains(m.inputValue, "Use Alt+V") || len(m.messages) > 0 && strings.Contains(m.messages[len(m.messages)-1].Content, "Use Alt+V") {
		t.Fatalf("IME dd must not hint Alt+V, messages=%+v input=%q", m.messages, m.inputValue)
	}
	if m.inputValue == "" {
		t.Fatalf("IME dd must not wipe composer, got empty")
	}
	// Next char after IME commit must insert, not be swallowed as flood tail.
	advance(3 * time.Millisecond)
	m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	m = m2.(*AppModel)
	if !strings.Contains(m.inputValue, "a") {
		t.Fatalf("next char after IME must insert, got %q", m.inputValue)
	}
	if strings.Contains(m.inputValue, "Use Alt+V") {
		t.Fatalf("next char must not trigger hint, got %q %+v", m.inputValue, m.messages)
	}
}

func TestIME_BackspaceResetsBurst(t *testing.T) {
	advance := clockAt(t)
	m := newPasteModel()
	m.rejectWindowsRawPaste = true
	// Build a chain of 2, then backspace should break it so next 2 don't arm reject.
	for _, r := range "ab" {
		advance(5 * time.Millisecond)
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	// Backspace resets burst.
	advance(5 * time.Millisecond)
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	m = m2.(*AppModel)
	if m.pasteBurst.active || m.pasteBurst.chainLen != 0 {
		t.Fatalf("Backspace must reset burst, got active=%v chainLen=%d", m.pasteBurst.active, m.pasteBurst.chainLen)
	}
	// Next 2 runes must not be treated as continuation of previous flood.
	for _, r := range "cd" {
		advance(5 * time.Millisecond)
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	if len(m.messages) > 0 && strings.Contains(m.messages[len(m.messages)-1].Content, "Use Alt+V") {
		t.Fatalf("after Backspace reset, new chain must not hint, got %+v", m.messages)
	}
}

func TestIME_SingleRepeatedChain_NotRejected(t *testing.T) {
	advance := clockAt(t)
	m := newPasteModel()
	m.rejectWindowsRawPaste = true
	// Holding 's' for tone: 12 rapid s's. Previously would wipe at 2; now must not
	// hint at 8 due to single-repeated exemption, and composer must keep s's.
	for i := 0; i < 12; i++ {
		advance(5 * time.Millisecond)
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
		m = m2.(*AppModel)
	}
	if len(m.messages) > 0 && strings.Contains(m.messages[len(m.messages)-1].Content, "Use Alt+V") {
		t.Fatalf("repeated single char must not hint Alt+V, got %+v", m.messages)
	}
	if m.inputValue == "" {
		t.Fatal("repeated s must stay in composer, not wiped")
	}
	if !strings.Contains(m.inputValue, "s") {
		t.Fatalf("expected s in input, got %q", m.inputValue)
	}
}

func TestWindowsReject_LongVariedPasteStillRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skip")
	}
	advance := clockAt(t)
	m := newPasteModel()
	m.rejectWindowsRawPaste = true
	// 20 varied runes -> must still be rejected (real paste)
	for _, r := range "abcdefghijklmnopqrst" {
		advance(5 * time.Millisecond)
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	if m.inputValue != "" {
		t.Fatalf("long varied flood must be rejected, got %q", m.inputValue)
	}
	if len(m.messages) == 0 || !strings.Contains(m.messages[len(m.messages)-1].Content, "Use Alt+V") {
		t.Fatalf("long varied flood must hint Alt+V, got %+v", m.messages)
	}
}

func TestIME_ShortVaried_NotRejectedBeforeThreshold(t *testing.T) {
	advance := clockAt(t)
	m := newPasteModel()
	m.rejectWindowsRawPaste = true
	// Only 4 varied runes (<8) must not hint.
	for _, r := range "abcd" {
		advance(5 * time.Millisecond)
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	if len(m.messages) > 0 && strings.Contains(m.messages[len(m.messages)-1].Content, "Use Alt+V") {
		t.Fatalf("4-char chain must not hint, got %+v", m.messages)
	}
	if !strings.Contains(m.inputValue, "abcd") {
		t.Fatalf("4-char varied must stay, got %q", m.inputValue)
	}
}

func TestIsSingleRepeatedRuneChain(t *testing.T) {
	if isSingleRepeatedRuneChain([]rune("s")) {
		t.Fatal("len 1 must be false")
	}
	if !isSingleRepeatedRuneChain([]rune("ssss")) {
		t.Fatal("ssss must be true")
	}
	if isSingleRepeatedRuneChain([]rune("sssd")) {
		t.Fatal("sssd must be false")
	}
	if isSingleRepeatedRuneChain([]rune("abab")) {
		t.Fatal("abab must be false")
	}
}
