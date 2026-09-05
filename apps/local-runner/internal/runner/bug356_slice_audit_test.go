package runner

// BUG-356 tests (new file — additive only): slice-only docs flows (cp-harness)
// declare no command.validate node, so audit must verify docs-only outputs
// instead of parking blocked_validation_failed with no forward path.

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

func TestFlowHasValidateNode(t *testing.T) {
	withValidate := []agentpack.FlowNode{
		{ID: "implement", Behavior: "agent.code"},
		{ID: "validate", Behavior: "command.validate"},
		{ID: "audit", Behavior: "artifact.audit_draft"},
	}
	if !flowHasValidateNode(withValidate) {
		t.Fatal("flow with command.validate must report true")
	}
	// cp-harness slice-only shape: plan loop + splitter + audit, no validate.
	sliceOnly := []agentpack.FlowNode{
		{ID: "preflight_contract_plan", Behavior: "agent.delegate"},
		{ID: "context", Behavior: "context.produce"},
		{ID: "cp_plan_writer", Behavior: "agent.delegate"},
		{ID: "cp_reviewer", Behavior: "agent.delegate"},
		{ID: "cp_synthesis", Behavior: "hub.inline"},
		{ID: "task_splitter", Behavior: "agent.delegate"},
		{ID: "audit", Behavior: "artifact.audit_draft"},
	}
	if flowHasValidateNode(sliceOnly) {
		t.Fatal("slice-only flow must report false")
	}
	if flowHasValidateNode(nil) {
		t.Fatal("empty nodes must report false")
	}
}

func TestVerifySliceOnlyOutputs(t *testing.T) {
	docsOnly := []string{
		"requirements/07-Coding-Plan/todo/CP-38-abs-benchmark.md",
		"requirements/08-Task/todo/Task-910-add-abs-to-format-go.md",
		"requirements/08-Task/todo/Task-911-create-abs-bench-test.md",
		"change-audit/CA-922-calc-core-lcm.md",
		".flowpilot/ledger/chat_summary.ndjson",
		".flowpilot/contracts/contracts.ndjson",
	}
	if !verifySliceOnlyOutputs(docsOnly) {
		t.Fatal("docs-only diff with CP/Task artifacts must verify")
	}
	withCode := append(append([]string{}, docsOnly...), "calc.go")
	if verifySliceOnlyOutputs(withCode) {
		t.Fatal("code change must refuse the docs-only path")
	}
	if verifySliceOnlyOutputs(nil) {
		t.Fatal("empty diff must refuse (nothing to verify)")
	}
	registryEdit := []string{
		"requirements/08-Task/todo/Task-910-x.md",
		"change-audit/FEATURE-KEYS.md",
	}
	if verifySliceOnlyOutputs(registryEdit) {
		t.Fatal("registry mutation must refuse (BUG-278 narrowness: only CA-*.md notes)")
	}
	rulesEdit := []string{
		"requirements/08-Task/todo/Task-910-x.md",
		".flowpilot/settings/flow-rules.json",
	}
	if verifySliceOnlyOutputs(rulesEdit) {
		t.Fatal("gate-rules write must refuse")
	}
	noArtifact := []string{
		"change-audit/CA-922-calc-core-lcm.md",
	}
	if verifySliceOnlyOutputs(noArtifact) {
		t.Fatal("diff without a CP/Task artifact must refuse")
	}
}

// bug356SliceNodes returns a cp-harness-shaped chain: plan loop + splitter +
// audit, deliberately WITHOUT any command.validate node.
func bug356SliceNodes() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	nodes := []agentpack.FlowNode{
		{ID: "preflight_contract_plan", Behavior: "agent.delegate"},
		{ID: "context", Behavior: "context.produce"},
		{ID: "cp_plan_writer", Behavior: "agent.delegate"},
		{ID: "cp_reviewer", Behavior: "agent.delegate"},
		{ID: "cp_synthesis", Behavior: "hub.inline", Lifecycle: "reinvoke", Join: "all"},
		{ID: "task_splitter", Behavior: "agent.delegate"},
		{ID: "audit", Behavior: "artifact.audit_draft", Lifecycle: "once"},
	}
	edges := []agentpack.FlowEdge{
		{From: "cp_synthesis", To: "task_splitter", When: "done", Kind: "forward"},
		{From: "task_splitter", To: "audit", When: "done", Kind: "forward"},
		{From: "audit", To: "done", When: "done", Kind: "forward"},
	}
	return edges, nodes
}

func bug356GitCommit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %v %s", args, err, out)
	}
}

// TestRunAuditNodeSliceOnlyDocsPasses drives the BUG-356 live shape end to
// end: cp-harness nodes (no validate), docs-only diff, registered key. The
// audit must record a passed slice-outputs verification instead of parking
// blocked_validation_failed. Provider matrix per cross-provider-parity (the
// path takes no providerKey — Case 1 agnostic, one representative loop).
func TestRunAuditNodeSliceOnlyDocsPasses(t *testing.T) {
	providers := []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
	for _, pk := range providers {
		t.Run(string(pk), func(t *testing.T) {
			ch := make(chan TurnRequest, 4)
			reg := newProviderRegistry()
			registerKeyedCapture(reg, pk, ch)
			svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
			parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: pk})
			if err != nil {
				t.Fatalf("createRun: %v", err)
			}
			runID := parent.RunID
			workspace := t.TempDir()
			initGitRepoForAuditFixture(t, workspace)
			// Registry committed BEFORE the flow diff, so it is not part of
			// the aggregate change under verification.
			p4WriteFile(t, workspace, "change-audit/FEATURE-KEYS.md", "- calc-core — arithmetic\n")
			bug356GitCommit(t, workspace, "git", "add", "change-audit/FEATURE-KEYS.md")
			bug356GitCommit(t, workspace, "git", "commit", "-m", "registry")
			// Docs-only flow diff: CP + Task artifacts, no code.
			p4WriteFile(t, workspace, "requirements/07-Coding-Plan/todo/CP-38-abs-benchmark.md", "# CP-38\n")
			p4WriteFile(t, workspace, "requirements/08-Task/todo/Task-910-add-abs-to-format-go.md", "# Task-910\n")
			edges, nodes := bug356SliceNodes()
			auditNode, ok := findFlowNode(nodes, "audit")
			if !ok {
				t.Fatal("fixture must declare an audit node")
			}
			svc.mu.Lock()
			rs := svc.runs[runID]
			rs.activeFlowEdges = edges
			rs.activeFlowNodes = nodes
			rs.flowEngineDriven = true
			rs.autoOrchestrate = true
			rs.workspaceCwd = workspace
			rs.flowStartGitHead = headOfRepo(t, workspace)
			// Deliberately NO flowValidationRetryState: slice-only flows never
			// run a validate node, so no state can exist (the BUG-356 shape).
			rs.planContextPackage = &FlowContextPackage{FeatureKey: "calc-core", FeatureConfidence: ConfidenceVerified}
			svc.mu.Unlock()
			svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running", Cap: 3, Mode: "explicit"})
			svc.reseedFlowStepRuntime(runID, nodes)

			svc.runAuditNode(context.Background(), runID, edges, nodes, auditNode, "split CP-38 into 3 tasks")
			svc.mu.Lock()
			st := svc.runs[runID].flowValidationRetryState
			svc.mu.Unlock()
			if st == nil || st.Status != "passed" {
				t.Fatalf("%s: validation state = %+v, want passed slice-outputs", pk, st)
			}
			if !strings.Contains(st.ValidationCommand, "slice-outputs-check") {
				t.Fatalf("%s: command = %q, want the slice-outputs marker", pk, st.ValidationCommand)
			}
			loop := svc.agentOrchestrator.loopStateFor(runID)
			if loop.Status == "blocked" && strings.Contains(loop.GateReason, "blocked_validation_failed") {
				t.Fatalf("%s: still parked on validation: %q", pk, loop.GateReason)
			}
		})
	}
}

// TestRunAuditNodeStillBlocksWhenCodeTouched pins the fail-closed boundary:
// a flow WITHOUT a validate node that touched code must still park (the
// docs-only fast path refuses, existing validation block stands).
func TestRunAuditNodeStillBlocksWhenCodeTouched(t *testing.T) {
	ch := make(chan TurnRequest, 4)
	reg := newProviderRegistry()
	registerKeyedCapture(reg, ProviderKeyCodex, ch)
	svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	runID := parent.RunID
	workspace := t.TempDir()
	initGitRepoForAuditFixture(t, workspace)
	p4WriteFile(t, workspace, "change-audit/FEATURE-KEYS.md", "- calc-core — arithmetic\n")
	bug356GitCommit(t, workspace, "git", "add", "change-audit/FEATURE-KEYS.md")
	bug356GitCommit(t, workspace, "git", "commit", "-m", "registry")
	p4WriteFile(t, workspace, "requirements/08-Task/todo/Task-910-x.md", "# Task\n")
	p4WriteFile(t, workspace, "calc.go", "package calc\n")
	edges, nodes := bug356SliceNodes()
	auditNode, ok := findFlowNode(nodes, "audit")
	if !ok {
		t.Fatal("fixture must declare an audit node")
	}
	svc.mu.Lock()
	rs := svc.runs[runID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.workspaceCwd = workspace
	rs.flowStartGitHead = headOfRepo(t, workspace)
	rs.planContextPackage = &FlowContextPackage{FeatureKey: "calc-core", FeatureConfidence: ConfidenceVerified}
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(runID, AgentLoopState{Status: "running", Cap: 3, Mode: "explicit"})
	svc.reseedFlowStepRuntime(runID, nodes)

	svc.runAuditNode(context.Background(), runID, edges, nodes, auditNode, "split with stray code edit")
	svc.mu.Lock()
	st := svc.runs[runID].flowValidationRetryState
	svc.mu.Unlock()
	if st != nil && st.Status == "passed" {
		t.Fatal("code-touching diff must never verify via the docs-only path")
	}
	loop := svc.agentOrchestrator.loopStateFor(runID)
	if loop.Status != "blocked" {
		t.Fatalf("loop = %q, want blocked (fail-closed stands)", loop.Status)
	}
}
