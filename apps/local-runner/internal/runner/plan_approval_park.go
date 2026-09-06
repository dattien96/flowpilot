package runner

import (
	"context"
	"fmt"
	"strings"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
)

// Task-325: conditional plan-approval park.
//
// A plan loop that already churned (plan_writer re-entered >= 1 — the AI
// reviewer bounced the plan at least once) parks for human read + approve
// after plan_synthesis approves and BEFORE anything freezes. Clean
// first-pass plans never park (unattended runs keep working).
//
// Reuses only proven machinery: explicit WAITING step stamp (BUG-244/233
// ordering), mutateLoop blocked (any BlockReason exempts the hub watchdog),
// parkFlowForAwaitingUser, emit + persist, and POST agent-loop/continue
// feedback for resume. No new edge, no new node type.

const (
	// planApprovalBlockReason is the LoopState.BlockReason for a parked
	// plan approval. New value, schemaless strings — the hub watchdog
	// early-returns on ANY blocked reason, and both clients render default
	// Continue/Stop chips (Allow only on drift).
	planApprovalBlockReason = "plan_approval"
	planSynthesisNodeID     = "plan_synthesis"
	planWriterNodeID        = "plan_writer"
	planFreezeNodeID        = "preflight_contract_freeze"
)

// cachePreflightDraftLocked caches a parseable preflight draft on the parent
// run while the child turn result is still alive in-session (BUG-360).
// Post-restart the transient scout child is gone and freeze would otherwise
// strict-parse prose and escalate in a dead-end loop. Parse-gated with the
// same call freeze uses. Last parseable wins; a scout-labeled completion
// with unparseable output clears a stale stash so a re-run scout failure
// fails closed (escalate) instead of freezing on outdated scope. Returns
// true when the cache was written. Caller must hold s.mu.
func (s *InteractiveService) cachePreflightDraftLocked(rs *interactiveRun, finalMsg string) bool {
	if s == nil || rs == nil || strings.TrimSpace(rs.parentRunID) == "" {
		return false
	}
	parent := s.runs[rs.parentRunID]
	if parent == nil {
		return false
	}
	msg := strings.TrimSpace(finalMsg)
	if msg == "" {
		return false
	}
	if _, err := changecontract.ParsePreflightDraft(msg); err == nil {
		parent.preflightDraftResult = msg
		return true
	}
	if strings.EqualFold(strings.TrimSpace(rs.label), "preflight_contract_plan") {
		parent.preflightDraftResult = ""
	}
	return false
}

// planLoopChurned reports whether the plan_writer child of parentRunID was
// re-entered at least once, plus its max activationSeq (re-entry count).
// activationSeq is zero-based (0 = first activation, ++ per reinvoke in
// reinvokeMatchingFlowChild), so churned <=> max >= 1. Loop-specific: code
// loop churn (implement re-entries, shared LoopState.Round bumps) never
// counts here.
func (s *InteractiveService) planLoopChurned(parentRunID string) (bool, int) {
	if s == nil || strings.TrimSpace(parentRunID) == "" {
		return false, 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	maxSeq := -1
	for _, r := range s.runs {
		if r == nil || r.parentRunID != parentRunID || r.label != planWriterNodeID {
			continue
		}
		if r.activationSeq > maxSeq {
			maxSeq = r.activationSeq
		}
	}
	if maxSeq < 1 {
		return false, 0
	}
	return true, maxSeq
}

// parkPlanForApproval parks parentRunID after an approved-but-churned plan.
// Caller resolved hubID/target already; insertion sits between edge
// resolution and successor dispatch so freezing never starts (Task-325 D-2).
// Mirrors the escalate ordering: step first, then loop, then park, then
// emit + persist.
func (s *InteractiveService) parkPlanForApproval(parentRunID string, writerRounds int) (FlowControlResult, bool) {
	// 1. Step first (BUG-244/233): explicit plan_synthesis WAITING — never
	// the setFlowStepAwaitingUser first-match fallback, which could stamp
	// the wrong hub on dual-hub flows.
	s.setFlowStepStatus(context.Background(), parentRunID, planSynthesisNodeID, StepStatusWaitingUserApr)
	// 2. Loop blocked with the new reason. GateReason names the plan doc
	// when the BUG-357 recorder already captured it.
	gateReason := fmt.Sprintf("Plan revised after review (writer round %d) — read the plan doc before it freezes. Approve to continue, or send feedback to revise the plan.", writerRounds)
	if recs, ok := s.runArtifacts.forRun(parentRunID); ok {
		for _, a := range recs {
			if strings.TrimSpace(a.NodeID) == planWriterNodeID && strings.TrimSpace(a.Path) != "" {
				gateReason += " Plan: " + strings.TrimSpace(a.Path)
				break
			}
		}
	}
	snap := s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = planApprovalBlockReason
		st.GateReason = gateReason
		return st
	})
	// 3. Freeze the flow behind the card (same as escalate).
	s.parkFlowForAwaitingUser(parentRunID)
	s.emitAgentGraph(parentRunID, snap)
	go s.persistParentSession(parentRunID)
	s.flowDiagLog(parentRunID, "plan_approval_park", "churned plan parked for human approval",
		"writer_rounds", writerRounds,
		"round", snap.LoopState.Round,
		"cap", effectiveCap(snap.LoopState),
	)
	// 4. Stamp the one-decision guard like the generic-successor branch so
	// BUG-226 never re-escalates the approving turn as decision-less.
	s.mu.Lock()
	if rs := s.runs[parentRunID]; rs != nil && rs.currentTurnID != "" {
		rs.lastFlowControlTurnID = rs.currentTurnID
	}
	s.mu.Unlock()
	st := s.agentOrchestrator.loopStateFor(parentRunID)
	return FlowControlResult{Status: "blocked", Round: st.Round, Cap: effectiveCap(st), OpenIssues: st.OpenIssues, NextAction: "awaiting_user"}, true
}

// resumePlanApproval handles Continue off a plan_approval park. Approve
// (empty feedback, or the legacy "continue" token both TUI surfaces send
// for bare /continue and the Retry chip) advances FORWARD to freeze — stock
// resume would retry the writer, which is wrong here: the park is a gate,
// not a failure. Genuine human feedback re-enters plan_writer with the note.
// A literal human "continue" revision note is indistinguishable from the
// legacy token and intentionally counts as approve (recoverable either way).
func (s *InteractiveService) resumePlanApproval(parentRunID, feedback string, snap AgentGraphSnapshot) (AgentGraphSnapshot, bool) {
	s.mu.Lock()
	var edges []agentpack.FlowEdge
	var nodes []agentpack.FlowNode
	if rs := s.runs[parentRunID]; rs != nil {
		edges = rs.activeFlowEdges
		nodes = rs.activeFlowNodes
	}
	s.mu.Unlock()

	if fb := strings.TrimSpace(feedback); fb == "" || strings.EqualFold(fb, "continue") {
		// Approve: drive the resolved done-edge directly. Never reinvoke the
		// hub to re-decide — its done would hit the still-churned park again.
		s.advanceToNextInlineOrDelegate(context.Background(), parentRunID, edges, nodes, planSynthesisNodeID, "done", "")
		s.emitAgentGraph(parentRunID, snap)
		s.flowDiagLog(parentRunID, "plan_approval_approve", "human approved churned plan, advancing to freeze")
		return snap, true
	}
	node, ok := findFlowNode(nodes, planWriterNodeID)
	if !ok {
		return snap, false
	}
	resumePrompt := strings.TrimSpace(feedback) + "\n\n---\n\n[flow-engine] The plan was approved pending your revisions below. Revise the plan document with this guidance, then resubmit."
	resumePrompt = composeFlowNodeAgentPrompt(s.workspaceCwdFor(parentRunID), resumePrompt, node)
	if !s.reinvokeMatchingFlowChild(parentRunID, resumePrompt, func(child *interactiveRun) bool {
		return child.label == planWriterNodeID
	}) {
		return snap, false
	}
	s.setFlowStepStatus(context.Background(), parentRunID, planWriterNodeID, StepStatusRunning)
	s.emitAgentGraph(parentRunID, snap)
	s.flowDiagLog(parentRunID, "plan_approval_feedback", "human feedback re-entered plan_writer")
	return snap, true
}
