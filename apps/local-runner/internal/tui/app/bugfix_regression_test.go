package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestCmdShutdownAndQuit_SkipsKillWhenRunnerReused(t *testing.T) {
	m := New(config.ChatConfig{OwnsRunner: false}, "http://127.0.0.1:4317")
	cmd := m.cmdShutdownAndQuit()
	msg := cmd()
	if _, ok := msg.(QuitMsg); !ok {
		t.Fatalf("got %T, want QuitMsg without contacting runner", msg)
	}
}

func TestSessionDefaultsMsg_DoesNotRepeatReadyOnRefresh(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.5"}, "http://127.0.0.1:4317")
	m2, _ := m.Update(SessionDefaultsMsg{Provider: "claude", Model: "opus", AccountLabel: "first"})
	am := m2.(*AppModel)
	readyCount := 0
	for _, msg := range am.messages {
		if strings.Contains(msg.Content, "Ready —") {
			readyCount++
		}
	}
	if readyCount != 1 {
		t.Fatalf("first load readyCount=%d", readyCount)
	}
	if am.provider != "grok" {
		t.Fatalf("provider overwritten to %q, saved grok must win", am.provider)
	}

	m3, _ := am.Update(SessionDefaultsMsg{
		Provider:     "claude",
		AccountLabel: "second",
		Account:      &client.ProviderAccountSummary{ID: "a2", ProviderKey: "grok", DisplayLabel: "Grok 2"},
	})
	am = m3.(*AppModel)
	readyCount = 0
	for _, msg := range am.messages {
		if strings.Contains(msg.Content, "Ready —") {
			readyCount++
		}
	}
	if readyCount != 1 {
		t.Fatalf("refresh must not reprint Ready, readyCount=%d", readyCount)
	}
	if am.accountLabel != "Grok 2" {
		t.Fatalf("accountLabel=%q want Grok 2 after refresh", am.accountLabel)
	}
}

func TestTurnStreamOpened_HighReasoningShowsThinking(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.reasoningEffort = "high"
	m2, _ := m.Update(turnStreamOpenedMsg{EvCh: make(chan client.ProviderEvent)})
	am := m2.(*AppModel)
	if am.statusMsg != "thinking…" {
		t.Fatalf("statusMsg=%q want thinking…", am.statusMsg)
	}
	m3, _ := am.Update(client.ProviderEvent{}) // no-op if wrong type
	_ = m3
	m4, _ := am.handleEvent(client.ProviderEvent{Type: "message_delta", Text: "hi"})
	am = m4.(*AppModel)
	if am.statusMsg != "streaming…" {
		t.Fatalf("after delta statusMsg=%q want streaming…", am.statusMsg)
	}
}

func TestTurnStreamClosedError_ClearsRunHandle(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.runHandle = &client.RunHandle{RunID: "run-dead"}
	m2, _ := m.Update(turnStreamClosedMsg{Err: errString("connection refused")})
	am := m2.(*AppModel)
	if am.runHandle != nil {
		t.Fatalf("runHandle=%+v, want nil after stream error", am.runHandle)
	}
}

func TestShortID_KeepsPrefixedNumericIDs(t *testing.T) {
	if got := shortID("run-105935"); got != "run-105935" {
		t.Fatalf("shortID(run-105935)=%q", got)
	}
	if got := shortID("turn-12"); got != "turn-12" {
		t.Fatalf("shortID(turn-12)=%q", got)
	}
}

func TestWindowSizeMsg_ClampsStatusAndInput(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok", Model: "grok-4.5-very-long-model-name"}, "http://127.0.0.1:4317")
	m.reasoningEffort = "medium"
	m.asciiMode = true
	m.inputValue = strings.Repeat("x", 80)
	m2, _ := m.Update(tea.WindowSizeMsg{Width: 36, Height: 12})
	am := m2.(*AppModel)
	view := am.View()
	for i, line := range strings.Split(view, "\n") {
		if n := len([]rune(line)); n > 36 {
			t.Fatalf("line %d width %d > 36: %q", i, n, line)
		}
	}
}

type errString string

func (e errString) Error() string { return string(e) }
