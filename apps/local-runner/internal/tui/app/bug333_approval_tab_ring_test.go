package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
)

// BUG-333 (operator report, E1 re-test): the approval gate card shows
// Approve/Deny but pressing Tab does not move the keyboard selection — the
// Tab-cycles-ring mechanism (BUG-328) must work for approval cards exactly
// like ←/→, including opencode approvals that carry typed Decisions.

func bug333ApprovalModel() *AppModel {
	m := bug328Model()
	m.approval = &ApprovalState{
		ID:    "appr-1",
		RunID: "run-1",
		Decisions: []client.ApprovalDecisionOption{
			{Value: "approve", Label: "Approve"},
			{Value: "deny", Label: "Deny"},
		},
	}
	m.syncActionRingCard()
	return m
}

func TestBug333_Approval_TabCyclesRing(t *testing.T) {
	m := bug333ApprovalModel()
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	am := m2.(*AppModel)
	if am.actionRingIdx != 1 {
		t.Fatalf("Tab must cycle the approval ring to idx 1 (Deny), got idx=%d focus=%v sugg=%d", am.actionRingIdx, am.actionRingFocus, len(am.collectSuggestions()))
	}
	m3, _ := am.Update(tea.KeyMsg{Type: tea.KeyTab})
	am3 := m3.(*AppModel)
	if am3.actionRingIdx != 0 {
		t.Fatalf("second Tab must wrap to idx 0 (Approve), got %d", am3.actionRingIdx)
	}
}

func TestBug333_Approval_ShiftTabCyclesBack(t *testing.T) {
	m := bug333ApprovalModel()
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	am := m2.(*AppModel)
	if am.actionRingIdx != 1 {
		t.Fatalf("Shift+Tab must cycle back to idx 1 (Deny), got %d", am.actionRingIdx)
	}
}

func TestBug333_Approval_TabThenEnterDenies(t *testing.T) {
	m := bug333ApprovalModel()
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	am := m2.(*AppModel)
	_, cmd := am.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter on the Tab-selected ring item must dispatch the decision")
	}
}

// bug333PopulatedModel returns the approval model with every passive
// suggestion source populated (history, remote chats, flows, providers,
// accounts, workspace files) — the closest unit approximation of the live
// session where the ring keys died.
func bug333PopulatedModel() *AppModel {
	m := bug333ApprovalModel()
	m.project = &client.Project{ID: "p1"}
	m.chatList = []client.RunHistoryItem{{RunID: "run-old", LastPrompt: "old chat"}}
	m.remoteChatList = []client.RemoteChatSessionSummary{{RunID: "run-remote", SourceRunID: "remote chat"}}
	m.flowBuiltins = []client.BuiltinFlowOption{{FlowRef: "pack/flow-1", Label: "Flow One"}}
	m.providers = []client.Provider{{Key: "codex", Models: []client.ProviderModel{{ID: "o3"}}}}
	m.providerAccounts = []client.ProviderAccountSummary{{ProviderKey: "codex", DisplayLabel: "acc"}}
	m.workspaceFiles = []string{"main.go"}
	return m
}

func TestBug333_Approval_TabCyclesEvenWithSuggestionSourcesLoaded(t *testing.T) {
	m := bug333PopulatedModel()
	if m.actionRingKeysActive() == false {
		t.Fatal("a pending approval with empty input must keep the ring keys active")
	}
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	am := m2.(*AppModel)
	if am.actionRingIdx != 1 {
		t.Fatalf("Tab must cycle the ring (idx 1) even when suggestion sources are loaded, got %d", am.actionRingIdx)
	}
	if am.approval == nil {
		t.Fatal("approval card must still be mounted")
	}
}

func TestBug333_Approval_ArrowsCycleEvenWithSuggestionSourcesLoaded(t *testing.T) {
	m := bug333PopulatedModel()
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	am := m2.(*AppModel)
	if am.actionRingIdx != 1 {
		t.Fatalf("Right must cycle the ring, got %d", am.actionRingIdx)
	}
	m3, _ := am.Update(tea.KeyMsg{Type: tea.KeyLeft})
	am3 := m3.(*AppModel)
	if am3.actionRingIdx != 0 {
		t.Fatalf("Left must cycle back, got %d", am3.actionRingIdx)
	}
}

func TestBug333_Approval_TypingStillGetsSuggestions(t *testing.T) {
	m := bug333PopulatedModel()
	m.inputValue = "/ap"
	m.syncTextareaValue()
	if m.actionRingKeysActive() {
		t.Fatal("typing with a card pending must hand keys back to the composer")
	}
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	am := m2.(*AppModel)
	if am.actionRingIdx != 0 {
		t.Fatalf("Tab while typing must NOT cycle the ring, got idx %d", am.actionRingIdx)
	}
}

func TestBug333_Approval_TabNeverCyclesPostureWhileCardPending(t *testing.T) {
	m := bug333PopulatedModel()
	m.sessionDefaultsLoaded = true
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	am := m2.(*AppModel)
	if am.chatPosturePending != "" {
		t.Fatalf("Tab with a pending approval must never schedule a posture apply, got %q", am.chatPosturePending)
	}
	if am.actionRingIdx != 1 {
		t.Fatalf("Tab must cycle the ring, got %d", am.actionRingIdx)
	}
}

func TestBug333_Approval_HighlightVisibleWhileCardPending(t *testing.T) {
	m := bug333PopulatedModel()
	if got := m.ringHighlightFor("approval"); got != 0 {
		t.Fatalf("approval surface must highlight idx 0 while the card is pending, got %d", got)
	}
}
