package runner

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// BUG-373 / R-TK-D3: delete SS+CP+Task then reopen while chatFlowRef is
// vibe-sprint must restart ingest_reader — not park "Resume from tdd?".
func TestBUG373_MissingAllOnVibeSprintRestartsIngest(t *testing.T) {
	svc, _ := newTestServer(t)
	cwd := t.TempDir()
	// No SS / CP / Task files on disk.
	runID := "run-bug373-d3"
	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:                 runID,
		providerKey:        ProviderKeyCodex,
		workingMode:        workingmode.Vibe,
		workspaceCwd:       cwd,
		chatFlowRef:        workingmode.PackPrefix + vibeSprintFlowID,
		vibeCheckpointNode: "tdd",
		vibeSprintIndex:    1,
		vibeTaskPlan: []string{
			"requirements/08-Task/todo/Task-904-snake-core.md",
		},
		lastPrompt: "Build terminal snake MVP",
		activeFlowNodes: []agentpack.FlowNode{
			{ID: "tdd", Behavior: "agent.code"},
			{ID: "coder", Behavior: "agent.code"},
		},
		subs: map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()

	if !svc.restartVibeIngestForMissingSS(runID) {
		t.Fatal("SS+CP+Task gone on vibe-sprint must restart ingest")
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	rs := svc.runs[runID]
	if rs.vibeResumeConfirm || rs.vibeResumeFromNode == "tdd" {
		t.Fatalf("must not park Resume from tdd: confirm=%v from=%q", rs.vibeResumeConfirm, rs.vibeResumeFromNode)
	}
	if workingmode.BareFlowID(rs.chatFlowRef) != vibeIngestFlowID {
		t.Fatalf("chatFlowRef=%q want vibe-ingest", rs.chatFlowRef)
	}
	if rs.vibeSprintIndex != 0 || len(rs.vibeTaskPlan) != 0 {
		t.Fatalf("sprint cursor must clear: idx=%d plan=%v", rs.vibeSprintIndex, rs.vibeTaskPlan)
	}
}

func TestBUG373_ResumeOKMissingSSForcesIngestNotTdd(t *testing.T) {
	svc, _ := newTestServer(t)
	cwd := t.TempDir()
	runID := "run-bug373-resume"
	svc.mu.Lock()
	svc.runs[runID] = &interactiveRun{
		id:                 runID,
		providerKey:        ProviderKeyCodex,
		workingMode:        workingmode.Vibe,
		workspaceCwd:       cwd,
		chatFlowRef:        workingmode.PackPrefix + vibeSprintFlowID,
		vibeCheckpointNode: "tdd",
		vibeResumeConfirm:  true,
		vibeResumeFromNode: "tdd",
		status:             RunStatusCancelled,
		lastPrompt:         "snake",
		subs:               map[int64]chan ProviderEvent{},
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
	if workingmode.BareFlowID(rs.chatFlowRef) != vibeIngestFlowID {
		t.Fatalf("OK with SS missing must ingest, got %q", rs.chatFlowRef)
	}
}

func TestBUG373_ProvidersAgnostic(t *testing.T) {
	for _, pk := range []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok} {
		t.Run(string(pk), func(t *testing.T) {
			rs := &interactiveRun{
				providerKey: pk,
				workingMode: workingmode.Vibe,
				chatFlowRef: workingmode.PackPrefix + vibeSprintFlowID,
			}
			if !vibeIngestHasSSLockTopology(rs) {
				t.Fatalf("%s: vibe-sprint must count as ingest-recover topology", pk)
			}
		})
	}
}
