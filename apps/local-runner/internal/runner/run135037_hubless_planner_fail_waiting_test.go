package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

func ragHarnessNodes() []agentpack.FlowNode {
	return []agentpack.FlowNode{
		{ID: "preflight_contract_plan", Behavior: "agent.delegate", Agent: "agents/contract-planner.md"},
		{ID: "preflight_contract_freeze", Behavior: "contract.freeze"},
		{ID: "context", Behavior: "context.produce"},
		{ID: "implement", Behavior: "agent.code", Agent: "agents/coder.md"},
		{ID: "validate", Behavior: "command.validate"},
		{ID: "audit", Behavior: "artifact.audit_draft"},
	}
}

// CA-616: hub-less rag-harness delegate fail must park blocked/delegate_failed
// with FAILED + RejectionNote, not reinvoke hub, and hasActiveFlowChild false.
func TestRun135037_HublessPlannerFailParksBlockedWithReason(t *testing.T) {
	for _, tc := range []struct {
		name     string
		provider ProviderKey
		model    string
	}{
		{"grok", ProviderKeyGrok, "grok-4.5"},
		{"codex", ProviderKeyCodex, "gpt-5.4-mini"},
		{"claude", ProviderKeyClaude, "claude-sonnet"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
			// Always create with codex+gpt-5.4-mini (registered) then mutate to desired provider/model.
			parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini"})
			if err != nil {
				t.Fatalf("createRun: %v", err)
			}
			pid := parent.RunID
			svc.agentOrchestrator.setLoop(pid, AgentLoopState{Status: "running", Mode: "explicit", Cap: 3, RoundCap: 3})
			nodes := ragHarnessNodes()
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
			svc.setFlowStepStatus(context.Background(), pid, "preflight_contract_plan", StepStatusRunning)

			child, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini"})
			if err != nil {
				t.Fatalf("child create: %v", err)
			}
			svc.mu.Lock()
			crs := svc.runs[child.RunID]
			crs.parentRunID = pid
			crs.agentName = "contract-planner"
			crs.label = "preflight_contract_plan"
			crs.role = "contract-planner"
			crs.providerKey = tc.provider
			crs.modelName = tc.model
			crs.status = RunStatusRunning
			crs.agentStatus = string(RunStatusRunning)
			crs.waitForResult = false
			svc.mu.Unlock()
			svc.agentOrchestrator.registerChild(pid, child.RunID)

			errMsg := `The 'gpt-5.4' model is not supported when using Codex with a ChatGPT account.`
			svc.mu.Lock()
			svc.emitLocked(crs, ProviderEvent{Type: EventTurnFailed, Error: errMsg})
			svc.mu.Unlock()
			// let emit settle
			time.Sleep(30 * time.Millisecond)

			if got := flowStepStatus(t, svc, pid, "preflight_contract_plan"); got != StepStatusFailed {
				t.Fatalf("step FAILED = %v, want FAILED", got)
			}
			steps, _ := svc.workflowStore.LoadRunSteps(context.Background(), pid)
			var note string
			for _, st := range steps {
				if st.ID == "preflight_contract_plan" {
					note = st.RejectionNote
				}
			}
			if !strings.Contains(note, "gpt-5.4") {
				t.Fatalf("RejectionNote missing gpt-5.4, got %q", note)
			}
			loop := svc.agentOrchestrator.loopStateFor(pid)
			if loop.Status != "blocked" || loop.BlockReason != "delegate_failed" {
				t.Fatalf("loop = %+v, want blocked/delegate_failed", loop)
			}
			if !strings.Contains(loop.GateReason, "gpt-5.4") {
				t.Fatalf("GateReason missing gpt-5.4, got %q", loop.GateReason)
			}
			// must not have armed a hub reinvoke (hub-less)
			svc.mu.Lock()
			armed := svc.runs[pid].reinvokeInFlight || svc.runs[pid].pendingHubReinvoke
			failedID := svc.runs[pid].lastFailedDelegateNodeID
			svc.mu.Unlock()
			if armed {
				t.Fatal("hub-less fail must not arm hub reinvoke")
			}
			if failedID != "preflight_contract_plan" {
				t.Fatalf("lastFailedDelegateNodeID=%q, want preflight_contract_plan", failedID)
			}
			if svc.hasActiveFlowChild(pid) {
				t.Fatal("hasActiveFlowChild must be false after planner terminal Failed")
			}
			if svc.shouldParkHubWriteTurn(pid) {
				t.Fatal("shouldParkHubWriteTurn must be false after terminal fail (no hub_parked)")
			}
		})
	}
}

// Pre-adapter path (handleChildStartTurnFailure) also parks.
func TestRun135037_HublessPreflightStartFailParksBlocked(t *testing.T) {
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	parent, _ := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini"})
	pid := parent.RunID
	svc.agentOrchestrator.setLoop(pid, AgentLoopState{Status: "running", Mode: "explicit", Cap: 3, RoundCap: 3})
	nodes := ragHarnessNodes()
	svc.mu.Lock()
	p := svc.runs[pid]
	p.activeFlowNodes = nodes
	p.autoOrchestrate = true
	p.flowEngineDriven = true
	p.chatFlowRef = "flowpilot-core-flow-pack/rag-harness"
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(pid, nodes)
	svc.setFlowStepStatus(context.Background(), pid, "preflight_contract_plan", StepStatusRunning)

	child, _ := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini"})
	svc.mu.Lock()
	crs := svc.runs[child.RunID]
	crs.parentRunID = pid
	crs.agentName = "contract-planner"
	crs.label = "preflight_contract_plan"
	crs.role = "contract-planner"
	crs.providerKey = ProviderKeyCodex
	crs.status = RunStatusRunning
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(pid, child.RunID)
	svc.handleChildStartTurnFailure(child.RunID, pid, `The 'gpt-5.4' model is not supported when using Codex with a ChatGPT account.`)
	time.Sleep(20 * time.Millisecond)
	loop := svc.agentOrchestrator.loopStateFor(pid)
	if loop.Status != "blocked" || loop.BlockReason != "delegate_failed" {
		t.Fatalf("preflight start fail loop = %+v, want blocked/delegate_failed", loop)
	}
}

// Review-loop WITH hub.inline must keep CA-355 reinvoke behavior (no regress).
func TestRun135037_ReviewLoopWithHubStillReinvokesOnFail(t *testing.T) {
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "hub recovery"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, _ := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	pid := parent.RunID
	svc.agentOrchestrator.setLoop(pid, AgentLoopState{Status: "running", Mode: "explicit", Cap: 3, RoundCap: 3})
	nodes := []agentpack.FlowNode{
		{ID: "preflight_contract_plan", Behavior: "agent.delegate", Agent: "agents/contract-planner.md"},
		{ID: "preflight_contract_freeze", Behavior: "contract.freeze"},
		{ID: "coder", Behavior: "agent.delegate", Agent: "agents/coder.md"},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md"},
	}
	svc.mu.Lock()
	p := svc.runs[pid]
	p.activeFlowNodes = nodes
	p.autoOrchestrate = true
	p.flowEngineDriven = true
	p.activeHubNodeID = "synthesis"
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(pid, nodes)
	svc.setFlowStepStatus(context.Background(), pid, "preflight_contract_plan", StepStatusRunning)

	child, _ := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	svc.mu.Lock()
	crs := svc.runs[child.RunID]
	crs.parentRunID = pid
	crs.agentName = "contract-planner"
	crs.label = "preflight_contract_plan"
	crs.role = "contract-planner"
	crs.status = RunStatusRunning
	crs.agentStatus = string(RunStatusRunning)
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(pid, child.RunID)
	svc.mu.Lock()
	svc.emitLocked(crs, ProviderEvent{Type: EventTurnFailed, Error: "sim fail"})
	armedImmediate := svc.runs[pid].reinvokeInFlight
	svc.mu.Unlock()
	// CA-355: hub reinvoke should arm (reinvokeInFlight or pendingHubReinvoke or turnInFlight)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		svc.mu.Lock()
		pr := svc.runs[pid]
		armed := pr.reinvokeInFlight || pr.pendingHubReinvoke || pr.turnInFlight || pr.hubReinvokeStartFailCount > 0 || pr.turnCount > 0
		svc.mu.Unlock()
		if armed || armedImmediate {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("review-loop with hub.inline must still arm hub reinvoke on delegate fail (CA-355)")
}

// Continue after hub-less delegate fail must retry planner via reinvoke.
func TestRun135037_ContinueRetriesFailedPlanner(t *testing.T) {
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	parent, _ := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini"})
	pid := parent.RunID
	svc.agentOrchestrator.setLoop(pid, AgentLoopState{Status: "blocked", Mode: "explicit", Cap: 3, RoundCap: 3, BlockReason: "delegate_failed", GateReason: "gpt-5.4 not supported"})
	nodes := ragHarnessNodes()
	svc.mu.Lock()
	p := svc.runs[pid]
	p.activeFlowNodes = nodes
	p.activeFlowAcceptanceNodes = []string{"validate", "audit"}
	p.autoOrchestrate = true
	p.flowEngineDriven = true
	p.chatFlowRef = "flowpilot-core-flow-pack/rag-harness"
	p.lastFailedDelegateNodeID = "preflight_contract_plan"
	// Keep codex so reinvoked planner has a registered provider and doesn't immediately fail again.
	p.modelName = "gpt-5.4-mini"
	p.providerKey = ProviderKeyCodex
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(pid, nodes)
	svc.setFlowStepStatus(context.Background(), pid, "preflight_contract_plan", StepStatusFailed)

	child, _ := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini"})
	svc.mu.Lock()
	crs := svc.runs[child.RunID]
	crs.parentRunID = pid
	crs.agentName = "contract-planner"
	crs.label = "preflight_contract_plan"
	crs.role = "contract-planner"
	crs.status = RunStatusFailed
	crs.agentStatus = string(RunStatusFailed)
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(pid, child.RunID)
	svc.agentOrchestrator.upsertSummary(pid, AgentRunSummary{RunID: child.RunID, AgentName: "contract-planner", Role: "contract-planner", Label: "preflight_contract_plan", Status: RunStatusFailed, ParentRunID: pid})

	_, err := svc.resumeFlowWithFeedback(pid, "retry with correct model")
	if err != nil {
		t.Fatalf("resumeFlowWithFeedback: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	svc.mu.Lock()
	childAfter := svc.runs[child.RunID]
	status := childAfter.status
	lastFailed := svc.runs[pid].lastFailedDelegateNodeID
	loop := svc.agentOrchestrator.loopStateFor(pid)
	svc.mu.Unlock()
	if status == RunStatusFailed {
		t.Fatalf("child status after Continue = %v, want not Failed (reused)", status)
	}
	if lastFailed != "" {
		t.Fatalf("lastFailedDelegateNodeID not cleared, got %q", lastFailed)
	}
	if loop.Status != "running" {
		t.Fatalf("loop after Continue = %+v, want running", loop)
	}
	if got := flowStepStatus(t, svc, pid, "preflight_contract_plan"); got != StepStatusRunning {
		t.Fatalf("step after Continue = %v, want RUNNING", got)
	}
}
