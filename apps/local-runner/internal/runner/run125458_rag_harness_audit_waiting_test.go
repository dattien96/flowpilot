package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// run-125458: rag-harness audit blocked_missing_feature_key left audit RUNNING
// instead of WAITING_USER_APPROVAL because setFlowStepAwaitingUser only
// handled hub.inline (review-loop). This file adds the rag-harness fallback.

func TestSetFlowStepAwaitingUser_RagHarnessNoHub_MarksRunningAuditWaiting(t *testing.T) {
	svc, _ := newTestServer(t)
	run, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	// Mark flow-engine-driven and seed rag-harness steps (no hub.inline).
	svc.markFlowEngineDriven(run.RunID)
	nodes := []agentpack.FlowNode{
		{ID: "context", Behavior: "context.produce"},
		{ID: "implement", Behavior: "agent.delegate"},
		{ID: "validate", Behavior: "command.validate"},
		{ID: "audit", Behavior: "artifact.audit_draft"},
	}
	svc.mu.Lock()
	svc.runs[run.RunID].activeFlowNodes = nodes
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(run.RunID, nodes)
	// Simulate validate DONE, audit RUNNING (just before escalate).
	svc.setFlowStepStatus(context.Background(), run.RunID, "context", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), run.RunID, "implement", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), run.RunID, "validate", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), run.RunID, "audit", StepStatusRunning)

	svc.setFlowStepAwaitingUser(context.Background(), run.RunID)

	steps, loadErr := svc.workflowStore.LoadRunSteps(context.Background(), run.RunID)
	if loadErr != nil {
		t.Fatalf("LoadRunSteps: %v", loadErr)
	}
	var auditStatus string
	for _, s := range steps {
		if s.ID == "audit" {
			auditStatus = string(s.Status)
			break
		}
	}
	if auditStatus != string(StepStatusWaitingUserApr) {
		t.Fatalf("audit status = %q, want %q (rag-harness no-hub fallback)", auditStatus, StepStatusWaitingUserApr)
	}
}

func TestSetFlowStepAwaitingUser_ReviewLoopHubStillMarksHub(t *testing.T) {
	svc, _ := newTestServer(t)
	run, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.markFlowEngineDriven(run.RunID)
	nodes := []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.delegate"},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	svc.mu.Lock()
	svc.runs[run.RunID].activeFlowNodes = nodes
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(run.RunID, nodes)
	svc.setFlowStepStatus(context.Background(), run.RunID, "synthesis", StepStatusRunning)

	svc.setFlowStepAwaitingUser(context.Background(), run.RunID)

	steps, _ := svc.workflowStore.LoadRunSteps(context.Background(), run.RunID)
	for _, s := range steps {
		if s.ID == "synthesis" && s.Status != StepStatusWaitingUserApr {
			t.Fatalf("synthesis status = %q, want WAITING_USER (hub path)", s.Status)
		}
		if s.ID == "coder" && s.Status == StepStatusWaitingUserApr {
			t.Fatal("coder should not be WAITING_USER when hub exists")
		}
	}
}

func TestSetFlowStepAwaitingUser_RagHarnessNoRunning_FallbackToAudit(t *testing.T) {
	svc, _ := newTestServer(t)
	run, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatal(err)
	}
	svc.markFlowEngineDriven(run.RunID)
	nodes := []agentpack.FlowNode{
		{ID: "context", Behavior: "context.produce"},
		{ID: "implement", Behavior: "agent.delegate"},
		{ID: "validate", Behavior: "command.validate"},
		{ID: "audit", Behavior: "artifact.audit_draft"},
	}
	svc.mu.Lock()
	svc.runs[run.RunID].activeFlowNodes = nodes
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(run.RunID, nodes)
	// All DONE/DONE/DONE, no RUNNING — still escalate should mark audit WAITING.
	svc.setFlowStepStatus(context.Background(), run.RunID, "context", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), run.RunID, "implement", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), run.RunID, "validate", StepStatusDone)
	svc.setFlowStepStatus(context.Background(), run.RunID, "audit", StepStatusDone)

	svc.setFlowStepAwaitingUser(context.Background(), run.RunID)
	steps, _ := svc.workflowStore.LoadRunSteps(context.Background(), run.RunID)
	for _, s := range steps {
		if s.ID == "audit" && s.Status != StepStatusWaitingUserApr {
			t.Fatalf("audit fallback WAITING_USER even when no RUNNING, got %q", s.Status)
		}
	}
}
