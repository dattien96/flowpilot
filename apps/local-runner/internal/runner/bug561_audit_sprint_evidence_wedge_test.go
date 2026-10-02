package runner

// BUG-561 (live run-60899): the audit escalate for "sprint evidence
// incomplete (open issues or unfinished coder leg)" had two gaps:
//
//  1. The not-ready defer was cohort-only. Vibe sprint coder legs are hub
//     ad-hoc spawn_agent calls (flow_cohort_id=""), so a leg mid-run without
//     an open cohort was still exposed to the BUG-560 kill pattern — audit
//     escalate → park → in-flight leg cancelled ("interrupted by user"),
//     then its OpenIssues never get adjudicated. A RUNNING non-audit flow
//     step is owed a terminal transition that re-drives the flow, so a
//     not-ready audit must defer (step back to PENDING) exactly like the
//     open-cohort case.
//  2. The park's Continue had no remediation owner: it re-entered the audit
//     node on byte-identical state and re-escalated forever. The sprint hub
//     owns remediation — it spawned the legs and it alone emits
//     continue-with-issues. Continue must reinvoke the hub with a
//     remediation note.
//
// New file; no pre-existing test is modified.

import (
	"context"
	"strings"
	"testing"
	"time"
)

// A RUNNING upstream leg without an open cohort must defer the audit the
// same way an open cohort does — the leg's settle is the pending evidence.
func TestBug561_AuditDefersWhileUpstreamStepRunning(t *testing.T) {
	svc, runID, nodes, edges, auditNode := ca1096Setup(t, 3, true)

	// Reviewer-class leg in flight but NOT inside a cohort barrier (vibe
	// coder/reviewer legs can be hub ad-hoc spawns with no flow_cohort_id).
	svc.setFlowStepStatus(context.Background(), runID, "validate", StepStatusRunning)
	if svc.agentOrchestrator.hasOpenCohort(runID) {
		t.Fatal("fixture must have no open cohort — the step-RUNNING path is the one under test")
	}

	if !svc.runAuditNode(context.Background(), runID, edges, nodes, auditNode, "task partial") {
		t.Fatal("runAuditNode must handle the dispatch")
	}
	if loop := svc.agentOrchestrator.loopStateFor(runID); loop.Status == "blocked" {
		t.Fatalf("audit must defer while an upstream leg is RUNNING, not escalate/park: %+v", loop)
	}
	if got := flowStepStatus(t, svc, runID, "audit"); got != StepStatusPending {
		t.Fatalf("deferred audit step = %v, want PENDING (re-dispatched when the leg settles)", got)
	}
}

// Terminal upstream steps still escalate — deferral is a pause for in-flight
// evidence, not a swallow of genuinely incomplete sprints.
func TestBug561_AuditEscalatesWhenNoStepInFlight(t *testing.T) {
	svc, runID, nodes, edges, auditNode := ca1096Setup(t, 3, true)

	svc.runAuditNode(context.Background(), runID, edges, nodes, auditNode, "task partial")

	if loop := svc.agentOrchestrator.loopStateFor(runID); loop.Status != "blocked" {
		t.Fatalf("all legs terminal + open issues → audit must escalate as before: %+v", loop)
	}
}

// Bare Continue on a sprint-evidence audit park must re-drive the hub (the
// remediation owner), not re-enter the audit node — the audit cannot change
// its own evidence.
func TestBug561_SprintEvidenceContinueReinvokesHub(t *testing.T) {
	providers := []ProviderKey{ProviderKeyClaude, ProviderKeyCodex, ProviderKeyGrok}
	for _, pk := range providers {
		t.Run(string(pk), func(t *testing.T) {
			ch := make(chan TurnRequest, 8)
			reg := newProviderRegistry()
			registerKeyedCapture(reg, pk, ch)
			svc := newInteractiveService(reg, newInteractiveCatalog(), newFakeWorkflowStore())
			parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: pk})
			if err != nil {
				t.Fatalf("createRun: %v", err)
			}
			edges, nodes := ca1098VerdictTopology()
			setRun198699Topology(t, svc, parent.RunID, edges, nodes, "synthesis")

			// Park exactly as the live audit escalate left it: loop blocked on
			// escalate, audit node stamped as the escalated inline node.
			svc.stampLastEscalatedInlineNode(parent.RunID, "audit")
			svc.setFlowStepStatus(context.Background(), parent.RunID, "audit", StepStatusWaitingUserApr)
			svc.agentOrchestrator.mutateLoop(parent.RunID, func(st AgentLoopState) AgentLoopState {
				st.Status = "blocked"
				st.BlockReason = "escalate"
				st.GateReason = "Audit blocked: sprint evidence incomplete (open issues or unfinished coder leg); cannot auto-finalize."
				st.OpenIssues = 2
				return st
			})

			if _, err := svc.resumeFlowWithFeedback(parent.RunID, ""); err != nil {
				t.Fatalf("%s: resumeFlowWithFeedback: %v", pk, err)
			}

			deadline := time.Now().Add(10 * time.Second)
			for {
				select {
				case req := <-ch:
					if req.RunID != parent.RunID {
						continue
					}
					// The hub turn must carry the remediation directive — a bare
					// re-prompt would let it resubmit done on the same evidence.
					if !strings.Contains(req.Prompt, "sprint evidence is incomplete") {
						t.Fatalf("%s: hub reinvoke missing remediation note (prompt=%q)", pk, truncate1098(req.Prompt, 200))
					}
					return
				case <-time.After(time.Until(deadline)):
					t.Fatalf("%s: no hub turn — Continue re-entered the audit node and re-parked (the wedge)", pk)
				}
			}
		})
	}
}
