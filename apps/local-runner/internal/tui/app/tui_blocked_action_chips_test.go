package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// FlowAwaitingUserCard parity for the TUI (BUG-231): a blocked (awaiting-user)
// flow must surface clickable [Continue]/[Stop] action chips above the composer,
// mirroring the Approve/Deny + dispatch-attention chip pattern — no slash command
// required. run-189839 blocked flows previously only advertised /continue and
// /stop as text.

func blockedChipModel(pk string) *AppModel {
	m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.asciiMode = true
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf", Label: pk + "-flow"}
	m.runHandle = &client.RunHandle{RunID: "run-63960", Status: "running"}
	m.connStatus = ConnWaiting
	m.statusMsg = "awaiting your decision"
	m.flowLoopStatus = "blocked"
	m.flowBlockReason = "cap"
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-63960", AgentName: "main", Role: "main", Status: "completed"},
	}
	return m
}

// TestBlockedBar_RendersContinueStopChips: a blocked (awaiting-user) flow renders
// clickable [Continue] and [Stop] chips in the input bar.
func TestBlockedBar_RendersContinueStopChips(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := blockedChipModel(pk)
			view := stripANSI(m.View())
			if !strings.Contains(view, "[Continue]") {
				t.Fatalf("%s: blocked view must render [Continue] chip:\n%s", pk, view)
			}
			if !strings.Contains(view, "[Stop]") {
				t.Fatalf("%s: blocked view must render [Stop] chip:\n%s", pk, view)
			}
		})
	}
}

// TestBlockedBar_NoChipsWhenNotBlocked: a non-blocked flow must NOT render the
// [Continue]/[Stop] action chips.
func TestBlockedBar_NoChipsWhenNotBlocked(t *testing.T) {
	for _, st := range []string{"running", "done"} {
		m := blockedChipModel("codex")
		m.flowLoopStatus = st
		view := stripANSI(m.View())
		if strings.Contains(view, "[Continue]") {
			t.Fatalf("%s: view must not render [Continue] when not blocked:\n%s", st, view)
		}
		if strings.Contains(view, "[Stop]") {
			t.Fatalf("%s: view must not render [Stop] action chip when not blocked:\n%s", st, view)
		}
	}
}

// TestBlockedBar_NoChipsWhenRunningChild: a blocked loop with a live child keeps
// turnIsActive()==true and must NOT render the blocked action bar (BUG-231 legacy
// "running child wins").
func TestBlockedBar_NoChipsWhenRunningChild(t *testing.T) {
	m := blockedChipModel("codex")
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-63960", AgentName: "main", Role: "main", Status: "completed"},
		{RunID: "run-63961", AgentName: "coder", Label: "codex-coder", Status: "running"},
	}
	if m.flowLoopBlocked() {
		t.Fatal("blocked loop with a running child must not read as parked")
	}
	view := stripANSI(m.View())
	if strings.Contains(view, "[Continue]") {
		t.Fatal("running-child blocked flow must not render [Continue] chip")
	}
}

// TestBlockedBar_ClickContinueUnparks: clicking [Continue] on a parked blocked
// flow POSTs /agent-loop/continue and applies the returned running graph —
// Desktop FlowAwaitingUser Continue parity, no slash command.
func TestBlockedBar_ClickContinueUnparks(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			var continueHit bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/agent-loop/continue") {
					continueHit = true
					json.NewEncoder(w).Encode(client.AgentGraphSnapshot{
						ParentRunID: "run-63960",
						LoopState:   client.AgentLoopState{Status: "running", Round: 4, RoundCap: 5},
						Runs: []client.AgentRunSummary{
							{RunID: "run-63960", AgentName: "main", Role: "main", Status: "running"},
						},
					})
					return
				}
				http.NotFound(w, r)
			}))
			defer srv.Close()

			m := New(config.ChatConfig{Provider: pk}, srv.URL)
			m.width, m.height = 100, 30
			m.asciiMode = true
			m.mode = ModeFlow
			m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf", Label: pk + "-flow"}
			m.runHandle = &client.RunHandle{RunID: "run-63960", Status: "running"}
			m.flowLoopStatus = "blocked"
			m.flowBlockReason = "cap"
			m.agentRuns = []client.AgentRunSummary{
				{RunID: "run-63960", AgentName: "main", Role: "main", Status: "completed"},
			}

			x, y, ok := findClickTarget(m, "continue")
			if !ok {
				t.Fatalf("%s: expected a clickable [Continue] chip", pk)
			}
			m2, cmd := m.dispatchMouseClick(x, y)
			if cmd == nil {
				t.Fatalf("%s: clicking [Continue] returned nil cmd", pk)
			}
			_ = m2
			msg := cmd()
			if _, ok := msg.(AgentGraphHydratedMsg); !ok {
				t.Fatalf("%s: [Continue] cmd returned %T, want AgentGraphHydratedMsg", pk, msg)
			}
			if !continueHit {
				t.Fatalf("%s: clicking [Continue] did not POST /agent-loop/continue", pk)
			}
			m3, _ := m.Update(msg)
			am := m3.(*AppModel)
			if am.flowLoopStatus != "running" {
				t.Fatalf("%s: after Continue click flowLoopStatus=%q want running", pk, am.flowLoopStatus)
			}
		})
	}
}

// TestBlockedBar_ClickStopEnds: clicking [Stop] on a parked blocked flow calls
// StopAgentLoop and seals the loop as stopped — Desktop FlowAwaitingUser Stop
// parity, no slash command.
func TestBlockedBar_ClickStopEnds(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			var stopHit bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/agent-loop/stop") {
					stopHit = true
					w.WriteHeader(http.StatusOK)
					return
				}
				if strings.Contains(r.URL.Path, "/interrupt") || strings.Contains(r.URL.Path, "/agents") {
					w.WriteHeader(http.StatusOK)
					return
				}
				http.NotFound(w, r)
			}))
			defer srv.Close()

			m := New(config.ChatConfig{Provider: pk}, srv.URL)
			m.width, m.height = 100, 30
			m.asciiMode = true
			m.mode = ModeFlow
			m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf", Label: pk + "-flow"}
			m.runHandle = &client.RunHandle{RunID: "run-63960", Status: "running"}
			m.flowLoopStatus = "blocked"
			m.flowBlockReason = "cap"
			m.agentRuns = []client.AgentRunSummary{
				{RunID: "run-63960", AgentName: "main", Role: "main", Status: "completed"},
			}

			x, y, ok := findClickTarget(m, "stop")
			if !ok {
				t.Fatalf("%s: expected a clickable [Stop] chip", pk)
			}
			m2, cmd := m.dispatchMouseClick(x, y)
			if cmd == nil {
				t.Fatalf("%s: clicking [Stop] returned nil cmd", pk)
			}
			_ = m2
			msg := cmd()
			if _, ok := msg.(StoppedMsg); !ok {
				t.Fatalf("%s: [Stop] cmd returned %T, want StoppedMsg", pk, msg)
			}
			if !stopHit {
				t.Fatalf("%s: clicking [Stop] did not POST /agent-loop/stop", pk)
			}
		})
	}
}

// TestBlockedBar_ContinueChipGoneAfterUnpark: once the flow is no longer blocked
// (e.g. Continue applied a running graph), the [Continue] chip disappears.
func TestBlockedBar_ContinueChipGoneAfterUnpark(t *testing.T) {
	m := blockedChipModel("codex")
	m2, _ := m.Update(AgentGraphMsg{Graph: &client.AgentGraphSnapshot{
		ParentRunID: "run-63960",
		LoopState:   client.AgentLoopState{Status: "running", Round: 4, RoundCap: 5},
		Runs: []client.AgentRunSummary{
			{RunID: "run-63960", AgentName: "main", Role: "main", Status: "running"},
		},
	}})
	am := m2.(*AppModel)
	view := stripANSI(am.View())
	if strings.Contains(view, "[Continue]") {
		t.Fatal("[Continue] chip must disappear once the flow is no longer blocked")
	}
}
