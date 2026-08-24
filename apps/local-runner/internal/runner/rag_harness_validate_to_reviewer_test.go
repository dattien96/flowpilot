package runner

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// Task-293: after validate passes, rag-harness's validate --done--> reviewer
// edge must spawn the single review-cohort reviewer (with a cohort id so its
// completion joins the synthesis barrier), not fall into the old inline
// validate->audit path. New file — no pre-existing test is modified.

func validateToReviewerFixture() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	edges := []agentpack.FlowEdge{
		{From: "implement", To: "validate", When: "done", Kind: "forward"},
		{From: "validate", To: "reviewer", When: "done", Kind: "forward"},
		{From: "reviewer", To: "synthesis", When: "done", Kind: "forward"},
		{From: "validate", To: "implement", When: "continue", Kind: "back"},
		{From: "synthesis", To: "implement", When: "continue", Kind: "back"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "implement", Behavior: "agent.code", Agent: "agents/coder.md"},
		{ID: "validate", Behavior: "command.validate"},
		{ID: "reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md", Cohort: "review", Join: "all"},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Join: "all"},
	}
	return edges, nodes
}

func newValidatePassWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	initGitRepoForAuditFixture(t, dir)
	writeBaseline(t, dir, "true")
	settingsDir := filepath.Join(dir, ".flowpilot", "settings")
	if err := os.MkdirAll(settingsDir, 0o755); err != nil {
		t.Fatalf("mkdir settings dir: %v", err)
	}
	// Pin the command so ensureBaseline's async recapture cannot empty it.
	if err := os.WriteFile(filepath.Join(settingsDir, "test-config.json"), []byte(`{"test_command":"true"}`), 0o644); err != nil {
		t.Fatalf("write test-config.json: %v", err)
	}
	return dir
}

func TestValidatePassedSpawnsReviewerCohortMember(t *testing.T) {
	svc := newFreezeTestService(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	edges, nodes := validateToReviewerFixture()
	dir := newValidatePassWorkspace(t)

	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.workspaceCwd = dir
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parent.RunID, nodes)

	// implement completes -> validate runs ("true" passes) -> reviewer spawns.
	if !svc.tryAdvanceFlowFromNode(parent.RunID, "implement", "fixed it") {
		t.Fatal("expected tryAdvanceFlowFromNode(implement) to advance into validate/reviewer")
	}

	var reviewerID, cohortID string
	waitLoop(t, "reviewer cohort member spawned after validate passed", 3*time.Second, func() bool {
		svc.mu.Lock()
		defer svc.mu.Unlock()
		for _, run := range svc.runs {
			if run.parentRunID == parent.RunID && run.label == "reviewer" {
				reviewerID = run.id
				cohortID = run.flowCohortId
				return true
			}
		}
		return false
	})
	_ = reviewerID
	if cohortID == "" {
		t.Fatal("reviewer must carry a flowCohortId so its completion joins the synthesis barrier")
	}

	// Reviewer turn auto-completes (fake adapter) -> cohort join drains the
	// barrier and drives the synthesis hub step RUNNING synchronously (the joined
	// cohort entry is deleted on drain, so the barrier itself is no longer
	// observable; the hub step status is).
	waitLoop(t, "synthesis hub step driven RUNNING after cohort join", 3*time.Second, func() bool {
		return flowStepStatus(t, svc, parent.RunID, "synthesis") == StepStatusRunning
	})

	// Synthesis step must remain RUNNING (armed hub turn, not settled).
	if got := flowStepStatus(t, svc, parent.RunID, "synthesis"); got != StepStatusRunning {
		t.Fatalf("synthesis step status = %v, want RUNNING after cohort join", got)
	}
}

func TestValidatePassedStillChainsInlineAuditForLegacyEdges(t *testing.T) {
	// The old validate --done--> audit (inline) shape must keep working through
	// the same runValidateNode passed branch.
	svc := newFreezeTestService(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
	edges, nodes := flowFixtureEdgesNodes()
	dir := newValidatePassWorkspace(t)

	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.activeFlowEdges = edges
	rs.activeFlowNodes = nodes
	rs.flowEngineDriven = true
	rs.autoOrchestrate = true
	rs.workspaceCwd = dir
	svc.mu.Unlock()
	svc.reseedFlowStepRuntime(parent.RunID, nodes)

	if !svc.tryAdvanceFlowFromNode(parent.RunID, "implement", "fixed it") {
		t.Fatal("expected tryAdvanceFlowFromNode(implement) to advance into validate/audit")
	}
	// validate -> audit is a single inline target; audit runs in-process (its
	// BuildAuditDraft observation fails without a validation state, escalating
	// rather than silently settling — the pre-existing behavior).
	if got := flowStepStatus(t, svc, parent.RunID, "audit"); got != StepStatusRunning && got != StepStatusWaitingUserApr {
		t.Fatalf("audit step status = %v, want RUNNING or WAITING_USER_APPROVAL (legacy inline chain preserved)", got)
	}
}
