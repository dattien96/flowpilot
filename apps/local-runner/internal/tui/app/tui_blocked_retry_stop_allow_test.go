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

// Task-309: Retry/Stop/Allow chips for frozen-contract scope drift.

// TestParseDriftedPaths extracts drifted paths from gate reason.
func TestParseDriftedPaths(t *testing.T) {
	tests := []struct {
		name string
		gate string
		want []string
	}{
		{"nil when not drift", "escalate", nil},
		{"single path", "flow scope drift: wrote outside the frozen contract's declared paths: calc_test.go", []string{"calc_test.go"}},
		{"multi path", "flow scope drift: wrote outside the frozen contract's declared paths: a.go, b.go", []string{"a.go", "b.go"}},
		{"with period", "flow scope drift: wrote outside the frozen contract's declared paths: calc_test.go.", []string{"calc_test.go"}},
		{"prefix text", "flow gate block: flow scope drift: wrote outside the frozen contract's declared paths: calc_test.go", []string{"calc_test.go"}},
		{"empty after marker", "wrote outside the frozen contract's declared paths: ", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDriftedPaths(tt.gate)
			if len(got) != len(tt.want) {
				t.Fatalf("parseDriftedPaths(%q)=%v want %v", tt.gate, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("parseDriftedPaths(%q)[%d]=%q want %q", tt.gate, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func driftBlockedModel(pk string, gateReason string) *AppModel {
	m := New(config.ChatConfig{Provider: pk}, "http://127.0.0.1:4317")
	m.width, m.height = 100, 30
	m.asciiMode = true
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf", Label: pk + "-flow"}
	m.runHandle = &client.RunHandle{RunID: "run-1", Status: "running"}
	m.flowLoopStatus = "blocked"
	m.flowBlockReason = "escalate"
	m.flowGateReason = gateReason
	m.agentRuns = []client.AgentRunSummary{{RunID: "run-1", AgentName: "main", Role: "main", Status: "completed"}}
	return m
}

func TestBlockedBar_RetryStopAlways_AllowOnlyOnDrift(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk+"_drift", func(t *testing.T) {
			m := driftBlockedModel(pk, "flow scope drift: wrote outside the frozen contract's declared paths: calc_test.go")
			view := stripANSI(m.View())
			if !strings.Contains(view, "[Retry]") {
				t.Fatalf("%s: drift view must contain [Retry]", pk)
			}
			if !strings.Contains(view, "[Stop]") {
				t.Fatalf("%s: drift view must contain [Stop]", pk)
			}
			if !strings.Contains(view, "[Allow]") {
				t.Fatalf("%s: drift view must contain [Allow] when drift gate", pk)
			}
			if !strings.Contains(view, "run again with old scope") {
				t.Fatalf("%s: drift view must contain Retry description", pk)
			}
			if !strings.Contains(view, "continue with new scope") {
				t.Fatalf("%s: drift view must contain Allow description", pk)
			}
		})
		t.Run(pk+"_cap_no_allow", func(t *testing.T) {
			m := driftBlockedModel(pk, "")
			m.flowBlockReason = "cap"
			m.flowGateReason = ""
			view := stripANSI(m.View())
			if !strings.Contains(view, "[Retry]") || !strings.Contains(view, "[Stop]") {
				t.Fatalf("%s: cap must still have Retry+Stop", pk)
			}
			if strings.Contains(view, "[Allow]") {
				t.Fatalf("%s: cap must NOT have Allow", pk)
			}
		})
		t.Run(pk+"_escalate_no_drift_no_allow", func(t *testing.T) {
			m := driftBlockedModel(pk, "Reviewer requested escalate: missing clamp")
			view := stripANSI(m.View())
			if strings.Contains(view, "[Allow]") {
				t.Fatalf("%s: non-drift escalate must NOT have Allow", pk)
			}
		})
		t.Run(pk+"_member_stalled_no_allow", func(t *testing.T) {
			m := driftBlockedModel(pk, "flow scope drift: wrote outside the frozen contract's declared paths: a.go")
			m.flowBlockReason = "member_stalled"
			m.flowGateReason = "flow scope drift: wrote outside the frozen contract's declared paths: a.go"
			view := stripANSI(m.View())
			// member_stalled uses retry/skip/stop path, but our implementation hides Allow for stalled/cap.
			// For now blockedReason member_stalled still goes through flowLoopBlocked but drift is hidden via stalled check.
			// We assert no Allow when stalled (defensive).
			if strings.Contains(view, "[Allow]") {
				t.Fatalf("%s: member_stalled must NOT show Allow (shows Retry member instead)", pk)
			}
		})
	}
}

func TestBlockedBar_ClickAllowPostsAmend(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			var amendHit bool
			var amendBody map[string][]string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/agent-loop/amend") {
					amendHit = true
					_ = json.NewDecoder(r.Body).Decode(&amendBody)
					json.NewEncoder(w).Encode(client.AgentGraphSnapshot{
						ParentRunID: "run-1",
						LoopState:   client.AgentLoopState{Status: "running", Round: 4, RoundCap: 5},
						Runs:        []client.AgentRunSummary{{RunID: "run-1", AgentName: "main", Role: "main", Status: "running"}},
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
			m.runHandle = &client.RunHandle{RunID: "run-1", Status: "running"}
			m.flowLoopStatus = "blocked"
			m.flowBlockReason = "escalate"
			m.flowGateReason = "flow scope drift: wrote outside the frozen contract's declared paths: calc_test.go"
			m.agentRuns = []client.AgentRunSummary{{RunID: "run-1", AgentName: "main", Role: "main", Status: "completed"}}

			x, y, ok := findClickTarget(m, "allow")
			if !ok {
				t.Fatalf("%s: expected clickable [Allow] chip", pk)
			}
			m2, cmd := m.dispatchMouseClick(x, y)
			if cmd == nil {
				t.Fatalf("%s: clicking [Allow] returned nil cmd", pk)
			}
			_ = m2
			msg := cmd()
			if _, ok := msg.(AgentGraphHydratedMsg); !ok {
				t.Fatalf("%s: [Allow] cmd returned %T want AgentGraphHydratedMsg", pk, msg)
			}
			if !amendHit {
				t.Fatalf("%s: clicking [Allow] did not POST /agent-loop/amend", pk)
			}
			if len(amendBody["paths"]) != 1 || amendBody["paths"][0] != "calc_test.go" {
				t.Fatalf("%s: amend body paths=%v want [calc_test.go]", pk, amendBody["paths"])
			}
			m3, _ := m.Update(msg)
			am := m3.(*AppModel)
			if am.flowLoopStatus != "running" {
				t.Fatalf("%s: after Allow click flowLoopStatus=%q want running", pk, am.flowLoopStatus)
			}
		})
	}
}

func TestBlockedBar_ClickRetryPostsContinueOnDrift(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			var continueHit bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/agent-loop/continue") {
					continueHit = true
					json.NewEncoder(w).Encode(client.AgentGraphSnapshot{
						ParentRunID: "run-1",
						LoopState:   client.AgentLoopState{Status: "running"},
					})
					return
				}
				http.NotFound(w, r)
			}))
			defer srv.Close()

			m := driftBlockedModel(pk, "flow scope drift: wrote outside the frozen contract's declared paths: calc_test.go")
			m.runnerURL = srv.URL
			x, y, ok := findClickTarget(m, "retry")
			if !ok {
				t.Fatalf("%s: expected clickable [Retry] chip", pk)
			}
			_, cmd := m.dispatchMouseClick(x, y)
			if cmd == nil {
				t.Fatalf("%s: clicking [Retry] returned nil cmd", pk)
			}
			_ = cmd()
			if !continueHit {
				t.Fatalf("%s: drift Retry must POST /agent-loop/continue (old scope)", pk)
			}
		})
	}
}

func TestBlockedBar_ClickAllowMultiPath(t *testing.T) {
	var captured []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string][]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		captured = body["paths"]
		json.NewEncoder(w).Encode(client.AgentGraphSnapshot{
			ParentRunID: "run-1",
			LoopState: client.AgentLoopState{Status: "running"},
		})
	}))
	defer srv.Close()
	m := New(config.ChatConfig{Provider: "codex"}, srv.URL)
	m.width, m.height = 100, 30
	m.asciiMode = true
	m.mode = ModeFlow
	m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf", Label: "codex-flow"}
	m.runHandle = &client.RunHandle{RunID: "run-1", Status: "running"}
	m.flowLoopStatus = "blocked"
	m.flowBlockReason = "escalate"
	m.flowGateReason = "flow scope drift: wrote outside the frozen contract's declared paths: a.go, b.go"
	m.agentRuns = []client.AgentRunSummary{{RunID: "run-1", AgentName: "main", Role: "main", Status: "completed"}}
	x, y, ok := findClickTarget(m, "allow")
	if !ok {
		t.Fatal("expected allow chip for multi-path drift")
	}
	_, cmd := m.dispatchMouseClick(x, y)
	if cmd == nil {
		t.Fatal("allow cmd nil")
	}
	_ = cmd()
	if len(captured) != 2 || captured[0] != "a.go" || captured[1] != "b.go" {
		t.Fatalf("multi-path amend got %v want [a.go b.go]", captured)
	}
}
