package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/config"
)

// run-96217: stream closed with thinking… left on screen after tools, no answer.
func TestTurnStreamClosed_ClearsStrandedThinking(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:9")
	m.connStatus = ConnRunning
	m.addMessage("user", "tôi gửi cho bạn mấy ảnh", "")
	m.addMessage("assistant", "thinking…", "thinking")
	m.addMessage("tool", "→ read_file", "tool")
	m.addMessage("tool", "→ read_file", "tool")

	m2, _ := m.Update(turnStreamClosedMsg{})
	am := m2.(*AppModel)
	if am.thinkingIndex() >= 0 {
		t.Fatal("thinking placeholder must not remain after stream close")
	}
	if am.statusMsg != "done" {
		t.Fatalf("statusMsg=%q want done", am.statusMsg)
	}
	found := false
	for _, msg := range am.messages {
		if msg.Role == "system" && strings.Contains(msg.Content, "no assistant text") {
			found = true
		}
		if msg.Role == "assistant" && msg.FormatHint == "thinking" {
			t.Fatalf("leftover thinking: %+v", msg)
		}
	}
	if !found {
		t.Fatalf("expected system notice, messages=%+v", am.messages)
	}
}
