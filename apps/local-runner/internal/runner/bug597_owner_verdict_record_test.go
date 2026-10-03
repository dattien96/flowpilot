package runner

import (
	"context"
	"strings"
	"testing"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

// BUG-597 (live run-100368): on a healthy owner-debate mount, an owner
// leg's submit_review_outcome is rejected by the cohort-member guard —
// requireVerdict only covers plan_synthesis/synthesis/cp_synthesis hubs, so
// the owner_debate cohort falls to the generic "this is a cohort review
// step" rejection. The verdict never lands in pendingReviewVerdictByLabel,
// the join note carries only prose, and the owner leg gets re-driven with
// "verdict never recorded" reprompts until the debate churns.
//
// The verdict record is an inert buffer unless the hub's done-edge gates
// on it (hubInboundCohortName) — recording a cohort member's
// submit_review_outcome is always safe and is what lets the
// owner_debate cohort report structured verdicts at all.

func TestBUG597OwnerCohortVerdictRecordedOnDebateMount(t *testing.T) {
	store := newFakeWorkflowStore()
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)
	parentID := "run-bug597-parent"
	ownerID := "run-bug597-owner1"

	parent := &interactiveRun{
		id:               parentID,
		projectID:        "proj",
		status:           RunStatusRunning,
		agentStatus:      string(RunStatusRunning),
		workingMode:      workingmode.Vibe,
		chatFlowRef:      workingmode.PackPrefix + vibeOwnerDebateFlowID,
		autoOrchestrate:  true,
		flowEngineDriven: true,
		activeFlowNodes:  bug567DebateGraph(),
	}
	owner := &interactiveRun{
		id:           ownerID,
		projectID:    "proj",
		parentRunID:  parentID,
		label:        "owner_1",
		agentName:    "owner",
		flowCohortId: "owner_debate",
		status:       RunStatusRunning,
		agentStatus:  string(RunStatusRunning),
	}
	svc.mu.Lock()
	svc.runs[parentID] = parent
	svc.runs[ownerID] = owner
	svc.mu.Unlock()

	bridge := &turnBridge{svc: svc, rs: owner, ctx: context.Background(), turnID: "t-owner1"}
	res, err := bridge.SubmitFlowControl(FlowControlInput{
		Status:              "approved",
		Summary:             "verdict: gate violation is real",
		viaReviewOutcome:    true,
		reviewOutcomeStatus: "changes_requested",
	})
	if err != nil {
		t.Fatalf("owner submit_review_outcome rejected on a healthy debate mount: %v", err)
	}
	if res.NextAction != "review_verdict_recorded" {
		t.Fatalf("owner verdict not recorded: NextAction=%q", res.NextAction)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	got := parent.pendingReviewVerdictByLabel["owner_1"]
	if got != "changes_requested" {
		t.Fatalf("pendingReviewVerdictByLabel[owner_1]=%q want %q", got, "changes_requested")
	}
}

// The generic rejection must stay for non-verdict calls — a cohort member
// driving done/continue still terminates the flow on its own.
func TestBUG597CohortMemberNonVerdictCallStillRejected(t *testing.T) {
	store := newFakeWorkflowStore()
	svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)
	parentID := "run-bug597b-parent"
	ownerID := "run-bug597b-owner1"

	parent := &interactiveRun{
		id:              parentID,
		projectID:       "proj",
		status:          RunStatusRunning,
		workingMode:     workingmode.Vibe,
		chatFlowRef:     workingmode.PackPrefix + vibeOwnerDebateFlowID,
		activeFlowNodes: bug567DebateGraph(),
	}
	owner := &interactiveRun{
		id:           ownerID,
		parentRunID:  parentID,
		label:        "owner_1",
		flowCohortId: "owner_debate",
		status:       RunStatusRunning,
	}
	svc.mu.Lock()
	svc.runs[parentID] = parent
	svc.runs[ownerID] = owner
	svc.mu.Unlock()

	bridge := &turnBridge{svc: svc, rs: owner, ctx: context.Background(), turnID: "t-owner1"}
	_, err := bridge.SubmitFlowControl(FlowControlInput{Status: "done", Summary: "settle"})
	if err == nil || !strings.Contains(err.Error(), "cohort review step") {
		t.Fatalf("cohort member's non-verdict flow_control was not rejected: %v", err)
	}
}

// BUG-597 follow-up (recorded verdicts now land in lastReviewCohortVerdicts):
// a prose-finishing sprint `synthesis` hub must derive the transition from
// ITS OWN inbound review cohort only — a stray owner_debate verdict must
// never drive a sprint continue/done. Every other gate reader already
// filters to expected labels; the prose-derive must too.
func TestBUG597OwnerVerdictsNeverDriveSprintHubDerive(t *testing.T) {
	sprintNodes := []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.delegate", Agent: "agents/coder.md"},
		{ID: "reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md", Cohort: "review"},
		{ID: "synthesis", Behavior: "hub.inline"},
	}
	cases := []struct {
		name     string
		verdicts map[string]string
		want     string
	}{
		// Owner changes_requested must not drag the sprint into a continue
		// round — the reviewer's approval is the only verdict that counts.
		{"owner changes + reviewer approved", map[string]string{
			"owner_1": "changes_requested", "owner_2": "approved", "reviewer": "approved",
		}, "done"},
		// Owner approvals alone can never satisfy the review cohort — the
		// reviewer verdict is still missing, so keep the BUG-226 escalate.
		{"owner approvals only", map[string]string{
			"owner_1": "approved", "owner_2": "approved",
		}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newFakeWorkflowStore()
			svc, _ := newTestServerWith(t, DefaultProviderRegistry(), newInteractiveCatalog(), store)
			parentID := "run-bug597c-" + strings.NewReplacer(" ", "-", "+", "").Replace(tc.name)
			svc.mu.Lock()
			svc.runs[parentID] = &interactiveRun{
				id:                       parentID,
				projectID:                "proj",
				status:                   RunStatusRunning,
				agentStatus:              string(RunStatusRunning),
				autoOrchestrate:          true,
				flowEngineDriven:         true,
				activeFlowNodes:          sprintNodes,
				activeHubNodeID:          "synthesis",
				lastReviewCohortVerdicts: tc.verdicts,
			}
			svc.mu.Unlock()
			if got := svc.hubProseVerdictDerivesFlowStatus(parentID); got != tc.want {
				t.Fatalf("derived = %q, want %q", got, tc.want)
			}
		})
	}
}
