package app

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/config"
)

// CA-645: Windows console input can wedge while the app stays alive (session
// pid 25708: 0 KeyMsg / 0 mouse clicks for 9+ min after flow start). The
// watchdog detects the silent stall, logs a fingerprint, and shows a banner —
// then clears itself on the next input.

func configForWatchdogTest() config.ChatConfig {
	return config.ChatConfig{}
}

func TestInputWatchdog_StallDetectedOnce(t *testing.T) {
	m := New(configForWatchdogTest(), "http://127.0.0.1:9")
	now := time.Now()
	m.lastInputAt = now.Add(-60 * time.Second) // no input for 60s > 45s threshold

	first, _ := m.Update(inputWatchdogMsg{at: now})
	m2 := first.(*AppModel)
	if !m2.inputStallLogged {
		t.Fatal("stall must be flagged after threshold")
	}
	if m2.statusMsg == "" {
		t.Fatal("stall must surface a visible status banner")
	}

	// Second tick must not re-log or duplicate the banner (still stalled).
	second, _ := m2.Update(inputWatchdogMsg{at: now.Add(10 * time.Second)})
	m3 := second.(*AppModel)
	if !m3.inputStallLogged {
		t.Fatal("stall flag must persist across ticks")
	}
	if got := m3.statusMsg; got != m2.statusMsg {
		t.Fatalf("stall banner must not be replaced on repeat ticks, got %q", got)
	}
}

func TestInputWatchdog_NoStallBeforeThreshold(t *testing.T) {
	m := New(configForWatchdogTest(), "http://127.0.0.1:9")
	now := time.Now()
	m.lastInputAt = now.Add(-10 * time.Second) // under threshold
	before := m.statusMsg                      // New() seeds "connecting..."

	updated, _ := m.Update(inputWatchdogMsg{at: now})
	m2 := updated.(*AppModel)
	if m2.inputStallLogged {
		t.Fatal("no stall below threshold")
	}
	if m2.statusMsg != before {
		t.Fatalf("no banner below threshold: statusMsg changed from %q to %q", before, m2.statusMsg)
	}
}

func TestInputWatchdog_FirstTickArmsBaseline(t *testing.T) {
	m := New(configForWatchdogTest(), "http://127.0.0.1:9")
	now := time.Now()
	if !m.lastInputAt.IsZero() {
		t.Fatal("fresh model must have zero lastInputAt")
	}
	updated, _ := m.Update(inputWatchdogMsg{at: now})
	m2 := updated.(*AppModel)
	if m2.lastInputAt.IsZero() {
		t.Fatal("first tick must arm the baseline timestamp")
	}
	if m2.inputStallLogged {
		t.Fatal("first tick must never flag a stall")
	}
}

func TestInputWatchdog_KeyResumesAfterStall(t *testing.T) {
	m := New(configForWatchdogTest(), "http://127.0.0.1:9")
	now := time.Now()
	m.lastInputAt = now.Add(-90 * time.Second)
	m.inputStallLogged = true // stall already logged
	m.statusMsg = "input stalled: ..."

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	m2 := updated.(*AppModel)
	if m2.inputStallLogged {
		t.Fatal("an arriving key must clear the stall flag")
	}
	if m2.statusMsg != "" {
		t.Fatalf("an arriving key must clear the stall banner, got %q", m2.statusMsg)
	}
	if m2.lastInputAt.Before(now) {
		t.Fatal("an arriving key must refresh lastInputAt")
	}
}

func TestInputWatchdog_MouseClickResumesAfterStall(t *testing.T) {
	m := New(configForWatchdogTest(), "http://127.0.0.1:9")
	now := time.Now()
	m.lastInputAt = now.Add(-90 * time.Second)
	m.inputStallLogged = true

	updated, _ := m.Update(tea.MouseMsg{Type: tea.MouseLeft, X: 2, Y: 3})
	m2 := updated.(*AppModel)
	if m2.inputStallLogged {
		t.Fatal("an arriving mouse click must clear the stall flag")
	}
}

func TestInputWatchdog_CmdReArms(t *testing.T) {
	cmd := cmdInputWatchdog()
	if cmd == nil {
		t.Fatal("cmdInputWatchdog must return a command")
	}
}

func TestInputWatchdog_NormalKeyDoesNotTouchBanner(t *testing.T) {
	m := New(configForWatchdogTest(), "http://127.0.0.1:9")
	now := time.Now()
	m.lastInputAt = now.Add(-2 * time.Second)
	m.statusMsg = "step: preflight_contract_freeze" // legitimate live status

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(*AppModel)
	if m2.statusMsg != "step: preflight_contract_freeze" {
		t.Fatalf("a normal key must not clobber the live status banner, got %q", m2.statusMsg)
	}
	if m2.inputStallLogged {
		t.Fatal("a normal key must not flag a stall")
	}
}