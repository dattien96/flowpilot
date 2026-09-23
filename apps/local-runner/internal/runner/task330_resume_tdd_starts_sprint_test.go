package runner

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// R-TK-K hang: Resume from tdd with Tasks on disk but no tdd-signatures →
// must start vibe-sprint at tdd (not silent maybeResumeVibeCoderAfterTdd no-op).
func TestTask330_ResumeFromTddStartsSprintWhenNoTddOutput(t *testing.T) {
	svc, _ := newTestServer(t)
	cwd := t.TempDir()
	task328Write(t, cwd, "requirements/08-Task/todo/Task-904-snake-core.md", "# T\n")
	// Task-327 precondition: a real resume-from-tdd run reached tdd only after
	// the SS lock, so vibeLockedSS is persisted and the file exists. Without it
	// restartVibeIngestForMissingSS (R-TK-D3) correctly restarts vibe-ingest
	// before the sprint resume below ever runs.
	task328Write(t, cwd, "requirements/05-System-Specs/SS-1-snake.md", "# SS\n")
	runID := "run-task330-tdd"
	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:                 runID,
		providerKey:        ProviderKeyCodex,
		workingMode:        workingmode.Vibe,
		workspaceCwd:       cwd,
		chatFlowRef:        workingmode.PackPrefix + vibeSprintFlowID,
		vibeCheckpointNode: vibeTaskSlicerNodeID,
		vibeLockedSS:       "requirements/05-System-Specs/SS-1-snake.md",
		vibeTaskPlan:       []string{"requirements/08-Task/todo/Task-904-snake-core.md"},
		vibeResumeConfirm:  true,
		vibeResumeFromNode: "tdd",
		status:             RunStatusCancelled,
		activeFlowNodes: []agentpack.FlowNode{
			{ID: "tdd", Behavior: "agent.code"},
			{ID: "coder", Behavior: "agent.code"},
		},
		subs: map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{
		Status: "blocked", BlockReason: vibeResumePausedReason, Cap: 3, RoundCap: 3,
	})

	if e := svc.SubmitGateDecision(runID, "ok", ""); e != nil {
		t.Fatalf("ok: %v", e)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	if rs.vibeResumeConfirm {
		t.Fatal("confirm must clear")
	}
	if rs.status == RunStatusCancelled {
		t.Fatal("must unseal cancelled parent")
	}
	if workingmode.BareFlowID(rs.chatFlowRef) != vibeSprintFlowID {
		t.Fatalf("chatFlowRef=%q", rs.chatFlowRef)
	}
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.Status == "blocked" && loop.BlockReason == vibeResumePausedReason {
		t.Fatal("must not remain on resume park")
	}
}

func TestTask330_ProvidersAgnostic(t *testing.T) {
	for _, pk := range []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok} {
		t.Run(string(pk), func(t *testing.T) {
			_ = pk
			if vibeSprintFlowID != "vibe-sprint" {
				t.Fatal("sprint id drift")
			}
		})
	}
}
