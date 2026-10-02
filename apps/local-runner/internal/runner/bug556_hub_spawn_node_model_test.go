package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// BUG-556 (live run-38799, PrivateVault CP-02): the vibe-sprint reviewer node
// is configured grok-4.7 in step_definitions, but the hub's ad-hoc MCP
// spawn_agent call carries no model (SpawnAgentInput.Model is json:"-" —
// internal-only) — the child spawned as devin/swe-2-high, the parent run's
// inherited model. The UI showed two different models for one leg: the step
// card (config) vs the child record (actual). Fix: spawnChildRun resolves the
// node's configured model from the parent's active flow nodes when the spawn
// carries none — the same resolveFlowNodeModel the engine dispatch uses.

// bug556CompleteAdapter stands in for the providers the test registry lacks a
// controlled runtime for (grok/claude in this harness).
type bug556CompleteAdapter struct{ key ProviderKey }

func (a *bug556CompleteAdapter) Key() ProviderKey { return a.key }
func (a *bug556CompleteAdapter) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{Streaming: true}
}
func (a *bug556CompleteAdapter) SendTurn(_ context.Context, _ TurnRequest, bridge TurnBridge) error {
	bridge.Emit(ProviderEvent{Type: EventTurnCompleted, FinalMessage: "done"})
	return nil
}

func bug556Registry() *ProviderRegistry {
	reg := DefaultProviderRegistry()
	for _, key := range []ProviderKey{ProviderKeyGrok, ProviderKeyClaude, ProviderKeyDevin} {
		k := key
		reg.register(ProviderRegistration{
			Key:          k,
			DisplayName:  string(k),
			Status:       ProviderStatusAvailable,
			Capabilities: ProviderCapabilities{Streaming: true},
			newAdapter:   func() ProviderRuntimeAdapter { return &bug556CompleteAdapter{key: k} },
		})
	}
	return reg
}

func TestBug556_HubSpawnResolvesNodeConfiguredModel(t *testing.T) {
	catalog := newInteractiveCatalog()
	catalog.steps["wf"] = []Step{{
		ID:       "vibe_sprint_reviewer",
		Name:     "Reviewer",
		NodeID:   "reviewer",
		AgentRef: "agents/reviewer.md",
		Model:    "grok-4.7",
	}}
	svc := newInteractiveService(bug556Registry(), catalog, newFakeWorkflowStore())

	parent, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Cwd: t.TempDir(),
	})
	if apiErr != nil {
		t.Fatalf("parent createRun: %v", apiErr)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.modelName = "devin/swe-2-high"
	rs.flowEngineDriven = true
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md"},
	}
	svc.mu.Unlock()

	// The hub's wire shape: agent + prompt, no model (Model is internal-only).
	res, err := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent:  "reviewer",
		Prompt: "review the change",
		Label:  "reviewer",
	})
	if err != nil {
		t.Fatalf("spawnChildRun: %v", err)
	}
	svc.mu.Lock()
	got := svc.runs[res.RunID].modelName
	svc.mu.Unlock()
	if got != "grok-4.7" {
		t.Fatalf("hub-spawned node child model = %q, want step-configured grok-4.7 (live bug: inherited devin/swe-2-high)", got)
	}
}

// No matching node → inherit the parent model exactly as before.
func TestBug556_HubSpawnWithoutMatchingNodeStillInherits(t *testing.T) {
	svc := newInteractiveService(bug556Registry(), newInteractiveCatalog(), newFakeWorkflowStore())

	parent, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Cwd: t.TempDir(),
	})
	if apiErr != nil {
		t.Fatalf("parent createRun: %v", apiErr)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].modelName = "gpt-5.4-mini"
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[parent.RunID].activeFlowNodes = []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
	}
	svc.mu.Unlock()

	res, err := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent:  "scout",
		Prompt: "look around",
	})
	if err != nil {
		t.Fatalf("spawnChildRun: %v", err)
	}
	svc.mu.Lock()
	got := svc.runs[res.RunID].modelName
	svc.mu.Unlock()
	if got != "gpt-5.4-mini" {
		t.Fatalf("non-node spawn must inherit the parent model, got %q", got)
	}
}

// Explicit in.Model (engine dispatch path, BUG-228) still wins over the node
// row — the new lookup only fills the empty channel.
func TestBug556_ExplicitSpawnModelStillWins(t *testing.T) {
	catalog := newInteractiveCatalog()
	catalog.steps["wf"] = []Step{{
		ID:       "vibe_sprint_reviewer",
		Name:     "Reviewer",
		NodeID:   "reviewer",
		AgentRef: "agents/reviewer.md",
		Model:    "grok-4.7",
	}}
	svc := newInteractiveService(bug556Registry(), catalog, newFakeWorkflowStore())

	parent, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Cwd: t.TempDir(),
	})
	if apiErr != nil {
		t.Fatalf("parent createRun: %v", apiErr)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].modelName = "gpt-5.4-mini"
	svc.runs[parent.RunID].flowEngineDriven = true
	svc.runs[parent.RunID].activeFlowNodes = []agentpack.FlowNode{
		{ID: "reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md"},
	}
	svc.mu.Unlock()

	res, err := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent:  "reviewer",
		Prompt: "review",
		Model:  "claude-sonnet-4-5",
	})
	if err != nil {
		t.Fatalf("spawnChildRun: %v", err)
	}
	svc.mu.Lock()
	got := svc.runs[res.RunID].modelName
	svc.mu.Unlock()
	if got != "claude-sonnet-4-5" {
		t.Fatalf("explicit in.Model must win over the node row, got %q", got)
	}
}
