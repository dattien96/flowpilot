package runner

import (
	"context"
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

func ca814AuditGraph() ([]agentpack.FlowNode, []agentpack.FlowEdge) {
	nodes := []agentpack.FlowNode{
		{ID: "validate", Behavior: "command.validate"},
		{ID: "audit", Behavior: "artifact.audit_draft"},
	}
	edges := []agentpack.FlowEdge{
		{From: "audit", To: "done", When: "done", Kind: "forward"},
	}
	return nodes, edges
}

func TestCA814_VibeAuditMissingFeatureKeyAutoFinalizes(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	workspace := t.TempDir()
	initGitRepoForAuditFixture(t, workspace)
	nodes, edges := ca814AuditGraph()
	auditNode, ok := findFlowNode(nodes, "audit")
	if !ok {
		t.Fatal("audit node")
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = workingmode.Vibe
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.workspaceCwd = workspace
	rs.activeFlowNodes = nodes
	rs.activeFlowEdges = edges
	rs.flowValidationRetryState = &FlowValidationRetryState{Status: "passed"}
	rs.planContextPackage = &FlowContextPackage{FeatureKey: "", FeatureConfidence: ConfidenceUnresolved}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, Mode: "explicit"})
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	svc.runAuditNode(context.Background(), parent.RunID, edges, nodes, auditNode, "snake tests green")
	loop := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if loop.Status == "blocked" {
		t.Fatalf("vibe must not park missing feature key: %+v", loop)
	}
	if got := flowStepStatus(t, svc, parent.RunID, "audit"); got != StepStatusDone {
		t.Fatalf("audit=%q want DONE", got)
	}
}

func TestCA814_NonVibeStillEscalatesMissingFeatureKey(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	workspace := t.TempDir()
	initGitRepoForAuditFixture(t, workspace)
	nodes, edges := ca814AuditGraph()
	auditNode, ok := findFlowNode(nodes, "audit")
	if !ok {
		t.Fatal("audit node")
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = workingmode.Dev
	rs.flowEngineDriven = true
	rs.workspaceCwd = workspace
	rs.activeFlowNodes = nodes
	rs.activeFlowEdges = edges
	rs.flowValidationRetryState = &FlowValidationRetryState{Status: "passed"}
	rs.planContextPackage = &FlowContextPackage{FeatureKey: "", FeatureConfidence: ConfidenceUnresolved}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, Mode: "explicit"})
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	svc.runAuditNode(context.Background(), parent.RunID, edges, nodes, auditNode, "docs")
	loop := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if loop.Status != "blocked" || !strings.Contains(loop.GateReason, "feature key") {
		t.Fatalf("non-vibe must escalate missing key: %+v", loop)
	}
}

func TestCA814_VibeRetryDoesNotReparkMissingKey(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex,
		WorkingMode: "vibe", Client: "tui",
	})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	workspace := t.TempDir()
	initGitRepoForAuditFixture(t, workspace)
	nodes, edges := ca814AuditGraph()
	auditNode, ok := findFlowNode(nodes, "audit")
	if !ok {
		t.Fatal("audit node")
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.workingMode = workingmode.Vibe
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.workspaceCwd = workspace
	rs.activeFlowNodes = nodes
	rs.activeFlowEdges = edges
	rs.lastEscalatedInlineNodeID = "audit"
	rs.flowValidationRetryState = &FlowValidationRetryState{Status: "passed"}
	rs.planContextPackage = &FlowContextPackage{FeatureKey: "", FeatureConfidence: ConfidenceUnresolved}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Status: "blocked", BlockReason: "escalate",
		GateReason: "Audit blocked: feature key missing or unverified; cannot finalize.",
		Cap:        3, Mode: "explicit",
	})
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	svc.setFlowStepStatus(context.Background(), parent.RunID, "audit", StepStatusWaitingUserApr)
	if _, err := svc.resumeFlowWithFeedback(parent.RunID, ""); err != nil {
		t.Fatalf("retry: %v", err)
	}
	svc.runAuditNode(context.Background(), parent.RunID, edges, nodes, auditNode, "retry")
	loop := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if loop.Status == "blocked" {
		t.Fatalf("Retry must auto-finalize, not re-park: %+v", loop)
	}
}
