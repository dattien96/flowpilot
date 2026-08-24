package app

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-610: hover motion must not reach Update/View; drag motion, clicks and
// wheel must. Paste settle on Windows must not pulse mouse.

func TestTuiMsgFilter_DropsHoverMotion(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{}, "http://127.0.0.1:9")
			m.provider = pk
			m.mouseDrag = mouseDrag{down: false}
			msg := tea.MouseMsg{X: 10, Y: 5, Button: tea.MouseButtonNone, Action: tea.MouseActionMotion, Shift: false}
			got := tuiMsgFilter(m, msg)
			if got != nil {
				t.Fatalf("%s: hover motion must be filtered to nil, got %T", pk, got)
			}
		})
	}
}

func TestTuiMsgFilter_KeepsDragMotion(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{}, "http://127.0.0.1:9")
			m.provider = pk
			m.mouseDrag = mouseDrag{down: true, x0: 0, y0: 0}
			msg := tea.MouseMsg{X: 10, Y: 5, Button: tea.MouseButtonLeft, Action: tea.MouseActionMotion, Shift: false}
			got := tuiMsgFilter(m, msg)
			if got == nil {
				t.Fatalf("%s: drag motion must not be filtered", pk)
			}
		})
	}
}

func TestTuiMsgFilter_KeepsClickAndWheel(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.mouseDrag = mouseDrag{down: false}
	clicks := []tea.MouseMsg{
		{X: 10, Y: 5, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress},
		{X: 10, Y: 5, Button: tea.MouseButtonLeft, Action: tea.MouseActionRelease},
		{X: 10, Y: 5, Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress},
		{X: 10, Y: 5, Button: tea.MouseButtonWheelDown, Action: tea.MouseActionPress},
	}
	for i, msg := range clicks {
		got := tuiMsgFilter(m, msg)
		if got == nil {
			t.Fatalf("click/wheel %d must not be filtered", i)
		}
	}
}

func TestTuiMsgFilter_ShiftDragMotionKept(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.mouseDrag = mouseDrag{down: false}
	msg := tea.MouseMsg{X: 10, Y: 5, Button: tea.MouseButtonNone, Action: tea.MouseActionMotion, Shift: true}
	got := tuiMsgFilter(m, msg)
	if got == nil {
		t.Fatal("Shift+motion must not be filtered (native selection path)")
	}
}

func TestPasteSettle_WindowsReject_NoMousePulse(t *testing.T) {
	advance := clockAt(t)
	m := newPasteModel()
	m.rejectWindowsRawPaste = true
	// Arm a raw flood: 2 rapid runes -> active.
	for _, r := range "ab" {
		advance(5 * time.Millisecond)
		m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = m2.(*AppModel)
	}
	if !m.pasteBurst.active {
		t.Fatal("burst must be active after 2 rapid runes")
	}
	advance(200 * time.Millisecond)
	m2, cmd := m.Update(pasteBurstSettleMsg{})
	m = m2.(*AppModel)
	if m.pasteBurst.active {
		t.Fatal("burst must reset after settle even when rejected")
	}
	if cmd != nil {
		t.Fatalf("Windows reject settle must not pulse mouse (cmd must be nil), got %T", cmd)
	}
}

func TestPulseMouseTracking_IsNoop(t *testing.T) {
	cmd := pulseMouseTracking()
	if cmd == nil {
		// Accept nil as no-op too, but current impl returns a no-op func for
		// Batch shape compatibility.
		return
	}
	if msg := cmd(); msg != nil {
		t.Fatalf("pulseMouseTracking must be no-op (nil msg), got %T %v", msg, msg)
	}
}

func TestTuiProgramOpts_IncludesFilter(t *testing.T) {
	opts := tuiProgramOpts()
	if len(opts) != 3 {
		t.Fatalf("opts must be AltScreen+MouseCellMotion+Filter, got %d", len(opts))
	}
}

func TestTypingAfterOpenAndMotion_Inserts(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{}, "http://127.0.0.1:9")
			m.provider = pk
			m.sessionLoading = false
			m.authPhase = AuthNone
			m.width = 120
			m.height = 40
			// Open a completed flow (same shape as run-125458 screenshot).
			handle := client.RunHandle{
				RunID:      "run-125458",
				RunKind:    "workflow",
				WorkflowID: "1c115dd7-ee35-4986-ab0e-d19630de71ed",
				Status:     "completed",
			}
			meta := client.RunHistoryItem{
				RunID:      "run-125458",
				WorkflowID: "1c115dd7-ee35-4986-ab0e-d19630de71ed",
				RunKind:    "workflow",
				Status:     "completed",
			}
			m2, _ := m.Update(ChatOpenedMsg{
				Handle:      handle,
				Messages:    []ChatMessage{{Role: "user", Content: "hello"}},
				HistoryMeta: meta,
			})
			m = m2.(*AppModel)
			// Hover motion must be filtered, not reaching Update.
			m.mouseDrag = mouseDrag{down: false}
			motion := tea.MouseMsg{X: 10, Y: 5, Button: tea.MouseButtonNone, Action: tea.MouseActionMotion}
			if got := tuiMsgFilter(m, motion); got != nil {
				t.Fatalf("%s: hover motion should be nil before Update", pk)
			}
			// Typing must still insert.
			m3, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
			am := m3.(*AppModel)
			if !strings.Contains(am.inputValue, "h") {
				t.Fatalf("%s: typing after open+motion must insert, got input=%q", pk, am.inputValue)
			}
		})
	}
}
