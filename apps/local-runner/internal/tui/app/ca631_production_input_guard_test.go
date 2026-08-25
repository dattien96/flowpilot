package app

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// CA-631: CA-630 removed the Run() assignment of rejectWindowsRawPaste, so
// production Windows never armed the Ctrl+V flood rejection while every test
// set the field directly. Live log pid 16512 (08:28:16) shows the regression:
// 8 rapid runes (" vao for", 3-8ms apart) ended as "burst collapse active=false
// inputLen=16" instead of "(windows reject)" — the flood inserted + collapsed
// into a paste token, filled the 64-slot conhost queue while View ~357ms with
// the forced-open F2 sidebar, and no KeyMsg arrived for minutes afterwards.
//
// These tests lock: (1) the production guard helper arms on windows and stays
// off elsewhere, (2) with the guard armed the exact pid-16512 flood is rejected
// and subsequent typing stays live, (3) without the guard the same flood
// collapses into a token (the CA-630 regression), for all three providers.

func TestCA631_ProductionInputGuards_WindowsArms_OthersNoop(t *testing.T) {
	m := newTestModelWithProvider("claude")

	applyProductionInputGuards(m, "linux")
	if m.rejectWindowsRawPaste {
		t.Fatalf("linux must not arm rejectWindowsRawPaste")
	}

	applyProductionInputGuards(m, "darwin")
	if m.rejectWindowsRawPaste {
		t.Fatalf("darwin must not arm rejectWindowsRawPaste")
	}

	applyProductionInputGuards(m, "windows")
	if !m.rejectWindowsRawPaste {
		t.Fatalf("windows must arm rejectWindowsRawPaste")
	}
}

func TestCA631_Replay16512_FloodRejectedAndTypingLive_ClaudeCodexGrok(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			advance := clockAt(t)
			m := newTestModelWithProvider(provider)
			m.rejectWindowsRawPaste = true // what Run() now arms on windows

			// Exact pid-16512 sequence: 8 varied runes " vao for", 3-8ms gaps.
			for _, r := range " vao for" {
				advance(3 * time.Millisecond)
				m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				m = m2.(*AppModel)
			}
			if !m.pasteBurst.rejectArmed {
				t.Fatalf("[%s] 8 varied rapid runes must arm reject, chainLen=%d", provider, m.pasteBurst.chainLen)
			}
			if m.inputValue != "" {
				t.Fatalf("[%s] reject must wipe armed region to start, got %q", provider, m.inputValue)
			}

			// Settle: reject branch wipes to start and resets.
			advance(200 * time.Millisecond)
			m2, _ := m.Update(pasteBurstSettleMsg{})
			m = m2.(*AppModel)
			if m.pasteBurst.active || m.pasteBurst.rejectArmed {
				t.Fatalf("[%s] settle must fully reset burst state", provider)
			}
			if m.inputValue != "" {
				t.Fatalf("[%s] settle must leave input empty, got %q", provider, m.inputValue)
			}

			// Typing immediately after must stay live (the CA-610/630 hang).
			for _, r := range "abc" {
				m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				m = m2.(*AppModel)
			}
			if m.inputValue != "abc" {
				t.Fatalf("[%s] typing after rejected flood must insert abc, got %q", provider, m.inputValue)
			}
		})
	}
}

func TestCA631_Replay16512_WithoutGuardCollapsesToken_ClaudeCodexGrok(t *testing.T) {
	// Locks the CA-630 regression: without the production guard the same flood
	// is inserted and collapsed into "[Pasted 8 chars]" — the composer looks
	// hijacked and the flood already filled the conhost queue by then.
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			advance := clockAt(t)
			m := newTestModelWithProvider(provider)

			for _, r := range " vao for" {
				advance(3 * time.Millisecond)
				m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				m = m2.(*AppModel)
			}
			advance(200 * time.Millisecond)
			m2, _ := m.Update(pasteBurstSettleMsg{})
			m = m2.(*AppModel)

			if !strings.HasPrefix(m.inputValue, "[Pasted 8 chars]") {
				t.Fatalf("[%s] without guard flood must collapse to paste token, got %q", provider, m.inputValue)
			}
			if m.pasteBurst.active || m.pasteBurst.rejectArmed {
				t.Fatalf("[%s] settle must reset burst state, got active=%v reject=%v", provider, m.pasteBurst.active, m.pasteBurst.rejectArmed)
			}
		})
	}
}