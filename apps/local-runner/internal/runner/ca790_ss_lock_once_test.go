package runner

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
)

func TestResumeVibeLock_SealsAndIgnoresSecondPark(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.activeHubNodeID = vibeSSValidatorNodeID
	rs.vibeLockNodeID = vibeSSLockNodeID
	rs.pendingHubReinvoke = true
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: vibeSSValidatorNodeID, Behavior: "hub.inline"},
		{ID: vibeSSLockNodeID, Behavior: "user.confirm", Lifecycle: "once"},
		{ID: "cp_writer", Behavior: "agent.delegate", Agent: "agents/doc-writer.md"},
	}
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: vibeSSLockNodeID, To: "cp_writer", When: "done", Kind: "forward"},
		{From: "cp_writer", To: "done", When: "done", Kind: "forward"},
	}
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parent.RunID, []agentpack.FlowNode{
		{ID: vibeSSValidatorNodeID, Behavior: "hub.inline"},
		{ID: vibeSSLockNodeID, Behavior: "user.confirm"},
		{ID: "cp_writer", Behavior: "agent.delegate"},
	})
	svc.setFlowStepStatus(t.Context(), parent.RunID, vibeSSLockNodeID, StepStatusWaitingUserApr)
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Status: "blocked", BlockReason: vibeLockBlockReason, ActiveNode: vibeSSLockNodeID, Cap: 3, RoundCap: 3,
	})

	if _, ok := svc.resumeVibeLock(parent.RunID, "continue", AgentGraphSnapshot{}); !ok {
		t.Fatal("resumeVibeLock")
	}
	svc.mu.Lock()
	rs = svc.runs[parent.RunID]
	if !rs.vibeSSSealed {
		svc.mu.Unlock()
		t.Fatal("ss_lock resume must seal")
	}
	if rs.pendingHubReinvoke {
		svc.mu.Unlock()
		t.Fatal("pending hub reinvoke must be cleared on lock")
	}
	if rs.activeHubNodeID != "" {
		svc.mu.Unlock()
		t.Fatalf("activeHubNodeID=%q want empty after seal", rs.activeHubNodeID)
	}
	svc.mu.Unlock()

	if !svc.parkVibeLock(parent.RunID, vibeSSLockNodeID) {
		t.Fatal("park after seal should no-op true")
	}
	if got := flowStepStatus(t, svc, parent.RunID, vibeSSLockNodeID); got == StepStatusWaitingUserApr {
		t.Fatal("sealed ss_lock must not re-enter WAITING_USER_APPROVAL")
	}
}

func TestMaybeAutoReinvokeHub_SkipsAfterSSLockSeal(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.vibeSSSealed = true
	rs.activeHubNodeID = vibeSSValidatorNodeID
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: vibeSSValidatorNodeID, Behavior: "hub.inline", Agent: "agents/synthesizer.md"},
		{ID: vibeSSLockNodeID, Behavior: "user.confirm"},
	}
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parent.RunID, []agentpack.FlowNode{
		{ID: vibeSSValidatorNodeID, Behavior: "hub.inline"},
		{ID: vibeSSLockNodeID, Behavior: "user.confirm"},
	})
	svc.setFlowStepStatus(t.Context(), parent.RunID, vibeSSValidatorNodeID, StepStatusDone)
	svc.setFlowStepStatus(t.Context(), parent.RunID, vibeSSLockNodeID, StepStatusDone)
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3, Mode: "explicit"})

	svc.maybeAutoReinvokeHubWithNote(parent.RunID, "cp_writer joined")

	if got := flowStepStatus(t, svc, parent.RunID, vibeSSValidatorNodeID); got == StepStatusRunning {
		t.Fatal("sealed lock must not revive ss_validator")
	}
	if got := flowStepStatus(t, svc, parent.RunID, vibeSSLockNodeID); got == StepStatusWaitingUserApr {
		t.Fatal("sealed lock must not re-park ss_lock")
	}
}
