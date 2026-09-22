package app

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// BUG-328: Cursor sends F2/F3/F4 as SS3 "\x1bOQ"/"\x1bOR"/"\x1bOS"; Windows
// ConPTY drops the ESC and the record reader yields 'O','Q' as separate rune
// records (tui.log pid 18400). ss3FKey must rebuild the F-key.

func TestBug328_SS3F2FromOQPair(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.authPhase = AuthNone
	m.sessionDefaultsLoaded = true

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'O'}})
	am := got.(*AppModel)
	if am.inputValue != "" {
		t.Fatalf("'O' must be held for the SS3 suffix, input=%q", am.inputValue)
	}
	got2, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Q'}})
	am2 := got2.(*AppModel)
	if len(am2.messages) == 0 {
		t.Fatal("OQ pair must fire the F2 /info dump")
	}
	if am2.inputValue != "" {
		t.Fatalf("OQ pair must not insert text, input=%q", am2.inputValue)
	}
}

func TestBug328_SS3F4FromOSPair(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.authPhase = AuthNone
	m.sessionDefaultsLoaded = true

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'O'}})
	got2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'S'}})
	am2 := got2.(*AppModel)
	if len(am2.messages) != 0 {
		t.Fatal("OS pair (F4) must be a no-op, no messages")
	}
	if am2.inputValue != "" {
		t.Fatalf("OS pair must not insert text, input=%q", am2.inputValue)
	}
}

func TestBug328_SS3F3FromORPair(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.authPhase = AuthNone
	m.sessionDefaultsLoaded = true
	m.selectedSkills = []client.SkillSelection{{Name: "golang"}}

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'O'}})
	got2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'R'}})
	am2 := got2.(*AppModel)
	if len(am2.messages) != 0 {
		t.Fatal("OR pair (F3) must be a no-op, no messages")
	}
	if am2.inputValue != "" {
		t.Fatalf("OR pair must not insert text, input=%q", am2.inputValue)
	}
}

func TestBug328_SS3NonSuffixFlushesHeldO(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.authPhase = AuthNone
	m.sessionDefaultsLoaded = true

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'O'}})
	got2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	am2 := got2.(*AppModel)
	if am2.inputValue != "Ox" {
		t.Fatalf("held 'O' must flush before the non-suffix rune, input=%q", am2.inputValue)
	}
}

func TestBug328_SS3NonRuneKeyFlushesHeldO(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.authPhase = AuthNone
	m.sessionDefaultsLoaded = true
	m.inputValue = "ab"
	m.setInputCaret(2)

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'O'}})
	got2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
	am2 := got2.(*AppModel)
	if am2.inputValue != "ab" {
		t.Fatalf("held 'O' must be flushed then deleted by Backspace, input=%q", am2.inputValue)
	}
}

func TestBug328_SS3ExpiredOFlushesOnNextRune(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.authPhase = AuthNone
	m.sessionDefaultsLoaded = true

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'O'}})
	m.ss3.at = time.Now().Add(-ss3FKeyWindow - time.Millisecond)
	got2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'Q'}})
	am2 := got2.(*AppModel)
	if am2.inputValue != "OQ" {
		t.Fatalf("expired pair must be plain text, input=%q", am2.inputValue)
	}
}

func TestBug328_SS3PasteOQStaysText(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.authPhase = AuthNone
	m.sessionDefaultsLoaded = true

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("OQ"), Paste: true})
	am := got.(*AppModel)
	if am.inputValue != "OQ" {
		t.Fatalf("a single-paste OQ must stay text, input=%q", am.inputValue)
	}
	if am.ss3.waiting {
		t.Fatal("paste must not arm the SS3 holder")
	}
}

func TestBug328_SS3PlainOThenNothingStaysPending(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.authPhase = AuthNone
	m.sessionDefaultsLoaded = true

	m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'O'}})
	if !m.ss3.waiting {
		t.Fatal("bare 'O' must be held pending the SS3 suffix")
	}
}
