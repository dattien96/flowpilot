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

// plan_approval rename (live-tested run-206538): operators typed "approve ..."
// as feedback and looped the writer, because [Retry] read as "run again"
// while it really approves the plan and forwards freeze. The park now renders
// [Approve] (same retry target) plus a decision highlight. New file; the one
// touched old expectation lives in blocked_revise_chip_test.go.

// The plan_approval bar renders [Approve] (never [Retry]) with the approve
// copy and a decision highlight naming the only exit.
func TestPlanApproval_RendersApproveDecision(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			view := stripANSI(reviseBlockedModel(pk).View())
			if !strings.Contains(view, "[Approve]") {
				t.Fatalf("%s: plan_approval view must render [Approve]:\n%s", pk, view)
			}
			if strings.Contains(view, "[Retry]") {
				t.Fatalf("%s: plan_approval view must not render [Retry]:\n%s", pk, view)
			}
			if !strings.Contains(view, "approve plan, forward freeze") {
				t.Fatalf("%s: approve chip must carry the forward-freeze copy:\n%s", pk, view)
			}
			if !strings.Contains(view, "decision:") || !strings.Contains(view, "plan failed review") {
				t.Fatalf("%s: bar must highlight the failed-review decision:\n%s", pk, view)
			}
			for _, want := range []string{"[Revise]", "[Stop]"} {
				if !strings.Contains(view, want) {
					t.Fatalf("%s: bar must keep %s:\n%s", pk, want, view)
				}
			}
		})
	}
}

// Any other park keeps [Retry] and shows no plan decision line.
func TestPlanApproval_OtherParksKeepRetry(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			view := stripANSI(blockedChipModel(pk).View())
			if !strings.Contains(view, "[Retry]") {
				t.Fatalf("%s: cap park must keep [Retry]:\n%s", pk, view)
			}
			if strings.Contains(view, "[Approve]") {
				t.Fatalf("%s: cap park must not render [Approve]:\n%s", pk, view)
			}
			if strings.Contains(view, "plan failed review") {
				t.Fatalf("%s: cap park must not show the plan decision line:\n%s", pk, view)
			}
		})
	}
}

// Ring items keep target "retry" at index 0 under both labels, so Tab order,
// mouse dispatch, and the legacy /continue path never shift.
func TestPlanApproval_RingTargetStable(t *testing.T) {
	m := reviseBlockedModel("codex")
	items := m.actionRingItems()
	if len(items) < 3 || items[0].target != "retry" || items[0].label != "[Approve]" {
		t.Fatalf("plan_approval ring[0] = %+v, want {retry [Approve]}", items[0])
	}
	c := blockedChipModel("codex")
	citems := c.actionRingItems()
	if len(citems) < 2 || citems[0].target != "retry" || citems[0].label != "[Retry]" {
		t.Fatalf("cap ring[0] = %+v, want {retry [Retry]}", citems[0])
	}
}

func TestPlanApproval_ClickApproveUnparks(t *testing.T) {
	for _, pk := range []string{"claude", "codex", "grok"} {
		t.Run(pk, func(t *testing.T) {
			var continueHit bool
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/agent-loop/continue") {
					continueHit = true
					json.NewEncoder(w).Encode(client.AgentGraphSnapshot{
						ParentRunID: "run-1",
						LoopState:   client.AgentLoopState{Status: "running", Round: 0, RoundCap: 5},
						Runs: []client.AgentRunSummary{
							{RunID: "run-1", AgentName: "main", Role: "main", Status: "running"},
						},
					})
					return
				}
				http.NotFound(w, r)
			}))
			defer srv.Close()

			// Same model shape as TestBlockedBar_ClickContinueUnparks, plan park.
			m := New(config.ChatConfig{Provider: pk}, srv.URL)
			m.width, m.height = 100, 30
			m.asciiMode = true
			m.mode = ModeFlow
			m.launch = LaunchArm{Mode: ModeFlow, WorkflowID: "wf", Label: pk + "-flow"}
			m.runHandle = &client.RunHandle{RunID: "run-1", Status: "running"}
			m.flowLoopStatus = "blocked"
			m.flowBlockReason = "plan_approval"
			m.flowGateReason = "Plan revised after review (writer round 1)"
			m.agentRuns = []client.AgentRunSummary{{RunID: "run-1", AgentName: "main", Role: "main", Status: "completed"}}

			x, y, ok := findClickTarget(m, "retry")
			if !ok {
				t.Fatalf("%s: expected a clickable [Approve] (retry target) chip", pk)
			}
			m3, cmd := m.dispatchMouseClick(x, y)
			if cmd == nil {
				t.Fatalf("%s: clicking [Approve] returned nil cmd", pk)
			}
			msg := cmd()
			if _, ok := msg.(AgentGraphHydratedMsg); !ok {
				t.Fatalf("%s: [Approve] cmd returned %T, want AgentGraphHydratedMsg", pk, msg)
			}
			if !continueHit {
				t.Fatalf("%s: clicking [Approve] did not POST /agent-loop/continue", pk)
			}
			m4, _ := m3.(*AppModel).Update(msg)
			if am := m4.(*AppModel); am.flowLoopStatus != "running" {
				t.Fatalf("%s: after Approve click flowLoopStatus=%q want running", pk, am.flowLoopStatus)
			}
		})
	}
}
