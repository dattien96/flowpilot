package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

func ragHarnessNodes142155() []agentpack.FlowNode {
	return []agentpack.FlowNode{
		{ID: "preflight_contract_plan", Behavior: "agent.delegate", Agent: "agents/contract-planner.md"},
		{ID: "preflight_contract_freeze", Behavior: "contract.freeze"},
		{ID: "context", Behavior: "context.produce"},
		{ID: "test_signatures", Behavior: "agent.delegate", Agent: "agents/tester.md"},
		{ID: "implement", Behavior: "agent.code", Agent: "agents/coder.md"},
		{ID: "validate", Behavior: "command.validate"},
		{ID: "reviewer", Behavior: "agent.review", Agent: "agents/reviewer.md"},
		{ID: "synthesis", Behavior: "hub.inline"},
		{ID: "audit", Behavior: "artifact.audit_draft"},
	}
}

// CA-632: run-142155 — rag-harness reviewer (cohort member) failed with
// "no connected local account found for provider \"codex\"" but the TUI showed
// "(no detail from runner)". Root cause: the cohort EventTurnFailed branch
// settled the member's node with setFlowStepStatusLocked (no RejectionNote),
// unlike the non-cohort delegate-fail path (CA-616). The branch now stamps the
// real error via setFlowStepFailedWithReasonLocked.

func TestRun142155_CohortMemberFailStampsRejectionNote(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider ProviderKey
		model    string
	}{
		{"grok", ProviderKeyGrok, "grok-4.5"},
		{"codex", ProviderKeyCodex, "gpt-5.4"},
		{"claude", ProviderKeyClaude, "claude-sonnet"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
			parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini"})
			if err != nil {
				t.Fatalf("createRun parent: %v", err)
			}
			pid := parent.RunID
			svc.agentOrchestrator.setLoop(pid, AgentLoopState{Status: "running", Mode: "explicit", Cap: 3, RoundCap: 3})
			nodes := ragHarnessNodes142155()
			svc.mu.Lock()
			p := svc.runs[pid]
			p.activeFlowNodes = nodes
			p.activeFlowAcceptanceNodes = []string{"validate", "audit"}
			p.autoOrchestrate = true
			p.flowEngineDriven = true
			p.chatFlowRef = "flowpilot-core-flow-pack/rag-harness"
			p.modelName = tc.model
			p.providerKey = tc.provider
			svc.mu.Unlock()
			svc.reseedFlowStepRuntime(pid, nodes)

			child, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini"})
			if err != nil {
				t.Fatalf("child create: %v", err)
			}
			svc.mu.Lock()
			crs := svc.runs[child.RunID]
			crs.parentRunID = pid
			crs.agentName = "reviewer"
			crs.label = "reviewer"
			crs.role = "reviewer"
			crs.providerKey = tc.provider
			crs.modelName = tc.model
			crs.status = RunStatusRunning
			crs.agentStatus = string(RunStatusRunning)
			crs.waitForResult = false
			crs.flowCohortId = "flow-auto-validate-round-0"
			svc.mu.Unlock()
			svc.agentOrchestrator.registerChild(pid, child.RunID)

			errMsg := `no connected local account found for provider "codex"`
			svc.mu.Lock()
			svc.emitLocked(crs, ProviderEvent{Type: EventTurnFailed, Error: errMsg})
			svc.mu.Unlock()
			time.Sleep(30 * time.Millisecond)

			if got := flowStepStatus(t, svc, pid, "reviewer"); got != StepStatusFailed {
				t.Fatalf("[%s] reviewer step FAILED = %v, want FAILED", tc.name, got)
			}
			steps, _ := svc.workflowStore.LoadRunSteps(context.Background(), pid)
			var note string
			for _, st := range steps {
				if st.ID == "reviewer" {
					note = st.RejectionNote
				}
			}
			if !strings.Contains(note, "codex") {
				t.Fatalf("[%s] RejectionNote missing codex, got %q", tc.name, note)
			}
		})
	}
}
