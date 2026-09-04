package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// run-199617 (CP-58 S6 / task-harness): plan_synthesis hub called
// submit_review_outcome in the TUI (tool_call display) but the runner saw no
// flow_control_received; the hub's provider turn finished in prose, and the
// run sat "RUNNING" until the 2m watchdog parked hub_stalled — because BUG-226
// ("hub synthesis turn completed without calling submit_review_outcome") never
// fired: `completed` (status Completed || pendingFlowGateSettle) is false for a
// flow-engine hub that stays status=running while the loop is live. New file;
// no pre-existing test is modified.

func run199617HarnessTopology(t *testing.T, svc *InteractiveService, runID string) {
	t.Helper()
	edges := []agentpack.FlowEdge{
		{From: "plan_reviewer", To: "plan_synthesis", When: "done", Kind: "forward"},
		{From: "plan_synthesis", To: "plan_writer", When: "continue", Kind: "back"},
		{From: "plan_synthesis", To: "preflight_contract_freeze", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "plan_writer", Behavior: "agent.delegate", Agent: "agents/doc-writer.md", Lifecycle: "reinvoke"},
		{ID: "plan_reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md", Lifecycle: "spawn"},
		{ID: "plan_synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke", Join: "all"},
		{ID: "preflight_contract_freeze", Behavior: "contract.freeze", Lifecycle: "once"},
	}
	setRun198699Topology(t, svc, runID, edges, nodes, "plan_synthesis")
	svc.setFlowStepStatus(context.Background(), runID, "plan_reviewer", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), runID, "plan_synthesis", StepStatusRunning)
}

func waitHubTurnSettled(t *testing.T, svc *InteractiveService, runID, name string, when func() bool) {
	t.Helper()
	waitLoop(t, name+" settles", 3*time.Second, when)
}

// TestRun199617HubProseWithoutToolEscalatesImmediately locks the reported
// repro across Claude/Codex/Grok (provider-agnostic shared engine path): a hub
// synthesis turn that finishes in prose without submit_review_outcome must
// BUG-226 escalate immediately — plan_synthesis WAITING_USER_APPROVAL + loop
// blocked with the prose-escalate reason — instead of idling 2 minutes into
// hub_stalled.
func TestRun199617HubProseWithoutToolEscalatesImmediately(t *testing.T) {
	providers := []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
	for _, pk := range providers {
		t.Run(string(pk), func(t *testing.T) {
			svc, runID, _ := run198699HarnessService(t, pk)
			run199617HarnessTopology(t, svc, runID)

			// Turn 1 — the hub's pre-cohort turn: review tool is not offered
			// yet (turnCount 1), so a prose completion must NOT escalate.
			if _, apiErr := svc.startTurn(runID, TurnInput{StepID: "step-hub-1", Prompt: "first hub turn"}, "", ""); apiErr != nil {
				t.Fatalf("%s: startTurn(1): %s", pk, apiErr.msg)
			}
			waitHubTurnSettled(t, svc, runID, "hub turn 1", func() bool {
				svc.mu.Lock()
				defer svc.mu.Unlock()
				return !svc.runs[runID].turnInFlight
			})
			if got := flowStepStatus(t, svc, runID, "plan_synthesis"); got != StepStatusRunning {
				t.Fatalf("%s: turn 1 prose must not escalate: plan_synthesis = %v", pk, got)
			}
			if st := svc.agentOrchestrator.loopStateFor(runID).Status; st != "running" {
				t.Fatalf("%s: turn 1 loop = %q, want running", pk, st)
			}

			// Turn 2 — the genuine synthesis turn (run-199617): tool offered,
			// model prose-answers without submit_review_outcome.
			if _, apiErr := svc.startTurn(runID, TurnInput{StepID: "step-hub-2", Prompt: "synthesis turn"}, "", ""); apiErr != nil {
				t.Fatalf("%s: startTurn(2): %s", pk, apiErr.msg)
			}
			waitHubTurnSettled(t, svc, runID, "hub turn 2", func() bool {
				return flowStepStatus(t, svc, runID, "plan_synthesis") == StepStatusWaitingUserApr
			})
			if got := flowStepStatus(t, svc, runID, "plan_synthesis"); got != StepStatusWaitingUserApr {
				t.Fatalf("%s: prose hub synthesis must escalate plan_synthesis, got %v", pk, got)
			}
			st := svc.agentOrchestrator.loopStateFor(runID)
			if st.Status != "blocked" {
				t.Fatalf("%s: loop = %q, want blocked (BUG-226 escalate)", pk, st.Status)
			}
			if !strings.Contains(st.GateReason, "completed without calling submit_review_outcome") {
				t.Fatalf("%s: gateReason = %q, want the BUG-226 prose-escalate reason", pk, st.GateReason)
			}
			if st.BlockReason == "hub_stalled" {
				t.Fatalf("%s: must escalate immediately, not wait 2m for hub_stalled", pk)
			}
		})
	}
}

// TestRun199617OpenCohortStillSkipsProseEscalate locks the near-miss guard
// (run-5296 / CA-360) against the run-199617 fix: when a reviewer cohort is
// open / in flight, the hub's prose turn must NOT BUG-226 escalate even though
// the provider turn completed — the fix widened "turn finished" to
// EventTurnCompleted, so it must not also bypass the open-cohort skip.
func TestRun199617OpenCohortStillSkipsProseEscalate(t *testing.T) {
	svc, runID, _ := run198699HarnessService(t, ProviderKeyCodex)
	run199617HarnessTopology(t, svc, runID)

	if _, apiErr := svc.startTurn(runID, TurnInput{StepID: "step-hub-1", Prompt: "first hub turn"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn(1): %s", apiErr.msg)
	}
	waitHubTurnSettled(t, svc, runID, "hub turn 1", func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[runID].turnInFlight
	})

	// Round-1 reviewers just spawned (continue) — open cohort, nothing buffered.
	svc.agentOrchestrator.preRegisterCohort(runID, "flow-auto-plan_writer-round-1", 1)

	if _, apiErr := svc.startTurn(runID, TurnInput{StepID: "step-hub-2", Prompt: "gate prose while reviewers run"}, "", ""); apiErr != nil {
		t.Fatalf("startTurn(2): %s", apiErr.msg)
	}
	waitHubTurnSettled(t, svc, runID, "hub turn 2", func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		return !svc.runs[runID].turnInFlight
	})

	if got := flowStepStatus(t, svc, runID, "plan_synthesis"); got != StepStatusRunning {
		t.Fatalf("open cohort must skip prose escalate (CA-360): plan_synthesis = %v", got)
	}
	if st := svc.agentOrchestrator.loopStateFor(runID).Status; st != "running" {
		t.Fatalf("open cohort loop = %q, want running", st)
	}
}