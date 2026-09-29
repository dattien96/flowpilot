package runner

import (
	"context"
	"strings"
	"testing"
)

// spawnFlowGuardParent creates a minimal parent run for spawn guard tests.
func spawnFlowGuardParent(t *testing.T, svc *InteractiveService) string {
	t.Helper()
	reg := newProviderRegistry()
	reg.register(ProviderRegistration{
		Key:          ProviderKeyClaude,
		DisplayName:  "Claude",
		Status:       ProviderStatusAvailable,
		Capabilities: ProviderCapabilities{Streaming: true, SkillSelection: true, ApprovalEvents: true},
		newAdapter:   func() ProviderRuntimeAdapter { return &captureTurnAdapter{ch: make(chan TurnRequest, 8)} },
	})
	svc.registry = reg
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyClaude, Model: "claude-sonnet-4-6"})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	return parent.RunID
}

// TestSpawnAgentRejectsBareFlowID guards R.3#3 option (c): a bare builtin flow
// id like "vibe-sprint" passed as the spawn_agent agent name must be refused —
// today it silently mints a plain chat run LABELLED as the flow (no flow
// mount, no contract freeze, no gate chain), so two different "sprint"
// execution modes coexist.
func TestSpawnAgentRejectsBareFlowID(t *testing.T) {
	svc := NewInteractiveService()
	parentID := spawnFlowGuardParent(t, svc)

	for _, name := range []string{"vibe-sprint", "review-loop", "vibe-owner-debate"} {
		res, err := svc.spawnChildRun(context.Background(), parentID, SpawnAgentInput{
			Agent: name, Prompt: "do sprint work",
		})
		if err == nil {
			t.Fatalf("spawn_child_run(%q) = run %q, want typed refusal", name, res.RunID)
		}
		if !strings.Contains(err.Error(), "flow") {
			t.Fatalf("spawn_child_run(%q) error = %q, want message naming the flow contract", name, err.Error())
		}
	}
}

// TestSpawnAgentRejectsQualifiedFlowRef covers the canonical pack-qualified
// form "flowpilot-core-flow-pack/vibe-sprint" — same refusal contract.
func TestSpawnAgentRejectsQualifiedFlowRef(t *testing.T) {
	svc := NewInteractiveService()
	parentID := spawnFlowGuardParent(t, svc)

	res, err := svc.spawnChildRun(context.Background(), parentID, SpawnAgentInput{
		Agent: "flowpilot-core-flow-pack/vibe-sprint", Prompt: "do sprint work",
	})
	if err == nil {
		t.Fatalf("spawn_child_run(qualified ref) = run %q, want typed refusal", res.RunID)
	}
	if !strings.Contains(err.Error(), "flow") {
		t.Fatalf("error = %q, want message naming the flow contract", err.Error())
	}
}

// TestSpawnAgentUnknownNameStillSpawns pins the existing degrade-to-raw-prompt
// contract for names that are neither agents nor flow ids — option (c) only
// closes the flow-ref hole, not the unknown-name path.
func TestSpawnAgentUnknownNameStillSpawns(t *testing.T) {
	svc := NewInteractiveService()
	parentID := spawnFlowGuardParent(t, svc)

	res, err := svc.spawnChildRun(context.Background(), parentID, SpawnAgentInput{
		Agent: "totally-made-up-agent-name", Prompt: "p", Wait: false,
	})
	if err != nil {
		t.Fatalf("unknown agent name should still spawn raw-prompt child, got %v", err)
	}
	if res.RunID == "" {
		t.Fatal("expected a minted child run id")
	}
}
