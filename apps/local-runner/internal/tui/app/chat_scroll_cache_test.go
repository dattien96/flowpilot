package app

import (
	"testing"

	"flowpilot-runner/internal/tui/config"
)

func TestChatRows_ScrollReusesRowCache(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.addMessage("user", "scroll-cache-ask", "")
	m.addMessage("assistant", "# ScrollCache\n\nA **bold** paragraph.\n\n```go\nfunc scroll() {}\n```\n", "")
	first := m.chatRows()
	sig := m.rowCacheSig
	m.viewport.offset = 4
	_ = m.View()
	second := m.chatRows()
	if m.rowCacheSig != sig {
		t.Fatal("scroll/View should reuse chat row cache")
	}
	if len(first) != len(second) {
		t.Fatalf("cached rows changed length %d -> %d", len(first), len(second))
	}
}
