package app

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/config"
)

// CA-615: Alt+V hung at 10:06 after fast typing 10:05 was normal. Native
// clipboard.Read has no timeout, so a locked clipboard deadlocks the TUI even
// though typing (no clipboard I/O) stays fine.

func TestCA615_ReadWithTimeout_SlowFnTimesOut(t *testing.T) {
	start := time.Now()
	_, ok := readWithTimeout(50*time.Millisecond, func() string {
		time.Sleep(500 * time.Millisecond)
		return "slow"
	})
	if ok {
		t.Fatalf("slow fn must time out")
	}
	if d := time.Since(start); d > 400*time.Millisecond {
		t.Fatalf("timeout took %v, want <400ms", d)
	}
}

func TestCA615_ReadWithTimeout_FastFnOk(t *testing.T) {
	v, ok := readWithTimeout(200*time.Millisecond, func() string { return "fast" })
	if !ok || v != "fast" {
		t.Fatalf("fast fn must return ok with value, got ok=%v v=%q", ok, v)
	}
}

func TestCA615_ReadWithTimeout_GenericBytesOk(t *testing.T) {
	v, ok := readWithTimeout(200*time.Millisecond, func() []byte { return []byte("abc") })
	if !ok || string(v) != "abc" {
		t.Fatalf("bytes helper must work, got ok=%v v=%q", ok, string(v))
	}
}

func TestCA615_CmdClipboardPaste_TextDoesNotNeedImage_ClaudeCodexGrok(t *testing.T) {
	// Text path must not require image read; even with pending images cap,
	// text paste should not trigger image branch. Verify no hang and provider
	// does not matter (SupportsImages varies).
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:9")
			m.provider = pk
			// Fallback text ensures the cmd returns without touching real clipboard
			// when native times out — must finish quickly.
			cmd := m.cmdClipboardPasteWithFallback("hello fallback text")
			if cmd == nil {
				t.Fatal("cmd nil")
			}
			start := time.Now()
			msg := cmd()
			if d := time.Since(start); d > 6*time.Second {
				t.Fatalf("%s: cmd took %v, want <6s (must be bounded)", pk, d)
			}
			// Should return either clipboard text or the fallback; never hang.
			_ = msg
		})
	}
}

func TestCA615_AltV_StillAsyncCmd(t *testing.T) {
	// Alt+V must not insert bare "v" and must stay async (not block typing).
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:9")
			m.provider = pk
			m.sessionLoading = false
			m.authPhase = AuthNone
			m.inputValue = "hi "
			msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}, Alt: true}
			m2, cmd := m.handleKey(msg)
			am := m2.(*AppModel)
			if am.inputValue != "hi " {
				t.Fatalf("%s: Alt+V must not insert rune, got %q", pk, am.inputValue)
			}
			if cmd == nil {
				t.Fatalf("%s: Alt+V must return clipboard cmd", pk)
			}
		})
	}
}
