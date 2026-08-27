package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-633: TUI "hang" residual — cold start forced the F2 sidebar open
// (ConnectedMsg set Collapsed=false; zero value already false) and every
// 530ms cursor tick rebuilt the 126×50 chat+sidebar cellbuf (~300ms+ with
// sessions/steps). Hover motion + keys share the Windows conhost 64-slot
// queue, so keys never reached Update (logs 16512/24144: 0 KeyMsg after
// session ready). Old tests never saw this: CA-610 injects KeyMsg straight
// into Update and CA-621 measures View with the sidebar off (hasContent
// false), so the expensive compose path was never exercised end to end.
//
// Fix: ConnectedMsg collapses the sidebar (F2 / flow auto-open still expand
// it) and composeCellBuf — a pure function of its inputs — is cached so idle
// frames (caret pinned steady) skip the heavy cellbuf merge entirely.

func TestCA633_ConnectedMsg_StartsSidebarCollapsed_ClaudeCodexGrok(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.width, m.height = 126, 50
			m2, _ := m.Update(ConnectedMsg{RunnerURL: "http://127.0.0.1:4317"})
			am := m2.(*AppModel)
			if am.useRightSidebar() {
				t.Fatalf("[%s] cold start must NOT force the F2 sidebar open", pk)
			}
			// F2 still toggles it open (existing contract).
			m3, _ := am.handleKey(tea.KeyMsg{Type: tea.KeyF2})
			if !m3.(*AppModel).useRightSidebar() {
				t.Fatalf("[%s] F2 must still open the sidebar", pk)
			}
			m4, _ := m3.(*AppModel).handleKey(tea.KeyMsg{Type: tea.KeyF2})
			if m4.(*AppModel).useRightSidebar() {
				t.Fatalf("[%s] F2 must still collapse the sidebar", pk)
			}
		})
	}
}

func TestCA633_KeyAfterSessionReady_Inserts(t *testing.T) {
	// The exact cold-start bridge: connect → defaults → key must reach the
	// composer with the sidebar collapsed (was 0 KeyMsg for minutes on live
	// Windows terminals, log 24144).
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.width, m.height = 126, 50
			m2, _ := m.Update(ConnectedMsg{RunnerURL: "http://127.0.0.1:4317"})
			am := m2.(*AppModel)
			m3, _ := am.Update(SessionDefaultsMsg{Provider: pk, Model: "m", Projects: []client.Project{{ID: "p1", Name: "proj"}}})
			am = m3.(*AppModel)
			if am.sessionLoading {
				t.Fatalf("[%s] sessionLoading must clear after defaults", pk)
			}
			m4, _ := am.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
			am = m4.(*AppModel)
			if am.inputValue != "a" {
				t.Fatalf("[%s] key after ready must insert, got %q", pk, am.inputValue)
			}
			if !strings.Contains(am.View(), "a") {
				t.Fatalf("[%s] View must show typed key", pk)
			}
		})
	}
}

func TestCA633_IdleCursorTick_SkipsComposeCache(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 126, 50
	m.sessionLoading = false
	m.sessionDefaultsLoaded = true
	m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
	m.sessionPanel.Collapsed = false // sidebar ON → compose path exercised
	for i := 0; i < 10; i++ {
		m.addMessage("assistant", "line", "")
	}
	m.View()
	if m.composeBuilds != 1 {
		t.Fatalf("first View must compose once, builds=%d", m.composeBuilds)
	}
	// First idle tick pins the caret on → one recompose renders it steady.
	m2, _ := m.Update(cursorTickMsg{})
	am := m2.(*AppModel)
	if !am.cursorOn {
		t.Fatal("idle caret must be pinned on (steady, no blink)")
	}
	pinned := am.View()
	if am.composeBuilds != 2 {
		t.Fatalf("caret pin must recompose once, builds=%d", am.composeBuilds)
	}
	// Subsequent idle ticks: caret steady → chatRaw identical → cache hit.
	m3, _ := am.Update(cursorTickMsg{})
	am = m3.(*AppModel)
	cached := am.View()
	if am.composeBuilds != 2 {
		t.Fatalf("idle View must hit compose cache, builds=%d", am.composeBuilds)
	}
	if cached != pinned {
		t.Fatal("cached compose output must equal the pinned-caret frame")
	}
	// Repeated idle ticks: still zero compose calls.
	for i := 0; i < 3; i++ {
		am2, _ := am.Update(cursorTickMsg{})
		am = am2.(*AppModel)
		_ = am.View()
	}
	if am.composeBuilds != 2 {
		t.Fatalf("idle ticks must never recompose, builds=%d", am.composeBuilds)
	}
}

func TestCA633_TypingRebuildsCompose(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 126, 50
	m.sessionLoading = false
	m.sessionDefaultsLoaded = true
	m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
	m.sessionPanel.Collapsed = false
	m.View() // compose once

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	am := m2.(*AppModel)
	view := am.View()
	if am.composeBuilds != 2 {
		t.Fatalf("typing must recompose, builds=%d", am.composeBuilds)
	}
	if !strings.Contains(view, "a") {
		t.Fatalf("recomposed frame must show typed char, got:\n%s", view)
	}
}

func TestCA633_LiveCursorStillRecomposes(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 126, 50
	m.sessionLoading = false
	m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
	m.sessionPanel.Collapsed = false
	m.turnStream = &turnStreamState{}
	m.View() // compose once

	// Live caret blinks → chatRaw changes every tick → compose rebuilds.
	m2, _ := m.Update(cursorTickMsg{})
	am := m2.(*AppModel)
	_ = am.View()
	if am.composeBuilds != 2 {
		t.Fatalf("live cursor tick must recompose, builds=%d", am.composeBuilds)
	}
}

func TestCA633_BlockedBar_StillRenderedAfterCompose(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 126, 50
	m.asciiMode = true
	m.mode = ModeFlow
	m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
	m.sessionPanel.Collapsed = false
	m.View()
	m2, _ := m.Update(AgentGraphMsg{Graph: &client.AgentGraphSnapshot{
		LoopState: client.AgentLoopState{Status: "blocked", BlockReason: "escalate", GateReason: "codex account missing"},
		Runs:      []client.AgentRunSummary{{RunID: "run-x", AgentName: "main", Status: "completed"}},
	}})
	am := m2.(*AppModel)
	view := stripANSI(am.View())
	if !strings.Contains(view, "[Retry]") || !strings.Contains(view, "codex account missing") {
		t.Fatalf("blocked bar must render after recompose, got:\n%s", view)
	}
	// Blocked caret stays meaningful → keeps recomposing (fresh chips).
	m3, _ := am.Update(cursorTickMsg{})
	am = m3.(*AppModel)
	_ = am.View()
	if am.composeBuilds < 3 {
		t.Fatalf("blocked caret must keep recomposing, builds=%d", am.composeBuilds)
	}
}