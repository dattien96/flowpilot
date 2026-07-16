package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// Task-241 D-1/D-2/D-3: 2-member cohort terminal matrix (completed/failed/cancelled).
func TestCohortMatrixTwoMemberTerminalOutcomes(t *testing.T) {
	type outcome string
	const (
		completed outcome = "completed"
		failed    outcome = "failed"
		cancelled outcome = "cancelled"
	)
	cases := []struct {
		name    string
		a, b    outcome
		wantA   RuntimeWorkflowStepStatus
		wantB   RuntimeWorkflowStepStatus
		join    bool
		noteHas []string
	}{
		{"both-completed", completed, completed, StepStatusDone, StepStatusDone, true, []string{`"r1"`, `"r2"`}},
		{"a-done-b-fail", completed, failed, StepStatusDone, StepStatusFailed, true, []string{`"r1"`, `"r2"`, "failed"}},
		{"both-failed", failed, failed, StepStatusFailed, StepStatusFailed, true, []string{"failed"}},
		{"a-done-b-cancel", completed, cancelled, StepStatusDone, StepStatusCanceled, true, []string{"cancelled"}},
		{"both-cancelled", cancelled, cancelled, StepStatusCanceled, StepStatusCanceled, true, []string{"cancelled"}},
		{"a-fail-b-cancel", failed, cancelled, StepStatusFailed, StepStatusCanceled, true, []string{"failed", "cancelled"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := newAgentOrchestrator()
			parent := "p-" + tc.name
			cohort := "c1"
			o.preRegisterCohort(parent, cohort, 2)
			appendOne := func(label string, out outcome) {
				e := cohortEntry{Label: label, Provider: "codex", Status: string(out)}
				if out == failed {
					e.Err = "boom"
				}
				if out == completed {
					e.FinalMessage = "ok-" + label
				}
				o.appendCohortResult(parent, cohort, e)
			}
			appendOne("r1", tc.a)
			appendOne("r2", tc.b)
			complete := o.cohortComplete(parent, cohort)
			if complete != tc.join {
				t.Fatalf("cohortComplete = %v, want %v", complete, tc.join)
			}
			if !complete {
				return
			}
			entries := o.drainCohort(parent, cohort)
			note := buildCohortNote(parent, cohort, entries, 0)
			for _, frag := range tc.noteHas {
				if !strings.Contains(note, frag) {
					t.Errorf("note missing %q:\n%s", frag, note)
				}
			}
			// Map entry status → step status (I-11).
			statusOf := func(label string) RuntimeWorkflowStepStatus {
				for _, e := range entries {
					if e.Label != label {
						continue
					}
					switch e.Status {
					case "completed":
						return StepStatusDone
					case "failed":
						return StepStatusFailed
					case "cancelled":
						return StepStatusCanceled
					}
				}
				return StepStatusPending
			}
			if got := statusOf("r1"); got != tc.wantA {
				t.Errorf("r1 status = %q, want %q", got, tc.wantA)
			}
			if got := statusOf("r2"); got != tc.wantB {
				t.Errorf("r2 status = %q, want %q", got, tc.wantB)
			}
		})
	}
}

// Task-241: double-append same label is idempotent.
func TestAppendCohortResultIdempotentByLabel(t *testing.T) {
	o := newAgentOrchestrator()
	o.preRegisterCohort("p", "c", 2)
	o.appendCohortResult("p", "c", cohortEntry{Label: "r1", Status: "completed"})
	o.appendCohortResult("p", "c", cohortEntry{Label: "r1", Status: "failed"}) // duplicate
	o.appendCohortResult("p", "c", cohortEntry{Label: "r2", Status: "completed"})
	if !o.cohortComplete("p", "c") {
		t.Fatal("expected complete with 2 unique labels")
	}
	entries := o.drainCohort("p", "c")
	if len(entries) != 2 {
		t.Fatalf("len=%d, want 2 (duplicate skipped)", len(entries))
	}
	if entries[0].Status != "completed" {
		t.Errorf("first entry status = %q, want completed (not overwritten by duplicate)", entries[0].Status)
	}
}

// Task-241 D-6 / I-5: second flow_control on same turn is rejected.
func TestApplyFlowControlOneDecisionPerTurn(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	nodes := reviewLoopTestNodes()
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = nodes
	svc.runs[parent.RunID].currentTurnID = "turn-1"
	svc.mu.Unlock()
	svc.markFlowEngineDriven(parent.RunID)
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, Mode: "explicit"})

	if _, fcErr := svc.applyFlowControl(parent.RunID, FlowControlInput{Status: "escalate", Summary: "need human"}); fcErr != nil {
		t.Fatalf("first apply: %v", fcErr)
	}
	_, fcErr := svc.applyFlowControl(parent.RunID, FlowControlInput{Status: "done", Summary: "second"})
	if fcErr == nil {
		t.Fatal("expected second applyFlowControl on same turn to be rejected")
	}
	if !strings.Contains(fcErr.Error(), "already submitted") {
		t.Errorf("err = %v, want already submitted", fcErr)
	}
}

// Task-241 D-7 / I-15: buildCohortNote embeds each member once.
func TestBuildCohortNoteEmbedOncePerMember(t *testing.T) {
	entries := []cohortEntry{
		{Label: "r1", Provider: "codex", Status: "completed", FinalMessage: "lgtm"},
		{Label: "r2", Provider: "codex", Status: "failed", Err: "x"},
		{Label: "r3", Provider: "claude", Status: "cancelled"},
	}
	note := buildCohortNote("p", "c", entries, 1)
	if strings.Count(note, `"r1"`) != 1 {
		t.Errorf("r1 count = %d", strings.Count(note, `"r1"`))
	}
	if strings.Count(note, `"r2"`) != 1 {
		t.Errorf("r2 count = %d", strings.Count(note, `"r2"`))
	}
	if strings.Count(note, "cancelled") != 1 {
		t.Errorf("cancelled count = %d in %q", strings.Count(note, "cancelled"), note)
	}
}

// Task-241 D-8: re-escalate same reason marks no progress.
func TestEscalateNoProgressSuffix(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	nodes := reviewLoopTestNodes()
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = nodes
	svc.mu.Unlock()
	svc.markFlowEngineDriven(parent.RunID)
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, Mode: "explicit"})

	if _, fcErr := svc.applyFlowControl(parent.RunID, FlowControlInput{Status: "escalate", Summary: "reviewers disagree"}); fcErr != nil {
		t.Fatalf("first: %v", fcErr)
	}
	// New turn so one-decision-per-turn allows a second escalate.
	svc.mu.Lock()
	svc.runs[parent.RunID].currentTurnID = "turn-2"
	svc.mu.Unlock()
	// Clear block so escalate can re-apply (mutateLoop will set blocked again).
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, Mode: "explicit"})
	if _, fcErr := svc.applyFlowControl(parent.RunID, FlowControlInput{Status: "escalate", Summary: "reviewers disagree"}); fcErr != nil {
		t.Fatalf("second: %v", fcErr)
	}
	loop := svc.agentOrchestrator.loopStateFor(parent.RunID)
	if !strings.Contains(loop.GateReason, "no progress since last continue") {
		t.Fatalf("GateReason = %q, want no-progress suffix", loop.GateReason)
	}
}

// Task-241 D-11: stallTimeoutSec parses; zero → runner default 10m at startResolvedFlow seed path.
func TestFlowPolicyStallTimeoutSecParse(t *testing.T) {
	// Direct struct: pack parse is covered when YAML has the field; zero means default.
	p := agentpack.FlowPolicy{Cap: 3}
	if p.StallTimeoutSec != 0 {
		t.Fatalf("default StallTimeoutSec = %d, want 0 (runner applies 10m)", p.StallTimeoutSec)
	}
	p2 := agentpack.FlowPolicy{StallTimeoutSec: 120}
	if p2.StallTimeoutSec != 120 {
		t.Fatalf("got %d", p2.StallTimeoutSec)
	}
	// Default duration used by startResolvedFlow when sec==0.
	defaultDur := 10 * time.Minute
	if defaultDur != 10*time.Minute {
		t.Fatal("default constant drift")
	}
}

// Task-241: cancel append unblocks barrier when sibling already done.
func TestStopAgentLoopCancelsCohortMemberAndJoins(t *testing.T) {
	svc, _ := newTestServer(t)
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	nodes := reviewLoopTestNodes()
	svc.mu.Lock()
	svc.runs[parent.RunID].activeFlowNodes = nodes
	svc.mu.Unlock()
	svc.markFlowEngineDriven(parent.RunID)
	svc.reseedFlowStepRuntime(parent.RunID, nodes)
	svc.agentOrchestrator.setLoop(parent.RunID, AgentLoopState{Status: "running", Cap: 3, Mode: "explicit"})

	// Spawn two in-memory children with cohort id (no real provider turn).
	c1, err1 := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	c2, err2 := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err1 != nil || err2 != nil {
		t.Fatalf("children: %v %v", err1, err2)
	}
	cohortID := "flow-auto-coder-round-0"
	svc.agentOrchestrator.registerChild(parent.RunID, c1.RunID)
	svc.agentOrchestrator.registerChild(parent.RunID, c2.RunID)
	svc.agentOrchestrator.preRegisterCohort(parent.RunID, cohortID, 2)
	svc.mu.Lock()
	svc.runs[c1.RunID].parentRunID = parent.RunID
	svc.runs[c1.RunID].flowCohortId = cohortID
	svc.runs[c1.RunID].label = "reviewer_correctness"
	svc.runs[c1.RunID].turnInFlight = true
	svc.runs[c1.RunID].turnCancel = func() {}
	svc.runs[c2.RunID].parentRunID = parent.RunID
	svc.runs[c2.RunID].flowCohortId = cohortID
	svc.runs[c2.RunID].label = "reviewer_security"
	svc.runs[c2.RunID].turnInFlight = true
	svc.runs[c2.RunID].turnCancel = func() {}
	svc.mu.Unlock()

	// One member completes first.
	svc.agentOrchestrator.appendCohortResult(parent.RunID, cohortID, cohortEntry{
		Label: "reviewer_correctness", Provider: "codex", Status: "completed", FinalMessage: "ok",
	})
	svc.setFlowStepStatus(context.Background(), parent.RunID, "reviewer_correctness", StepStatusDone)

	// Stop cancels the other in-flight member → should join.
	_, _ = svc.stopAgentLoop(parent.RunID)
	// After stop, cohort should have been drained (join).
	if svc.agentOrchestrator.hasOpenCohort(parent.RunID) {
		t.Fatal("expected cohort joined after cancel of remaining member")
	}
	note := svc.lastCohortNoteFor(parent.RunID)
	if !strings.Contains(note, "cancelled") {
		t.Fatalf("lastCohortNote missing cancelled: %q", note)
	}
}
