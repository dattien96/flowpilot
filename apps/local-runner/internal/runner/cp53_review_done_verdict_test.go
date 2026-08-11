package runner

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
)

func cp53SeedSynthesisAcceptanceRun(t *testing.T, svc *InteractiveService, parentID string) {
	t.Helper()
	svc.mu.Lock()
	rs := svc.runs[parentID]
	if rs == nil {
		t.Fatalf("run %q missing", parentID)
	}
	rs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.delegate", Agent: "agents/coder.md"},
		{ID: "reviewer_correctness", Behavior: "agent.delegate", Agent: "agents/reviewer.md", DependsOn: []string{"coder"}, Cohort: "review"},
		{ID: "reviewer_security", Behavior: "agent.delegate", Agent: "agents/reviewer.md", DependsOn: []string{"coder"}, Cohort: "review"},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md"},
	}
	rs.activeFlowAcceptanceNodes = []string{synthesisAcceptanceNodeID}
	rs.autoOrchestrate = true
	rs.flowEngineDriven = true
	svc.mu.Unlock()
	svc.agentOrchestrator.setLoop(parentID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})
}

func cp53RecordReviewerVerdicts(t *testing.T, svc *InteractiveService, parentID, cohortID string) {
	t.Helper()
	for _, label := range []string{"reviewer_correctness", "reviewer_security"} {
		childID := parentID + "-" + label
		svc.mu.Lock()
		svc.runs[childID] = &interactiveRun{
			id:           childID,
			parentRunID:  parentID,
			flowCohortId: cohortID,
			label:        label,
			subs:         map[int64]chan ProviderEvent{},
			idempotency:  map[string]string{},
		}
		child := svc.runs[childID]
		svc.mu.Unlock()
		fc, err := reviewOutcomeToFlowControl(ReviewOutcomeInput{Status: "approved"})
		if err != nil {
			t.Fatalf("reviewOutcomeToFlowControl: %v", err)
		}
		if _, err := (&turnBridge{svc: svc, rs: child}).SubmitFlowControl(fc); err != nil {
			t.Fatalf("reviewer %q verdict: %v", label, err)
		}
		svc.agentOrchestrator.appendCohortResult(parentID, cohortID, cohortEntry{
			Label:          label,
			Status:         "completed",
			MachineVerdict: "approved",
		})
	}
	svc.snapshotReviewCohortVerdicts(parentID, []cohortEntry{
		{Label: "reviewer_correctness", MachineVerdict: "approved"},
		{Label: "reviewer_security", MachineVerdict: "approved"},
	})
}

func TestCP53ReviewDoneVerdictHubApprovedWithoutReviewerVerdictBlocked(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, runErr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if runErr != nil {
		t.Fatalf("createRun: %v", runErr)
	}
	cp53SeedSynthesisAcceptanceRun(t, svc, parent.RunID)

	fc, err := reviewOutcomeToFlowControl(ReviewOutcomeInput{Status: "approved", Feedback: "ship it"})
	if err != nil {
		t.Fatalf("reviewOutcomeToFlowControl: %v", err)
	}
	if _, err := svc.applyFlowControl(parent.RunID, fc); err == nil {
		t.Fatal("expected approved→done to fail without reviewer machine verdicts")
	}
	if st := svc.agentOrchestrator.loopStateFor(parent.RunID); st.Status == "done" {
		t.Fatalf("loop must not reach done without reviewer verdicts, got %q", st.Status)
	}
}

func TestCP53ReviewDoneVerdictFailReviewerBlocksHubApproved(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, runErr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if runErr != nil {
		t.Fatalf("createRun: %v", runErr)
	}
	cp53SeedSynthesisAcceptanceRun(t, svc, parent.RunID)

	svc.snapshotReviewCohortVerdicts(parent.RunID, []cohortEntry{
		{Label: "reviewer_correctness", MachineVerdict: "approved"},
		{Label: "reviewer_security", MachineVerdict: "changes_requested"},
	})
	fc, err := reviewOutcomeToFlowControl(ReviewOutcomeInput{Status: "approved"})
	if err != nil {
		t.Fatalf("reviewOutcomeToFlowControl: %v", err)
	}
	if _, err := svc.applyFlowControl(parent.RunID, fc); err == nil {
		t.Fatal("expected approved→done to fail when a reviewer verdict is not approved")
	}
}

func TestCP53ReviewDoneVerdictPassAllowsDone(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, runErr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if runErr != nil {
		t.Fatalf("createRun: %v", runErr)
	}
	cp53SeedSynthesisAcceptanceRun(t, svc, parent.RunID)
	cp53RecordReviewerVerdicts(t, svc, parent.RunID, "review-round-0")

	fc, err := reviewOutcomeToFlowControl(ReviewOutcomeInput{Status: "approved", Feedback: "all clear"})
	if err != nil {
		t.Fatalf("reviewOutcomeToFlowControl: %v", err)
	}
	if _, err := svc.applyFlowControl(parent.RunID, fc); err != nil {
		t.Fatalf("approved→done with reviewer PASS verdicts: %v", err)
	}
	if st := svc.agentOrchestrator.loopStateFor(parent.RunID); st.Status != "done" {
		t.Fatalf("loop.Status = %q, want done", st.Status)
	}
}

func TestCP53ReviewDoneVerdictContinueUnaffected(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, runErr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if runErr != nil {
		t.Fatalf("createRun: %v", runErr)
	}
	cp53SeedSynthesisAcceptanceRun(t, svc, parent.RunID)

	fc, err := reviewOutcomeToFlowControl(ReviewOutcomeInput{
		Status:   "changes_requested",
		Feedback: "fix the nil check",
	})
	if err != nil {
		t.Fatalf("reviewOutcomeToFlowControl: %v", err)
	}
	res, err := svc.applyFlowControl(parent.RunID, fc)
	if err != nil {
		t.Fatalf("changes_requested→continue: %v", err)
	}
	if res.Status != "continue" {
		t.Fatalf("result.Status = %q, want continue", res.Status)
	}
	if st := svc.agentOrchestrator.loopStateFor(parent.RunID); st.Status == "done" {
		t.Fatal("continue must not mark loop done")
	}
}

func TestCP53ReviewDoneVerdictNormalChatUnaffected(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, runErr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if runErr != nil {
		t.Fatalf("createRun: %v", runErr)
	}
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, RoundCap: 3})

	fc, err := reviewOutcomeToFlowControl(ReviewOutcomeInput{Status: "approved"})
	if err != nil {
		t.Fatalf("reviewOutcomeToFlowControl: %v", err)
	}
	if _, err := svc.applyFlowControl(parent.RunID, fc); err != nil {
		t.Fatalf("normal chat approved→done must not require review cohort verdicts: %v", err)
	}
}

func TestCP53ReviewDoneVerdictReviewerRecordsWithoutAdvancingFlow(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, runErr := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if runErr != nil {
		t.Fatalf("createRun: %v", runErr)
	}
	cp53SeedSynthesisAcceptanceRun(t, svc, parent.RunID)

	const childID = "child-reviewer-correctness"
	svc.mu.Lock()
	svc.runs[childID] = &interactiveRun{
		id:           childID,
		parentRunID:  parent.RunID,
		flowCohortId: "review-cohort",
		label:        "reviewer_correctness",
		subs:         map[int64]chan ProviderEvent{},
		idempotency:  map[string]string{},
	}
	child := svc.runs[childID]
	svc.mu.Unlock()

	fc, err := reviewOutcomeToFlowControl(ReviewOutcomeInput{Status: "approved", Feedback: "looks good"})
	if err != nil {
		t.Fatalf("reviewOutcomeToFlowControl: %v", err)
	}
	res, err := (&turnBridge{svc: svc, rs: child}).SubmitFlowControl(fc)
	if err != nil {
		t.Fatalf("reviewer record-only submit_review_outcome: %v", err)
	}
	if res.NextAction != "review_verdict_recorded" {
		t.Fatalf("NextAction = %q, want review_verdict_recorded", res.NextAction)
	}
	if st := svc.agentOrchestrator.loopStateFor(parent.RunID); st.Status == "done" {
		t.Fatal("reviewer verdict must not advance loop to done")
	}
	svc.mu.Lock()
	got := svc.runs[parent.RunID].pendingReviewVerdictByLabel["reviewer_correctness"]
	svc.mu.Unlock()
	if got != "approved" {
		t.Fatalf("pendingReviewVerdictByLabel = %q, want approved", got)
	}
}

func TestCP53ReviewDoneVerdictProviderMatrix(t *testing.T) {
	providers := []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
	for _, pk := range providers {
		t.Run(string(pk), func(t *testing.T) {
			svc, _ := newTestServer(t)
			parent, runErr := svc.createRun(StartRunInput{ProjectID: "p", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
			if runErr != nil {
				t.Fatalf("createRun: %v", runErr)
			}
			cp53SeedSynthesisAcceptanceRun(t, svc, parent.RunID)

			for _, label := range []string{"reviewer_correctness", "reviewer_security"} {
				childID := parent.RunID + "-" + label + "-" + string(pk)
				svc.mu.Lock()
				svc.runs[childID] = &interactiveRun{
					id:           childID,
					parentRunID:  parent.RunID,
					flowCohortId: "review-cohort",
					label:        label,
					providerKey:  pk,
					subs:         map[int64]chan ProviderEvent{},
					idempotency:  map[string]string{},
				}
				child := svc.runs[childID]
				svc.mu.Unlock()
				fc, mapErr := reviewOutcomeToFlowControl(ReviewOutcomeInput{Status: "approved"})
				if mapErr != nil {
					t.Fatalf("reviewOutcomeToFlowControl: %v", mapErr)
				}
				if _, submitErr := (&turnBridge{svc: svc, rs: child}).SubmitFlowControl(fc); submitErr != nil {
					t.Fatalf("provider %q reviewer %q submit_review_outcome: %v", pk, label, submitErr)
				}
			}
			svc.snapshotReviewCohortVerdicts(parent.RunID, []cohortEntry{
				{Label: "reviewer_correctness", MachineVerdict: "approved"},
				{Label: "reviewer_security", MachineVerdict: "approved"},
			})

			hubFC, mapErr := reviewOutcomeToFlowControl(ReviewOutcomeInput{Status: "approved", Feedback: "matrix " + string(pk)})
			if mapErr != nil {
				t.Fatalf("reviewOutcomeToFlowControl hub: %v", mapErr)
			}
			if _, fcErr := svc.applyFlowControl(parent.RunID, hubFC); fcErr != nil {
				t.Fatalf("provider %q hub approved→done: %v", pk, fcErr)
			}
			if st := svc.agentOrchestrator.loopStateFor(parent.RunID); st.Status != "done" {
				t.Fatalf("provider %q loop.Status = %q, want done", pk, st.Status)
			}
		})
	}
}

func TestFlowRequiresSynthesisMachineVerdict(t *testing.T) {
	rs := &interactiveRun{activeFlowAcceptanceNodes: []string{"synthesis"}}
	if !flowRequiresSynthesisMachineVerdict(rs) {
		t.Fatal("expected synthesis acceptance to require machine verdict")
	}
	rs2 := &interactiveRun{activeFlowAcceptanceNodes: []string{"validate"}}
	if flowRequiresSynthesisMachineVerdict(rs2) {
		t.Fatal("validate-only acceptance must not trigger synthesis verdict gate")
	}
	labels := reviewCohortNodeLabels([]agentpack.FlowNode{
		{ID: "reviewer_correctness", Cohort: "review"},
		{ID: "reviewer_security", Cohort: "review"},
		{ID: "coder", Cohort: ""},
	})
	if len(labels) != 2 || labels[0] != "reviewer_correctness" || labels[1] != "reviewer_security" {
		t.Fatalf("reviewCohortNodeLabels = %#v", labels)
	}
}
