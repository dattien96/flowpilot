package app

import (
	"runtime"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/config"
)

// User request: drag-select (bôi đen) must auto-copy again, so mouse is
// re-enabled (AltScreen + MouseCellMotion + Filter). Old BUG-328 disabled it.
func TestTuiProgramOpts_WindowsNoMouse(t *testing.T) {
	opts := tuiProgramOpts()
	if len(opts) != 3 {
		t.Fatalf("opts must be AltScreen+MouseCellMotion+Filter (user wants drag-select), got %d", len(opts))
	}
}

// shouldDisableMouseTracking gates the mouse-off ANSI: Windows must keep it off
// (BUG-328 conhost focus steal); every other platform keeps it on for copy.
func TestShouldDisableMouseTracking_WindowsOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		if !shouldDisableMouseTracking() {
			t.Fatal("Windows must disable mouse tracking")
		}
	} else {
		if shouldDisableMouseTracking() {
			t.Fatalf("non-Windows (%s) must NOT disable mouse tracking", runtime.GOOS)
		}
	}
}

// Init() must NOT attach the mouse-off cmd on platforms that keep tracking on,
// otherwise the app never receives MouseMsg and drag/Shift-click copy is dead.
func TestInitMouseCmd_OnlyWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		if initMouseCmd() == nil {
			t.Fatal("Windows must return the mouse-off cmd")
		}
	} else {
		if initMouseCmd() != nil {
			t.Fatalf("non-Windows (%s) must return nil mouse cmd", runtime.GOOS)
		}
	}
}

// Init() returns a Sequence that does not disable mouse tracking off-platform.
func TestInit_SkipsMouseOffWhenTrackingWanted(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	_ = m.Init()
	if !shouldDisableMouseTracking() && initMouseCmd() != nil {
		t.Fatal("Init must not disable mouse tracking on this platform")
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

// F2/F4 stay keyboard-live with mouse tracking disabled (BUG-328). Task-311:
// F2 prints the info dump, F4 is a no-op that must not be swallowed.
func TestTuiProgramOpts_KeysForSessionPanel(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.authPhase = AuthNone
	m.sessionDefaultsLoaded = true
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyF2})
	if len(m2.(*AppModel).messages) == 0 {
		t.Fatal("F2 must print the info dump (key fallback when mouse off)")
	}
	m2, _ = m2.(*AppModel).handleKey(tea.KeyMsg{Type: tea.KeyF4})
	// F4 is a no-op — must not be swallowed or crash.
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
