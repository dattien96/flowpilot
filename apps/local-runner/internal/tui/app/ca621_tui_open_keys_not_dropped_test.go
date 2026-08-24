package app

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-621: opening with large history must not dump bodies into tui.log nor make View slow enough to drop keys.

func TestCA621_ChatListMsg_DoesNotBlockKeys(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 126, 50
	m.sessionLoading = false
	// Simulate large ChatListMsg as on cold open (200 items)
	items := make([]client.RunHistoryItem, 200)
	for i := range items {
		items[i] = client.RunHistoryItem{RunID: "run-" + strings.Repeat("x", 20), ProjectID: "proj"}
	}
	m2, _ := m.Update(ChatListMsg{Items: items})
	am := m2.(*AppModel)
	// Immediate key after large ChatListMsg must still be processed (not dropped by slow View/log)
	m3, _ := am.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	am3 := m3.(*AppModel)
	if !strings.Contains(am3.inputValue, "a") {
		t.Fatalf("key after large ChatListMsg must insert, got %q", am3.inputValue)
	}
}

func TestCA621_View_NotSlowWithManySteps(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 126, 50
	m.sessionLoading = false
	m.flowSteps = make([]client.WorkflowStepRuntime, 50)
	for i := range m.flowSteps {
		m.flowSteps[i] = client.WorkflowStepRuntime{NodeID: "step-" + strings.Repeat("y", 10), Status: "PENDING"}
	}
	m.agentRuns = []client.AgentRunSummary{{RunID: "run-1", AgentName: "main", Status: "completed"}}
	start := time.Now()
	_ = m.View()
	dur := time.Since(start)
	// Allow 200ms on CI; original was 100-800ms with sidebar
	if dur > 200*time.Millisecond {
		t.Fatalf("View with 50 steps took %v, want <200ms (CA-621)", dur)
	}
}

func TestCA621_ViewSlowLog_Throttled(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 126, 50
	m.lastViewSlowLog = time.Now()
	// Second View slow within 5s must not update lastViewSlowLog (throttled)
	before := m.lastViewSlowLog
	// Simulate View slow path without actually sleeping: directly test throttle logic by calling View with fake slow?
	// Instead verify that View does not reset lastViewSlowLog when throttled
	_ = m.View() // likely not slow now (empty view <100ms), so no log
	if !m.lastViewSlowLog.Equal(before) {
		t.Fatalf("View without slow must not change lastViewSlowLog")
	}
	// Force a slow View by making chat history large
	m.messages = make([]ChatMessage, 500)
	for i := range m.messages {
		m.messages[i] = ChatMessage{Role: "user", Content: strings.Repeat("hello ", 50)}
	}
	_ = m.View()
	// If it was slow, it should be throttled (not update) since <5s
	if m.lastViewSlowLog.Sub(before) < 4*time.Second && !m.lastViewSlowLog.Equal(before) {
		// If it did update within 4s, throttling failed
		t.Fatalf("View slow log not throttled, lastViewSlowLog updated too soon")
	}
}

func TestCA621_KeysNotDropped_AfterOpen(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.width, m.height = 126, 50
	m.sessionLoading = false
	// Simulate open with sidebar
	m.flowStepsActive = "implement"
	m.flowSteps = []client.WorkflowStepRuntime{{NodeID: "implement", Status: "RUNNING"}}
	m.agentRuns = []client.AgentRunSummary{{RunID: "run-1", Status: "running"}}
	// Rapid keys a,b,c must all insert
	for _, ch := range []string{"a", "b", "c"} {
		m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(ch)})
		m = m2.(*AppModel)
	}
	if m.inputValue != "abc" {
		t.Fatalf("rapid keys after open must be abc, got %q", m.inputValue)
	}
}
