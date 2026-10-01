package runner

import (
	"context"
	"strings"
	"testing"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// CA-1098 (live run-31884, vibe-tasks scratch run): a reviewer cohort member
// that completes without a recorded submit_review_outcome verdict — the
// settle-time reprompt budget (verdictRepromptCount, CP-67) exhausted on a
// provider that never calls the tool — joins the cohort verdict-less and the
// synthesis hub's approved→done is rejected by hubDoneVerdictError. The
// escalate parks the run on the synthesis node with BlockReason=escalate and
// GateReason="…done blocked — missing machine verdict from review
// reviewer(s): reviewer". Every operator Continue then fell through to the
// generic hub reinvoke: the hub re-submitted approved, was rejected
// identically, and re-parked — observed live as round 0/3 repeating forever
// (the hub can never manufacture the missing member verdict).
//
// The fix mirrors CA-1074/run-202550: Continue on a missing-or-deficient
// machine-verdict escalate re-drives the deficient review-cohort member(s)
// with an explicit verdict instruction — their settle rejoins the cohort,
// refreshes lastReviewCohortVerdicts, and the hub re-decides on its own
// edges. Each member's verdictRepromptCount resets so the retried member
// keeps its full auto-reprompt budget.
//
// New file; no pre-existing test is modified. The engine path takes no
// providerKey, so the matrix guards future provider drift.

// ca1098VerdictTopology returns the minimal sprint tail the wedge needs:
// a review-cohort member feeding the synthesis hub, whose done edge rolls
// forward and whose escalate edge parks on ask_user.
func ca1098VerdictTopology() ([]agentpack.FlowEdge, []agentpack.FlowNode) {
	edges := []agentpack.FlowEdge{
		{From: "coder", To: "validate", When: "done", Kind: "forward"},
		{From: "validate", To: "reviewer", When: "done", Kind: "forward"},
		{From: "reviewer", To: "synthesis", When: "done", Kind: "forward"},
		{From: "synthesis", To: "ask_user", When: "escalate", Kind: "forward"},
		{From: "synthesis", To: "audit", When: "done", Kind: "forward"},
		{From: "audit", To: "done", When: "done", Kind: "forward"},
	}
	nodes := []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md", Lifecycle: "reinvoke"},
		{ID: "validate", Behavior: "command.validate", Lifecycle: "once"},
		{ID: "reviewer", Behavior: "agent.delegate", Agent: "agents/reviewer.md", Lifecycle: "reinvoke", Cohort: "review"},
		{ID: "synthesis", Behavior: "hub.inline", Agent: "agents/synthesizer.md", Lifecycle: "reinvoke", Join: "all"},
		{ID: "audit", Behavior: "artifact.audit_draft", Lifecycle: "once"},
	}
	return edges, nodes
}

const ca1098MissingVerdictGateReason = "advanceHubDoneThroughEdge: synthesis done blocked — missing machine verdict from review reviewer(s): reviewer"

// ca1098ParkedVerdictWedge builds the exact live wedge: parent flow run
// parked on the missing-verdict escalate, one reviewer member completed
// verdict-less with its reprompt budget exhausted. Returns (runID, childID).
func ca1098ParkedVerdictWedge(t *testing.T, svc *InteractiveService, runID string, pk ProviderKey) string {
	t.Helper()
	edges, nodes := ca1098VerdictTopology()
	setRun198699Topology(t, svc, runID, edges, nodes, "synthesis")

	childID := runID + "-rev"
	svc.mu.Lock()
	// Same-provider children inherit the parent's ambient account stamp —
	// without it startTurn rejects the re-drive as provider_account_changed.
	accountID := svc.runs[runID].providerAccountID
	svc.runs[childID] = &interactiveRun{
		id:                   childID,
		parentRunID:          runID,
		label:                "reviewer",
		role:                 "reviewer",
		agentName:            "reviewer",
		status:               RunStatusCompleted,
		agentStatus:          string(RunStatusCompleted),
		providerKey:          pk,
		providerAccountID:    accountID,
		flowCohortId:         "flow-auto-validate-round-0",
		stepID:               "reviewer",
		verdictRepromptCount: 2,
		subs:                 map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(runID, childID)

	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = "escalate"
		st.GateReason = ca1098MissingVerdictGateReason
		return st
	})
	svc.setFlowStepStatus(context.Background(), runID, "synthesis", StepStatusWaitingUserApr)
	return childID
}

// TestCA1098MissingVerdictContinueRedrivesReviewer locks the remediation
// routing: Continue on a missing-verdict escalate park must re-drive the
// deficient reviewer member — never re-prompt the synthesis hub, which
// structurally cannot produce the member's verdict.
//
// The reprompt-budget reset is asserted behaviorally: the member enters the
// wedge with verdictRepromptCount already at cap (2), so a settle-time CP-67
// reprompt ("Your previous turn ended…") can ONLY fire if the user-driven
// retry reset the budget — without the reset the member joins verdict-less
// and the hub re-parks, the exact live wedge.
func TestCA1098MissingVerdictContinueRedrivesReviewer(t *testing.T) {
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
			childID := ca1098ParkedVerdictWedge(t, svc, parent.RunID, pk)

			if _, err := svc.resumeFlowWithFeedback(parent.RunID, "continue"); err != nil {
				t.Fatalf("%s: resumeFlowWithFeedback: %v", pk, err)
			}

			deadline := time.Now().Add(10 * time.Second)
			redriven, reprompted := false, false
			for !(redriven && reprompted) {
				select {
				case req := <-ch:
					if req.RunID != childID {
						if req.RunID == parent.RunID && !redriven {
							t.Fatalf("%s: hub reinvoke fired before any member re-drive (prompt=%q) — the wedge", pk, truncate1098(req.Prompt, 120))
						}
						continue
					}
					if strings.Contains(req.Prompt, "machine verdict was missing or not approved") {
						redriven = true
						continue
					}
					if strings.Contains(req.Prompt, "Your previous turn ended without the required submit_review_outcome") {
						if !redriven {
							t.Fatalf("%s: settle-time reprompt fired before the CA-1098 re-drive", pk)
						}
						reprompted = true // only reachable with a reset budget
						continue
					}
					t.Fatalf("%s: unexpected reviewer prompt: %q", pk, truncate1098(req.Prompt, 200))
				case <-time.After(50 * time.Millisecond):
					if time.Now().After(deadline) {
						t.Fatalf("%s: Continue never re-dispatched the deficient reviewer child (redriven=%v reprompted=%v)", pk, redriven, reprompted)
					}
				}
			}
		})
	}
}

// TestCA1098HubTurnNeverPrecedesMemberRedrive is the ordering leg of the
// wedge: the failure shape was a hub re-prompt fired with NO member re-drive
// in front of it. After the fix the only legitimate parent-hub turn is the
// cohort re-join's reinvoke — which can only follow the member's re-driven
// turn. A parent-hub request arriving first (or with no member request at
// all) is the bug.
func TestCA1098HubTurnNeverPrecedesMemberRedrive(t *testing.T) {
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
			runID := parent.RunID
			childID := ca1098ParkedVerdictWedge(t, svc, runID, pk)

			if _, err := svc.resumeFlowWithFeedback(runID, "continue"); err != nil {
				t.Fatalf("%s: resumeFlowWithFeedback: %v", pk, err)
			}

			deadline := time.Now().Add(10 * time.Second)
			childSeen := false
			for {
				select {
				case req := <-ch:
					if req.RunID == childID {
						childSeen = true
						continue
					}
					if req.RunID == runID && !childSeen {
						t.Fatalf("%s: generic hub reinvoke fired with no member re-drive first (prompt=%q) — the wedge", pk, truncate1098(req.Prompt, 120))
					}
					if req.RunID == runID && childSeen {
						return // member re-drive → cohort rejoin → hub: correct order
					}
				case <-time.After(50 * time.Millisecond):
					if time.Now().After(deadline) {
						t.Fatalf("%s: neither member re-drive nor hub turn observed after Continue", pk)
					}
				}
			}
		})
	}
}

// TestCA1098UnrelatedEscalateKeepsGenericResume pins the boundary: an
// escalate park whose gate reason is NOT the missing/deficient-verdict
// family must keep the existing generic hub-reinvoke resume (no member
// re-drive hijack).
func TestCA1098UnrelatedEscalateKeepsGenericResume(t *testing.T) {
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
			runID := parent.RunID
			childID := ca1098ParkedVerdictWedge(t, svc, runID, pk)
			// Repark on an unrelated escalate reason — the missing-verdict
			// routing must NOT fire here.
			svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
				st.Status = "blocked"
				st.BlockReason = "escalate"
				st.GateReason = "audit found uncommitted changes"
				return st
			})

			if _, err := svc.resumeFlowWithFeedback(runID, "continue"); err != nil {
				t.Fatalf("%s: resumeFlowWithFeedback: %v", pk, err)
			}

			svc.mu.Lock()
			status := svc.runs[childID].status
			svc.mu.Unlock()
			if status == RunStatusRunning {
				t.Fatalf("%s: unrelated escalate must not re-drive the reviewer child", pk)
			}
		})
	}
}

func truncate1098(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
