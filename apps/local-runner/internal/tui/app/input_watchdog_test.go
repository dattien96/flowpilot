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
	m.approval = &ApprovalState{ID: "ap-1", RunID: "run-1"} // input-requiring state
	m.lastInputAt = now.Add(-60 * time.Second)               // no input for 60s > 45s threshold

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

func TestInputWatchdog_StallEligibleViaGateAndQuestion(t *testing.T) {
	now := time.Now()
	for name, arm := range map[string]func(*AppModel){
		"approval": func(m *AppModel) { m.approval = &ApprovalState{ID: "ap-1"} },
		"question": func(m *AppModel) { m.question = &QuestionState{} },
		"gate":     func(m *AppModel) { m.gate = &GateState{Options: []string{"continue", "stop"}} },
	} {
		t.Run(name, func(t *testing.T) {
			m := New(configForWatchdogTest(), "http://127.0.0.1:9")
			arm(m)
			m.lastInputAt = now.Add(-90 * time.Second)
			updated, _ := m.Update(inputWatchdogMsg{at: now})
			m2 := updated.(*AppModel)
			if !m2.inputStallLogged {
				t.Fatalf("stall must be flagged in %s-requiring state", name)
			}
		})
	}
}

func TestInputWatchdog_NotEligibleWhenIdleLogsButNoBanner(t *testing.T) {
	// CA-645 v3: idle silence (sessions 21472/25580/12300/9176) must be logged
	// as a fingerprint but must NOT raise the visible banner.
	m := New(configForWatchdogTest(), "http://127.0.0.1:9")
	now := time.Now()
	m.connStatus = ConnIdle
	m.lastInputAt = now.Add(-90 * time.Second) // long silence, nothing awaits input
	before := m.statusMsg                      // New() seeds "connecting..."
	beforeMsgs := len(m.messages)

	updated, _ := m.Update(inputWatchdogMsg{at: now})
	m2 := updated.(*AppModel)
	if !m2.inputStallLogged {
		t.Fatal("idle stall must still be logged/flagged (diagnostic fingerprint)")
	}
	if m2.statusMsg != before {
		t.Fatalf("idle silence must not raise a banner: statusMsg changed from %q to %q", before, m2.statusMsg)
	}
	if len(m2.messages) != beforeMsgs {
		t.Fatalf("idle silence must not add a transcript message, got %d -> %d", beforeMsgs, len(m2.messages))
	}
}

func TestInputWatchdog_EligibleLaterRaisesBannerWithoutRelog(t *testing.T) {
	// A stall first logged while idle must raise the banner once the app
	// enters an input-requiring state, without a second stall log.
	m := New(configForWatchdogTest(), "http://127.0.0.1:9")
	now := time.Now()
	m.connStatus = ConnIdle
	m.lastInputAt = now.Add(-120 * time.Second)

	first, _ := m.Update(inputWatchdogMsg{at: now})
	m2 := first.(*AppModel)
	if !m2.inputStallLogged {
		t.Fatal("idle stall must be flagged")
	}
	if m2.statusMsg == "" {
		t.Fatal("idle stall flag alone must not raise a banner")
	}

	m2.approval = &ApprovalState{ID: "ap-1"} // now the operator must act
	second, _ := m2.Update(inputWatchdogMsg{at: now.Add(10 * time.Second)})
	m3 := second.(*AppModel)
	if m3.statusMsg == "" {
		t.Fatal("banner must appear once an input-requiring state is active")
	}
}

func TestInputWatchdog_NoStallBeforeThreshold(t *testing.T) {
	m := New(configForWatchdogTest(), "http://127.0.0.1:9")
	now := time.Now()
	m.approval = &ApprovalState{ID: "ap-1", RunID: "run-1"} // input-requiring state
	m.lastInputAt = now.Add(-10 * time.Second)               // under threshold
	before := m.statusMsg                                    // New() seeds "connecting..."

	updated, _ := m.Update(inputWatchdogMsg{at: now})
	m2 := updated.(*AppModel)
	if m2.inputStallLogged {
		t.Fatal("no stall below threshold")
	}
	if m2.statusMsg != before {
		t.Fatalf("no banner below threshold: statusMsg changed from %q to %q", before, m2.statusMsg)
	}
}

func TestInputWatchdog_StartupStallAfterSessionReady(t *testing.T) {
	m := New(configForWatchdogTest(), "http://127.0.0.1:9")
	now := time.Now()
	m.sessionDefaultsLoaded = true
	m.inputExpectedSince = now.Add(-60 * time.Second)

	updated, _ := m.Update(inputWatchdogMsg{at: now})
	m2 := updated.(*AppModel)
	if !m2.inputStallLogged {
		t.Fatal("startup stall must be flagged when session is ready but no keys arrived")
	}
	if m2.statusMsg == "" {
		t.Fatal("startup stall must surface a visible status banner")
	}
}

func TestInputWatchdog_FirstTickDoesNotArmBaseline(t *testing.T) {
	// BUG-328: baseline is not seeded from the watchdog tick alone — only a
	// real KeyMsg/MouseMsg arms lastInputAt (see markInputAlive).
	m := New(configForWatchdogTest(), "http://127.0.0.1:9")
	now := time.Now()
	if !m.lastInputAt.IsZero() {
		t.Fatal("fresh model must have zero lastInputAt")
	}
	updated, _ := m.Update(inputWatchdogMsg{at: now})
	m2 := updated.(*AppModel)
	if !m2.lastInputAt.IsZero() {
		t.Fatal("first tick must not arm lastInputAt without a real key")
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

func TestInputWatchdog_MotionStampsLivenessAtFilter(t *testing.T) {
	m := New(configForWatchdogTest(), "http://127.0.0.1:9")
	before := time.Now().Add(-time.Hour)
	m.lastMotionAt = before

	motion := tea.MouseMsg{X: 10, Y: 5, Button: tea.MouseButtonNone, Action: tea.MouseActionMotion}
	if got := tuiMsgFilter(m, motion); got != nil {
		t.Fatalf("hover motion must still be dropped by the filter, got %v", got)
	}
	if m.lastMotionAt.Before(before) {
		t.Fatal("hover motion must stamp lastMotionAt at the filter level")
	}

	// A motion stamp alone must NOT clear/reset a logged key/click stall.
	m.inputStallLogged = true
	m.lastInputAt = time.Now().Add(-90 * time.Second)
	if got := tuiMsgFilter(m, motion); got != nil {
		t.Fatalf("hover motion must be dropped even during a stall, got %v", got)
	}
	if !m.inputStallLogged {
		t.Fatal("hover motion must never clear the stall flag — only keys/clicks do")
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