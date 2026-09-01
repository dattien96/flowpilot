package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// BUG-341: 417944 first turn ended on 6 tool calls with a blank assistant until
// the next prompt's replay. The TUI marked done too early (turnStream closed
// before turn_completed via orch). turnLive keeps it busy until the terminal.
func TestFirstTurnStreamClosedKeepsLiveUntilTerminal(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.runHandle = &client.RunHandle{RunID: "run-417944", RunKind: "chat", ChatID: "cht_a"}
	m.connStatus = ConnRunning
	m.statusMsg = "turn running…"
	m.turnLive = true
	m.addMessage("user", "test B4 in-place, toi la Nam", "")
	for i := 0; i < 6; i++ {
		m.addMessage("tool", "→ read_file", "tool")
	}
	// No thinking placeholder (CA-537) and no assistant yet — the blank case.
	if m.thinkingIndex() >= 0 {
		t.Fatal("precondition: no thinking placeholder for normal chat")
	}
	if m.hasAssistantContent() {
		t.Fatal("precondition: no assistant content yet")
	}
	m2, _ := m.Update(turnStreamClosedMsg{})
	am := m2.(*AppModel)
	if !am.turnLive {
		t.Fatal("turnLive must stay true until turn_completed via orch")
	}
	if am.connStatus != ConnRunning {
		t.Fatalf("connStatus=%v want ConnRunning (keep busy), statusMsg=%q", am.connStatus, am.statusMsg)
	}
	// Must not have emitted the "no assistant text" error yet — that is for
	// the genuine done case (run-96217). The blank will be filled by orch.
	for _, msg := range am.messages {
		if msg.Role == "system" && strings.Contains(msg.Content, "no assistant text") {
			t.Fatalf("should not emit no-assistant error while live: %+v", msg)
		}
	}
	// Should have started orch listening (verified via state: runHandle present
	// and orch will be started by Update's Batch — we just ensure no panic and
	// still live).
}

func TestFirstTurnOrchTurnCompletedRendersReply(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.runHandle = &client.RunHandle{RunID: "run-417944", RunKind: "chat", ChatID: "cht_a"}
	m.connStatus = ConnRunning
	m.statusMsg = "turn running…"
	m.turnLive = true
	m.addMessage("user", "test B4", "")
	for i := 0; i < 6; i++ {
		m.addMessage("tool", "→ bash", "tool")
	}
	// Simulate orch delivering the late terminal.
	m2, _ := m.Update(turnStreamClosedMsg{})
	am := m2.(*AppModel)
	am.orchStream = &orchStreamState{evCh: make(<-chan client.ProviderEvent)}
	// Orch event: turn_completed with a real final message (opencode case).
	ev := client.ProviderEvent{Type: "turn_completed", FinalMessage: "Chao Nam! Da ghi nho HCM + Mi."}
	m3, _ := am.Update(orchStreamEventMsg{Ev: ev})
	am3 := m3.(*AppModel)
	if am3.turnLive {
		t.Fatal("turnLive must clear after turn_completed")
	}
	if am3.connStatus != ConnIdle || am3.statusMsg != "done" {
		t.Fatalf("after terminal status=%v msg=%q want done/Idle", am3.connStatus, am3.statusMsg)
	}
	if !am3.hasAssistantContent() {
		t.Fatalf("assistant must be rendered after orch terminal, messages=%+v", am3.messages)
	}
	last := am3.messages[len(am3.messages)-1]
	if last.Role != "assistant" || !strings.Contains(last.Content, "Chao Nam") {
		t.Fatalf("last message = %+v want assistant Chao Nam", last)
	}
}

// When the stream closed *with* thinking placeholder, the old run-96217 path
// still clears it and emits the notice — but only after turnLive is false.
// With turnLive true and thinking present, we keep busy, not emit yet.
func TestFirstTurnWithThinkingStaysLive(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	m.runHandle = &client.RunHandle{RunID: "run-1", RunKind: "chat", ChatID: "cht_a"}
	m.connStatus = ConnRunning
	m.turnLive = true
	m.addMessage("assistant", "thinking…", "thinking")
	m.addMessage("tool", "→ read_file", "tool")
	m2, _ := m.Update(turnStreamClosedMsg{})
	am := m2.(*AppModel)
	if !am.turnLive {
		t.Fatal("with thinking and live, must stay live")
	}
	if am.thinkingIndex() < 0 {
		t.Fatal("thinking placeholder must remain while live")
	}
}

// Provider-agnostic: blank-first-turn logic is in the TUI, not per-provider.
func TestFirstTurnBlankIsProviderAgnostic(t *testing.T) {
	for _, prov := range []string{"grok", "codex", "claude", "opencode"} {
		t.Run(prov, func(t *testing.T) {
			m := New(config.ChatConfig{}, "http://127.0.0.1:1")
			m.provider = prov
			m.runHandle = &client.RunHandle{RunID: "run-1", RunKind: "chat", ChatID: "cht_a", ProviderKey: prov}
			m.connStatus = ConnRunning
			m.turnLive = true
			m.addMessage("user", "hello", "")
			for i := 0; i < 3; i++ {
				m.addMessage("tool", "→ tool", "tool")
			}
			m2, _ := m.Update(turnStreamClosedMsg{})
			am := m2.(*AppModel)
			if !am.turnLive || am.connStatus != ConnRunning {
				t.Fatalf("%s: must stay live, got live=%v status=%v", prov, am.turnLive, am.connStatus)
			}
			am.orchStream = &orchStreamState{evCh: make(<-chan client.ProviderEvent)}
			ev := client.ProviderEvent{Type: "turn_completed", FinalMessage: "hi from " + prov}
			m3, _ := am.Update(orchStreamEventMsg{Ev: ev})
			if m3.(*AppModel).turnLive {
				t.Fatalf("%s: must clear live after terminal", prov)
			}
			if !strings.Contains(m3.(*AppModel).lastAssistantText(), prov) {
				t.Fatalf("%s: assistant not rendered", prov)
			}
		})
	}
}
