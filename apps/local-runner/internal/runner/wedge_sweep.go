package runner

import (
	"strings"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// wedgedWaitGraceBounds how long a run may carry a waiting_* status without
// any live card id before the sweep heals it. The emit-then-stamp window is
// sub-second (both writes happen under s.mu in the same code path), so a
// minute of silence is unambiguous orphan evidence, not a race.
var wedgedWaitGrace = 60 * time.Second

// sweepWedgedFlowWork converges live-run state that armed an intent or wait
// but lost its driver — the in-session counterpart of the reconstruct-time
// heals. Each class was observed wedged in live run-100368 until an operator
// Continue poked it:
//
//  1. BUG-571: pendingFlowGateSettle armed with no live gate eval and no
//     claim → resumePendingFlowGate (single-flight; safe to re-enter).
//  2. BUG-572: a PENDING artifact.audit_draft node whose forward-done
//     predecessors all settled while the defer's open-cohort/running-step
//     reasons are gone → re-dispatch through the normal inline path.
//  3. BUG-581: waiting_question / waiting_approval with no pending card id
//     past the grace → heal to running (mirrors the reconstruct heal at
//     interactive_resume/rehydrate — BUG-470 precedent).
//  4. BUG-579: waiting_user_approval / waiting_* whose resume-approval
//     reconciliation id points at a card record that does not exist in
//     approvals/questions — an orphaned wait nobody can answer; heal to
//     running so the loop can re-drive or re-raise a real card.
func (s *InteractiveService) sweepWedgedFlowWork() {
	if s == nil {
		return
	}
	type deferredAudit struct {
		runID string
		edges []agentpack.FlowEdge
		nodes []agentpack.FlowNode
		node  agentpack.FlowNode
	}
	var gateRuns []string
	var deferred []deferredAudit
	type healWait struct {
		runID string
	}
	var heals []healWait

	now := time.Now().UTC()
	s.mu.Lock()
	for id, rs := range s.runs {
		if rs == nil {
			continue
		}
		// (1) armed settle, driverless.
		if rs.pendingFlowGateSettle && rs.gateClaimID == "" &&
			!gateCancelLive(rs.postTurnGateStartedAt, rs.postTurnGateCancel) {
			gateRuns = append(gateRuns, id)
		}
		// (3)+(4) orphaned waits — never heal a wait backed by a live card id
		// or a mounted decision surface.
		cardBacked := rs.pendingApprovalID != "" || rs.pendingQuestionID != "" ||
			rs.decisionCard != nil
		if !cardBacked && wedgedWaitStaleFor(rs, now) {
			waitOrphan := rs.status == RunStatusWaitingQuestion ||
				rs.status == RunStatusWaitingApproval
			if rs.status == RunStatusWaitingUserApr {
				// waiting_user_approval on a child is a legitimate park while
				// the parent loop is blocked awaiting a human decision — the
				// parent carries the decision surface, healing the child would
				// be a silent auto-allow. Only heal when the park has no owner:
				// no parent, or a parent loop that is not blocked.
				owner := true
				if rs.parentRunID != "" && s.agentOrchestrator != nil {
					owner = s.agentOrchestrator.loopStateFor(rs.parentRunID).Status == "blocked"
				}
				if !owner {
					waitOrphan = true
				}
			}
			if waitOrphan {
				heals = append(heals, healWait{runID: id})
			}
		}
		// A waiting_* run whose resume-reconciliation id references a card
		// record that does not exist is orphaned even while waiting (the card
		// can never be answered — nobody can see it).
		if rid := strings.TrimSpace(rs.pendingResumeApprovalID); rid != "" &&
			now.Sub(rs.lastProviderEventAt) >= wedgedWaitGrace {
			_, hasA := s.approvals[rid]
			_, hasQ := s.questions[rid]
			if !hasA && !hasQ {
				heals = append(heals, healWait{runID: id})
			}
		}
		// (2) deferred audit — collect under lock, dispatch outside.
		if rs.parentRunID == "" && rs.flowEngineDriven && len(rs.activeFlowNodes) > 0 {
			for _, n := range rs.activeFlowNodes {
				canonical, ok := agentpack.NormalizeBehaviorID(n.Behavior)
				if !ok || canonical != "artifact.audit_draft" {
					continue
				}
				deferred = append(deferred, deferredAudit{
					runID: id,
					edges: append([]agentpack.FlowEdge(nil), rs.activeFlowEdges...),
					nodes: append([]agentpack.FlowNode(nil), rs.activeFlowNodes...),
					node:  n,
				})
			}
		}
	}
	s.mu.Unlock()

	for _, id := range gateRuns {
		go s.resumePendingFlowGate(id)
	}
	for _, h := range heals {
		s.healOrphanedWait(h.runID)
	}
	for _, d := range deferred {
		s.maybeRedispatchDeferredAudit(d.runID, d.edges, d.nodes, d.node)
	}
}

// wedgedWaitStaleFor is split out so the grace window stays test-tunable.
func wedgedWaitStaleFor(rs *interactiveRun, now time.Time) bool {
	return rs.lastProviderEventAt.IsZero() || now.Sub(rs.lastProviderEventAt) >= wedgedWaitGrace
}

// healOrphanedWait returns a waiting_* run to running when no card/id backs
// the wait (BUG-579/581): the run parked on a surface nobody can answer.
// Mirrors the BUG-470 reconstruct heal — a real wait is always backed by a
// live card, so anything reaching here is a phantom/orphan.
func (s *InteractiveService) healOrphanedWait(runID string) {
	s.mu.Lock()
	rs := s.runs[runID]
	if rs == nil {
		s.mu.Unlock()
		return
	}
	// A dangling resume-reconciliation id is bookkeeping-safe to clear even
	// while a real card backs the wait — the card it references no longer
	// exists, so the reconcile can never fire.
	orphanResume := false
	if rid := strings.TrimSpace(rs.pendingResumeApprovalID); rid != "" {
		_, hasA := s.approvals[rid]
		_, hasQ := s.questions[rid]
		orphanResume = !hasA && !hasQ
		if orphanResume {
			rs.pendingResumeApprovalID = ""
			rs.pendingResumeDecision = ""
			rs.pendingResumeQuestionChoices = nil
		}
	}
	// Re-check under lock: a card may have been armed between the scan and now.
	if rs.pendingApprovalID != "" || rs.pendingQuestionID != "" || rs.decisionCard != nil {
		if orphanResume {
			snap := sessionStateOf(rs)
			s.mu.Unlock()
			_ = s.persistProviderSession(snap)
			return
		}
		s.mu.Unlock()
		return
	}
	waitOrphan := rs.status == RunStatusWaitingQuestion ||
		rs.status == RunStatusWaitingApproval
	if rs.status == RunStatusWaitingUserApr && rs.parentRunID != "" && s.agentOrchestrator != nil {
		// Same owner rule as the scan: a child parked while the parent loop is
		// blocked is awaiting the parent's human-decision surface — not orphan.
		if s.agentOrchestrator.loopStateFor(rs.parentRunID).Status != "blocked" {
			waitOrphan = true
		}
	} else if rs.status == RunStatusWaitingUserApr {
		waitOrphan = true // no parent loop to own the park
	}
	if !waitOrphan && !orphanResume {
		s.mu.Unlock()
		return
	}
	prev := rs.status
	rs.pendingResumeApprovalID = ""
	rs.pendingResumeDecision = ""
	rs.pendingResumeQuestionChoices = nil
	if waitOrphan {
		rs.status = RunStatusRunning
		rs.agentStatus = string(RunStatusRunning)
	}
	touchHubProgressLocked(rs)
	snap := sessionStateOf(rs)
	s.mu.Unlock()
	_ = s.persistProviderSession(snap)
	s.flowDiagLog(runID, "orphaned_wait_healed",
		"waiting status had no live card; healed to running",
		"prev_status", string(prev))
	s.notifyTurnIdle(runID)
}

// maybeRedispatchDeferredAudit re-drives an audit node left PENDING by the
// BUG-560/561 defer once its reason is gone (BUG-572, live run-100368): the
// defer promised "the join → hub → done-edge path re-dispatches", but when
// the upstream synthesis done-edge was already consumed the node sat PENDING
// forever — evidence DONE, audit never ran, run marked running. Re-dispatch
// only when the defer's own conditions are gone: no open cohort, no other
// RUNNING sprint step, and every forward-done predecessor terminal.
func (s *InteractiveService) maybeRedispatchDeferredAudit(parentRunID string, edges []agentpack.FlowEdge, nodes []agentpack.FlowNode, node agentpack.FlowNode, resultMessage ...string) {
	if s.lookupFlowStepStatus(parentRunID, node.ID) != StepStatusPending {
		return // already dispatched or settled
	}
	if s.agentOrchestrator != nil && s.agentOrchestrator.hasOpenCohort(parentRunID) {
		return
	}
	if s.hasRunningSprintStep(parentRunID, node.ID) {
		return
	}
	for _, predID := range flowForwardDonePredecessors(edges, node.ID) {
		switch s.lookupFlowStepStatus(parentRunID, predID) {
		case StepStatusDone, StepStatusSkipped:
		default:
			return // an upstream predecessor is still owed work
		}
	}
	if !s.loopIsAdvancing(parentRunID) {
		return
	}
	s.flowDiagLog(parentRunID, "deferred_audit_redispatched",
		"audit node PENDING with settled predecessors and resolved cohort; re-dispatching",
		"node_id", node.ID)
	ctx := s.flowInlineContext(parentRunID)
	msg := ""
	if len(resultMessage) > 0 {
		msg = resultMessage[0]
	}
	s.runAuditNode(ctx, parentRunID, edges, nodes, node, msg)
}
