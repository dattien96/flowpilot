package app

import (
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/config"
)

// Windows: mouse must be off so Enter/F2/F4 survive a WT paste flood
// (log 18936 Enable-only was a no-op, 22964 needed a click). Non-Windows
// keeps mouse for click affordances. Ctrl+V/Alt+V stay live on both.
func TestTuiProgramOpts_WindowsNoMouse(t *testing.T) {
	opts := tuiProgramOpts()
	if len(opts) == 0 {
		t.Fatal("opts must not be empty (WithAltScreen)")
	}

	// Probe by constructing programs with the returned opts — we assert the
	// count rather than deep option internals: Windows = 1 (AltScreen only),
	// other = 2 (AltScreen + MouseCellMotion). This is stable and additive.
	if runtime.GOOS == "windows" {
		if len(opts) != 1 {
			t.Fatalf("windows opts must be AltScreen only, got %d", len(opts))
		}
	} else {
		if len(opts) != 2 {
			t.Fatalf("non-windows opts must include mouse, got %d", len(opts))
		}
	}
}

// Keys stay live after a raw paste on both platforms — burst collapse
// followed by Enter must still submit.
func TestBurst_WindowsFix_KeysLive_AfterBurstEnterSubmits(t *testing.T) {
	m := newPasteModel()
	// Simulate a paste already collapsed (no mouse quirk involved).
	m.inputValue = "[Pasted 20 chars]"
	m.pasteSegments = []pasteSegment{{token: "[Pasted 20 chars]", text: "hello world paste"}}
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	sent := m2.(*AppModel)
	if len(sent.messages) == 0 {
		t.Fatal("Enter after collapsed paste must submit (keys must stay live)")
	}
}

// F2/F4 are key-only fallbacks when mouse is off on Windows.
func TestTuiProgramOpts_KeysForSessionPanel(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.authPhase = AuthNone
	m.sessionPanel.Collapsed = true
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyF2})
	if m2.(*AppModel).sessionPanel.Collapsed {
		t.Fatal("F2 must toggle session panel (key fallback when mouse off)")
	}
	m2, _ = m2.(*AppModel).handleKey(tea.KeyMsg{Type: tea.KeyF4})
	// F4 toggles status details — must not be swallowed.
	if m2 == nil {
		t.Fatal("F4 handleKey must return model")
	}
}

// Ctrl+V and Alt+V must remain clipboard paths even with mouse off.
func TestTuiProgramOpts_ClipboardKeysStayLive(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.authPhase = AuthNone
	if runtime.GOOS == "windows" {
		// Windows: Ctrl+V is intercepted to guide to Alt+V (WT steals Ctrl+V).
		m2, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlV})
		am := m2.(*AppModel)
		if cmd == nil {
			t.Fatal("Ctrl+V on Windows must return hint toast")
		}
		if len(am.messages) == 0 || am.messages[len(am.messages)-1].Content == "" {
			t.Fatal("Ctrl+V on Windows must add hint message")
		}
	} else {
		if _, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlV}); cmd == nil {
			t.Fatal("Ctrl+V must return cmdClipboardPaste (mouse off must not kill it)")
		}
	}
	if _, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}, Alt: true}); cmd == nil {
		t.Fatal("Alt+V must return cmdClipboardPaste")
	}
}

// Windows Ctrl+V must be stopped and show Alt+V hint (user request).
func TestCtrlV_OnWindows_ShowsAltVHint(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only Ctrl+V hint")
	}
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.authPhase = AuthNone
	m2, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlV})
	am := m2.(*AppModel)
	if cmd == nil {
		t.Fatal("Ctrl+V on Windows must return hint toast cmd")
	}
	if len(am.messages) == 0 {
		t.Fatal("hint message must be added")
	}
	last := am.messages[len(am.messages)-1].Content
	if last == "" || !containsIgnoreCase(last, "Alt+V") {
		t.Fatalf("hint must mention Alt+V, got %q", last)
	}
	if am.inputValue != "" {
		t.Fatalf("Ctrl+V on Windows must not paste, input=%q", am.inputValue)
	}
}

func containsIgnoreCase(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}
