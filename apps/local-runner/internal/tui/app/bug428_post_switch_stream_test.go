package app

import (
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// BUG-428 (live BUG-LIVE-UI-5, run-786): after a /provider switch the new leg's
// seed turn streamed nowhere — the old leg's orchestration stream was left
// attached, so cmdStartOrchestrationStream bailed on orchStream != nil. The
// seed turn's turn_started/permission_required never reached the TUI: no
// approval card, turnActive=false, /stop a no-op against an invisible
// waiting_approval. applyChatSwitched must release the old leg's stream so the
// post-switch cmdStartOrchestrationStream reattaches to the new run.
func TestBug428_SwitchStopsOldOrchestrationStream(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.runHandle = &client.RunHandle{RunID: "run-old", RunKind: "chat", ChatID: "cht_a"}
	m.provider = "grok"
	m.model = "grok-4.5"

	cancelled := false
	oldCh := make(chan client.ProviderEvent)
	m.orchStream = &orchStreamState{
		evCh:   oldCh,
		cancel: func() { cancelled = true; close(oldCh) },
	}

	m.applyChatSwitched(ChatSwitchedMsg{Resp: &client.ChatSwitchResponse{
		Handle: client.RunHandle{RunID: "run-new", RunKind: "chat", ChatID: "cht_a", ProviderKey: "codex"},
		Model:  "gpt-5.4-mini",
	}})

	if !cancelled {
		t.Fatal("applyChatSwitched must cancel the old leg's orchestration stream")
	}
	if m.orchStream != nil {
		t.Fatal("orchStream must be cleared on switch so the new leg's stream can attach")
	}
	if cmd := m.cmdStartOrchestrationStream(); cmd == nil {
		t.Fatal("cmdStartOrchestrationStream must open the new leg's stream after a switch")
	}
}

// BUG-428 race: the old stream's in-flight cmdPollOrchStream is already blocked
// on the old channel when the switch cancels it. Its close event must not kill
// the NEW stream that opened in the meantime — otherwise the same invisible-
// turn wedge returns one event-loop tick later.
func TestBug428_StaleOrchPollCloseCannotKillNewStream(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.runHandle = &client.RunHandle{RunID: "run-new", RunKind: "chat", ChatID: "cht_a"}

	// Old leg's stream with a poll already in flight.
	oldCh := make(chan client.ProviderEvent)
	m.orchStream = &orchStreamState{evCh: oldCh, cancel: func() { close(oldCh) }}
	poll := m.cmdPollOrchStream()

	// Switch stops the old stream; the new leg's stream opens.
	m.stopOrchestrationStream()
	newCancelled := false
	m.orchStream = &orchStreamState{
		evCh:   make(chan client.ProviderEvent),
		cancel: func() { newCancelled = true },
	}

	// The stale poll unblocks with the old channel's close. Whatever message it
	// returns must not tear down the new stream.
	if msg := poll(); msg != nil {
		m2, _ := m.Update(msg)
		m = m2.(*AppModel)
	}
	if newCancelled || m.orchStream == nil {
		t.Fatal("stale close from the pre-switch stream killed the new leg's orchestration stream")
	}
}

// BUG-428 event side: a stale event delivered by the cancelled stream's
// in-flight poll must not bump lastEventSeq or reach handleEvent — the old
// run's seq numbers would corrupt the new run's stream cursor.
func TestBug428_StaleOrchPollEventDropped(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.runHandle = &client.RunHandle{RunID: "run-new", RunKind: "chat", ChatID: "cht_a"}

	oldCh := make(chan client.ProviderEvent, 1)
	oldCh <- client.ProviderEvent{Type: "turn_started", Seq: 9999}
	m.orchStream = &orchStreamState{evCh: oldCh, cancel: func() { close(oldCh) }}
	poll := m.cmdPollOrchStream()

	m.stopOrchestrationStream()
	m.orchStream = &orchStreamState{evCh: make(chan client.ProviderEvent)}
	m.lastEventSeq = 5

	if msg := poll(); msg != nil {
		m2, _ := m.Update(msg)
		m = m2.(*AppModel)
	}
	if m.lastEventSeq != 5 {
		t.Fatalf("stale pre-switch event advanced the new run's cursor: lastEventSeq=%d", m.lastEventSeq)
	}
}
