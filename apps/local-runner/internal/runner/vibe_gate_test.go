package runner

import (
	"testing"

	"flowpilot-runner/internal/flowgate"
	"flowpilot-runner/internal/workingmode"
)

func TestClassifyVibeGate_DevPassthrough(t *testing.T) {
	got := classifyVibeGate(workingmode.Dev, flowgate.EnforceResult{
		Action: "block",
		Violations: []flowgate.Violation{{
			Rule: flowgate.Rule{ID: "r-reg"},
		}},
	})
	if got != vibeGatePassthrough {
		t.Fatalf("got %d, want passthrough", got)
	}
}

func TestClassifyVibeGate_RequirementWins(t *testing.T) {
	got := classifyVibeGate(workingmode.Vibe, flowgate.EnforceResult{
		Action: "block",
		Violations: []flowgate.Violation{{
			Rule: flowgate.Rule{ID: flowgate.RequirementRuleID},
		}},
	})
	if got != vibeGateRequirement {
		t.Fatalf("got %d, want requirement", got)
	}
}

func TestClassifyVibeGate_OwnerDebateOnReg(t *testing.T) {
	got := classifyVibeGate(workingmode.Vibe, flowgate.EnforceResult{
		Action: "block",
		Violations: []flowgate.Violation{{
			Rule: flowgate.Rule{ID: "r-reg"},
		}},
	})
	if got != vibeGateOwnerDebate {
		t.Fatalf("got %d, want owner debate", got)
	}
}

func TestSpawnChildInheritsWorkingMode(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	child, spawnErr := svc.spawnChildRun(t.Context(), parent.RunID, SpawnAgentInput{
		Agent: "coder", Prompt: "do it", Provider: "codex", Wait: false,
	})
	if spawnErr != nil {
		t.Fatalf("spawn: %v", spawnErr)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[child.RunID]
	if rs == nil || rs.workingMode != workingmode.Vibe {
		t.Fatalf("child workingMode=%v", rs)
	}
}
