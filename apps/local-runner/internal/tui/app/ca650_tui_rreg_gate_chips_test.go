package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-650 (run-161570): the r-reg gate card rendered "Options: ..." as plain
// text with no clickable UI, and the custom path silently 400'd because the
// TUI client never sent customText. Chips + custom flow added for desktop
// parity. Additive only — legacy suites untouched.

func gateFixtureWithRregOptions(t *testing.T) *AppModel {
	t.Helper()
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.sessionLoading = false
	m.authPhase = AuthNone
	m.width = 120
	m.height = 40
	updated, _ := m.Update(EventMsg{Ev: client.ProviderEvent{
		Type:               "flow_gate_violation",
		Status:             "block",
		Error:              "Flow gate: Tests failed: suite_regressed",
		GateOptions:        []string{"keep-test-fix-code", "suggest-requirement-change", "custom"},
		GateRegressedTests: []string{"suite_regressed"},
		WorkflowRunID:      "run-161570",
	}})
	return updated.(*AppModel)
}

func TestRregGateChipsClickable(t *testing.T) {
	m := gateFixtureWithRregOptions(t)

	for _, opt := range m.gate.Options {
		x, y, ok := findClickTarget(m, "gopt:"+opt)
		if !ok {
			t.Fatalf("gate chip for option %q must be clickable", opt)
		}
		if got := m.clickTargetAt(x, y); got != "gopt:"+opt {
			t.Fatalf("clickTargetAt(%d,%d) = %q, want gopt:%s", x, y, got, opt)
		}
	}
}

func TestRregGateMessageRendersChips(t *testing.T) {
	msg := buildGateMessage("block", "", []string{"keep-test-fix-code", "suggest-requirement-change", "custom"}, []string{"suite_regressed"})
	for _, want := range []string{"[Fix code]", "[Suggest req]", "[Custom]", "Regressed tests: suite_regressed"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("gate message must contain %q, got:\n%s", want, msg)
		}
	}
}

func TestRregGateChipFixCodeSubmitsAndClears(t *testing.T) {
	m := gateFixtureWithRregOptions(t)
	x, y, ok := findClickTarget(m, "gopt:keep-test-fix-code")
	if !ok {
		t.Fatal("Fix code chip must exist")
	}
	updated, cmd := m.Update(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: y})
	m2 := updated.(*AppModel)
	if m2.gate != nil {
		t.Fatal("gate card must clear after choosing an option")
	}
	if cmd == nil {
		t.Fatal("chip click must dispatch the gate decision command")
	}
}

func TestRregGateTypeNumberStillWorks(t *testing.T) {
	m := gateFixtureWithRregOptions(t)
	updated, cmd := m.processInput("") // empty enter: must not clear the gate
	m2 := updated.(*AppModel)
	_ = cmd
	if m2.gate == nil {
		t.Fatal("enter with empty input must not clear the gate")
	}
	updated, cmd = m2.processInput("1") // number decision
	m3 := updated.(*AppModel)
	if m3.gate != nil {
		t.Fatal("Enter after typing 1 must clear the gate")
	}
	if cmd == nil {
		t.Fatal("number decision must dispatch a command")
	}
}

func TestRregGateCustomChipThenTypedReason(t *testing.T) {
	m := gateFixtureWithRregOptions(t)
	x, y, ok := findClickTarget(m, "gopt:custom")
	if !ok {
		t.Fatal("Custom chip must exist")
	}
	updated, _ := m.Update(tea.MouseMsg{Type: tea.MouseLeft, X: x, Y: y})
	m2 := updated.(*AppModel)
	if m2.gate == nil || !m2.gate.AwaitingCustom {
		t.Fatal("Custom chip must arm AwaitingCustom without clearing the card")
	}
	if m2.statusMsg == "" {
		t.Fatal("custom flow must surface a status prompt")
	}

	// Type the reason, then Enter submits custom + text.
	updated, _ = m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("because it is flaky")})
	m3 := updated.(*AppModel)
	updated, cmd := m3.processInput(m3.inputValue)
	m4 := updated.(*AppModel)
	if m4.gate != nil {
		t.Fatal("Enter with the typed reason must clear the gate")
	}
	if cmd == nil {
		t.Fatal("custom submission must dispatch a command")
	}
}

func TestRregGateTypedCustomWithInlineText(t *testing.T) {
	m := gateFixtureWithRregOptions(t)
	// "custom <text>" then Enter.
	updated, cmd := m.processInput("custom rewrite the test")
	m2 := updated.(*AppModel)
	if m2.gate != nil {
		t.Fatal("'custom <text>' must clear the gate on Enter")
	}
	if cmd == nil {
		t.Fatal("inline custom must dispatch a command")
	}
}

func TestRregGateChipsHideContinueStop(t *testing.T) {
	m := gateFixtureWithRregOptions(t)
	if m.flowLoopBlocked() {
		t.Fatal("an armed gate card must not render Continue/Stop (chips take over)")
	}
}