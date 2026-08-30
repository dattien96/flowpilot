package app

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

func TestSessionLoading_TypingAndF2NeverFrozen(t *testing.T) {
	m := New(config.ChatConfig{Provider: "codex"}, "http://127.0.0.1:4317")
	m.sessionLoading = true
	m2, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if m2.(*AppModel).inputValue != "h" {
		t.Fatalf("typing while loading=%q want h", m2.(*AppModel).inputValue)
	}
	enableSidebarForTest(m)
	m3, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyF2})
	if len(m3.(*AppModel).messages) == 0 {
		t.Fatal("F2 must print the info dump while sessionLoading")
	}
}

// CA-514 fourth pass: the FlowPilot banner stays up until SessionDefaultsMsg
// decides the catalog (project bound or failed). A stale sessionKeysUnlockMsg
// must NOT clear it early — typing is never hard-locked, only send is blocked.
func TestSessionLoading_BannerPersistsUntilSessionDefaults(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.sessionLoading = true
	m.statusMsg = "connected"
	m2, _ := m.Update(sessionKeysUnlockMsg{})
	am := m2.(*AppModel)
	if !am.sessionLoading {
		t.Fatal("stale keys-unlock must not clear the banner")
	}
	if am.sessionDefaultsLoaded {
		t.Fatal("banner state must not mark defaults loaded")
	}
	path := t.TempDir()
	m3, _ := am.Update(SessionDefaultsMsg{
		Provider: "codex",
		Model:    "o3",
		Projects: []client.Project{{ID: "p1", Name: "proj", Path: path}},
		Project:  &client.Project{ID: "p1", Name: "proj", Path: path},
	})
	ready := m3.(*AppModel)
	if ready.sessionLoading {
		t.Fatal("SessionDefaultsMsg must clear the banner")
	}
	if ready.project == nil || ready.project.ID != "p1" {
		t.Fatalf("catalog in SessionDefaultsMsg must bind the project: %+v", ready.project)
	}
}

func TestSessionLoadTimeoutMsg_OnlyWhenDefaultsNeverLoaded(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.sessionLoading = true
	m.sessionDefaultsLoaded = true // defaults already applied
	m2, _ := m.Update(sessionLoadTimeoutMsg{})
	if m2.(*AppModel).connStatus == ConnError && m2.(*AppModel).statusMsg == "session load failed" {
		t.Fatal("must not clobber a successful session with hard timeout")
	}

	m3 := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m3.sessionLoading = true
	m3.sessionDefaultsLoaded = false
	m4, _ := m3.Update(sessionLoadTimeoutMsg{})
	am := m4.(*AppModel)
	if am.sessionLoading {
		t.Fatal("hard timeout must unlock keyboard when defaults never arrived")
	}
	// CA-514 contract: the timeout must NOT mark defaults loaded, so a late
	// SessionDefaultsMsg still counts as first load (persist + flow restore).
	if am.sessionDefaultsLoaded {
		t.Fatal("timeout must not mark sessionDefaultsLoaded; late defaults still matter")
	}
	if am.connStatus != ConnError || am.statusMsg != "session load failed" {
		t.Fatalf("connStatus=%v statusMsg=%q", am.connStatus, am.statusMsg)
	}
}

func TestProjectsCatalogMsg_BindsProjectAfterSlowCatalog(t *testing.T) {
	m := New(config.ChatConfig{ProjectPath: `D:\working\gate-sandbox`}, "http://127.0.0.1:4317")
	m.sessionDefaultsLoaded = true
	m.project = nil
	next, _ := m.Update(ProjectsCatalogMsg{
		Projects: []client.Project{{ID: "p1", Name: "gate-sandbox", Path: `D:\working\gate-sandbox`}},
		Project:  &client.Project{ID: "p1", Name: "gate-sandbox", Path: `D:\working\gate-sandbox`},
	})
	am := next.(*AppModel)
	if am.project == nil || am.project.ID != "p1" {
		t.Fatalf("project=%+v", am.project)
	}
}

func TestSessionDefaults_CatalogTimeoutSchedulesRetry(t *testing.T) {
	m := New(config.ChatConfig{ProjectPath: `D:\working\gate-sandbox`}, "http://127.0.0.1:4317")
	_, cmd := m.Update(SessionDefaultsMsg{
		Provider:   "grok",
		Model:      "grok-4.5",
		CatalogErr: "context deadline exceeded",
	})
	if cmd == nil {
		t.Fatal("slow catalog must schedule background project retry")
	}
}

func TestShouldPollStepsRuntime_PausesAfterDialFailures(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:9")
	m.mode = ModeFlow
	m.runHandle = &client.RunHandle{RunID: "run-1", Status: "running"}
	m.connStatus = ConnRunning
	if !m.shouldPollStepsRuntime() {
		t.Fatal("live flow should poll")
	}
	for i := 0; i < runnerPollFailPauseAfter; i++ {
		m.noteRunnerPollResult("dial tcp 127.0.0.1:4317: connectex: No connection could be made")
	}
	if m.shouldPollStepsRuntime() {
		t.Fatal("after fail streak must pause auto-poll")
	}
	if m.connStatus != ConnError {
		t.Fatalf("connStatus=%v want ConnError", m.connStatus)
	}
	if !strings.Contains(m.statusMsg, "runner offline") {
		t.Fatalf("status=%q", m.statusMsg)
	}
}

func TestStepsRuntimeMsg_ClearsInFlightAndTracksUnreachable(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m.runHandle = &client.RunHandle{RunID: "run-1", Status: "running"}
	m.stepsPollInFlight = true
	m2, _ := m.Update(StepsRuntimeMsg{RunID: "run-1", Err: "connection refused"})
	am := m2.(*AppModel)
	if am.stepsPollInFlight {
		t.Fatal("in-flight must clear on error")
	}
	if am.runnerPollFailStreak != 1 {
		t.Fatalf("streak=%d", am.runnerPollFailStreak)
	}
	// Success resets streak.
	am.stepsPollInFlight = true
	m3, _ := am.Update(StepsRuntimeMsg{RunID: "run-1", Steps: nil})
	if m3.(*AppModel).runnerPollFailStreak != 0 {
		t.Fatal("success should reset streak")
	}
}

func TestCmdLoadSessionDefaults_HasBoundedTimeout(t *testing.T) {
	// Point at a black-hole port that accepts nothing; must return within ~10s+slack.
	m := New(config.ChatConfig{}, "http://127.0.0.1:1")
	cmd := m.cmdLoadSessionDefaults()
	if cmd == nil {
		t.Fatal("nil cmd")
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case msg := <-done:
		sd, ok := msg.(SessionDefaultsMsg)
		if !ok {
			t.Fatalf("msg type %T", msg)
		}
		// Unreachable runner → catalog error (or empty catalogs).
		_ = sd
	case <-time.After(15 * time.Second):
		t.Fatal("cmdLoadSessionDefaults hung — sessionLoading would freeze TUI")
	}
}

func TestShouldPollStepsRuntime_IdleTerminalOpenDoesNotAutoPoll(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m.runHandle = &client.RunHandle{RunID: "run-done", Status: "completed"}
	m.connStatus = ConnIdle
	if m.shouldPollStepsRuntime() {
		t.Fatal("completed idle open must not auto-poll every 1.6s")
	}
	// One-shot refresh still allowed.
	if c := m.cmdRefreshStepsRuntime(); c == nil {
		t.Fatal("one-shot refresh should still work for completed open")
	}
}

func TestShouldPollStepsRuntime_RunningOpenStillPolls(t *testing.T) {
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m.runHandle = &client.RunHandle{RunID: "run-live", Status: "running"}
	m.connStatus = ConnIdle
	if !m.shouldPollStepsRuntime() {
		t.Fatal("non-terminal opened workflow must keep auto-poll")
	}
}

func TestShouldPollStepsRuntime_OrchAloneOnCompletedDoesNotPoll(t *testing.T) {
	// Regression: CA-513 hydrate-on-poll + CA-502 always-on orch after /open
	// must not auto-poll forever on a completed flow (floods dead runner → freeze).
	m := New(config.ChatConfig{}, "http://127.0.0.1:4317")
	m.mode = ModeFlow
	m.runHandle = &client.RunHandle{RunID: "run-done", Status: "completed"}
	m.connStatus = ConnIdle
	m.orchStream = &orchStreamState{} // attached late-event listener only
	if m.shouldPollStepsRuntime() {
		t.Fatal("completed open + orch listener must not trigger 1.6s steps/agent poll")
	}
}
