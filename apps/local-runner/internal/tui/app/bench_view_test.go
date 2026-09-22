package app

import (
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func benchModel(chats int, panelExpanded bool) *AppModel {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m.connStatus = ConnIdle
	m.width, m.height = 136, 48
	m.chatList = make([]client.RunHistoryItem, 0, chats)
	for i := 0; i < chats; i++ {
		m.chatList = append(m.chatList, client.RunHistoryItem{RunID: "run-" + string(rune('a'+i%26)) + string(rune('a'+i/26)), LastPrompt: "sample chat title " + string(rune('a'+i%26))})
	}
	if panelExpanded {
		enableSidebarForTest(m)
	}
	return m
}

func BenchmarkView_NoPanel(b *testing.B)    { benchView(b, 0, false) }
func BenchmarkView_Sidebar100(b *testing.B) { benchView(b, 100, true) }
func BenchmarkView_Sidebar0(b *testing.B)   { benchView(b, 0, true) }

func benchView(b *testing.B, chats int, collapsed bool) {
	m := benchModel(chats, collapsed)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = m.View()
	}
}
