package runner

// CA-1096: the vibe missing-feature-key auto-finalize must not settle the
// run done while the sprint is still incomplete. Live run-3362: Task-015's
// audit auto-finalized on blocked_missing_feature_key while the loop carried
// openIssues=3 and the coder leg was mid-remediation — a false green.

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// Sprint-shaped graph: coder writer exists so its leg completeness is
// verifiable; audit is the terminal node.
func ca1096SprintGraph() ([]agentpack.FlowNode, []agentpack.FlowEdge) {
	nodes := []agentpack.FlowNode{
		{ID: "tdd", Behavior: "agent.scaffold"},
		{ID: "coder", Behavior: "agent.code"},
		{ID: "validate", Behavior: "command.validate"},
		{ID: "audit", Behavior: "artifact.audit_draft"},
	}
	edges := []agentpack.FlowEdge{
		{From: "audit", To: "done", When: "done", Kind: "forward"},
	}
	return nodes, edges
}

func ca1096Setup(t *testing.T, openIssues int, coderDone bool) (*InteractiveService, string, []agentpack.FlowNode, []agentpack.FlowEdge, agentpack.FlowNode) {
	t.Helper()
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
	nodes, edges := ca1096SprintGraph()
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
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{
		Status: "running", Cap: 5, Mode: "explicit", OpenIssues: openIssues,
	})
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	if coderDone {
		// BUG-648: a "completed sprint" means the whole write/verify path
		// reached DONE — coder alone is not evidence (a FAILED or skipped
		// tdd/validate leg must veto the audit exactly like a missing coder).
		svc.setFlowStepStatus(context.Background(), parent.RunID, "tdd", StepStatusDone)
		svc.setFlowStepStatus(context.Background(), parent.RunID, "coder", StepStatusDone)
		svc.setFlowStepStatus(context.Background(), parent.RunID, "validate", StepStatusDone)
	}
	return svc, parent.RunID, nodes, edges, auditNode
}

// openIssues>0 → the audit must escalate, not auto-finalize done.
func TestCA1096_VibeAuditOpenIssuesBlocksAutoFinalize(t *testing.T) {
	svc, runID, nodes, edges, auditNode := ca1096Setup(t, 3, true)

	svc.runAuditNode(context.Background(), runID, edges, nodes, auditNode, "task-015 partial")

	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.Status != "blocked" {
		t.Fatalf("audit with open issues must escalate, not settle done: %+v", loop)
	}
	if got := flowStepStatus(t, svc, runID, "audit"); got == StepStatusDone {
		t.Fatal("audit must not stamp DONE while open issues remain")
	}
}

// A declared coder leg that never reached DONE → same block even with zero
// open issues (the sprint never produced its implementation).
func TestCA1096_VibeAuditMissingCoderLegBlocksAutoFinalize(t *testing.T) {
	svc, runID, nodes, edges, auditNode := ca1096Setup(t, 0, false)

	svc.runAuditNode(context.Background(), runID, edges, nodes, auditNode, "no coder output")

	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.Status != "blocked" {
		t.Fatalf("audit without a completed coder leg must escalate, not settle done: %+v", loop)
	}
}

// The designed auto-finalize stays: clean loop + completed coder leg +
// missing feature key still settles done without parking.
func TestCA1096_VibeAuditCleanSprintStillAutoFinalizes(t *testing.T) {
	svc, runID, nodes, edges, auditNode := ca1096Setup(t, 0, true)

	svc.runAuditNode(context.Background(), runID, edges, nodes, auditNode, "all green")

	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.Status == "blocked" {
		t.Fatalf("clean sprint must keep the auto-finalize: %+v", loop)
	}
	if got := flowStepStatus(t, svc, runID, "audit"); got != StepStatusDone {
		t.Fatalf("audit=%q want DONE", got)
	}
}
