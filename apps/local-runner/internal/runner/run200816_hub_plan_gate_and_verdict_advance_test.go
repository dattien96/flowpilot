package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/flowgate"
)

// run-200816 (CP-58 S6 / task-harness): two issues on the plan_synthesis hub.
// 1) The flow hub ran the dev-doc gates (r-task fired "task reference detected
// but no task document found" because the hub's prose names Task-NNN while the
// plan phase is not the development phase) and auto-reprompted the hub.
// 2) Even after CA-735 made the prose turn escalate immediately, an approved
// plan still parked Retry on plan_synthesis instead of advancing to freeze.
// New file; no pre-existing test is modified.

// TestRun200816FlowHubGateRulesExcludeDocScope locks the Part-A rule filter:
// a Flow-engine hub keeps test/artifact rules but drops the dev-doc/scope
// family; Normal chat keeps everything.
func TestRun200816FlowHubGateRulesExcludeDocScope(t *testing.T) {
	rules := flowgate.DefaultRules()
	hubRules := flowHubGateRules(rules, true)
	for _, r := range hubRules {
		if flowgate.IsDocScopeRule(r.ID) {
			t.Fatalf("flow hub rules must drop doc/scope rule %q", r.ID)
		}
	}
	chatRules := flowHubGateRules(rules, false)
	if len(chatRules) != len(rules) {
		t.Fatalf("normal chat rules changed: %d -> %d", len(rules), len(chatRules))
	}
	foundTask := false
	for _, r := range chatRules {
		if r.ID == "r-task" {
			foundTask = true
		}
	}
	if !foundTask {
		t.Fatal("normal chat must keep r-task (BUG-152 root gate)")
	}
}

// TestRun200816FlowHubGateSkipsTaskDocReprompt locks the reported repro
// end-to-end through runFlowGateAtEpoch: a flow-engine hub whose prose names
// Task-NNN (with no task doc yet — the plan is in todo/, not done/) must NOT be
// r-task reprompted. The control (same turn, not flow-engine-driven) MUST be.
func TestRun200816FlowHubGateSkipsTaskDocReprompt(t *testing.T) {
	dir, head := newContractFreezeTestRepo(t)
	svc := newFreezeTestService(t)
	parent, startErr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if startErr != nil {
		t.Fatal(startErr)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	mk := func(flowHub bool) *interactiveRun {
		rs := newP4ChildRun(svc, "hub-turn", "", dir, head)
		rs.id = parent.RunID
		svc.mu.Lock()
		svc.runs[parent.RunID] = rs
		rs.workspaceCwd = dir
		rs.flowEngineDriven = flowHub
		svc.mu.Unlock()
		return rs
	}

	msg := "Plan review APPROVED for Task-904. Advancing to scope freeze."

	hub := mk(true)
	if svc.runFlowGateAtEpoch(context.Background(), hub, "turn-1", finalizeInput{FinalMessage: msg}, 0) {
		t.Fatal("flow hub gate must not r-task reprompt on a Task-NNN prose mention")
	}

	control := mk(false)
	if !svc.runFlowGateAtEpoch(context.Background(), control, "turn-1", finalizeInput{FinalMessage: msg}, 0) {
		t.Fatal("control (normal chat / non-flow root) must still block on r-task")
	}
}

// TestRun200816HubProseAllApprovedAdvancesToFreeze locks the Part-B reported
// repro across Claude/Codex/Grok: reviewers recorded approved machine verdicts,
// the hub finished in prose without submit_review_outcome → the flow must
// advance plan_synthesis to DONE (freeze), NOT park WAITING_USER_APPROVAL/Retry.
func TestRun200816HubProseAllApprovedAdvancesToFreeze(t *testing.T) {
	providers := []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
	for _, pk := range providers {
		t.Run(string(pk), func(t *testing.T) {
			svc, runID, _ := run198699HarnessService(t, pk)
			run199617HarnessTopology(t, svc, runID)
			seedHubProseVerdicts(t, svc, runID, map[string]string{"plan_reviewer": "approved"})

			if _, apiErr := svc.startTurn(runID, TurnInput{StepID: "step-hub-1", Prompt: "first hub turn"}, "", ""); apiErr != nil {
				t.Fatalf("%s: startTurn(1): %s", pk, apiErr.msg)
			}
			waitHubTurnSettled(t, svc, runID, "hub turn 1", func() bool {
				svc.mu.Lock()
				defer svc.mu.Unlock()
				return !svc.runs[runID].turnInFlight
			})

			if _, apiErr := svc.startTurn(runID, TurnInput{StepID: "step-hub-2", Prompt: "synthesis turn"}, "", ""); apiErr != nil {
				t.Fatalf("%s: startTurn(2): %s", pk, apiErr.msg)
			}
			waitHubTurnSettled(t, svc, runID, "hub turn 2", func() bool {
				return flowStepStatus(t, svc, runID, "plan_synthesis") == StepStatusDone
			})

			if got := flowStepStatus(t, svc, runID, "plan_synthesis"); got != StepStatusDone {
				t.Fatalf("%s: approved plan must advance plan_synthesis to DONE (freeze), got %v", pk, got)
			}
			if st := svc.agentOrchestrator.loopStateFor(runID); st.BlockReason == "hub_stalled" {
				t.Fatalf("%s: must not stall on an approved plan", pk)
			}
		})
	}
}

// TestRun200816HubProseChangesRequestedContinues locks the near-miss: a
// changes_requested machine verdict must route the prose hub turn back into the
// writer loop (continue → plan_writer), not park Retry.
func TestRun200816HubProseChangesRequestedContinues(t *testing.T) {
	svc, runID, ch := run198699HarnessService(t, ProviderKeyCodex)
	run199617HarnessTopology(t, svc, runID)
	seedHubProseVerdicts(t, svc, runID, map[string]string{"plan_reviewer": "changes_requested"})

	if _, apiErr := svc.startTurn(runID, TurnInput{StepID: "step-hub-1", Prompt: "first hub turn"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn(1): %s", apiErr.msg)
	}
	waitHubTurnSettled(t, svc, runID, "hub turn 1", func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[runID].turnInFlight
	})

	if _, apiErr := svc.startTurn(runID, TurnInput{StepID: "step-hub-2", Prompt: "synthesis turn"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn(2): %s", apiErr.msg)
	}
	waitHubTurnSettled(t, svc, runID, "hub turn 2", func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[runID].turnInFlight
	})

	if got := flowStepStatus(t, svc, runID, "plan_synthesis"); got == StepStatusWaitingUserApr {
		t.Fatal("changes_requested prose hub turn must continue, not park Retry")
	}
	if st := svc.agentOrchestrator.loopStateFor(runID); st.Status == "blocked" {
		t.Fatalf("loop must not block on changes_requested: %s", st.GateReason)
	}
	select {
	case req := <-ch:
		if strings.TrimSpace(req.Prompt) == "" {
			t.Fatalf("continue-spawned plan_writer prompt empty: %q", req.Prompt)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("plan_writer was not re-entered after changes_requested continue")
	}
	waitLoop(t, "plan_writer child spawned", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		for _, r := range svc.runs {
			if r.parentRunID == runID && r.label == "plan_writer" {
				return true
			}
		}
		return false
	})
}

// TestRun200816HubProseVerdictDerivationMatrix locks the derive helper: empty /
// blocked / mixed verdicts must keep BUG-226 escalate (CA-735), approved →
// done, any changes_requested → continue.
func TestRun200816HubProseVerdictDerivationMatrix(t *testing.T) {
	cases := []struct {
		name     string
		verdicts map[string]string
		want     string
	}{
		{"empty", nil, ""},
		{"all approved", map[string]string{"plan_reviewer": "approved", "reviewer": "approved"}, "done"},
		{"single approved", map[string]string{"plan_reviewer": "approved"}, "done"},
		{"any changes_requested", map[string]string{"plan_reviewer": "approved", "reviewer": "changes_requested"}, "continue"},
		{"blocked", map[string]string{"plan_reviewer": "blocked"}, ""},
		{"mixed unknown", map[string]string{"plan_reviewer": "approved", "reviewer": "weird"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, runID, _ := run198699HarnessService(t, ProviderKeyCodex)
			seedHubProseVerdicts(t, svc, runID, tc.verdicts)
			if got := svc.hubProseVerdictDerivesFlowStatus(runID); got != tc.want {
				t.Fatalf("derived = %q, want %q", got, tc.want)
			}
		})
	}
}

func seedHubProseVerdicts(t *testing.T, svc *InteractiveService, runID string, verdicts map[string]string) {
	t.Helper()
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if rs := svc.runs[runID]; rs != nil {
		rs.lastReviewCohortVerdicts = verdicts
	}
}