package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// CA-1221 (live run-297392): on vibe sprint chains a resolved model pin that
// routes cross-provider is valid only while the target provider has a
// connected local account. Mirrored step_definitions rows outlive the run
// that minted them — vibe_adopt_sprint_* rows still carried gpt-5.4 from
// run-295434's dead codex route, and merge-duplicate upserts never clear
// model — so every later adopt mount escalated on the stranded pin before
// the fix. The resolution must skip it and let the child inherit the
// hub's session provider.

func vibeChainParent(t *testing.T, svc *InteractiveService, flowRef string) string {
	t.Helper()
	parent, err := svc.createRun(StartRunInput{
		ProjectID:   "proj",
		ChatMode:    "normal_chat",
		ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].chatFlowRef = workingmode.PackPrefix + flowRef
	svc.mu.Unlock()
	return parent.RunID
}

func TestCA1221_AdoptSprintSkipsStrandedStepRowPin(t *testing.T) {
	svc, _ := newTestServer(t)
	isolateProviderHome(t)

	parentID := vibeChainParent(t, svc, vibeAdoptSprintFlowID)

	catalog := newInteractiveCatalog()
	catalog.steps["wf"] = []Step{{
		ID:       "vibe_adopt_sprint_spec_align",
		Name:     "Spec align",
		NodeID:   "spec_align",
		AgentRef: "agents/spec-aligner.md",
		Model:    "claude-sonnet-4-5",
	}}
	svc.catalog = catalog

	got := svc.resolveFlowNodeModel(context.Background(), parentID, agentpack.FlowNode{
		ID: "spec_align", Behavior: "agent.delegate", Agent: "agents/spec-aligner.md",
	})
	if got != "" {
		t.Fatalf("resolveFlowNodeModel(stranded claude pin) = %q, want empty inherit", got)
	}
}

func TestCA1221_AdoptSprintSkipsStrandedYamlPin(t *testing.T) {
	svc, _ := newTestServer(t)
	isolateProviderHome(t)

	parentID := vibeChainParent(t, svc, vibeAdoptSprintFlowID)

	got := svc.resolveFlowNodeModel(context.Background(), parentID, agentpack.FlowNode{
		ID: "spec_align", Behavior: "agent.delegate", Agent: "agents/spec-aligner.md",
		Model: "grok-4-5",
	})
	if got != "" {
		t.Fatalf("resolveFlowNodeModel(stranded yaml grok pin) = %q, want empty inherit", got)
	}
}

func TestCA1221_ConnectedCrossProviderPinStillHonored(t *testing.T) {
	svc, _ := newTestServer(t)
	isolateProviderHome(t)
	writeTask447Accounts(t, []ProviderAccount{
		task447Account("cl-0", "claude", 0, true),
	})

	parentID := vibeChainParent(t, svc, vibeAdoptSprintFlowID)

	got := svc.resolveFlowNodeModel(context.Background(), parentID, agentpack.FlowNode{
		ID: "tdd", Behavior: "agent.scaffold", Agent: "agents/scaffold-architect.md",
		Model: "claude-sonnet-4-5",
	})
	if got != "claude-sonnet-4-5" {
		t.Fatalf("resolveFlowNodeModel(connected claude pin) = %q, want claude-sonnet-4-5", got)
	}
}

func TestCA1221_SameProviderPinStillHonored(t *testing.T) {
	svc, _ := newTestServer(t)
	isolateProviderHome(t)

	parentID := vibeChainParent(t, svc, vibeAdoptSprintFlowID)

	got := svc.resolveFlowNodeModel(context.Background(), parentID, agentpack.FlowNode{
		ID: "spec_align", Behavior: "agent.delegate", Agent: "agents/spec-aligner.md",
		Model: "gpt-5.4-mini",
	})
	if got != "gpt-5.4-mini" {
		t.Fatalf("resolveFlowNodeModel(same-provider pin) = %q, want gpt-5.4-mini", got)
	}
}

func TestCA1221_NonVibeFlowKeepsStrandedPin(t *testing.T) {
	svc, _ := newTestServer(t)
	isolateProviderHome(t)

	parent, err := svc.createRun(StartRunInput{
		ProjectID:   "proj",
		ChatMode:    "normal_chat",
		ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].chatFlowRef = workingmode.PackPrefix + "review-loop"
	svc.mu.Unlock()

	got := svc.resolveFlowNodeModel(context.Background(), parent.RunID, agentpack.FlowNode{
		ID: "spec_align", Behavior: "agent.delegate", Agent: "agents/spec-aligner.md",
		Model: "claude-sonnet-4-5",
	})
	if got != "claude-sonnet-4-5" {
		t.Fatalf("resolveFlowNodeModel(non-vibe flow) = %q, want claude-sonnet-4-5 (guard is vibe-scoped)", got)
	}
}

func TestCA1221_VibeSprintChainAlsoSkipsStrandedPin(t *testing.T) {
	svc, _ := newTestServer(t)
	isolateProviderHome(t)

	parentID := vibeChainParent(t, svc, vibeSprintFlowID)

	got := svc.resolveFlowNodeModel(context.Background(), parentID, agentpack.FlowNode{
		ID: "spec_align", Behavior: "agent.delegate", Agent: "agents/spec-aligner.md",
		Model: "claude-sonnet-4-5",
	})
	if got != "" {
		t.Fatalf("resolveFlowNodeModel(vibe-sprint stranded pin) = %q, want empty inherit", got)
	}
}
