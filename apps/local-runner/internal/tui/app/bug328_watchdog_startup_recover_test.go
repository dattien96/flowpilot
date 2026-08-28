package app

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestBug328_WatchdogRecoverUsesExpectedSinceWhenNoKeyBaseline(t *testing.T) {
	m := New(configForWatchdogTest(), "http://127.0.0.1:9")
	now := time.Now()
	m.inputExpectedSince = now.Add(-60 * time.Second)
	m.inputStallLogged = true
	if !m.lastInputAt.IsZero() {
		t.Fatal("startup stall has zero lastInputAt")
	}

	updated, _ := m.Update(tea.MouseMsg{Type: tea.MouseLeft, X: 2, Y: 3})
	m2 := updated.(*AppModel)
	if m2.inputStallLogged {
		t.Fatal("click must clear the stall flag")
	}
	if m2.lastInputAt.IsZero() {
		t.Fatal("click must arm lastInputAt")
	}
}
