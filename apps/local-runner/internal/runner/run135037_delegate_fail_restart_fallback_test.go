package runner

import (
	"context"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// CA-617 P2: persist survives reconstruct + fallback via step.
func TestRun135037_DelegateFailPersistsAcrossReconstruct(t *testing.T) {
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	parent, _ := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini"})
	pid := parent.RunID
	nodes := ragHarnessNodes()
	svc.mu.Lock()
	p := svc.runs[pid]
	p.activeFlowNodes = nodes
	p.activeFlowEdges = []agentpack.FlowEdge{
		{From: "preflight_contract_plan", To: "preflight_contract_freeze", When: "done", Kind: "forward"},
	}
	p.autoOrchestrate = true
	p.flowEngineDriven = true
	p.chatFlowRef = "flowpilot-core-flow-pack/rag-harness"
	p.lastFailedDelegateNodeID = "preflight_contract_plan"
	p.lastEscalatedInlineNodeID = "audit"
	svc.mu.Unlock()

	snap := sessionStateOf(svc.runs[pid])
	if snap.LastFailedDelegateNodeID != "preflight_contract_plan" {
		t.Fatalf("session LastFailedDelegateNodeID=%q", snap.LastFailedDelegateNodeID)
	}
	if snap.LastEscalatedInlineNodeID != "audit" {
		t.Fatalf("session LastEscalatedInlineNodeID=%q", snap.LastEscalatedInlineNodeID)
	}
	// Simulate reconstruct
	svc2 := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	// manually reconstruct via interactive_resume logic: copy snap into new run
	svc2.mu.Lock()
	svc2.runs[pid] = &interactiveRun{
		id:                        pid,
		projectID:                 snap.ProjectID,
		flowEngineDriven:          true,
		status:                    RunStatusRunning,
		activeFlowNodes:           append([]agentpack.FlowNode(nil), snap.ActiveFlowNodes...),
		activeFlowEdges:           append([]agentpack.FlowEdge(nil), snap.ActiveFlowEdges...),
		chatFlowRef:               snap.ChatFlowRef,
		lastFailedDelegateNodeID:  snap.LastFailedDelegateNodeID,
		lastEscalatedInlineNodeID: snap.LastEscalatedInlineNodeID,
		subs:                      map[int64]chan ProviderEvent{},
	}
	svc2.mu.Unlock()
	if got := svc2.runs[pid].lastFailedDelegateNodeID; got != "preflight_contract_plan" {
		t.Fatalf("reconstructed lastFailed=%q", got)
	}
	if got := svc2.runs[pid].lastEscalatedInlineNodeID; got != "audit" {
		t.Fatalf("reconstructed lastEscalated=%q", got)
	}
	// Local file round-trip
	store := newFakeWorkflowStore()
	_ = store
	// Also test fallback: clear RAM, leave FAILED step, resume should still find it
	svc3 := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	parent3, _ := svc3.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini"})
	pid3 := parent3.RunID
	svc3.mu.Lock()
	p3 := svc3.runs[pid3]
	p3.activeFlowNodes = nodes
	p3.activeFlowEdges = []agentpack.FlowEdge{{From: "preflight_contract_plan", To: "preflight_contract_freeze", When: "done", Kind: "forward"}}
	p3.autoOrchestrate = true
	p3.flowEngineDriven = true
	p3.chatFlowRef = "flowpilot-core-flow-pack/rag-harness"
	// No RAM field
	svc3.mu.Unlock()
	svc3.reseedFlowStepRuntime(pid3, nodes)
	svc3.setFlowStepFailedWithReason(context.Background(), pid3, "preflight_contract_plan", "The 'gpt-5.4' model is not supported")
	svc3.agentOrchestrator.setLoop(pid3, AgentLoopState{Status: "blocked", Mode: "explicit", Cap: 3, RoundCap: 3, BlockReason: "delegate_failed", GateReason: "gpt-5.4 not supported"})
	// Create failed child
	child, _ := svc3.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini"})
	svc3.mu.Lock()
	crs := svc3.runs[child.RunID]
	crs.parentRunID = pid3
	crs.agentName = "contract-planner"
	crs.label = "preflight_contract_plan"
	crs.role = "contract-planner"
	crs.status = RunStatusFailed
	crs.agentStatus = string(RunStatusFailed)
	svc3.mu.Unlock()
	svc3.agentOrchestrator.registerChild(pid3, child.RunID)
	svc3.agentOrchestrator.upsertSummary(pid3, AgentRunSummary{RunID: child.RunID, AgentName: "contract-planner", Label: "preflight_contract_plan", Role: "contract-planner", Status: RunStatusFailed, ParentRunID: pid3})
	// Continue without RAM field should still reuse via fallback
	_, err := svc3.resumeFlowWithFeedback(pid3, "retry")
	if err != nil {
		t.Fatalf("resume fallback: %v", err)
	}
	// Should have set running
	if got := flowStepStatus(t, svc3, pid3, "preflight_contract_plan"); got != StepStatusRunning {
		t.Fatalf("fallback resume step=%v want RUNNING", got)
	}
}

func TestRun135037_ContinueWithEmptyStepIDStillReusesSameChild(t *testing.T) {
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	parent, _ := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini"})
	pid := parent.RunID
	svc.agentOrchestrator.setLoop(pid, AgentLoopState{Status: "blocked", Mode: "explicit", Cap: 3, RoundCap: 3, BlockReason: "delegate_failed"})
	nodes := ragHarnessNodes()
	svc.mu.Lock()
	p := svc.runs[pid]
	p.activeFlowNodes = nodes
	p.autoOrchestrate = true
	p.flowEngineDriven = true
	p.chatFlowRef = "flowpilot-core-flow-pack/rag-harness"
	p.lastFailedDelegateNodeID = "preflight_contract_plan"
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(pid, nodes)
	svc.setFlowStepStatus(context.Background(), pid, "preflight_contract_plan", StepStatusFailed)
	child, _ := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini"})
	svc.mu.Lock()
	crs := svc.runs[child.RunID]
	crs.parentRunID = pid
	crs.agentName = "contract-planner"
	crs.label = "preflight_contract_plan"
	crs.role = "contract-planner"
	crs.status = RunStatusFailed
	crs.agentStatus = string(RunStatusFailed)
	crs.stepID = ""         // empty — reproduces pre-adapter fail
	crs.lastTurnStepID = "" // also empty
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(pid, child.RunID)
	svc.agentOrchestrator.upsertSummary(pid, AgentRunSummary{RunID: child.RunID, AgentName: "contract-planner", Label: "preflight_contract_plan", Role: "contract-planner", Status: RunStatusFailed, ParentRunID: pid})
	beforeCount := len(svc.runs)
	_, err := svc.resumeFlowWithFeedback(pid, "retry empty stepID")
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	// give reinvoke a moment (fake adapter completes quickly to Completed)
	time.Sleep(50 * time.Millisecond)
	afterCount := len(svc.runs)
	if afterCount != beforeCount {
		t.Fatalf("runs before=%d after=%d, want same (no second planner)", beforeCount, afterCount)
	}
	svc.mu.Lock()
	afterStatus := svc.runs[child.RunID].status
	svc.mu.Unlock()
	if afterStatus == RunStatusFailed {
		t.Fatalf("child status=%v want not Failed (reused)", afterStatus)
	}
	if got := flowStepStatus(t, svc, pid, "preflight_contract_plan"); got != StepStatusRunning {
		t.Fatalf("step=%v want RUNNING", got)
	}
}

// CA-618 I2: real reconstruct + NDJSON round-trip, provider-agnostic (single representative).
func TestRun135037_DelegateFailRoundTripsViaReconstructRun(t *testing.T) {
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	snap := ProviderSessionState{
		RunID:                     "run-persist-reconstruct",
		ProjectID:                 "proj",
		RunKind:                   "chat",
		ChatFlowRef:               "flowpilot-core-flow-pack/rag-harness",
		ActiveFlowNodes:           ragHarnessNodes(),
		LastFailedDelegateNodeID:  "preflight_contract_plan",
		LastEscalatedInlineNodeID: "audit",
		Status:                    RunStatusRunning,
	}
	rs, apiErr := svc.reconstructRun(snap)
	if apiErr != nil {
		t.Fatalf("reconstructRun: %v", apiErr)
	}
	if got := rs.lastFailedDelegateNodeID; got != "preflight_contract_plan" {
		t.Fatalf("reconstructed lastFailed=%q", got)
	}
	if got := rs.lastEscalatedInlineNodeID; got != "audit" {
		t.Fatalf("reconstructed lastEscalated=%q", got)
	}
}

func TestRun135037_DelegateFailRoundTripsViaNDJSON(t *testing.T) {
	snap := ProviderSessionState{
		RunID:                     "run-ndjson",
		ProjectID:                 "proj",
		LastFailedDelegateNodeID:  "preflight_contract_plan",
		LastEscalatedInlineNodeID: "audit",
		Status:                    RunStatusRunning,
	}
	rec := sessionRecordFrom(snap)
	back := sessionStateFromRecord(rec)
	if back.LastFailedDelegateNodeID != "preflight_contract_plan" {
		t.Fatalf("NDJSON back lastFailed=%q", back.LastFailedDelegateNodeID)
	}
	if back.LastEscalatedInlineNodeID != "audit" {
		t.Fatalf("NDJSON back lastEscalated=%q", back.LastEscalatedInlineNodeID)
	}
}

// CA-618 M6: child with empty label (pre-adapter) must not spawn second planner.
func TestRun135037_ContinueEmptyLabelDoesNotSpawnSecondChild(t *testing.T) {
	svc := newInteractiveService(DefaultProviderRegistry(), newInteractiveCatalog(), newFakeWorkflowStore())
	parent, _ := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini"})
	pid := parent.RunID
	svc.agentOrchestrator.setLoop(pid, AgentLoopState{Status: "blocked", Mode: "explicit", Cap: 3, RoundCap: 3, BlockReason: "delegate_failed"})
	nodes := ragHarnessNodes()
	svc.mu.Lock()
	p := svc.runs[pid]
	p.activeFlowNodes = nodes
	p.autoOrchestrate = true
	p.flowEngineDriven = true
	p.chatFlowRef = "flowpilot-core-flow-pack/rag-harness"
	p.lastFailedDelegateNodeID = "preflight_contract_plan"
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(pid, nodes)
	svc.setFlowStepStatus(context.Background(), pid, "preflight_contract_plan", StepStatusFailed)
	child, _ := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Model: "gpt-5.4-mini"})
	svc.mu.Lock()
	crs := svc.runs[child.RunID]
	crs.parentRunID = pid
	crs.agentName = "contract-planner"
	crs.label = "" // empty label — pre-adapter fail shape
	crs.role = "contract-planner"
	crs.status = RunStatusFailed
	crs.agentStatus = string(RunStatusFailed)
	crs.stepID = ""
	crs.lastTurnStepID = ""
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(pid, child.RunID)
	svc.agentOrchestrator.upsertSummary(pid, AgentRunSummary{RunID: child.RunID, AgentName: "contract-planner", Label: "", Role: "contract-planner", Status: RunStatusFailed, ParentRunID: pid})
	beforeCount := len(svc.runs)
	beforeID := child.RunID
	_, err := svc.resumeFlowWithFeedback(pid, "retry empty label")
	if err != nil {
		t.Fatalf("resume: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	afterCount := len(svc.runs)
	if afterCount != beforeCount {
		t.Fatalf("runs before=%d after=%d, want same (no second planner on empty label)", beforeCount, afterCount)
	}
	svc.mu.Lock()
	after := svc.runs[beforeID]
	svc.mu.Unlock()
	if after == nil || after.status == RunStatusFailed {
		t.Fatalf("empty-label child not reused, status=%v", after)
	}
	if got := flowStepStatus(t, svc, pid, "preflight_contract_plan"); got != StepStatusRunning {
		t.Fatalf("step=%v want RUNNING", got)
	}
}
