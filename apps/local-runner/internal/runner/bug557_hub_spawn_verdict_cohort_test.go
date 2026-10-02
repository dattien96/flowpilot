package runner

import (
	"context"
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// BUG-557 (live run-38799, PrivateVault CP-02): after a missing-verdict
// escalation the hub spawned reviewer "cautious-soil" ad-hoc. The child ran
// with flow_cohort_id="" so submit_review_outcome was never advertised on its
// tools/list — Devin ask-mode rejected the call outright and the machine
// verdict could never be recorded, wedging synthesis until the leg was
// re-driven through the engine path. Fix: a hub ad-hoc spawn matching a
// verdict-bearing active flow node gets a one-member ad-hoc cohort and the
// node's label so the verdict lands under the label synthesis expects.

func TestBug557_HubSpawnVerdictNodeStampsCohort(t *testing.T) {
	svc := newInteractiveService(bug556Registry(), newInteractiveCatalog(), newFakeWorkflowStore())

	parent, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Cwd: t.TempDir(),
	})
	if apiErr != nil {
		t.Fatalf("parent createRun: %v", apiErr)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.modelName = "gpt-5.4-mini"
	rs.flowEngineDriven = true
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md", Posture: PostureReadOnly, Cohort: "review"},
	}
	rs.activeFlowAcceptanceNodes = []string{synthesisAcceptanceNodeID}
	svc.mu.Unlock()

	res, err := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent:  "reviewer",
		Prompt: "review the change",
	})
	if err != nil {
		t.Fatalf("spawnChildRun: %v", err)
	}
	svc.mu.Lock()
	child := svc.runs[res.RunID]
	gotCohort, gotLabel := child.flowCohortId, child.label
	svc.mu.Unlock()
	if gotCohort != "flow-adhoc-reviewer" {
		t.Fatalf("verdict-node ad-hoc spawn cohort = %q, want flow-adhoc-reviewer", gotCohort)
	}
	if gotLabel != "reviewer" {
		t.Fatalf("verdict-node ad-hoc spawn label = %q, want node id reviewer (verdict records by label)", gotLabel)
	}
}

// A matched non-verdict node (writer/delegate, standard posture, no declared
// cohort) keeps the plain-child shape — stamping a cohort would reroute its
// flow_control calls through the review-step error.
func TestBug557_HubSpawnNonVerdictNodeKeepsNoCohort(t *testing.T) {
	svc := newInteractiveService(bug556Registry(), newInteractiveCatalog(), newFakeWorkflowStore())

	parent, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Cwd: t.TempDir(),
	})
	if apiErr != nil {
		t.Fatalf("parent createRun: %v", apiErr)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.modelName = "gpt-5.4-mini"
	rs.flowEngineDriven = true
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
	}
	svc.mu.Unlock()

	res, err := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent:  "coder",
		Prompt: "implement",
	})
	if err != nil {
		t.Fatalf("spawnChildRun: %v", err)
	}
	svc.mu.Lock()
	got := svc.runs[res.RunID].flowCohortId
	svc.mu.Unlock()
	if got != "" {
		t.Fatalf("non-verdict node ad-hoc spawn must not gain a cohort, got %q", got)
	}
}

// An explicit FlowCohortID on the input (engine dispatch, re-drive) is never
// overwritten by the ad-hoc stamp.
func TestBug557_ExplicitCohortStillWins(t *testing.T) {
	svc := newInteractiveService(bug556Registry(), newInteractiveCatalog(), newFakeWorkflowStore())

	parent, apiErr := svc.createRun(StartRunInput{
		ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex, Cwd: t.TempDir(),
	})
	if apiErr != nil {
		t.Fatalf("parent createRun: %v", apiErr)
	}
	svc.mu.Lock()
	rs := svc.runs[parent.RunID]
	rs.flowEngineDriven = true
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md", Posture: PostureReadOnly, Cohort: "review"},
	}
	svc.mu.Unlock()

	res, err := svc.spawnChildRun(context.Background(), parent.RunID, SpawnAgentInput{
		Agent:        "reviewer",
		Prompt:       "review",
		FlowCohortID: "flow-auto-validate-round-3",
		CohortSize:   1,
	})
	if err != nil {
		t.Fatalf("spawnChildRun: %v", err)
	}
	svc.mu.Lock()
	got := svc.runs[res.RunID].flowCohortId
	svc.mu.Unlock()
	if got != "flow-auto-validate-round-3" {
		t.Fatalf("explicit cohort must win, got %q", got)
	}
}
