package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
)

func TestResolveFlowNodeModelAgentCodeHonorsStepRow(t *testing.T) {
	cases := []struct {
		provider string
		model    string
	}{
		{"claude", "claude-sonnet-4-5"},
		{"codex", "gpt-5.4-mini"},
		{"grok", "grok-4-5"},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			catalog := newInteractiveCatalog()
			catalog.steps["wf"] = []Step{{
				ID:       "task_harness__implement",
				Name:     "Implement",
				NodeID:   "implement",
				AgentRef: "agents/coder.md",
				Model:    tc.model,
			}}
			svc := newInteractiveService(DefaultProviderRegistry(), catalog, newFakeWorkflowStore())
			got := svc.resolveFlowNodeModel(context.Background(), "", agentpack.FlowNode{
				ID: "implement", Behavior: "agent.code", Agent: "agents/coder.md",
			})
			if got != tc.model {
				t.Fatalf("resolveFlowNodeModel(agent.code implement) = %q, want step_definitions.model %q", got, tc.model)
			}
		})
	}
}

func TestResolveFlowNodeModelTestSignaturesHonorsStepRow(t *testing.T) {
	catalog := newInteractiveCatalog()
	catalog.steps["wf"] = []Step{{
		ID:       "task_harness__test_signatures",
		Name:     "Test signatures",
		NodeID:   "test_signatures",
		AgentRef: "agents/tester.md",
		Model:    "gpt-5.4-mini",
	}}
	svc := newInteractiveService(DefaultProviderRegistry(), catalog, newFakeWorkflowStore())
	got := svc.resolveFlowNodeModel(context.Background(), "", agentpack.FlowNode{
		ID: "test_signatures", Behavior: "agent.code", Agent: "agents/tester.md",
	})
	if got != "gpt-5.4-mini" {
		t.Fatalf("resolveFlowNodeModel(agent.code test_signatures) = %q, want gpt-5.4-mini", got)
	}
}

func TestResolveFlowNodeModelAgentCodeEmptyInherits(t *testing.T) {
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	got := svc.resolveFlowNodeModel(context.Background(), "", agentpack.FlowNode{
		ID: "implement", Behavior: "agent.code", Agent: "agents/coder.md",
	})
	if got != "" {
		t.Fatalf("resolveFlowNodeModel(agent.code, no row) = %q, want empty inherit", got)
	}
}

func TestResolveFlowNodeModelInlineIgnoresStepRow(t *testing.T) {
	catalog := newInteractiveCatalog()
	catalog.steps["wf"] = []Step{{
		ID:       "task_harness__validate",
		Name:     "Validate",
		NodeID:   "validate",
		AgentRef: "agents/coder.md",
		Model:    "gpt-5.4-mini",
	}}
	svc := newInteractiveService(DefaultProviderRegistry(), catalog, newFakeWorkflowStore())
	got := svc.resolveFlowNodeModel(context.Background(), "", agentpack.FlowNode{
		ID: "validate", Behavior: "command.validate",
	})
	if got != "" {
		t.Fatalf("resolveFlowNodeModel(command.validate) = %q, want empty (inline)", got)
	}
}

func TestSpawnFrozenWriterChildUsesStepDefinitionModel(t *testing.T) {
	reg := registryWithClaudeAvailable()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, req TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	catalog := newInteractiveCatalog()
	catalog.steps["wf"] = []Step{{
		ID:       "task_harness__implement",
		Name:     "Implement",
		NodeID:   "implement",
		AgentRef: "agents/coder.md",
		Model:    "gpt-5.4-mini",
	}}
	svc := newInteractiveService(reg, catalog, newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyClaude, Model: "claude-sonnet",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	svc.markFlowEngineDriven(parent.RunID)

	writer := agentpack.FlowNode{
		ID: "implement", Behavior: "agent.code", Agent: "agents/coder.md", Lifecycle: "reinvoke",
	}
	if err := svc.spawnFrozenWriterChild(context.Background(), parent.RunID, writer, changecontract.FrozenContractRecord{
		ContractID: "c-impl", RunID: parent.RunID, CoderStepID: "implement", Intent: "write the code",
	}); err != nil {
		t.Fatalf("spawnFrozenWriterChild: %v", err)
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	var child *interactiveRun
	for _, rs := range svc.runs {
		if rs.parentRunID == parent.RunID && rs.label == "implement" {
			child = rs
			break
		}
	}
	if child == nil {
		t.Fatal("expected implement child run")
	}
	if child.modelName != "gpt-5.4-mini" {
		t.Fatalf("implement child model = %q, want step_definitions gpt-5.4-mini (not parent inherit %q)", child.modelName, "claude-sonnet")
	}
	if child.providerKey != ProviderKeyCodex {
		t.Fatalf("implement child provider = %q, want codex derived from gpt-5.4-mini", child.providerKey)
	}
}

func TestSpawnFrozenWriterChildEmptyStepModelInheritsParent(t *testing.T) {
	reg := registryWithClaudeAvailable()
	reg.register(ProviderRegistration{
		Key: ProviderKeyCodex, Status: ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true},
		newAdapter: func() ProviderRuntimeAdapter {
			return fakeAdapterFunc(func(_ context.Context, _ TurnRequest, b TurnBridge) error {
				b.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "ok"})
				return nil
			})
		},
	})
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyClaude, Model: "claude-sonnet",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	writer := agentpack.FlowNode{
		ID: "implement", Behavior: "agent.code", Agent: "agents/coder.md",
	}
	if err := svc.spawnFrozenWriterChild(context.Background(), parent.RunID, writer, changecontract.FrozenContractRecord{
		ContractID: "c-inherit", RunID: parent.RunID, CoderStepID: "implement", Intent: "write the code",
	}); err != nil {
		t.Fatalf("spawnFrozenWriterChild: %v", err)
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	var child *interactiveRun
	for _, rs := range svc.runs {
		if rs.parentRunID == parent.RunID && rs.label == "implement" {
			child = rs
			break
		}
	}
	if child == nil {
		t.Fatal("expected implement child run")
	}
	if child.modelName != "claude-sonnet" {
		t.Fatalf("empty step model must inherit parent %q, got %q", "claude-sonnet", child.modelName)
	}
}

