package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/config"
)

// BUG-328: the Win32 pump/drain machinery (CA-665/666/667) and its 80ms
// debounce were removed. Every key parsed from the single input reader must
// reach handleKey — the old tuiMsgFilter dedupe silently dropped a second F2
// pressed within 80ms.
func TestBug328_FilterPassesRapidRepeatedSpecialKeys(t *testing.T) {
	for i := 0; i < 3; i++ {
		out := tuiMsgFilter(New(config.ChatConfig{}, "http://127.0.0.1:9"), tea.KeyMsg{Type: tea.KeyF2})
		km, ok := out.(tea.KeyMsg)
		if !ok || km.Type != tea.KeyF2 {
			t.Fatalf("rapid F2 #%d must pass the filter unchanged, got %v", i+1, out)
		}
	}
}

func TestBug328_FilterPassesRapidEscape(t *testing.T) {
	for i := 0; i < 3; i++ {
		out := tuiMsgFilter(New(config.ChatConfig{}, "http://127.0.0.1:9"), tea.KeyMsg{Type: tea.KeyEsc})
		if km, ok := out.(tea.KeyMsg); !ok || km.Type != tea.KeyEsc {
			t.Fatalf("Esc #%d must pass the filter, got %v", i+1, out)
		}
	}
}

func TestBug328_NoBracketedPasteMarkers(t *testing.T) {
	if strings.Contains(mouseTrackingOffANSI, "2004") {
		t.Fatal("?2004h must NOT be enabled: on the record path the 200~/201~ markers would be typed into the composer")
	}
}