package app

import (
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// CA-537 — the animated "Thinking" loading indicator is removed from the chat
// timeline. It lives on the status bottom line + the F2 RUNNING step instead,
// and must animate whenever a chat turn or flow step is live (and stop while
// the user is deciding / when idle). Provider-agnostic logic — parameterized
// over claude/codex/grok anyway so a future provider-specific branch trips here.

func ca537Model(pk string) *AppModel {
	m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
	m.width, m.height = 80, 24
	m.sessionLoading = false
	return m
}

func TestChatTimeline_NoThinkingRow(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := ca537Model(pk)
			m.addMessage("user", "fix it", "")
			m.addMessage("assistant", "thinking…", "thinking")
			if row := thinkingRowText(m); row != "" {
				t.Fatalf("%s: thinking placeholder must not render in the chat: %q", pk, row)
			}
		})
	}
}

func TestWorkIsLive_Matrix(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			cases := []struct {
				name string
				set  func(*AppModel)
				want bool
			}{
				{"idle", func(m *AppModel) {}, false},
				{"pendingPrompt", func(m *AppModel) { m.pendingPrompt = "go" }, true},
				{"connRunning", func(m *AppModel) { m.connStatus = ConnRunning }, true},
				{"connConnectingIsNotLive", func(m *AppModel) { m.connStatus = ConnConnecting }, false},
				{"connWaitingWithoutFlow", func(m *AppModel) { m.connStatus = ConnWaiting }, false},
				{"turnStream", func(m *AppModel) { m.turnStream = &turnStreamState{} }, true},
				{"activeAgent", func(m *AppModel) {
					m.agentRuns = []client.AgentRunSummary{{AgentName: "coder", Status: "running"}}
				}, true},
				{"gateBlocksLiveWork", func(m *AppModel) {
					m.connStatus = ConnRunning
					m.gate = &GateState{Options: []string{"continue"}, RunID: "run-1"}
				}, false},
				{"questionBlocksLiveWork", func(m *AppModel) {
					m.connStatus = ConnRunning
					m.question = &QuestionState{}
				}, false},
				{"approvalBlocksLiveWork", func(m *AppModel) {
					m.connStatus = ConnRunning
					m.approval = &ApprovalState{}
				}, false},
				{"approvalWithWaitingAgentStaysIdle", func(m *AppModel) {
					m.connStatus = ConnRunning
					m.approval = &ApprovalState{}
					m.agentRuns = []client.AgentRunSummary{{AgentName: "reviewer", Status: "waiting_user_approval"}}
				}, false},
				{"flowLiveHandle", func(m *AppModel) {
					m.mode = ModeFlow
					m.connStatus = ConnIdle
					m.runHandle = &client.RunHandle{RunID: "run-1", Status: "running"}
				}, true},
				{"flowTerminalHandleIdle", func(m *AppModel) {
					m.mode = ModeFlow
					m.connStatus = ConnIdle
					m.runHandle = &client.RunHandle{RunID: "run-1", Status: "completed"}
				}, false},
			}
			for _, c := range cases {
				m := ca537Model(pk)
				c.set(m)
				if got := m.workIsLive(); got != c.want {
					t.Errorf("%s: workIsLive=%v want %v", c.name, got, c.want)
				}
			}
		})
	}
}

func TestStatusSpinner_LiveWhileChatTurn(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := ca537Model(pk)
			m2, _ := m.processInput("go")
			am := m2.(*AppModel)
			if row := thinkingRowText(am); row != "" {
				t.Fatalf("%s: chat must not show a Thinking row: %q", pk, row)
			}
			if got := am.statusReadyLabel(); !strings.HasPrefix(stripANSI(got), "⠋ ") {
				t.Fatalf("%s: status must lead with the spinner: %q", pk, got)
			}
			// The spinner advances + reschedules while work is live.
			am.thinkingFrame = 0
			m3, cmd := am.Update(thinkingTickMsg{})
			am2 := m3.(*AppModel)
			if am2.thinkingFrame != 1 || cmd == nil {
				t.Fatalf("%s: tick must advance + reschedule while live (frame=%d cmd=%v)", pk, am2.thinkingFrame, cmd)
			}
		})
	}
}

func TestStatusSpinner_StopsWhenIdle(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := ca537Model(pk)
			m.connStatus = ConnRunning
			m.statusMsg = "thinking…"
			if got := m.statusReadyLabel(); !strings.Contains(got, "Thinking") {
				t.Fatalf("%s: live status must show spinner: %q", pk, got)
			}
			m.connStatus = ConnIdle
			if got := m.statusReadyLabel(); got != "thinking…" {
				t.Fatalf("%s: idle status must go static: %q", pk, got)
			}
		})
	}
}

func TestStatusSpinner_StopsDuringGate(t *testing.T) {
	m := ca537Model("grok")
	m.connStatus = ConnRunning
	m.gate = &GateState{Options: []string{"continue"}, RunID: "run-1"}
	if m.workIsLive() {
		t.Fatal("a pending gate must not count as live work")
	}
	if got := m.statusReadyLabel(); strings.Contains(got, "⠋") {
		t.Fatalf("no spinner while the gate waits on the user: %q", got)
	}
}

func TestCursorTick_ArmsTickerOnLiveEdge(t *testing.T) {
	m := ca537Model("codex")
	m.addMessage("user", "u", "")
	m2, _ := m.Update(cursorTickMsg{})
	if m2.(*AppModel).thinkingTickerActive {
		t.Fatal("ticker must not arm while idle")
	}
	m.connStatus = ConnRunning
	m.thinkingFrame = 42
	m3, _ := m.Update(cursorTickMsg{})
	am := m3.(*AppModel)
	if !am.thinkingTickerActive {
		t.Fatal("ticker must arm when work becomes live")
	}
	if am.thinkingFrame != 0 {
		t.Fatalf("live edge must reset thinkingFrame: got %d", am.thinkingFrame)
	}
}

func TestF2RunningStep_SpinnerGlyph(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
			m.mode = ModeFlow
			m.flowSteps = []client.WorkflowStepRuntime{
				{StepID: "s1", NodeID: "coder", Status: "DONE"},
				{StepID: "s2", NodeID: "my-reviewer", AgentRef: "my-reviewer", Status: "RUNNING"},
			}
			m.flowStepsActive = "my-reviewer"
			m.thinkingFrame = 0
			joined := strings.Join(m.flowStepsPanelLines(), "\n")
			if !strings.Contains(joined, "⠋") {
				t.Fatalf("%s: RUNNING step must use the spinner glyph:\n%s", pk, joined)
			}
			if !strings.Contains(joined, "RUNNING") {
				t.Fatalf("%s: RUNNING step must keep the RUNNING token:\n%s", pk, joined)
			}
			m.thinkingFrame = 1
			joined2 := strings.Join(m.flowStepsPanelLines(), "\n")
			if joined2 == joined {
				t.Fatalf("%s: F2 spinner glyph must advance with the frame", pk)
			}
		})
	}
}
