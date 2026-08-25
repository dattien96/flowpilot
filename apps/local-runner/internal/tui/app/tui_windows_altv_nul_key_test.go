package app

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// CA-630: Platform-agnostic input and paste hardening.
// Tests:
// 1. NUL key noise (alt+\x00, lone \x00) is filtered and never corrupts inputValue.
// 2. Alt+V and Ctrl+V clipboard paste collapse to [Pasted N chars] and allow immediate subsequent typing.
// 3. One Backspace or Delete removes the entire paste token even when NUL noise arrived around it.
// 4. Raw rapid char stream paste gathers safely and collapses to token on settle.
// 5. 3-Provider Parity: claude, codex, grok.

func TestCA630_NulKeyNoiseFiltered_ClaudeCodexGrok(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			m := newTestModelWithProvider(provider)

			// Simulate terminal sending alt+\x00 when Alt key is pressed/released
			m2, _ := m.handleKey(tea.KeyMsg{
				Type:  tea.KeyRunes,
				Runes: []rune{0},
				Alt:   true,
			})
			m = m2.(*AppModel)
			if m.inputValue != "" {
				t.Fatalf("[%s] alt+\\x00 must not insert into inputValue, got %q", provider, m.inputValue)
			}

			// Simulate terminal sending lone \x00 key
			m2, _ = m.handleKey(tea.KeyMsg{
				Type:  tea.KeyRunes,
				Runes: []rune{0},
			})
			m = m2.(*AppModel)
			if m.inputValue != "" {
				t.Fatalf("[%s] lone \\x00 must not insert into inputValue, got %q", provider, m.inputValue)
			}

			// Type normal characters
			m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h', 'e', 'l', 'l', 'o'}})
			m = m2.(*AppModel)
			if m.inputValue != "hello" {
				t.Fatalf("[%s] normal typing must work after NUL noise, got %q", provider, m.inputValue)
			}
		})
	}
}

func TestCA630_AltVPasteAndImmediateTyping_ClaudeCodexGrok(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			m := newTestModelWithProvider(provider)

			// Step 1: User presses Alt+V (terminal sends alt+\x00 then alt+v)
			m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{0}, Alt: true})
			m = m2.(*AppModel)

			m2, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}, Alt: true})
			if cmd == nil {
				t.Fatalf("[%s] Alt+V must dispatch clipboard command", provider)
			}
			m = m2.(*AppModel)

			// Step 2: Clipboard returns pasted text
			pastedText := "func ClampChecked(n, lo, hi int) (int, error) {\n\tif lo > hi {\n\t\treturn 0, errors.New(\"invalid\")\n\t}\n\treturn n, nil\n}"
			m2, _ = m.Update(ClipboardPasteMsg{Text: pastedText})
			m = m2.(*AppModel)

			if !strings.HasPrefix(m.inputValue, "[Pasted ") {
				t.Fatalf("[%s] Alt+V text must collapse to token, got %q", provider, m.inputValue)
			}
			token := m.inputValue

			// Step 3: User immediately types text after pasting — must NOT be swallowed or delayed
			m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
			m = m2.(*AppModel)
			m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k', 'i', 'e', 'm', ' ', 't', 'r', 'a'}})
			m = m2.(*AppModel)

			expected := token + " kiem tra"
			if m.inputValue != expected {
				t.Fatalf("[%s] typing after Alt+V paste must append immediately, got %q, want %q", provider, m.inputValue, expected)
			}

			// Step 4: Expand tokens on submit must produce the full text + suffix
			expanded := m.expandPasteTokens(m.inputValue)
			if !strings.Contains(expanded, pastedText) || !strings.HasSuffix(expanded, " kiem tra") {
				t.Fatalf("[%s] expanded prompt mismatch, got %q", provider, expanded)
			}
		})
	}
}

func TestCA630_PasteTokenSingleBackspaceDelete_ClaudeCodexGrok(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			m := newTestModelWithProvider(provider)

			// Alt+\x00 noise followed by Alt+V paste
			m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{0}, Alt: true})
			m = m2.(*AppModel)

			m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}, Alt: true})
			m = m2.(*AppModel)

			pastedText := "func Add(a, b int) int { return a + b }"
			m2, _ = m.Update(ClipboardPasteMsg{Text: pastedText})
			m = m2.(*AppModel)

			if !strings.HasPrefix(m.inputValue, "[Pasted ") {
				t.Fatalf("[%s] must have paste token, got %q", provider, m.inputValue)
			}

			// Press Backspace once -> token must be completely deleted
			m2, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyBackspace})
			m = m2.(*AppModel)

			if m.inputValue != "" {
				t.Fatalf("[%s] single Backspace must delete entire paste token, got %q", provider, m.inputValue)
			}
			if len(m.pasteSegments) != 0 {
				t.Fatalf("[%s] pasteSegments must be empty after deleting token, got %v", provider, m.pasteSegments)
			}
		})
	}
}

func TestCA630_RawStreamPasteCollapsesOnSettle_ClaudeCodexGrok(t *testing.T) {
	for _, provider := range []string{"claude", "codex", "grok"} {
		t.Run(provider, func(t *testing.T) {
			advance := clockAt(t)
			m := newTestModelWithProvider(provider)

			// Rapid stream of characters (<25ms interval) representing raw non-bracketed paste
			stream := "Them ham ClampChecked(n, lo, hi int) (int, error)"
			for _, r := range stream {
				advance(3 * time.Millisecond)
				m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
				m = m2.(*AppModel)
			}

			// Advance beyond settle window (150ms) and trigger settle tick
			advance(200 * time.Millisecond)
			m2, _ := m.Update(pasteBurstSettleMsg{})
			m = m2.(*AppModel)

			if !strings.HasPrefix(m.inputValue, "[Pasted ") {
				t.Fatalf("[%s] raw stream paste must collapse to [Pasted N chars] on settle, got %q", provider, m.inputValue)
			}

			expanded := m.expandPasteTokens(m.inputValue)
			if expanded != stream {
				t.Fatalf("[%s] expanded stream mismatch, got %q, want %q", provider, expanded, stream)
			}
		})
	}
}
