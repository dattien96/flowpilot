// CP-56 extended app tests — test signatures A7.1-A9.5.
// These tests exercise the Bubble Tea model directly without a live runner.
package app_test

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/app"
	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// ---- A7: AppModel state -----------------------------------------------------

func TestA7_1_InitialState_RendersWithoutPanic(t *testing.T) {
	m := app.New(config.ChatConfig{Provider: "codex", Model: "o3"}, "http://127.0.0.1:4317")
	if m == nil {
		t.Fatal("New returned nil")
	}
	view := m.View()
	if view == "" {
		t.Error("View() returned empty string on fresh model")
	}
}

func TestA7_2_InitialState_YoloTrueShowsIndicator(t *testing.T) {
	m := app.New(config.ChatConfig{Provider: "grok", Yolo: true}, "http://127.0.0.1:4317")
	if !strings.Contains(m.View(), "YOLO") {
		t.Errorf("View missing YOLO indicator:\n%s", m.View())
	}
}

func TestA7_3_WindowResize_NoPanic(t *testing.T) {
	m := app.New(config.ChatConfig{}, "http://127.0.0.1:4317")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	if updated == nil {
		t.Fatal("Update returned nil")
	}
	_ = updated.View()
}

func TestA7_4_ConnectedMsg_SetsIdleStatus(t *testing.T) {
	m := app.New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m2, _ := m.Update(app.ConnectedMsg{RunnerURL: "http://127.0.0.1:4317"})
	view := m2.(*app.AppModel).View()
	if !strings.Contains(view, "connected") && !strings.Contains(view, "idle") {
		t.Errorf("expected 'connected' or 'idle' in view:\n%s", view)
	}
}

func TestA7_5_RunStartedMsg_ShowsRunID(t *testing.T) {
	m := app.New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m2, _ := m.Update(app.RunStartedMsg{Handle: client.RunHandle{
		RunID: "run-abcdefgh", ProviderKey: "codex", Status: "running",
	}})
	view := m2.(*app.AppModel).View()
	if !strings.Contains(view, "run-abcd") {
		t.Errorf("expected short run ID in view:\n%s", view)
	}
}

func TestA7_6_TurnDoneMsg_ChatMode_DoesNotQuit(t *testing.T) {
	m := app.New(config.ChatConfig{Provider: "codex", Print: false}, "http://127.0.0.1:4317")
	_, cmd := m.Update(app.TurnDoneMsg{FinalMsg: "done"})
	if cmd != nil {
		// cmd() should NOT return tea.Quit in non-print mode
		msg := cmd()
		if _, isQuit := msg.(tea.QuitMsg); isQuit {
			t.Error("TurnDoneMsg in chat mode should not produce tea.Quit")
		}
	}
}

func TestA7_7_TurnDoneMsg_PrintMode_Quits(t *testing.T) {
	m := app.New(config.ChatConfig{Provider: "codex", Print: true}, "http://127.0.0.1:4317")
	_, cmd := m.Update(app.TurnDoneMsg{FinalMsg: "response"})
	if cmd == nil {
		t.Fatal("expected Quit command in Print mode")
	}
}

func TestA7_8_ErrMsg_ShowsInView(t *testing.T) {
	m := app.New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m2, _ := m.Update(app.ErrMsg{Err: fmt.Errorf("connection refused at port 4317")})
	view := m2.(*app.AppModel).View()
	if !strings.Contains(view, "connection refused") {
		t.Errorf("expected error in view:\n%s", view)
	}
}

func TestA7_9_TokenUsageEvent_UpdatesStatusline(t *testing.T) {
	m := app.New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	// Simulate RunStarted
	m2, _ := m.Update(app.RunStartedMsg{Handle: client.RunHandle{RunID: "r1", Status: "running"}})
	m = m2.(*app.AppModel)
	// Simulate token_usage_updated event via EventMsg
	totalTokens := int64(5000)
	m3, _ := m.Update(app.EventMsg{Ev: client.ProviderEvent{
		Type: "token_usage_updated",
		TokenUsage: &client.TokenUsageSnapshot{
			Last: &client.TokenUsageBreakdown{TotalTokens: totalTokens},
		},
	}})
	view := m3.(*app.AppModel).View()
	// Without a context window, statusline shows last-turn tokens.
	if !strings.Contains(view, "last 5.0k") {
		t.Errorf("expected 'last 5.0k' in statusline after token usage update:\n%s", view)
	}
}

func TestA7_10_AgentGraphEvent_UpdatesAgentRuns(t *testing.T) {
	m := app.New(config.ChatConfig{}, "http://127.0.0.1:4317")
	// Enable agents focus first.
	m2, _ := m.Update(app.EventMsg{Ev: client.ProviderEvent{
		Type: "agent_graph_updated",
		AgentGraph: &client.AgentGraphSnapshot{
			Runs: []client.AgentRunSummary{
				{RunID: "r-main", AgentName: "main", Status: "running"},
				{RunID: "r-sub", AgentName: "sub-agent", Status: "running"},
			},
		},
	}})
	// After the update, Tab should cycle agents (state is internal but View should work).
	_ = m2.(*app.AppModel).View()
}

func TestA7_11_GateViolation_ShowsOptionsInView(t *testing.T) {
	m := app.New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m2, _ := m.Update(app.EventMsg{Ev: client.ProviderEvent{
		Type:               "flow_gate_violation",
		GateOptions:        []string{"continue", "stop"},
		GateRegressedTests: []string{"TestFoo", "TestBar"},
		WorkflowRunID:      "run-gate",
	}})
	view := m2.(*app.AppModel).View()
	if !strings.Contains(view, "continue") || !strings.Contains(view, "stop") {
		t.Errorf("expected gate options in view:\n%s", view)
	}
}

func TestA7_12_Tab_CyclesAgentFocus(t *testing.T) {
	m := app.New(config.ChatConfig{}, "http://127.0.0.1:4317")
	// Inject agent runs via AgentGraphMsg.
	m2, _ := m.Update(app.AgentGraphMsg{Graph: &client.AgentGraphSnapshot{
		Runs: []client.AgentRunSummary{
			{RunID: "r1", AgentName: "alpha", Status: "running"},
			{RunID: "r2", AgentName: "beta", Status: "running"},
		},
	}})
	m = m2.(*app.AppModel)
	// Enable agents focus.
	m, _ = typeKeysExt(m, "/agents")
	m2, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = m2.(*app.AppModel)
	// Press Tab to cycle.
	m3, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	view := m3.(*app.AppModel).View()
	// Should show agent name after Tab.
	if !strings.Contains(view, "alpha") && !strings.Contains(view, "beta") {
		t.Errorf("expected agent name in view after Tab:\n%s", view)
	}
}

// ---- A8: Slash commands -----------------------------------------------------

func TestA8_1_SlashYolo_TogglesOn(t *testing.T) {
	m := app.New(config.ChatConfig{Provider: "codex", Yolo: false}, "http://127.0.0.1:4317")
	m, _ = typeKeysExt(m, "/yolo")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	view := m2.(*app.AppModel).View()
	if !strings.Contains(view, "YOLO mode: ON") {
		t.Errorf("expected 'YOLO mode: ON':\n%s", view)
	}
}

func TestA8_2_SlashYolo_TogglesOff(t *testing.T) {
	m := app.New(config.ChatConfig{Provider: "codex", Yolo: true}, "http://127.0.0.1:4317")
	m, _ = typeKeysExt(m, "/yolo")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	view := m2.(*app.AppModel).View()
	if !strings.Contains(view, "YOLO mode: OFF") {
		t.Errorf("expected 'YOLO mode: OFF':\n%s", view)
	}
}

func TestA8_3_SlashClear_EmptiesMessages(t *testing.T) {
	m := app.New(config.ChatConfig{}, "http://127.0.0.1:4317")
	// Add a message first.
	m2, _ := m.Update(app.ConnectedMsg{RunnerURL: "http://127.0.0.1:4317"})
	m = m2.(*app.AppModel)
	m, _ = typeKeysExt(m, "/clear")
	m3, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	view := m3.(*app.AppModel).View()
	if !strings.Contains(view, "Conversation cleared") {
		t.Errorf("expected 'Conversation cleared':\n%s", view)
	}
}

func TestA8_4_SlashStatus_ShowsState(t *testing.T) {
	m := app.New(config.ChatConfig{Provider: "claude"}, "http://127.0.0.1:4317")
	m, _ = typeKeysExt(m, "/status")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	view := m2.(*app.AppModel).View()
	if !strings.Contains(view, "Status:") {
		t.Errorf("expected 'Status:' in view:\n%s", view)
	}
}

func TestA8_5_SlashHelp_ShowsCommands(t *testing.T) {
	m := app.New(config.ChatConfig{}, "http://127.0.0.1:4317")
	tmp, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 60})
	m = tmp.(*app.AppModel)
	m, _ = typeKeysExt(m, "/help")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	view := m2.(*app.AppModel).View()
	for _, cmd := range []string{"/yolo", "/clear", "/exit", "/agents", "/skill", "/image", "/scan"} {
		if !strings.Contains(view, cmd) {
			t.Errorf("expected %q in /help view:\n%s", cmd, view)
		}
	}
}

func TestA8_6_SlashAgents_TogglesAgentsFocus(t *testing.T) {
	m := app.New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m, _ = typeKeysExt(m, "/agents")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	view := m2.(*app.AppModel).View()
	if !strings.Contains(view, "Agents focus: ON") {
		t.Errorf("expected 'Agents focus: ON':\n%s", view)
	}
}

func TestA8_7_SlashProvider_ChangesProvider(t *testing.T) {
	m := app.New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m, _ = typeKeysExt(m, "/provider claude")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	view := m2.(*app.AppModel).View()
	if !strings.Contains(view, "Provider set to: claude") {
		t.Errorf("expected 'Provider set to: claude':\n%s", view)
	}
}

func TestA8_8_SlashModel_ChangesModel(t *testing.T) {
	m := app.New(config.ChatConfig{Model: "o3"}, "http://127.0.0.1:4317")
	m, _ = typeKeysExt(m, "/model claude-4-5")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	view := m2.(*app.AppModel).View()
	if !strings.Contains(view, "Model set to: claude-4-5") {
		t.Errorf("expected 'Model set to: claude-4-5':\n%s", view)
	}
}

// ---- A9: Headless / gate / error paths --------------------------------------

func TestA9_1_PrintMode_TurnDoneMsg_Quits(t *testing.T) {
	m := app.New(config.ChatConfig{Print: true, Provider: "codex"}, "http://127.0.0.1:4317")
	_, cmd := m.Update(app.TurnDoneMsg{FinalMsg: "headless response"})
	if cmd == nil {
		t.Fatal("expected Quit cmd in Print mode TurnDoneMsg")
	}
}

func TestA9_2_CtrlC_ProducesQuitMsg(t *testing.T) {
	m := app.New(config.ChatConfig{}, "http://127.0.0.1:4317")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected cmd from Ctrl+C")
	}
	msg := cmd()
	if _, ok := msg.(app.QuitMsg); !ok {
		t.Errorf("expected QuitMsg from Ctrl+C, got %T", msg)
	}
}

func TestA9_3_ErrMsg_ShowsInView(t *testing.T) {
	m := app.New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m2, _ := m.Update(app.ErrMsg{Err: fmt.Errorf("connection refused")})
	if !strings.Contains(m2.(*app.AppModel).View(), "connection refused") {
		t.Errorf("expected error in view:\n%s", m2.(*app.AppModel).View())
	}
}

func TestA9_4_GateViolation_ShowsOptionsInView(t *testing.T) {
	m := app.New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m2, _ := m.Update(app.EventMsg{Ev: client.ProviderEvent{
		Type:          "flow_gate_violation",
		GateOptions:   []string{"continue", "stop"},
		WorkflowRunID: "run-g",
	}})
	view := m2.(*app.AppModel).View()
	if !strings.Contains(strings.ToLower(view), "gate") {
		t.Errorf("expected gate indicator in view:\n%s", view)
	}
	if !strings.Contains(view, "continue") {
		t.Errorf("expected 'continue' option in view:\n%s", view)
	}
}

func TestA9_5_TurnFailed_SetsConnError(t *testing.T) {
	m := app.New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m2, _ := m.Update(app.EventMsg{Ev: client.ProviderEvent{
		Type:  "turn_failed",
		Error: "provider timeout",
	}})
	view := m2.(*app.AppModel).View()
	// Should show error state in view.
	if !strings.Contains(strings.ToLower(view), "turn failed") && !strings.Contains(strings.ToLower(view), "provider timeout") {
		t.Errorf("expected turn_failed message in view:\n%s", view)
	}
}

// ---- helpers ----------------------------------------------------------------

// typeKeysExt simulates typing runes one at a time and returns the final model.
func typeKeysExt(m *app.AppModel, s string) (*app.AppModel, tea.Cmd) {
	var lastCmd tea.Cmd
	for _, r := range s {
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(*app.AppModel)
		lastCmd = cmd
	}
	return m, lastCmd
}
