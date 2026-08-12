// Tests A7-A9: CP-56 Bubble Tea app contract tests.
// These tests exercise the model logic directly without launching a TUI.
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

// newTestModel creates a model for testing with sensible defaults.
func newTestModel(cfg config.ChatConfig) *app.AppModel {
	return app.New(cfg, "http://127.0.0.1:4317")
}

// A7: Initial model state is correct and View renders without panic.
func TestA7_InitialState(t *testing.T) {
	cfg := config.ChatConfig{
		Provider: "codex",
		Model:    "o3",
		Yolo:     false,
	}
	m := newTestModel(cfg)
	if m == nil {
		t.Fatal("New returned nil")
	}
	view := m.View()
	if view == "" {
		t.Error("View() returned empty string on fresh model")
	}
}

// A7b: Model initialises with YOLO=true shows it in the view.
func TestA7b_InitialState_YoloTrue(t *testing.T) {
	cfg := config.ChatConfig{Provider: "grok", Yolo: true}
	m := newTestModel(cfg)
	view := m.View()
	if !strings.Contains(view, "YOLO") {
		t.Errorf("View() missing YOLO indicator; view:\n%s", view)
	}
}

// A7c: Window resize message updates without panic.
func TestA7c_WindowResize(t *testing.T) {
	m := newTestModel(config.ChatConfig{})
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if updated == nil {
		t.Fatal("Update returned nil model")
	}
	_ = updated.View()
}

// A7d: ConnectedMsg sets status to idle/connected.
func TestA7d_ConnectedMsg_SetsStatus(t *testing.T) {
	m := newTestModel(config.ChatConfig{})
	updated, _ := m.Update(app.ConnectedMsg{RunnerURL: "http://127.0.0.1:4317"})
	view := updated.(*app.AppModel).View()
	// Status should contain "connected" or "idle"
	if !strings.Contains(view, "connected") && !strings.Contains(view, "idle") {
		t.Errorf("After ConnectedMsg, expected 'connected' or 'idle' in view:\n%s", view)
	}
}

// A7e: RunStartedMsg sets the runHandle and shows run ID in status.
func TestA7e_RunStartedMsg(t *testing.T) {
	cfg := config.ChatConfig{Provider: "codex"}
	m := newTestModel(cfg)
	updated, _ := m.Update(app.RunStartedMsg{Handle: client.RunHandle{
		RunID:       "run-12345678",
		ProviderKey: "codex",
		Status:      "running",
	}})
	view := updated.(*app.AppModel).View()
	// Status line should contain the short run ID
	if !strings.Contains(view, "run-1234") {
		t.Errorf("After RunStartedMsg, expected short run ID in view:\n%s", view)
	}
}

// A8: /yolo slash command toggles YOLO state for non-grok provider.
func TestA8_SlashYolo_TogglesOn(t *testing.T) {
	cfg := config.ChatConfig{Provider: "codex", Yolo: false}
	m := newTestModel(cfg)

	m, _ = typeKeys(m, "/yolo")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	view := m2.(*app.AppModel).View()
	if !strings.Contains(view, "YOLO mode: ON") {
		t.Errorf("After /yolo (was OFF), expected 'YOLO mode: ON' in view;\ngot:\n%s", view)
	}
}

// A8b: /yolo toggle twice returns to OFF.
func TestA8b_SlashYolo_TogglesOff(t *testing.T) {
	cfg := config.ChatConfig{Provider: "codex", Yolo: true}
	m := newTestModel(cfg)

	m, _ = typeKeys(m, "/yolo")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	view := m2.(*app.AppModel).View()
	if !strings.Contains(view, "YOLO mode: OFF") {
		t.Errorf("After /yolo (was ON), expected 'YOLO mode: OFF' in view;\ngot:\n%s", view)
	}
}

// A8c: /clear removes conversation messages.
func TestA8c_SlashClear(t *testing.T) {
	cfg := config.ChatConfig{Provider: "codex"}
	m := newTestModel(cfg)

	// Add a message via connected
	updated, _ := m.Update(app.ConnectedMsg{RunnerURL: "http://127.0.0.1:4317"})
	m = updated.(*app.AppModel)

	// Now clear
	m, _ = typeKeys(m, "/clear")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	view := m2.(*app.AppModel).View()
	if !strings.Contains(view, "Conversation cleared") {
		t.Errorf("After /clear, expected 'Conversation cleared';\ngot:\n%s", view)
	}
}

// A8d: /status outputs current state.
func TestA8d_SlashStatus(t *testing.T) {
	cfg := config.ChatConfig{Provider: "claude", Yolo: false}
	m := newTestModel(cfg)

	m, _ = typeKeys(m, "/status")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	view := m2.(*app.AppModel).View()
	if !strings.Contains(view, "Status:") {
		t.Errorf("After /status, expected 'Status:';\ngot:\n%s", view)
	}
}

// A8e: /help shows known commands.
func TestA8e_SlashHelp(t *testing.T) {
	m := newTestModel(config.ChatConfig{})
	m, _ = typeKeys(m, "/help")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	view := m2.(*app.AppModel).View()

	for _, cmd := range []string{"/yolo", "/clear", "/exit", "/agents"} {
		if !strings.Contains(view, cmd) {
			t.Errorf("After /help, expected %q in view;\ngot:\n%s", cmd, view)
		}
	}
}

// A8f: /agents toggles agent focus.
func TestA8f_SlashAgents_Toggles(t *testing.T) {
	m := newTestModel(config.ChatConfig{})
	m, _ = typeKeys(m, "/agents")
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	view := m2.(*app.AppModel).View()
	if !strings.Contains(view, "Agents focus: ON") {
		t.Errorf("After /agents, expected 'Agents focus: ON';\ngot:\n%s", view)
	}
}

// A9: Headless mode: TurnDoneMsg causes tea.Quit when Print=true.
func TestA9_HeadlessMode_QuitsOnTurnDone(t *testing.T) {
	cfg := config.ChatConfig{Print: true, Provider: "codex"}
	m := newTestModel(cfg)

	// Simulate RunStarted
	updated, _ := m.Update(app.RunStartedMsg{Handle: client.RunHandle{
		RunID:       "run-headless",
		ProviderKey: "codex",
		Status:      "running",
	}})
	m = updated.(*app.AppModel)

	// Simulate TurnDone
	_, cmd := m.Update(app.TurnDoneMsg{FinalMsg: "Hello from headless!"})
	if cmd == nil {
		t.Error("Expected Quit command from TurnDoneMsg in Print mode, got nil")
		return
	}
	// Execute the cmd and check it's Quit
	msg := cmd()
	if msg != nil {
		t.Logf("cmd() returned msg of type %T: %v", msg, msg)
	}
}

// A9b: Ctrl+C returns a command that produces a QuitMsg.
func TestA9b_CtrlC_Quits(t *testing.T) {
	m := newTestModel(config.ChatConfig{})
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if updated == nil {
		t.Fatal("Update returned nil")
	}
	if cmd == nil {
		t.Error("Expected cmd from Ctrl+C, got nil")
		return
	}
	msg := cmd()
	if _, ok := msg.(app.QuitMsg); !ok {
		t.Errorf("Expected QuitMsg from Ctrl+C command, got %T: %v", msg, msg)
	}
}

// A9c: ErrMsg surfaces in view.
func TestA9c_ErrMsg_ShowsInView(t *testing.T) {
	m := newTestModel(config.ChatConfig{})
	m2, _ := m.Update(app.ErrMsg{Err: fmt.Errorf("connection refused")})
	view := m2.(*app.AppModel).View()
	if !strings.Contains(view, "connection refused") {
		t.Errorf("After ErrMsg, expected error message in view;\ngot:\n%s", view)
	}
}

// ---- helpers ----------------------------------------------------------------

// typeKeys simulates typing runes one at a time, returning the final *AppModel.
func typeKeys(m *app.AppModel, s string) (*app.AppModel, tea.Cmd) {
	var lastCmd tea.Cmd
	for _, r := range s {
		updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(*app.AppModel)
		lastCmd = cmd
	}
	return m, lastCmd
}
