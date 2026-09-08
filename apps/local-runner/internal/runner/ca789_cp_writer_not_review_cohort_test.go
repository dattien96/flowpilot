package runner

import (
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

func TestTryAdvanceFlowFromNode_CpWriterSpawnHasNoCohort(t *testing.T) {
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
	rs.chatFlowRef = workingmode.PackPrefix + vibeIngestFlowID
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "ss_lock", Behavior: "user.confirm"},
		{ID: "cp_writer", Behavior: "agent.delegate", Agent: "agents/doc-writer.md", Lifecycle: "once"},
	}
	rs.activeFlowEdges = []agentpack.FlowEdge{
		{From: "ss_lock", To: "cp_writer", When: "done", Kind: "forward"},
		{From: "cp_writer", To: "done", When: "done", Kind: "forward"},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3, Mode: "explicit"})

	if !svc.tryAdvanceFlowFromNode(parent.RunID, "ss_lock", "") {
		t.Fatal("ss_lock --done--> cp_writer must spawn")
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	found := false
	for _, child := range svc.runs {
		if child.parentRunID != parent.RunID || child.label != vibeCpWriterNodeID {
			continue
		}
		found = true
		if child.flowCohortId != "" {
			t.Fatalf("cp_writer FlowCohortID=%q want empty (not a review cohort)", child.flowCohortId)
		}
	}
	if !found {
		t.Fatal("cp_writer child not spawned")
	}
}

func TestCpWriterCohortJoin_DoesNotReparkSSLock(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", FlowRef: "vibe-ingest", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	const childID = "run-cp-writer-join"
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.workingMode = workingmode.Vibe
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
	svc.runs[childID] = &interactiveRun{
		id:           childID,
		parentRunID:  parent.RunID,
		label:        vibeCpWriterNodeID,
		agentName:    "doc-writer",
		status:       RunStatusRunning,
		agentStatus:  string(RunStatusRunning),
		flowCohortId: "flow-auto-ss_lock-round-0",
		subs:         map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parent.RunID, []agentpack.FlowNode{
		{ID: "ss_validator", Behavior: "hub.inline"},
		{ID: "ss_lock", Behavior: "user.confirm"},
		{ID: "cp_writer", Behavior: "agent.delegate"},
	})
	svc.setFlowStepStatus(t.Context(), parent.RunID, "ss_validator", StepStatusDone)
	svc.setFlowStepStatus(t.Context(), parent.RunID, "ss_lock", StepStatusDone)
	svc.agentOrchestrator.registerChild(parent.RunID, childID)
	svc.agentOrchestrator.preRegisterCohort(parent.RunID, "flow-auto-ss_lock-round-0", 1)
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3, Mode: "explicit"})

	svc.mu.Lock()
	child := svc.runs[childID]
	svc.settleFlowChildTurnCompletedLocked(child, "wrote CP", ProviderEvent{
		Type: EventTurnCompleted, FinalMessage: "wrote CP",
	})
	svc.mu.Unlock()

	if got := flowStepStatus(t, svc, parent.RunID, "ss_validator"); got == StepStatusRunning {
		t.Fatal("cp_writer cohort join revived ss_validator hub")
	}
	if got := flowStepStatus(t, svc, parent.RunID, "ss_lock"); got == StepStatusWaitingUserApr {
		t.Fatal("cp_writer cohort join re-parked ss_lock")
	}
	deadline := time.Now().Add(400 * time.Millisecond)
	for time.Now().Before(deadline) {
		if flowStepStatus(t, svc, parent.RunID, "ss_lock") == StepStatusWaitingUserApr {
			t.Fatal("async hub continue re-parked ss_lock")
		}
		st := svc.agentOrchestrator.loopStateFor(parent.RunID)
		if st.BlockReason == vibeLockBlockReason && st.ActiveNode == vibeSSLockNodeID {
			t.Fatalf("ss_lock re-parked via hub continue: %+v", st)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
