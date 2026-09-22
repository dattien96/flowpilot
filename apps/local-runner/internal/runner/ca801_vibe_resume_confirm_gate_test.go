package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

func TestCA801_ReconstructParksResumeConfirmNoCoder(t *testing.T) {
	nodes := []agentpack.FlowNode{
		{ID: "tdd", Behavior: "agent.code"},
		{ID: "coder", Behavior: "agent.code"},
	}
	for _, pk := range []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok} {
		t.Run(string(pk), func(t *testing.T) {
			svc, _ := newTestServer(t)
			runID := "run-pause-" + string(pk)
			rs, apiErr := svc.reconstructRun(ProviderSessionState{
				RunID:           runID,
				ProjectID:       "proj",
				ProviderKey:     pk,
				RunKind:         "chat",
				WorkingMode:     workingmode.Vibe,
				ChatFlowRef:     workingmode.PackPrefix + vibeSprintFlowID,
				Status:          RunStatusRunning,
				ActiveFlowNodes: nodes,
				ActiveFlowEdges: []agentpack.FlowEdge{{From: "tdd", To: "coder", When: "done", Kind: "forward"}},
				StartedAt:       "2026-09-09T00:00:00Z",
				UpdatedAt:       "2026-09-09T00:05:00Z",
			})
			if apiErr != nil {
				t.Fatalf("reconstructRun: %v", apiErr)
			}
			svc.setFlowStepStatus(context.Background(), runID, "tdd", StepStatusDone)
			svc.maybeParkVibeResumeConfirm(runID)
			if countChildrenWithLabel(svc, runID, "coder") != 0 {
				t.Fatal("reopen must not auto-spawn coder")
			}
			if !rs.vibeResumeConfirm {
				t.Fatal("want vibeResumeConfirm")
			}
			loop := svc.agentOrchestrator.loopStateFor(runID)
			if loop.BlockReason != vibeResumePausedReason {
				t.Fatalf("block=%q want paused", loop.BlockReason)
			}
			view, err := svc.runSnapshot(runID)
			if err != nil {
				t.Fatalf("runSnapshot: %v", err)
			}
			if view.PendingGate == nil || len(view.PendingGate.GateOptions) != 2 {
				t.Fatalf("pendingGate=%#v", view.PendingGate)
			}
		})
	}
}

func TestCA801_GateCancelDoesNotSpawnCoder(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = workingmode.Vibe
	rs.vibeResumeConfirm = true
	svc.mu.Unlock()
	if e := svc.SubmitGateDecision(parent.RunID, "cancel", ""); e != nil {
		t.Fatalf("cancel: %v", e)
	}
	if countChildrenWithLabel(svc, parent.RunID, "coder") != 0 {
		t.Fatal("cancel must not spawn coder")
	}
}

func TestCA801_GateOKClearsConfirm(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	cwd := t.TempDir()
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = workingmode.Vibe
	rs.vibeResumeConfirm = true
	rs.workspaceCwd = cwd
	rs.vibeCheckpointNode = "tdd"
	rs.activeFlowNodes = []agentpack.FlowNode{{ID: "tdd"}, {ID: "coder"}}
	rs.activeFlowEdges = []agentpack.FlowEdge{{From: "tdd", To: "coder", When: "done", Kind: "forward"}}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "blocked", BlockReason: vibeResumePausedReason, Cap: 3, RoundCap: 3})
	if e := svc.SubmitGateDecision(parent.RunID, "ok", ""); e != nil {
		t.Fatalf("ok: %v", e)
	}
	svc.mu.Lock()
	got := svc.runs[parent.RunID].vibeResumeConfirm
	svc.mu.Unlock()
	if got {
		t.Fatal("ok must clear vibeResumeConfirm")
	}
	_ = context.Background()
}

func TestCA801_RregDecisionStillWorksWhenConfirmFalse(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.vibeResumeConfirm = false
	rs.pendingGateBlock = &gateBlockInfo{regressedTests: []string{"TestFoo"}, stepID: "chat-" + parent.RunID}
	rs.lastTurnStepID = "chat-" + parent.RunID
	svc.mu.Unlock()
	if e := svc.SubmitGateDecision(parent.RunID, "keep-test-fix-code", ""); e != nil {
		t.Fatalf("r-reg keep-test must not 400 via resume-confirm branch: %v", e)
	}
}

func TestCA801_InMemoryResumeParksConfirm(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = workingmode.Vibe
	rs.flowEngineDriven = true
	rs.activeFlowNodes = []agentpack.FlowNode{{ID: "tdd"}, {ID: "coder"}}
	rs.activeFlowEdges = []agentpack.FlowEdge{{From: "tdd", To: "coder", When: "done", Kind: "forward"}}
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parent.RunID, []agentpack.FlowNode{{ID: "tdd"}, {ID: "coder"}})
	svc.setFlowStepStatus(context.Background(), parent.RunID, "tdd", StepStatusDone)
	if _, apiErr := svc.resumeRun(parent.RunID); apiErr != nil {
		t.Fatalf("resumeRun: %v", apiErr)
	}
	if countChildrenWithLabel(svc, parent.RunID, "coder") != 0 {
		t.Fatal("in-memory /open must not auto-spawn coder")
	}
	svc.mu.Lock()
	got := svc.runs[parent.RunID].vibeResumeConfirm
	svc.mu.Unlock()
	if !got {
		t.Fatal("in-memory resume must arm pause gate")
	}
	view, e := svc.runSnapshot(parent.RunID)
	if e != nil {
		t.Fatalf("runSnapshot: %v", e)
	}
	if view.PendingGate == nil {
		t.Fatal("snapshot must include ok/cancel gate")
	}
}
