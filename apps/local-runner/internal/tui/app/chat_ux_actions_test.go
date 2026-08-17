package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestIsPromptNewlineKey_CtrlEnterAndShiftEnter(t *testing.T) {
	if !isPromptNewlineKey(tea.KeyMsg{Type: tea.KeyCtrlJ}) {
		t.Fatal("ctrl+j should insert a newline")
	}
	if isPromptNewlineKey(tea.KeyMsg{Type: tea.KeyEnter}) {
		t.Fatal("plain Enter must still send")
	}
	if !isPromptNewlineKey(tea.KeyMsg{Type: tea.KeyEnter, Alt: true}) {
		t.Fatal("alt/option+enter should insert a newline")
	}
}

func TestHandleKey_CtrlJInsertsNewline(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m.inputValue = "hello"
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlJ})
	if got := m2.(*AppModel).inputValue; got != "hello\n" {
		t.Fatalf("inputValue=%q", got)
	}
}

func TestProcessInput_NoThinkingChatRow(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex", ProjectPath: t.TempDir()}, "http://127.0.0.1:9")
	m.sessionLoading = false
	m2, _ := m.processInput("hello")
	am := m2.(*AppModel)
	if am.statusMsg != "thinking…" {
		t.Fatalf("statusMsg=%q", am.statusMsg)
	}
	for _, msg := range am.messages {
		if msg.Role == "assistant" && msg.FormatHint == "thinking" {
			t.Fatalf("processInput must not add a thinking chat message: %+v", msg)
		}
	}
	if !am.workIsLive() {
		t.Fatal("a sent turn must count as live work (status spinner)")
	}
}

func TestProcessInput_ApproveDoesNotStartTurn(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m.runHandle = &client.RunHandle{RunID: "run-1"}
	m.connStatus = ConnWaiting
	m.approval = &ApprovalState{ID: "ap-1", RunID: "run-1"}
	m2, cmd := m.processInput("hello there")
	am := m2.(*AppModel)
	if cmd != nil {
		t.Fatal("chat turn during pending approval would 409")
	}
	if am.pendingPrompt != "" {
		t.Fatal("must not arm a new turn")
	}
	if am.approval == nil || am.approval.ID != "ap-1" {
		t.Fatal("approval must stay pending until POST /decision succeeds")
	}
}

func TestProcessInput_ApproveWordSubmitsWithoutClearing(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m.approval = &ApprovalState{ID: "ap-9", RunID: "run-1"}
	m2, cmd := m.processInput("approve")
	am := m2.(*AppModel)
	if cmd == nil {
		t.Fatal("expected SubmitApproval cmd")
	}
	if am.approval == nil {
		t.Fatal("must not clear approval until the API succeeds")
	}
}

func TestSlashApprove_KeepsPendingUntilResolved(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.approval = &ApprovalState{ID: "ap-2"}
	m2, cmd := m.handleSlashCommand("/approve")
	am := m2.(*AppModel)
	if cmd == nil {
		t.Fatal("expected approve cmd")
	}
	if am.approval == nil {
		t.Fatal("cleared too early — a failed POST then sends a new turn (409)")
	}
}

func TestCtrlC_StopsTurnInsteadOfQuit(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m.runHandle = &client.RunHandle{RunID: "run-1"}
	m.connStatus = ConnRunning
	_, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected stop cmd")
	}
	msg := cmd()
	if _, ok := msg.(QuitMsg); ok {
		t.Fatal("Ctrl-C while running must not quit the TUI")
	}
}

func TestApprovalBarAndStopAreClickable(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.asciiMode = true
	m.sessionLoading = false
	m.approval = &ApprovalState{ID: "ap-1"}
	m.runHandle = &client.RunHandle{RunID: "run-1"}
	m.connStatus = ConnWaiting
	if _, _, ok := findClickTarget(m, "approve"); !ok {
		t.Fatal("expected clickable /approve")
	}
	if _, _, ok := findClickTarget(m, "deny"); !ok {
		t.Fatal("expected clickable /deny")
	}
	if _, _, ok := findClickTarget(m, "stop"); !ok {
		t.Fatal("expected clickable [stop]")
	}
}

func TestCopyChipIsClickable(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.asciiMode = true
	m.sessionLoading = false
	m.addMessage("assistant", "hello from the model", "")
	if _, _, ok := findClickTarget(m, "copy:0"); !ok {
		t.Fatal("expected clickable [copy] on the answer")
	}
}

func TestRenderMarkdown_HeadingsAndCode(t *testing.T) {
	lines := renderMarkdown("# Title\n\nUse `code` and **bold**.\n\n```\nfn()\n```", 80)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(stripANSI(joined), "Title") {
		t.Fatalf("missing heading: %q", joined)
	}
	if !strings.Contains(stripANSI(joined), "fn()") {
		t.Fatalf("missing fenced code: %q", joined)
	}
	if !strings.Contains(stripANSI(joined), "bold") {
		t.Fatalf("missing bold text: %q", joined)
	}
}

func TestImagePathFromClipboardText(t *testing.T) {
	if imagePathFromClipboardText("hello") != "" {
		t.Fatal("plain text is not an image path")
	}
}
