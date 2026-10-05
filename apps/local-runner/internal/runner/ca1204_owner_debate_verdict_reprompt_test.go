package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// CA-1204 (live run-183756, run-174243): vibe-owner-debate owner legs are
// verdict_only cohort members whose verdicts feed debate_synthesis — but the
// overlay declares no acceptance_nodes and debate_synthesis is not a known
// cohort-gated hub, so the missing-verdict reprompt gate skipped them. A grok
// owner ending its turn with intent-prose ("locating owner def…") completed
// verdict-less, debate_synthesis had nothing to synthesize, submitted
// blocked, and the flow escalated to the user twice in one run.

var ca1204OwnerDebateNodes = []agentpack.FlowNode{
	{ID: "debate_trigger", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Join: "all"},
	{ID: "owner_1", Behavior: "agent.delegate", Agent: "agents/owner.md", Cohort: "owner_debate", Join: "all"},
	{ID: "owner_2", Behavior: "agent.delegate", Agent: "agents/owner.md", Cohort: "owner_debate", Join: "all"},
	{ID: "debate_synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Join: "all"},
}

func TestCA1204_OwnerDebateMemberMissingVerdictReprompts(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyGrok, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "still investigating the owner def"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyGrok})
	if err != nil {
		t.Fatalf("createRun(parent): %v", err)
	}
	child, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyGrok})
	if err != nil {
		t.Fatalf("createRun(child): %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.agentOrchestrator.preRegisterCohort(parent.RunID, "owner-debate-cohort", 2)
	svc.mu.Lock()
	p := svc.runs[parent.RunID]
	p.flowEngineDriven = true
	p.autoOrchestrate = true
	p.activeFlowNodes = ca1204OwnerDebateNodes
	rs := svc.runs[child.RunID]
	rs.parentRunID = parent.RunID
	rs.label = "owner_1"
	rs.stepID = "owner_1"
	rs.flowCohortId = "owner-debate-cohort"
	rs.status = RunStatusCompleted
	rs.pendingFlowGateSettle = false

	// Settle with NO recorded verdict — the owner leg must reprompt, not
	// append a verdict-less entry that starves debate_synthesis.
	svc.settleFlowChildTurnCompletedLocked(rs, "locating owner def / SS slice / TDD signatures", ProviderEvent{Type: EventTurnCompleted})
	if rs.verdictRepromptCount != 1 {
		svc.mu.Unlock()
		t.Fatalf("owner leg verdict-less completion must reprompt: count=%d, want 1", rs.verdictRepromptCount)
	}
	if rs.status != RunStatusRunning {
		svc.mu.Unlock()
		t.Fatalf("child status = %q after reprompt, want running", rs.status)
	}
	if entries := svc.agentOrchestrator.drainCohort(parent.RunID, "owner-debate-cohort"); len(entries) != 0 {
		svc.mu.Unlock()
		t.Fatalf("owner_debate cohort must NOT receive a verdict-less entry, got %d", len(entries))
	}
	svc.mu.Unlock()
}

// A non-owner leg inside the same mounted overlay (or a cohort member whose
// node's cohort is not owner_debate) keeps the old behavior — completing
// verdict-less without reprompt.
func TestCA1204_OwnerDebateNonCohortChildSkipsReprompt(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyGrok, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, _ := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyGrok})
	child, _ := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyGrok})
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.agentOrchestrator.preRegisterCohort(parent.RunID, "other-cohort", 1)
	svc.mu.Lock()
	p := svc.runs[parent.RunID]
	p.flowEngineDriven = true
	p.autoOrchestrate = true
	p.activeFlowNodes = ca1204OwnerDebateNodes
	rs := svc.runs[child.RunID]
	rs.parentRunID = parent.RunID
	rs.label = "debate_helper"
	rs.stepID = "debate_helper" // not an owner_debate cohort node
	rs.flowCohortId = "other-cohort"
	rs.status = RunStatusCompleted
	rs.pendingFlowGateSettle = false

	svc.settleFlowChildTurnCompletedLocked(rs, "done", ProviderEvent{Type: EventTurnCompleted})
	if rs.verdictRepromptCount != 0 {
		svc.mu.Unlock()
		t.Fatalf("non-owner-debate member must not reprompt, count=%d", rs.verdictRepromptCount)
	}
	svc.mu.Unlock()
}
