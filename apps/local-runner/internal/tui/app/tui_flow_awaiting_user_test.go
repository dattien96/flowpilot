package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"flowpilot-runner/internal/tui/client"
	"flowpilot-runner/internal/tui/config"
)

// BUG-231 parity with the Desktop awaiting-user suite
// (store.flow-blocked-terminal / store.flow-awaiting-user-409 /
// store.flow-awaiting-user-full-matrix): a flow whose orchestrator loop is
// "blocked" is deliberately parked awaiting a user Continue/Stop decision, NOT
// live-running. run-189839 showed [stop] on a parked blocked flow because the
// TUI treated the raw handle status "running" + orch SSE as live work.

func blockedFlowModel(pk string) *AppModel {
	m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf", Label: pk + "-flow"}
	m.runHandle = &client.RunHandle{RunID: "run-63960", Status: "running"}
	m.connStatus = ConnWaiting
	m.statusMsg = "flow running…"
	m.flowLoopStatus = "blocked"
	m.flowBlockReason = "cap"
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-63960", AgentName: "main", Role: "main", Status: "completed"},
		{RunID: "run-63961", AgentName: "coder", Label: "grok-coder", Status: "completed"},
	}
	return m
}

// TestTurnIsActive_BlockedFlowDoesNotArmStop mirrors the Desktop "blocked cap /
// escalate" terminal tests: a blocked loop with all children completed must NOT
// keep [stop] armed, even though the raw handle status is still "running" and an
// orch SSE listener is attached (run-189839).
func TestTurnIsActive_BlockedFlowDoesNotArmStop(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			m := blockedFlowModel(pk)
			m.orchStream = &orchStreamState{}
			if m.turnIsActive() {
				t.Fatalf("%s: blocked (awaiting-user) flow must not arm [stop]", pk)
			}
			if strings.Contains(stripANSI(m.renderStatusLine()), "[stop]") {
				t.Fatalf("%s: blocked flow statusline must not show [stop]:\n%s", pk, m.renderStatusLine())
			}
		})
	}
}

// TestTurnIsActive_BlockedFlowWithRunningChildStillArmsStop mirrors the Desktop
// "running child still wins over blocked loop (BUG-231)" legacy guard: a live
// child keeps [stop] armed even when the loop is blocked.
func TestTurnIsActive_BlockedFlowWithRunningChildStillArmsStop(t *testing.T) {
	m := blockedFlowModel("codex")
	m.agentRuns = []client.AgentRunSummary{
		{RunID: "run-63960", AgentName: "main", Role: "main", Status: "completed"},
		{RunID: "run-63961", AgentName: "coder", Label: "grok-coder", Status: "running"},
	}
	if !m.turnIsActive() {
		t.Fatal("blocked loop with a running child must keep [stop] armed")
	}
}

// TestAgentGraphMsg_BlockedSetsLoopAndBanner mirrors the Desktop 409→blocked +
// graph refresh mapping: an agent_graph_updated with loopState blocked must set
// flowLoopStatus + blockReason and surface a warn banner, and must NOT settle to
// a clean done state nor arm [stop].
func TestAgentGraphMsg_BlockedSetsLoopAndBanner(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		for _, br := range []string{"cap", "escalate", "member_stalled"} {
			t.Run(pk+"/"+br, func(t *testing.T) {
				m := blockedFlowModel(pk)
				m.flowLoopStatus = "running"
				m.flowBlockReason = ""
				m.messages = nil
				m2, _ := m.Update(AgentGraphMsg{Graph: &client.AgentGraphSnapshot{
					ParentRunID: "run-63960",
					LoopState:   client.AgentLoopState{Status: "blocked", Round: 3, RoundCap: 3, BlockReason: br},
					Runs: []client.AgentRunSummary{
						{RunID: "run-63960", AgentName: "main", Role: "main", Status: "completed"},
					},
				}})
				am := m2.(*AppModel)
				if am.flowLoopStatus != "blocked" {
					t.Fatalf("%s/%s: flowLoopStatus=%q want blocked", pk, br, am.flowLoopStatus)
				}
				if am.flowBlockReason != br {
					t.Fatalf("%s/%s: flowBlockReason=%q want %s", pk, br, am.flowBlockReason, br)
				}
				if am.turnIsActive() {
					t.Fatalf("%s/%s: blocked graph must not arm [stop]", pk, br)
				}
				if am.connStatus == ConnIdle || am.statusMsg == "done" {
					t.Fatalf("%s/%s: blocked loop must not settle to done (status=%v %q)", pk, br, am.connStatus, am.statusMsg)
				}
				if !hasWarnBanner(am.messages) {
					t.Fatalf("%s/%s: blocked graph must surface an awaiting-user warn banner", pk, br)
				}
			})
		}
	}
}

func hasWarnBanner(msgs []ChatMessage) bool {
	for _, mm := range msgs {
		if mm.FormatHint == "warn" && strings.Contains(mm.Content, "waiting for you") {
			return true
		}
	}
	return false
}

// TestAgentGraphMsg_BlockedThenDoneSettles: a blocked flow that later reports
// loop done must settle to a clean done state and drop [stop].
func TestAgentGraphMsg_BlockedThenDoneSettles(t *testing.T) {
	m := blockedFlowModel("codex")
	m2, _ := m.Update(AgentGraphMsg{Graph: &client.AgentGraphSnapshot{
		ParentRunID: "run-63960",
		LoopState:   client.AgentLoopState{Status: "done", Round: 1, RoundCap: 3},
		Runs: []client.AgentRunSummary{
			{RunID: "run-63960", AgentName: "main", Role: "main", Status: "completed"},
		},
	}})
	am := m2.(*AppModel)
	if am.flowLoopStatus != "done" {
		t.Fatalf("flowLoopStatus=%q want done", am.flowLoopStatus)
	}
	if am.connStatus != ConnIdle || am.statusMsg != "done" {
		t.Fatalf("done graph must settle (status=%v %q)", am.connStatus, am.statusMsg)
	}
	if am.turnIsActive() {
		t.Fatal("settled done flow must not arm [stop]")
	}
}

// TestAgentGraphMsg_StaleParentIgnored mirrors the Desktop "switching run discards
// late graph" + "late blocked refresh does not overwrite Stop" guards: a graph for
// a different parent must never overwrite the current loop state.
func TestAgentGraphMsg_StaleParentIgnored(t *testing.T) {
	m := blockedFlowModel("codex")
	m2, _ := m.Update(AgentGraphMsg{Graph: &client.AgentGraphSnapshot{
		ParentRunID: "run-OTHER",
		LoopState:   client.AgentLoopState{Status: "running", Round: 0, RoundCap: 3},
		Runs: []client.AgentRunSummary{
			{RunID: "run-OTHER", AgentName: "main", Role: "main", Status: "running"},
		},
	}})
	am := m2.(*AppModel)
	if am.flowLoopStatus != "blocked" {
		t.Fatalf("stale parent graph overwrote loop status: got %q want blocked", am.flowLoopStatus)
	}
	if am.flowBlockReason != "cap" {
		t.Fatalf("stale parent graph overwrote blockReason: got %q want cap", am.flowBlockReason)
	}
}

// TestTurnStreamClosed_FlowAwaitingUserParks mirrors the Desktop 409→blocked
// matrix: a freeform turn answered 409 flow_awaiting_user must PARK the flow
// (keep the handle, set blocked, warn banner, refresh graph) — never drop the
// handle to a dead error state.
func TestTurnStreamClosed_FlowAwaitingUserParks(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		for _, br := range []string{"cap", "escalate", "member_stalled"} {
			t.Run(pk+"/"+br, func(t *testing.T) {
				var graphHits, runsHits int
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch {
					case strings.Contains(r.URL.Path, "/agent-graph"):
						graphHits++
						json.NewEncoder(w).Encode(client.AgentGraphSnapshot{
							ParentRunID: "run-63960",
							LoopState:   client.AgentLoopState{Status: "blocked", Round: 3, RoundCap: 3, BlockReason: br},
							Runs: []client.AgentRunSummary{
								{RunID: "run-63960", AgentName: "main", Role: "main", Status: "completed"},
							},
						})
					case strings.Contains(r.URL.Path, "/agent-runs") || strings.Contains(r.URL.Path, "/agents"):
						runsHits++
						json.NewEncoder(w).Encode([]client.AgentRunSummary{})
					default:
						http.NotFound(w, r)
					}
				}))
				defer srv.Close()

				m := New(config.ChatConfig{Provider: pk}, srv.URL)
				m.width, m.height = 100, 30
				m.mode = ModeFlow
				m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf", Label: pk + "-flow"}
				m.runHandle = &client.RunHandle{RunID: "run-63960", Status: "running"}
				m.flowLoopStatus = "running"
				m.messages = nil

				m2, _ := m.Update(turnStreamClosedMsg{
					Err: &client.APIError{Status: 409, Code: "flow_awaiting_user", Message: "flow is waiting for your decision"},
				})
				am := m2.(*AppModel)
				if am.runHandle == nil {
					t.Fatalf("%s/%s: awaiting-user 409 must keep the run handle (was dropped)", pk, br)
				}
				if am.connStatus == ConnError || am.statusMsg == "turn failed" {
					t.Fatalf("%s/%s: awaiting-user 409 must not be an error (status=%v %q)", pk, br, am.connStatus, am.statusMsg)
				}
				if am.flowLoopStatus != "blocked" {
					t.Fatalf("%s/%s: flowLoopStatus=%q want blocked", pk, br, am.flowLoopStatus)
				}
				if !hasWarnBanner(am.messages) {
					t.Fatalf("%s/%s: awaiting-user park must surface a warn banner", pk, br)
				}
			})
		}
	}
}

// TestTurnStreamClosed_NonAwaitingUserStillErrors: a non-awaiting-user error
// (e.g. 500 or 409 invalid_request) must still take the failure path and drop
// the handle — never be misclassified as a blocked park.
func TestTurnStreamClosed_NonAwaitingUserStillErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"500 internal", &client.APIError{Status: 500, Code: "internal", Message: "boom"}},
		{"409 invalid_request", &client.APIError{Status: 409, Code: "invalid_request", Message: "bad shape"}},
		{"500 decision wording", &client.APIError{Status: 500, Code: "internal", Message: "waiting for your decision later"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := blockedFlowModel("codex")
			m2, _ := m.Update(turnStreamClosedMsg{Err: tc.err})
			am := m2.(*AppModel)
			if am.runHandle != nil {
				t.Fatalf("%s: non-awaiting-user error must drop handle, got %v", tc.name, am.runHandle)
			}
			if am.connStatus != ConnError {
				t.Fatalf("%s: connStatus=%v want ConnError", tc.name, am.connStatus)
			}
		})
	}
}

// TestSlashContinue_UnparksBlocked mirrors the Desktop "Continue after 409
// unparks": /continue on a parked blocked flow POSTs agent-loop/continue and the
// returned graph (loop running) must be applied.
func TestSlashContinue_UnparksBlocked(t *testing.T) {
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
			m.mode = ModeFlow
			m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf", Label: pk + "-flow"}
			m.runHandle = &client.RunHandle{RunID: "run-63960", Status: "running"}
			m.flowLoopStatus = "blocked"
			m.flowBlockReason = "cap"
			m.agentRuns = []client.AgentRunSummary{
				{RunID: "run-63960", AgentName: "main", Role: "main", Status: "completed"},
			}

			m2, cmd := m.handleSlashCommand("/continue")
			if cmd == nil {
				t.Fatalf("%s: /continue returned nil cmd", pk)
			}
			_ = m2
			// Run the returned cmd, expect an AgentGraphHydratedMsg.
			msg := cmd()
			if _, ok := msg.(AgentGraphHydratedMsg); !ok {
				t.Fatalf("%s: /continue cmd returned %T, want AgentGraphHydratedMsg", pk, msg)
			}
			if !continueHit {
				t.Fatalf("%s: /continue did not POST /agent-loop/continue", pk)
			}
			m3, _ := m.Update(msg)
			am := m3.(*AppModel)
			if am.flowLoopStatus != "running" {
				t.Fatalf("%s: after continue flowLoopStatus=%q want running", pk, am.flowLoopStatus)
			}
		})
	}
}

// TestSlashStop_OnBlockedStillStops: /stop remains valid on a parked blocked flow
// (Desktop FlowAwaitingUser Stop parity) and seals the loop as stopped.
func TestSlashStop_OnBlockedStillStops(t *testing.T) {
	m := blockedFlowModel("codex")
	_, cmd := m.handleSlashCommand("/stop")
	if cmd == nil {
		t.Fatal("/stop returned nil cmd")
	}
	msg := cmd()
	if _, ok := msg.(StoppedMsg); !ok {
		t.Fatalf("/stop cmd returned %T, want StoppedMsg", msg)
	}
	m3, _ := m.Update(msg)
	am := m3.(*AppModel)
	if am.flowLoopStatus != "stopped" {
		t.Fatalf("flowLoopStatus=%q want stopped after /stop", am.flowLoopStatus)
	}
}

// TestClient_GetAgentGraphAndContinueFlow verifies the client wire contracts for
// the new endpoints (GET /agent-graph, POST /agent-loop/continue).
func TestClient_GetAgentGraphAndContinueFlow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/agent-graph"):
			json.NewEncoder(w).Encode(client.AgentGraphSnapshot{
				ParentRunID: "run-63960",
				LoopState:   client.AgentLoopState{Status: "blocked", Round: 3, RoundCap: 3, BlockReason: "escalate", GateReason: "reviewer requested escalate", OpenIssues: 1, ActiveNode: "synthesis"},
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/agent-loop/continue"):
			json.NewEncoder(w).Encode(client.AgentGraphSnapshot{
				ParentRunID: "run-63960",
				LoopState:   client.AgentLoopState{Status: "running", Round: 4, RoundCap: 5},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	cl := client.New(srv.URL)

	g, err := cl.GetAgentGraph(context.Background(), "run-63960")
	if err != nil {
		t.Fatalf("GetAgentGraph: %v", err)
	}
	if g.LoopState.Status != "blocked" || g.LoopState.BlockReason != "escalate" || g.LoopState.OpenIssues != 1 {
		t.Fatalf("GetAgentGraph decode wrong: %+v", g.LoopState)
	}

	cg, err := cl.ContinueFlow(context.Background(), "run-63960")
	if err != nil {
		t.Fatalf("ContinueFlow: %v", err)
	}
	if cg.LoopState.Status != "running" {
		t.Fatalf("ContinueFlow status=%q want running", cg.LoopState.Status)
	}
}

// TestClient_IsFlowAwaitingUserErrorNotRetryable locks the classification: a 409
// flow_awaiting_user must not be retried by SendTurn's retry loop.
func TestClient_IsFlowAwaitingUserErrorNotRetryable(t *testing.T) {
	for _, code := range []string{"flow_awaiting_user", "flow_awaiting_user"} {
		err := &client.APIError{Status: 409, Code: code, Message: "flow is waiting for your decision"}
		if !client.IsFlowAwaitingUserError(err) {
			t.Fatalf("IsFlowAwaitingUserError(%q) = false, want true", code)
		}
		if client.IsRetryableAPIError(err) {
			t.Fatalf("IsRetryableAPIError(%q) = true, want false (must not retry)", code)
		}
	}
	// Non-awaiting 409 stays retryable.
	if !client.IsRetryableAPIError(&client.APIError{Status: 409, Code: "turn_in_progress", Message: "busy"}) {
		t.Fatal("plain 409 turn_in_progress must stay retryable")
	}
}
