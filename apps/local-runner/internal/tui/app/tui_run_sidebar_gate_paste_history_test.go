package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-534 — Run ID in the sidebar, thinking animation in flow chrome (incl.
// after a gate decision), text-first clipboard paste, and Up/Down prompt
// history. Provider-agnostic logic, parameterized over claude/codex/grok.

func flowThinkingModel(pk string) *AppModel {
	m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf", Label: "grok-flow"}
	m.runHandle = &client.RunHandle{RunID: "run-102521", Status: "running"}
	return m
}

func flowThinkingRows(m *AppModel) []string {
	var out []string
	for _, r := range m.chatRows() {
		if strings.Contains(stripANSI(r.Text), "Thinking") {
			out = append(out, stripANSI(r.Text))
		}
	}
	return out
}

func TestFlowMode_ShowsThinkingPlaceholder(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := flowThinkingModel(pk)
			m2, _ := m.processInput("continue the flow")
			am := m2.(*AppModel)
			rows := flowThinkingRows(am)
			if len(rows) == 0 {
				t.Fatalf("flow chat must show an animated thinking row:\n%s", m.View())
			}
			if !strings.Contains(rows[0], "Thinking") || !strings.Contains(rows[0], "0s") {
				t.Fatalf("expected spinner + elapsed: %q", rows[0])
			}
		})
	}
}

func TestGateDecision_ShowsThinkingPlaceholder(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := flowThinkingModel(pk)
			m.gate = &GateState{Options: []string{"continue", "stop"}, RunID: "run-102521"}
			m2, cmd := m.handleGateInput("1")
			am := m2.(*AppModel)
			if am.gate != nil {
				t.Fatal("gate must clear after a decision")
			}
			if cmd == nil {
				t.Fatal("expected gate submit cmd")
			}
			rows := flowThinkingRows(am)
			if len(rows) == 0 {
				t.Fatal("after a gate decision the flow keeps running and must show thinking")
			}
			if am.statusMsg != "thinking…" {
				t.Fatalf("statusMsg=%q want thinking…", am.statusMsg)
			}
		})
	}
}

func TestGateDecision_InvalidKeepsGateNoThinking(t *testing.T) {
	m := flowThinkingModel("grok")
	m.gate = &GateState{Options: []string{"continue", "stop"}, RunID: "run-102521"}
	m2, _ := m.handleGateInput("bogus")
	am := m2.(*AppModel)
	if am.gate == nil {
		t.Fatal("gate must stay pending on an invalid decision")
	}
	if len(flowThinkingRows(am)) != 0 {
		t.Fatal("invalid gate input must not add a thinking placeholder")
	}
}

func TestSessionPanel_ShowsRunID(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := flowThinkingModel(pk)
			m.refreshSessionPanel()
			found := false
			for _, line := range m.sessionPanel.lines() {
				if strings.HasPrefix(line, "Run: run-102521") {
					found = true
				}
			}
			if !found {
				t.Fatalf("sidebar must show the current Run ID, got %v", m.sessionPanel.lines())
			}
		})
	}
}

func TestSessionPanel_RunIDUpdatesWithHandle(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.sessionPanel.RunnerURL = "http://127.0.0.1:4317"
	m.refreshSessionPanel()
	for _, line := range m.sessionPanel.lines() {
		if strings.HasPrefix(line, "Run:") {
			t.Fatalf("no run yet must not show Run line: %v", m.sessionPanel.lines())
		}
	}
	m.runHandle = &client.RunHandle{RunID: "run-77777"}
	m.refreshSessionPanel()
	found := false
	for _, line := range m.sessionPanel.lines() {
		if strings.HasPrefix(line, "Run: run-77777") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Run line must appear once a run exists: %v", m.sessionPanel.lines())
	}
}

func TestRightSidebar_IncludesRunID(t *testing.T) {
	m := flowThinkingModel("grok")
	m.width, m.height = 120, 30
	m.fullWidth = 120
	m.sessionPanel.Collapsed = false
	m.refreshSessionPanel()
	lines := m.renderRightSidebar(m.height)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "run-102521") {
		t.Fatalf("right sidebar must include the Run ID:\n%s", joined)
	}
}

func TestPromptHistory_UpDownRecall(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m.runHandle = &client.RunHandle{RunID: "run-hist"}
	m.processInput("first question")
	m.connStatus = ConnIdle
	m.processInput("second question")
	m.connStatus = ConnIdle

	m.inputValue = "draft"
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyUp})
	if got := m2.(*AppModel).inputValue; got != "second question" {
		t.Fatalf("Up from draft=%q want newest prompt", got)
	}
	m3, _ := m2.(*AppModel).handleKey(tea.KeyMsg{Type: tea.KeyUp})
	if got := m3.(*AppModel).inputValue; got != "first question" {
		t.Fatalf("second Up=%q want older prompt", got)
	}
	m4, _ := m3.(*AppModel).handleKey(tea.KeyMsg{Type: tea.KeyDown})
	if got := m4.(*AppModel).inputValue; got != "second question" {
		t.Fatalf("Down after older=%q want newer prompt", got)
	}
	m5, _ := m4.(*AppModel).handleKey(tea.KeyMsg{Type: tea.KeyDown})
	am := m5.(*AppModel)
	if got := am.inputValue; got != "draft" {
		t.Fatalf("Down at newest must restore the draft, got %q", got)
	}
	if am.promptHistIdx != -1 {
		t.Fatalf("draft restore must reset browse index, got %d", am.promptHistIdx)
	}
}

func TestPromptHistory_ExcludesSlashAndDedupes(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m.runHandle = &client.RunHandle{RunID: "run-hist"}
	m.processInput("/help")
	m.processInput("hello")
	m.connStatus = ConnIdle
	m.processInput("hello")
	if len(m.promptHistory) != 1 || m.promptHistory[0] != "hello" {
		t.Fatalf("history must skip slash + collapse duplicates: %v", m.promptHistory)
	}
}

func TestPromptHistory_NoHistoryKeepsInput(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.inputValue = "kept"
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyUp})
	if got := m2.(*AppModel).inputValue; got != "kept" {
		t.Fatalf("Up with empty history must not clobber the draft, got %q", got)
	}
}

func TestClipboardPasteMsg_InsertsTextAtCursor(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.inputValue = "hello"
	m2, _ := m.Update(ClipboardPasteMsg{Text: " world", NoImage: true})
	am := m2.(*AppModel)
	if am.inputValue != "hello world" {
		t.Fatalf("text paste must insert into the composer, got %q", am.inputValue)
	}
}

func TestPromptHistory_NewSendResetsBrowse(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m.runHandle = &client.RunHandle{RunID: "run-hist"}
	m.processInput("a")
	m.connStatus = ConnIdle
	m.processInput("b")
	m.connStatus = ConnIdle
	m.inputValue = "x"
	m.handleKey(tea.KeyMsg{Type: tea.KeyUp}) // browse to newest "b"
	if m.promptHistIdx != 1 {
		t.Fatalf("browse index=%d want 1 (newest)", m.promptHistIdx)
	}
	m.processInput("fresh")
	if m.promptHistIdx != -1 {
		t.Fatalf("sending a new prompt must reset browsing, got %d", m.promptHistIdx)
	}
	if n := len(m.promptHistory); n != 3 {
		t.Fatalf("history=%v want 3 entries", m.promptHistory)
	}
}
