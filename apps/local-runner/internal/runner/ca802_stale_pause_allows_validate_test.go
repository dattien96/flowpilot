package runner

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

func TestCA802_StalePausedAllowsAdvanceAfterOK(t *testing.T) {
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
	rs.vibeResumeConfirm = false
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.code"},
		{ID: "validate", Behavior: "command.validate"},
	}
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "coder", To: "validate", When: "done", Kind: "forward"},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Status: "blocked", BlockReason: vibeResumePausedReason, Cap: 3, RoundCap: 3,
	})
	if !svc.loopIsAdvancing(parent.RunID) {
		t.Fatal("after OK, leftover blocked:paused must still advance coder→validate")
	}
}

func TestCA802_PauseGateStillBlocksAdvance(t *testing.T) {
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
	rs.vibeResumeConfirm = true
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Status: "blocked", BlockReason: vibeResumePausedReason, Cap: 3,
	})
	if svc.loopIsAdvancing(parent.RunID) {
		t.Fatal("open pause gate must not auto-advance")
	}
}

func TestCA802_LeftoverPausedTryAdvanceCoderToInline(t *testing.T) {
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
	rs.autoOrchestrate = true
	rs.vibeResumeConfirm = false
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.code"},
		{ID: "notify", Behavior: "hub.notify"},
	}
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "coder", To: "notify", When: "done", Kind: "forward"},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Status: "blocked", BlockReason: vibeResumePausedReason, Cap: 3, RoundCap: 3,
	})
	if !svc.tryAdvanceFlowFromNode(parent.RunID, "coder", "coder finished") {
		t.Fatal("leftover paused after OK must not skip coder→next inline")
	}
}

func TestCA802_NonPauseBlockedStillStopsAdvance(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	svc.runs[parent.RunID].vibeResumeConfirm = false
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Status: "blocked", BlockReason: "hub_stalled", Cap: 3,
	})
	if svc.loopIsAdvancing(parent.RunID) {
		t.Fatal("BUG-234: non-pause blocked must not auto-advance")
	}
}

func TestCA802_OKClearsPausedLoop(t *testing.T) {
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
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Status: "blocked", BlockReason: vibeResumePausedReason, Cap: 3, RoundCap: 3,
	})
	if e := svc.SubmitGateDecision(parent.RunID, "ok", ""); e != nil {
		t.Fatalf("ok: %v", e)
	}
	loop := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if loop.Status != "running" || loop.BlockReason != "" {
		t.Fatalf("ok must unblock loop, got status=%q block=%q", loop.Status, loop.BlockReason)
	}
}
