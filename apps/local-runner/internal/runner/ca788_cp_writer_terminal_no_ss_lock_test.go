package runner

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

func TestTryAdvanceFlowFromNode_CpWriterDoneDoesNotReparkSSLock(t *testing.T) {
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
	rs.workingMode = workingmode.Vibe
	// Already handed off so maybeStartVibeCpIngest is a no-op; this test
	// isolates the terminal-done claim (live run-214743 hub reinvoke).
	rs.chatFlowRef = workingmode.PackPrefix + vibeCpIngestFlowID
	rs.activeHubNodeID = "ss_validator"
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "ss_validator", Behavior: "hub.inline", Agent: "agents/synthesizer.md"},
		{ID: "ss_lock", Behavior: "user.confirm", Lifecycle: "once"},
		{ID: "ss_converter", Behavior: "agent.delegate", Agent: "agents/vibe-intake.md"},
		{ID: "cp_writer", Behavior: "agent.delegate", Agent: "agents/doc-writer.md"},
	}
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "ss_validator", To: "ss_lock", When: "done", Kind: "forward"},
		{From: "ss_lock", To: "cp_writer", When: "done", Kind: "forward"},
		{From: "ss_lock", To: "ss_converter", When: "continue", Kind: "back"},
		{From: "cp_writer", To: "done", When: "done", Kind: "forward"},
	}
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parent.RunID, []agentpack.FlowNode{
		{ID: "ss_validator", Behavior: "hub.inline"},
		{ID: "ss_lock", Behavior: "user.confirm"},
		{ID: "ss_converter", Behavior: "agent.delegate"},
		{ID: "cp_writer", Behavior: "agent.delegate"},
	})
	svc.setFlowStepStatus(t.Context(), parent.RunID, "ss_lock", StepStatusDone)
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3, Round: 0})

	svc.advanceOrNotifyHub(parent.RunID, "cp_writer", "doc-writer", "wrote CP")

	if got := flowStepStatus(t, svc, parent.RunID, "ss_lock"); got == StepStatusWaitingUserApr {
		t.Fatal("cp_writer --done--> done re-parked ss_lock (hub continue loop)")
	}
	st := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if st.BlockReason == vibeLockBlockReason && st.ActiveNode == vibeSSLockNodeID {
		t.Fatalf("ss_lock re-parked after cp_writer terminal: %+v", st)
	}
}

func TestTryAdvanceFlowFromNode_CpWriterTerminalDoneIsClaimed(t *testing.T) {
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
	rs.chatFlowRef = workingmode.PackPrefix + vibeCpIngestFlowID
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "cp_writer", Behavior: "agent.delegate", Agent: "agents/doc-writer.md"},
	}
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "cp_writer", To: "done", When: "done", Kind: "forward"},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	if !svc.tryAdvanceFlowFromNode(parent.RunID, "cp_writer", "wrote CP") {
		t.Fatal("cp_writer --done--> done must be claimed so hub is not reinvoked")
	}
}

func TestTryAdvanceFlowFromNode_TaskSlicerTerminalDoneIsClaimed(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-cp-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.chatFlowRef = workingmode.PackPrefix + vibeCpIngestFlowID
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "task_slicer", Behavior: "agent.delegate", Agent: "agents/doc-writer.md"},
	}
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "task_slicer", To: "done", When: "done", Kind: "forward"},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	if !svc.tryAdvanceFlowFromNode(parent.RunID, "task_slicer", "sliced") {
		t.Fatal("task_slicer --done--> done must be claimed so cp_validator hub is not reinvoked")
	}
}
