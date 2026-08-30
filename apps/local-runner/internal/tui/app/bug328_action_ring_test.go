package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func bug328Model() *AppModel {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.authPhase = AuthNone
	m.sessionDefaultsLoaded = true
	m.runHandle = &client.RunHandle{RunID: "run-1", Status: "blocked"}
	return m
}

func TestBug328_BlockedFlow_EmptyEnterActivatesRetry(t *testing.T) {
	m := driftBlockedModel("grok", "")
	m2, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("empty Enter on parked blocked flow must dispatch continue")
	}
	if m2 == nil {
		t.Fatal("must return model")
	}
}

func TestBug328_BlockedFlow_RightThenEnterStop(t *testing.T) {
	m := driftBlockedModel("grok", "")
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRight})
	am := m2.(*AppModel)
	if am.actionRingIdx != 1 {
		t.Fatalf("Right must highlight [Stop] (idx 1), got %d", am.actionRingIdx)
	}
	_, cmd := am.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter on [Stop] must dispatch stop")
	}
}

func TestBug328_Approval_DigitOneActivates(t *testing.T) {
	m := bug328Model()
	m.approval = &ApprovalState{ID: "ap-1", RunID: "run-1"}
	m2, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'1'}})
	if cmd == nil {
		t.Fatal("1 on empty input with approval card must activate first action")
	}
	if m2 == nil {
		t.Fatal("must return model")
	}
}
