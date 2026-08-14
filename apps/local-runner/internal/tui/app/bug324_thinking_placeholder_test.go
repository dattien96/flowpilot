package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// run-92955: follow-up turn_completed must replace thinking… even when a prior
// assistant message exists and tool rows were appended after the placeholder.

func TestTurnCompletedReplacesThinkingBehindToolMessages(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.6"}, "http://127.0.0.1:4317")
	m.addMessage("user", "chào, bạn đang dùng model gì", "")
	m.addMessage("assistant", "Grok 4.5", "")
	m.addMessage("user", "vậy model đang dùng là gì", "")
	m.addMessage("assistant", "thinking…", "thinking")
	m.addMessage("tool", "→ read_file", "tool")

	m2, _ := m.handleEvent(client.ProviderEvent{
		Type:         "turn_completed",
		FinalMessage: "Model đang dùng là **Grok 4.6**",
	})
	am := m2.(*AppModel)
	if am.thinkingIndex() >= 0 {
		t.Fatal("thinking placeholder must be gone after turn_completed")
	}
	found := false
	for _, msg := range am.messages {
		if msg.Role == "assistant" && strings.Contains(msg.Content, "Grok 4.6") {
			found = true
		}
		if msg.Role == "assistant" && msg.FormatHint == "thinking" {
			t.Fatalf("leftover thinking: %+v", msg)
		}
	}
	if !found {
		t.Fatalf("missing this-turn answer in messages=%+v", am.messages)
	}
}

func TestAppendAssistantDeltaReplacesThinkingNotLast(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.addMessage("assistant", "thinking…", "thinking")
	m.addMessage("tool", "→ read_file", "tool")
	m.appendAssistantDelta("Model đang dùng là Grok 4.6")
	if m.thinkingIndex() >= 0 {
		t.Fatal("thinking placeholder must be replaced by the delta")
	}
	last := m.messages[len(m.messages)-1]
	if last.Role != "assistant" || last.Content != "Model đang dùng là Grok 4.6" {
		t.Fatalf("last=%+v, want moved assistant with delta text", last)
	}
}

func TestTurnDoneMsgReplacesThinkingWhenPriorAssistantExists(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.addMessage("assistant", "previous turn", "")
	m.addMessage("assistant", "thinking…", "thinking")
	m2, _ := m.Update(TurnDoneMsg{FinalMsg: "Model đang dùng là Grok 4.6"})
	am := m2.(*AppModel)
	if am.thinkingIndex() >= 0 {
		t.Fatal("TurnDoneMsg must replace thinking even when a prior assistant exists")
	}
	last := am.messages[len(am.messages)-1]
	if last.Role != "assistant" || !strings.Contains(last.Content, "Grok 4.6") {
		t.Fatalf("last=%+v", last)
	}
}
