package runner

import (
	"context"
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
	var zombieDebates []string
	type healWait struct {
		runID string
	}
	var heals []healWait
	type deadDispatchRun struct {
		runID string
		nodes []agentpack.FlowNode
	}
	var deadDispatchRuns []deadDispatchRun

	now := time.Now().UTC()
	s.mu.Lock()
	for id, rs := range s.runs {
		if rs == nil {
			continue
		}
		// (5) zombie owner-debate claim — parkedNodes still held while the
		// debate can no longer resolve itself (BUG-589). The dedicated helper
		// re-checks liveness/shape under its own guards before acting.
		if len(rs.vibeParkedNodes) > 0 {
			zombieDebates = append(zombieDebates, id)
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
		// (5) live run-183756 (dead dispatch): a step stamped RUNNING whose
		// dispatch never materialized — no turn, no leg, no driver. coder was
		// RUNNING >5min after a batch reset with zero ACP traffic; the
		// synthesis hub reinvoke was consumed with no turn either. Only
		// collect when the run itself is live and every run-level driver is
		// quiet — an in-flight turn/settle/reinvoke owns the window.
		if rs.parentRunID == "" && rs.flowEngineDriven && len(rs.activeFlowNodes) > 0 &&
			rs.status == RunStatusRunning &&
			!rs.turnInFlight && !rs.pendingFlowGateSettle && !rs.pendingHubReinvoke &&
			!rs.reinvokeInFlight && !rs.vibeDebateMountInFlight &&
			!gateCancelLive(rs.postTurnGateStartedAt, rs.postTurnGateCancel) {
			deadDispatchRuns = append(deadDispatchRuns, deadDispatchRun{
				runID: id,
				nodes: append([]agentpack.FlowNode(nil), rs.activeFlowNodes...),
			})
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
	for _, id := range zombieDebates {
		s.maybeResolveZombieVibeDebate(id)
	}
	for _, c := range deadDispatchRuns {
		s.maybeRedriveDeadDispatchedSteps(c.runID, c.nodes)
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

// flowStepDeadDispatchBound is how old a RUNNING step stamp may be with no
// live work before it counts as a dead dispatch. The normal
// stamp→spawn→turn-start window is seconds; this mirrors the established
// vibeDebateTriggerWedgeBound for the same wedge class.
var flowStepDeadDispatchBound = vibeDebateTriggerWedgeBound

// maybeRedriveDeadDispatchedSteps heals the "node RUNNING, nothing executing"
// wedge (live run-183756: coder stamped RUNNING after a batch reset with zero
// provider traffic; synthesis hub reinvoke consumed, no turn row). A step can
// only be legitimately RUNNING while a leg, a hub turn, or a run-level driver
// owns it — the caller already filtered run-level drivers, so an aged RUNNING
// step that survives the checks below is a lost dispatch and gets re-driven
// through the same paths a fresh advance would use.
//
// Skips, in order:
//   - no step row / not RUNNING / fresh stamp (dispatch still in flight)
//   - any non-terminal child leg carrying the label — running, waiting_*,
//     idle/starting all count (a waiting_user_approval leg is a visible park
//     that owns its own resume, never residue)
//   - a persisted Completed leg — the leg finished; the settle/advance gap
//     that left the stamp is BUG-1197's class, not a dead dispatch
//
// Re-drive: hub.inline nodes re-invoke the hub turn; delegate nodes get a
// matching-leg reinvoke first (preserves leg lineage) and a fresh
// spawnChildRun otherwise — BUG-1194 cohort rebind inside covers open seats.
func (s *InteractiveService) maybeRedriveDeadDispatchedSteps(parentRunID string, nodes []agentpack.FlowNode) {
	if s == nil || s.workflowStore == nil || len(nodes) == 0 {
		return
	}
	steps, err := s.workflowStore.LoadRunSteps(context.Background(), parentRunID)
	if err != nil {
		return
	}
	byNode := make(map[string]RuntimeWorkflowStep, len(steps))
	for _, st := range steps {
		if st.NodeID != "" {
			byNode[st.NodeID] = st
		}
	}
	hubRedriven := false
	for _, node := range nodes {
		st, ok := byNode[strings.TrimSpace(node.ID)]
		if !ok || st.Status != StepStatusRunning {
			continue
		}
		started, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(st.StartedAt))
		if err != nil || time.Since(started) < flowStepDeadDispatchBound {
			continue
		}
		if s.vibeNodeHasLiveWork(parentRunID, node.ID) ||
			s.deadDispatchNonTerminalChildExists(parentRunID, node.ID) ||
			s.persistedCompletedChildExists(parentRunID, node.ID) {
			continue
		}
		if canonical, ok := agentpack.NormalizeBehaviorID(node.Behavior); ok && canonical == "hub.inline" {
			if hubRedriven {
				continue
			}
			s.flowDiagLog(parentRunID, "dead_dispatch_hub_redrive",
				"hub node RUNNING with no turn ever dispatched; re-driving hub reinvoke",
				"node_id", node.ID)
			s.maybeAutoReinvokeHubWithPrompt(parentRunID,
				"[flow-engine] Node \""+node.ID+"\" is stamped RUNNING but its hub turn was never dispatched — the reinvoke/dispatch was lost. Run that node's evaluation turn now: read the current step state and emit its flow_control outcome.")
			hubRedriven = true
			continue
		}
		agentName := flowNodeAgentName(node)
		if agentName == "" {
			s.flowDiagLog(parentRunID, "dead_dispatch_unhandled_kind",
				"step RUNNING with no live work but no re-drive path for its behavior",
				"node_id", node.ID, "behavior", node.Behavior)
			continue
		}
		s.redriveDeadDelegateLeg(parentRunID, node, agentName)
	}
}

// deadDispatchNonTerminalChildExists reports whether any child leg of
// parentRunID carries label in a non-terminal status. Broader than
// vibeNodeHasLiveWork: a waiting_user_approval leg is a carded park that owns
// its resume — counting it live keeps the sweep from double-dispatching it.
func (s *InteractiveService) deadDispatchNonTerminalChildExists(parentRunID, label string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, child := range s.runs {
		if child == nil || child.parentRunID != parentRunID || child.label != label {
			continue
		}
		switch child.status {
		case RunStatusCompleted, RunStatusFailed, RunStatusCancelled:
		default:
			return true
		}
	}
	return false
}

// redriveDeadDelegateLeg re-drives a delegate node whose dispatch was lost:
// reinvoke a matching prior leg when one exists (failed legs retry in place),
// else spawn a fresh leg labelled node.ID. Mirrors the failed-delegate
// Continue respawn contract — same prompt composition and contract inject.
func (s *InteractiveService) redriveDeadDelegateLeg(parentRunID string, node agentpack.FlowNode, agentName string) {
	cwd := s.workspaceCwdFor(parentRunID)
	prompt := composeFlowNodeAgentPrompt(cwd,
		"[flow-engine] This node is stamped RUNNING but no turn ever ran — its dispatch was lost. Do the node's work now and report your outcome.",
		node)
	prompt = appendResolvedVibeTemplatedInputs(prompt, node, s.vibeResolvedSlicerSource(parentRunID))
	prompt = appendChangeContractIfAnyWithSecret(cwd, parentRunID, prompt, s.markerSecret)
	nodeID := node.ID
	if s.reinvokeMatchingFlowChild(parentRunID, prompt, func(child *interactiveRun) bool {
		if child.status == RunStatusCancelled {
			return false
		}
		return child.label == nodeID ||
			(strings.TrimSpace(child.label) == "" && strings.EqualFold(strings.TrimSpace(child.agentName), agentName))
	}) {
		s.flowDiagLog(parentRunID, "dead_dispatch_leg_reinvoked",
			"delegate step RUNNING with dead dispatch; reinvoked matching leg",
			"node_id", nodeID)
		return
	}
	agentDef, _ := resolvePackAgentDefinition(agentName)
	if _, err := s.spawnChildRun(context.Background(), parentRunID, SpawnAgentInput{
		Agent:            agentName,
		Prompt:           prompt,
		Wait:             false,
		Label:            nodeID,
		AutoOrchestrate:  true,
		AgentDefOverride: agentDef,
		Model:            s.delegateSpawnModel(context.Background(), parentRunID, node),
	}); err != nil {
		s.flowDiagLog(parentRunID, "dead_dispatch_leg_spawn_failed",
			"delegate step RUNNING with dead dispatch; fresh leg spawn failed",
			"node_id", nodeID, "error", err.Error())
		return
	}
	s.flowDiagLog(parentRunID, "dead_dispatch_leg_spawned",
		"delegate step RUNNING with dead dispatch; spawned fresh leg",
		"node_id", nodeID)
}
