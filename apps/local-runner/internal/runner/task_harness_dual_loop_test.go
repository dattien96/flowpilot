package runner

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// CP-58 Task-305: live proofs that the task-harness dual loops re-enter their
// OWN writer. Fixture subsets follow the rag_harness_live_continue_back_edge
// pattern (CA-628): only edges whose targets are safe to advance in a fake
// provider environment — the freeze/validate inline handlers would escalate or
// fail-closed without a real workspace, so the done-path forward edges out of
// the hubs are omitted; the full topology is pinned by the agentpack pack
// tests instead. New file — no pre-existing test is modified.

// TestTaskHarnessPlanLoopContinueReusesPlanWriter drives the plan loop:
// plan_synthesis's flow_control("continue") must re-enter plan_writer on the
// SAME child session (lifecycle reinvoke) — never on implement, never a fresh
// child.
func TestTaskHarnessPlanLoopContinueReusesPlanWriter(t *testing.T) {
	var mu sync.Mutex
	var prompts []string
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				mu.Lock()
				prompts = append(prompts, req.Prompt)
				mu.Unlock()
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 3, RoundCap: 3})

	edges := []agentpack.FlowEdge{
		{From: "plan_writer", To: "plan_reviewer", When: "done", Kind: "forward"},
		{From: "plan_reviewer", To: "plan_synthesis", When: "done", Kind: "forward"},
		{From: "plan_synthesis", To: "plan_writer", When: "continue", Kind: "back"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "plan_writer", Behavior: "agent.delegate", Agent: "agents/coder.md", Lifecycle: "reinvoke"},
		{ID: "plan_reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md", Lifecycle: "spawn", Cohort: "plan", Join: "all"},
		{ID: "plan_synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke", Join: "all"},
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.activeHubNodeID = "plan_synthesis"
	svc.mu.Unlock()

	if _, err := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent: "agents/coder.md", Prompt: "write the Task plan doc", Label: "plan_writer", Wait: false,
	}); err != nil {
		t.Fatalf("spawn plan_writer: %v", err)
	}
	waitLoop(t, "plan_writer first turn completed", 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(prompts) >= 1
	})

	// Plan reviewer requested changes: the plan hub emits continue.
	fc, fcErr := svc.applyFlowControl(parent.RunID, FlowControlInput{
		Status: "continue", Summary: "plan missing DeclaredPaths on T-2",
	})
	if fcErr != nil {
		t.Fatalf("applyFlowControl(plan continue): %v", fcErr)
	}
	if fc.NextAction != "looping" {
		t.Fatalf("NextAction = %q, want looping", fc.NextAction)
	}

	waitLoop(t, "plan_writer reinvoked with plan review findings", 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, p := range prompts {
			if strings.Contains(p, "Feedback received") && strings.Contains(p, "missing DeclaredPaths") {
				return true
			}
		}
		return false
	})
	svc.mu.Lock()
	children := 0
	for _, run := range svc.runs {
		if run.parentRunID == parent.RunID && run.label == "plan_writer" {
			children++
		}
	}
	svc.mu.Unlock()
	if children != 1 {
		t.Fatalf("plan_writer child count = %d, want 1 (plan-loop session reuse)", children)
	}
}

// TestTaskHarnessCodeLoopContinueStaysOutOfPlanLoop proves the code hub's
// continue resolves the validate -> implement back-edge even though the
// plan loop's back-edge (plan_synthesis -> plan_writer) is declared earlier —
// the Task-304 source-aware routing — and never touches plan_writer.
func TestTaskHarnessCodeLoopContinueStaysOutOfPlanLoop(t *testing.T) {
	var mu sync.Mutex
	var prompts []string
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				mu.Lock()
				prompts = append(prompts, req.Prompt)
				mu.Unlock()
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Mode: "explicit", Cap: 3, RoundCap: 3})

	// BOTH back-edges declared, plan loop first — exactly the pack order the
	// pre-Task-304 first-match resolver would have mis-routed. The forward
	// edges feeding each hub are included because the hub-aware resolver picks
	// the loop anchor nearest the emitting hub via forward distance
	// (implement->validate is omitted per the rag-harness live-fixture
	// precedent: runValidateNode escalates without a real workspace).
	edges := []agentpack.FlowEdge{
		{From: "plan_synthesis", To: "plan_writer", When: "continue", Kind: "back"},
		{From: "plan_synthesis", To: "preflight_contract_freeze", When: "done", Kind: "forward"},
		{From: "preflight_contract_freeze", To: "test_signatures", When: "done", Kind: "forward"},
		{From: "test_signatures", To: "implement", When: "done", Kind: "forward"},
		{From: "reviewer", To: "synthesis", When: "done", Kind: "forward"},
		{From: "synthesis", To: "audit", When: "done", Kind: "forward"},
		{From: "validate", To: "implement", When: "continue", Kind: "back"},
		{From: "validate", To: "reviewer", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "plan_writer", Behavior: "agent.delegate", Agent: "agents/coder.md", Lifecycle: "reinvoke"},
		{ID: "plan_synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke", Join: "all"},
		{ID: "preflight_contract_freeze", Behavior: "contract.freeze", Lifecycle: "once"},
		{ID: "test_signatures", Behavior: "agent.code", Agent: "agents/tester.md", Lifecycle: "once"},
		{ID: "implement", Behavior: "agent.code", Agent: "agents/coder.md", Lifecycle: "reinvoke"},
		{ID: "validate", Behavior: "command.validate", Lifecycle: "once"},
		{ID: "reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md", Lifecycle: "spawn", Cohort: "review", Join: "all"},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke", Join: "all"},
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.activeHubNodeID = "synthesis"
	svc.mu.Unlock()

	if _, err := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent: "agents/coder.md", Prompt: "implement the approved plan", Label: "implement", Wait: false,
	}); err != nil {
		t.Fatalf("spawn implement: %v", err)
	}
	waitLoop(t, "implement first turn completed", 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(prompts) >= 1
	})

	fc, fcErr := svc.applyFlowControl(parent.RunID, FlowControlInput{
		Status: "continue", Summary: "code review: cover the restart case",
	})
	if fcErr != nil {
		t.Fatalf("applyFlowControl(code continue): %v", fcErr)
	}
	if fc.NextAction != "looping" {
		t.Fatalf("NextAction = %q, want looping", fc.NextAction)
	}

	waitLoop(t, "implement reinvoked with code review findings", 3*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		for _, p := range prompts {
			if strings.Contains(p, "Feedback received") && strings.Contains(p, "cover the restart case") {
				return true
			}
		}
		return false
	})
	svc.mu.Lock()
	implementChildren, planChildren := 0, 0
	for _, run := range svc.runs {
		if run.parentRunID != parent.RunID {
			continue
		}
		switch run.label {
		case "implement":
			implementChildren++
		case "plan_writer":
			planChildren++
		}
	}
	svc.mu.Unlock()
	if implementChildren != 1 {
		t.Fatalf("implement child count = %d, want 1 (code-loop session reuse)", implementChildren)
	}
	if planChildren != 0 {
		t.Fatalf("plan_writer child count = %d, want 0 (code hub continue must not enter the plan loop)", planChildren)
	}
}
