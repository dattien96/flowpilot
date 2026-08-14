package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestResolveQuestionChoice_NumberAndApproveAlias(t *testing.T) {
	opts := []map[string]string{
		{"label": "Approve — tạo commit demo"},
		{"label": "Deny — không làm gì"},
		{"label": "Chỉ show command, không chạy"},
	}
	if got := resolveQuestionChoice("1", opts); got != "Approve — tạo commit demo" {
		t.Fatalf("1 => %q", got)
	}
	if got := resolveQuestionChoice("approve", opts); !strings.HasPrefix(got, "Approve") {
		t.Fatalf("approve => %q", got)
	}
	if got := resolveQuestionChoice("/deny", opts); !strings.HasPrefix(got, "Deny") {
		t.Fatalf("/deny => %q", got)
	}
	if got := resolveQuestionChoice("3", opts); !strings.Contains(got, "show command") {
		t.Fatalf("3 => %q", got)
	}
}

func TestProcessInput_QuestionDoesNotStartTurn(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m.runHandle = &client.RunHandle{RunID: "run-1"}
	m.connStatus = ConnWaiting
	m.question = &QuestionState{
		ID:      "q-1",
		Prompt:  "Demo approval gate",
		Options: []map[string]string{{"label": "Approve — commit"}, {"label": "Deny — skip"}},
	}
	m2, cmd := m.processInput("approve")
	am := m2.(*AppModel)
	if cmd == nil {
		t.Fatal("expected AnswerQuestion cmd")
	}
	if am.question == nil || am.question.ID != "q-1" {
		t.Fatal("must not clear question until POST /answer succeeds")
	}
	if am.pendingPrompt != "" {
		t.Fatal("must not start a new turn while a question is pending")
	}
}

func TestSlashApprove_AnswersPendingQuestion(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.question = &QuestionState{
		ID:      "q-2",
		Options: []map[string]string{{"label": "Approve"}, {"label": "Deny"}},
	}
	m2, cmd := m.handleSlashCommand("/approve")
	am := m2.(*AppModel)
	if cmd == nil {
		t.Fatal("expected answer cmd")
	}
	if am.question == nil {
		t.Fatal("cleared too early")
	}
}

func TestQuestionBarIsClickable(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 24
	m.asciiMode = true
	m.sessionLoading = false
	m.question = &QuestionState{
		ID: "q-3",
		Options: []map[string]string{
			{"label": "Approve — commit"},
			{"label": "Deny — skip"},
			{"label": "Chỉ show command, không chạy"},
		},
	}
	if _, _, ok := findClickTarget(m, "approve"); !ok {
		t.Fatal("expected clickable Approve")
	}
	if _, _, ok := findClickTarget(m, "deny"); !ok {
		t.Fatal("expected clickable Deny")
	}
	if _, _, ok := findClickTarget(m, "qopt:2"); !ok {
		t.Fatal("expected clickable option 3")
	}
}

func TestQuestionResolved_ClearsPendingAndUnblocks(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.question = &QuestionState{ID: "q-4"}
	m.connStatus = ConnWaiting
	m2, _ := m.Update(QuestionResolvedMsg{ID: "q-4", Choice: "Approve — commit"})
	am := m2.(*AppModel)
	if am.question != nil {
		t.Fatal("question should clear after a successful answer")
	}
	if am.connStatus != ConnRunning {
		t.Fatalf("status=%v", am.connStatus)
	}
}

func TestMouseClick_ClearsShiftSelection(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.asciiMode = true
	m.sessionLoading = false
	m.mouseSel = mouseSelect{armed: true, x0: 0, y0: 8, x1: 20, y1: 10}
	m2, _ := m.Update(clickLeft(2, 12))
	am := m2.(*AppModel)
	if !am.mouseSel.empty() {
		t.Fatal("click outside should clear shift selection")
	}
}

func TestMouseRelease_ClearsSelectionOutsideRange(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.asciiMode = true
	m.sessionLoading = false
	m.mouseSel = mouseSelect{armed: true, x0: 2, y0: 6, x1: 18, y1: 8}
	m2, _ := m.Update(tea.MouseMsg{X: 4, Y: 20, Action: tea.MouseActionRelease, Button: tea.MouseButtonLeft})
	if !m2.(*AppModel).mouseSel.empty() {
		t.Fatal("release outside the highlight must clear it")
	}
}

func TestMouseClick_InsideSelectionAlsoClears(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.asciiMode = true
	m.sessionLoading = false
	m.mouseSel = mouseSelect{armed: true, x0: 0, y0: 8, x1: 40, y1: 10}
	m2, _ := m.Update(clickLeft(10, 9))
	if !m2.(*AppModel).mouseSel.empty() {
		t.Fatal("non-shift click must clear highlight, including inside the range")
	}
}

func TestShiftDrag_SelectsTranscript(t *testing.T) {
	m := New(config.ChatConfig{Provider: "grok"}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m2, _ := m.handleMouse(tea.MouseMsg{
		X: 4, Y: 6, Shift: true, Action: tea.MouseActionPress, Button: tea.MouseButtonLeft,
	})
	am := m2.(*AppModel)
	m3, _ := am.handleMouse(tea.MouseMsg{
		X: 10, Y: 9, Shift: true, Action: tea.MouseActionMotion, Button: tea.MouseButtonLeft,
	})
	am = m3.(*AppModel)
	if am.mouseSel.empty() || am.mouseSel.y1 != 9 {
		t.Fatalf("shift-drag should arm selection: %+v", am.mouseSel)
	}
}

func TestEscape_ClearsSelectionBeforePrompt(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.sessionLoading = false
	m.inputValue = "draft"
	m.mouseSel = mouseSelect{armed: true, y0: 1, y1: 3}
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEscape})
	am := m2.(*AppModel)
	if !am.mouseSel.empty() {
		t.Fatal("Esc should clear selection first")
	}
	if am.inputValue != "draft" {
		t.Fatal("Esc should not also wipe the prompt on the same press")
	}
}
