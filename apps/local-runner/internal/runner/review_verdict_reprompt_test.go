package runner

import (
	"context"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// TestCohortMemberMissingVerdictReprompts is the CP-67 live-finding regression
// (run-8853): on deferred-tool transports a reviewer can end its turn with the
// submit_review_outcome call cancelled in flight, so the verdict never reaches
// the bridge. Completing the member verdict-less parks the hub on
// missing_review_verdict forever. The settle path must reprompt the child
// (bounded) instead of appending a verdict-less cohort result.
func TestCohortMemberMissingVerdictReprompts(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyClaude, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyClaude})
	if err != nil {
		t.Fatalf("createRun(parent): %v", err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyClaude})
	if err != nil {
		t.Fatalf("createRun(child): %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.agentOrchestrator.preRegisterCohort(parent.RunID, "plan-cohort", 1)
	svc.mu.Lock()
	p := svc.runs[parent.RunID]
	p.flowEngineDriven = true
	p.autoOrchestrate = true
	p.stallTimeout = 5 * time.Second
	// task-harness shape: a plan cohort node feeding plan_synthesis means the
	// hub's done edge requires machine verdicts from cohort members.
	p.activeFlowNodes = []agentpack.FlowNode{
		{ID: "plan_reviewer", Cohort: "plan", Join: "all"},
		{ID: "plan_synthesis"},
	}
	rs := svc.runs[child.RunID]
	rs.parentRunID = parent.RunID
	rs.label = "plan_reviewer"
	rs.flowCohortId = "plan-cohort"
	rs.stepID = "step-plan-reviewer"
	rs.status = RunStatusCompleted
	rs.pendingFlowGateSettle = false

	// Settle with NO recorded verdict — must reprompt, not append.
	svc.settleFlowChildTurnCompletedLocked(rs, "reviewed in prose only", ProviderEvent{Type: EventTurnCompleted})
	if rs.verdictRepromptCount != 1 {
		svc.mu.Unlock()
		t.Fatalf("verdictRepromptCount = %d, want 1", rs.verdictRepromptCount)
	}
	if rs.status != RunStatusRunning {
		svc.mu.Unlock()
		t.Fatalf("child status = %q after reprompt, want running", rs.status)
	}
	if entries := svc.agentOrchestrator.drainCohort(parent.RunID, "plan-cohort"); len(entries) != 0 {
		svc.mu.Unlock()
		t.Fatalf("cohort must NOT receive a verdict-less entry, got %d", len(entries))
	}
	svc.mu.Unlock()
	// drainCohort clears the expected count too — re-register so the final
	// settle below completes the cohort through the normal join path.
	svc.agentOrchestrator.preRegisterCohort(parent.RunID, "plan-cohort", 1)

	// Second miss reprompts again; third miss exhausts the cap and the member
	// completes verdict-less (hub then blocks missing_review_verdict as before).
	svc.mu.Lock()
	rs.status = RunStatusCompleted
	svc.settleFlowChildTurnCompletedLocked(rs, "still no verdict", ProviderEvent{Type: EventTurnCompleted})
	if rs.verdictRepromptCount != 2 || rs.status != RunStatusRunning {
		svc.mu.Unlock()
		t.Fatalf("second reprompt: count=%d status=%q, want 2/running", rs.verdictRepromptCount, rs.status)
	}
	rs.status = RunStatusCompleted
	svc.settleFlowChildTurnCompletedLocked(rs, "third miss", ProviderEvent{Type: EventTurnCompleted})
	if rs.verdictRepromptCount != 2 {
		svc.mu.Unlock()
		t.Fatalf("reprompt cap exceeded: count=%d", rs.verdictRepromptCount)
	}
	// expected=1 so the append completes the cohort and the join drains it —
	// the verdict snapshot proves the member landed verdict-less.
	if got := p.lastReviewCohortVerdicts["plan_reviewer"]; got != "" {
		svc.mu.Unlock()
		t.Fatalf("after cap the member completes verdict-less, got %q", got)
	}
	svc.mu.Unlock()
}

// TestCohortMemberRecordedVerdictSkipsReprompt pins the happy path: a recorded
// verdict flows into the cohort entry and no reprompt fires.
func TestCohortMemberRecordedVerdictSkipsReprompt(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyClaude, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, _ := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyClaude})
	child, _ := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyClaude})
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.agentOrchestrator.preRegisterCohort(parent.RunID, "plan-cohort", 1)
	svc.mu.Lock()
	p := svc.runs[parent.RunID]
	p.flowEngineDriven = true
	p.autoOrchestrate = true
	p.activeFlowNodes = []agentpack.FlowNode{
		{ID: "plan_reviewer", Cohort: "plan", Join: "all"},
		{ID: "plan_synthesis"},
	}
	rs := svc.runs[child.RunID]
	rs.parentRunID = parent.RunID
	rs.label = "plan_reviewer"
	rs.flowCohortId = "plan-cohort"
	rs.status = RunStatusCompleted
	svc.mu.Unlock()

	svc.recordReviewCohortMemberVerdict(parent.RunID, "plan_reviewer", "approved")

	svc.mu.Lock()
	svc.settleFlowChildTurnCompletedLocked(rs, "approved with verdicts", ProviderEvent{Type: EventTurnCompleted})
	if rs.verdictRepromptCount != 0 {
		svc.mu.Unlock()
		t.Fatalf("recorded verdict must not reprompt, count=%d", rs.verdictRepromptCount)
	}
	if got := p.lastReviewCohortVerdicts["plan_reviewer"]; got != "approved" {
		svc.mu.Unlock()
		t.Fatalf("cohort verdict snapshot must carry approved, got %q", got)
	}
	svc.mu.Unlock()
}
