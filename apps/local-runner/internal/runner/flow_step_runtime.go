package runner

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"flowpilot-runner/internal/agentpack"
)

// flow_step_runtime.go wires the CP-42 flow executor to the step-runtime
// timeline (BUG-174). Before this, the two were disjoint: the executor spawned
// the coder/reviewer cohort and drove the loop, while the step timeline was
// seeded once and then bulk-completed by PlanWorkflowProgress after a single
// hub turn — so a Flow-Mode workflow-picker run showed the step stuck at 1 and
// then every step flipped to DONE at once, out of causal order. These helpers
// let the executor emit real per-node transitions instead.
//
// Everything here is scoped to flowEngineDriven runs and is best-effort: a
// step-timeline write must never break the flow's actual orchestration, so
// failures are logged, not propagated. On the local desktop runner the
// workflowStore is the in-memory fakeWorkflowStore (which implements
// workflowRunSeeder), so reseeding works; a Supabase-backed step store that
// doesn't implement the seeder simply keeps its catalog-seeded steps.

// LoopStatusTournamentEscalation (CP-65 P-4, Task-371 T-1) marks a run whose
// review/debate loop hit its cap or stalled and was rescued into a
// tournament instead of parking failed/stopped: a tournament-harness child
// run was dispatched with the stuck run's intent and contract, and the
// parent waits for its verdict. It is a non-terminal waiting state like
// "blocked" — never overwrite it with terminal/rejected transitions, and
// stall sweeps must leave it alone (the rescue is already in flight).
const LoopStatusTournamentEscalation = "tournament_escalation"

// isFlowEngineDriven reports whether runID's step timeline is owned by the flow
// executor rather than the legacy bulk planner. Takes s.mu — do not call while
// already holding the lock; use flowEngineDrivenUnlocked instead.
func (s *InteractiveService) isFlowEngineDriven(runID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.flowEngineDrivenUnlocked(runID)
}

// flowEngineDrivenUnlocked reads flowEngineDriven without taking s.mu.
// Caller MUST already hold s.mu.
func (s *InteractiveService) flowEngineDrivenUnlocked(runID string) bool {
	rs := s.runs[runID]
	return rs != nil && rs.flowEngineDriven
}

// markFlowEngineDriven flags runID so startTurn skips the bulk Progress planner
// and the executor owns its step transitions. Called from handleStartTurn once
// a workflow-picker launch has resolved to a flow-engine flow.
func (s *InteractiveService) markFlowEngineDriven(runID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rs := s.runs[runID]; rs != nil {
		rs.flowEngineDriven = true
	}
}

// activeFlowNodesFor returns a copy of parentRunID's tracked flow nodes, or nil.
func (s *InteractiveService) activeFlowNodesFor(parentRunID string) []agentpack.FlowNode {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rs := s.runs[parentRunID]; rs != nil {
		return append([]agentpack.FlowNode(nil), rs.activeFlowNodes...)
	}
	return nil
}

// activeFlowEdgesFor mirrors activeFlowNodesFor for the run's edge list.
func (s *InteractiveService) activeFlowEdgesFor(parentRunID string) []agentpack.FlowEdge {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rs := s.runs[parentRunID]; rs != nil {
		return append([]agentpack.FlowEdge(nil), rs.activeFlowEdges...)
	}
	return nil
}

// activeHubNodeIDFor returns the run's tracked active hub-driven node id
// (Task-235 / CP-58 Task-304), or "" when none is tracked. Dual-hub harness
// flows rely on this being set (cohort-join persist) so a hub's
// flow_control("continue") resolves against ITS loop's back-edge, not
// first-match.
func (s *InteractiveService) activeHubNodeIDFor(parentRunID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if rs := s.runs[parentRunID]; rs != nil {
		return rs.activeHubNodeID
	}
	return ""
}

// reseedFlowStepRuntime replaces parentRunID's step list with one row per flow
// node so the timeline reflects the flow's real topology (coder → reviewers →
// synthesis) with correct node identities, instead of the generic catalog
// steps a workflow-picker launch seeds. Each step's ID equals its node id, so
// setFlowStepStatus can transition it directly by node id. No-op unless the
// workflowStore supports seeding (see file header).
func (s *InteractiveService) reseedFlowStepRuntime(parentRunID string, nodes []agentpack.FlowNode) {
	seeder, ok := s.workflowStore.(workflowRunSeeder)
	if !ok || len(nodes) == 0 {
		return
	}
	seeder.seed(parentRunID, s.flowStepRowsFromNodes(context.Background(), parentRunID, nodes, StepStatusPending, ""))
}

// reseedFlowStepRuntimeForResume rebuilds runID's step list from persisted flow
// nodes after a server restart (BUG-178). The local runner keeps step-runtime
// state in memory, so a flow run's step list is gone on restart and its history
// timeline showed "No step-runtime data for this run yet". A completed run's
// nodes are restored as DONE (the flow finished); a non-completed (resumed or
// restart-cancelled) run's nodes are restored as PENDING, since per-step
// progress is not persisted. No-op if the store isn't a seeder or there are no
// nodes (a plain, non-flow run has none).
func (s *InteractiveService) reseedFlowStepRuntimeForResume(runID string, nodes []agentpack.FlowNode, completed bool) {
	status := StepStatusPending
	ts := ""
	if completed {
		status = StepStatusDone
		ts = time.Now().UTC().Format(time.RFC3339Nano)
	}
	s.seedFlowStepRuntimeRows(runID, s.flowStepRowsFromNodes(context.Background(), runID, nodes, status, ts))
}

func (s *InteractiveService) seedFlowStepRuntimeRows(runID string, rows []RuntimeWorkflowStep) {
	seeder, ok := s.workflowStore.(workflowRunSeeder)
	if !ok || len(rows) == 0 {
		return
	}
	seeder.seed(runID, rows)
}

// flowStepRowsFromNodes builds one step-runtime row per flow node (ID == NodeID
// so a transition can address it directly by node id). When status is DONE, ts
// stamps started/finished so the row reads as a settled step.
//
// BUG-228 display follow-up: each row's Provider/Model are resolved up front
// (resolveFlowNodeProviderModel — the node's own role, else the run's
// baseline) rather than left blank until the node is actually spawned. A
// step-timeline row for a still-PENDING node must show what it will actually
// run on, not the run's blanket baseline, once stampFlowNodePosture confirms
// it at spawn time — that later stamp is what actually took effect;
// resolving here too means a not-yet-spawned node shows the correct posture
// from the moment it is seeded, without waiting for it to start.
func (s *InteractiveService) flowStepRowsFromNodes(ctx context.Context, parentRunID string, nodes []agentpack.FlowNode, status RuntimeWorkflowStepStatus, ts string) []RuntimeWorkflowStep {
	steps := make([]RuntimeWorkflowStep, 0, len(nodes))
	for _, node := range nodes {
		behaviorID := ""
		if canonical, ok := agentpack.NormalizeBehaviorID(node.Behavior); ok {
			behaviorID = canonical
		}
		provider, model := s.resolveFlowNodeProviderModel(ctx, parentRunID, node)
		row := RuntimeWorkflowStep{
			ID:         node.ID,
			StepType:   node.ID,
			NodeID:     node.ID,
			BehaviorID: behaviorID,
			AgentRef:   flowNodeAgentName(node),
			Status:     status,
			Provider:   provider,
			Model:      model,
		}
		switch status {
		case StepStatusDone:
			row.StartedAt = ts
			row.FinishedAt = ts
		case StepStatusFailed, StepStatusSkipped, StepStatusCanceled:
			row.FinishedAt = ts
		}
		steps = append(steps, row)
	}
	return steps
}

// setFlowStepStatus transitions the flow-node step nodeID on parentRunID to
// status, stamping timestamps the way PlanWorkflowProgress does. Best-effort:
// an unknown node id no-ops (the fake store patches zero rows), and any error
// is logged rather than propagated.
//
// Call when s.mu is NOT held. From emitLocked / settleFlowChildTurnCompletedLocked
// (already under s.mu), use setFlowStepStatusLocked.
func (s *InteractiveService) setFlowStepStatus(ctx context.Context, parentRunID, nodeID string, status RuntimeWorkflowStepStatus) {
	s.setFlowStepStatusCore(ctx, parentRunID, nodeID, status, false /* muHeld */)
}

// setFlowStepStatusLocked is the same as setFlowStepStatus but for call sites
// that already hold s.mu (emitLocked settle path). Avoids re-locking for the
// flowEngineDriven check in the transition log.
func (s *InteractiveService) setFlowStepStatusLocked(ctx context.Context, parentRunID, nodeID string, status RuntimeWorkflowStepStatus) {
	s.setFlowStepStatusCore(ctx, parentRunID, nodeID, status, true /* muHeld */)
}

func (s *InteractiveService) setFlowStepStatusCore(ctx context.Context, parentRunID, nodeID string, status RuntimeWorkflowStepStatus, muHeld bool) {
	if nodeID == "" {
		return
	}
	s.flowDiagLog(parentRunID, "step_status_transition", "updating flow step status",
		"node_id", nodeID,
		"status", string(status),
	)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	patch := WorkflowStepPatch{Status: status}
	switch status {
	case StepStatusRunning:
		patch.StartedAt = strptr(now)
		patch.FinishedAt = strptr("")
	case StepStatusDone, StepStatusFailed, StepStatusCanceled, StepStatusSkipped:
		patch.FinishedAt = strptr(now)
	case StepStatusPending:
		patch.StartedAt = strptr("")
		patch.FinishedAt = strptr("")
	}
	if err := s.workflowStore.ApplyStepTransition(ctx, parentRunID, WorkflowStepTransition{
		StepID: nodeID,
		Patch:  patch,
	}); err != nil {
		log.Printf("[flow-step] set node %q -> %s on run %q failed: %v", nodeID, status, parentRunID, err)
		s.flowDiagLog(parentRunID, "step_status_transition_failed", "flow step status update failed",
			"node_id", nodeID,
			"status", string(status),
			"error", err.Error(),
		)
		return
	}
	// Task-239 / T-10: append best-effort transition log for flow-engine runs so
	// resume can replay settled status (I-17). Never blocks orchestration (T-5).
	s.appendStepTransitionLog(parentRunID, nodeID, string(status), "", "", muHeld)
}

// setFlowStepFailedWithReason is like setFlowStepStatus(FAILED) but also stores
// the failure's RejectionNote so the F2 timeline / step chat line can surface
// the real provider error instead of "(no detail from runner)".
// Additive — existing setFlowStepStatus call sites stay untouched.
func (s *InteractiveService) setFlowStepFailedWithReason(ctx context.Context, parentRunID, nodeID, reason string) {
	s.setFlowStepFailedWithReasonCore(ctx, parentRunID, nodeID, reason, false)
}
func (s *InteractiveService) setFlowStepFailedWithReasonLocked(ctx context.Context, parentRunID, nodeID, reason string) {
	s.setFlowStepFailedWithReasonCore(ctx, parentRunID, nodeID, reason, true)
}
func (s *InteractiveService) setFlowStepFailedWithReasonCore(ctx context.Context, parentRunID, nodeID, reason string, muHeld bool) {
	if nodeID == "" {
		return
	}
	reason = truncateDisplayField(strings.TrimSpace(reason), 500)
	s.flowDiagLog(parentRunID, "step_status_transition", "updating flow step status",
		"node_id", nodeID,
		"status", string(StepStatusFailed),
	)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	patch := WorkflowStepPatch{Status: StepStatusFailed, FinishedAt: strptr(now), RejectionNote: strptr(reason)}
	if err := s.workflowStore.ApplyStepTransition(ctx, parentRunID, WorkflowStepTransition{
		StepID: nodeID,
		Patch:  patch,
	}); err != nil {
		log.Printf("[flow-step] set node %q -> %s on run %q failed: %v", nodeID, StepStatusFailed, parentRunID, err)
		s.flowDiagLog(parentRunID, "step_status_transition_failed", "flow step status update failed",
			"node_id", nodeID,
			"status", string(StepStatusFailed),
			"error", err.Error(),
		)
		return
	}
	s.appendStepTransitionLog(parentRunID, nodeID, string(StepStatusFailed), "", "", muHeld)
}

// setFlowStepAwaitingUser transitions the flow's hub inline node (the
// control/synthesis node) to WAITING_USER_APPROVAL (BUG-231) when the loop
// pauses awaiting a human decision — escalate, or the round cap being
// reached — instead of leaving it RUNNING (which reads as a hang) or marking
// it FAILED (which reads as an error/terminal step). No-op for a run that
// isn't flow-engine-driven or has no hub inline node. Best-effort, like
// setFlowStepStatus.
func (s *InteractiveService) setFlowStepAwaitingUser(ctx context.Context, parentRunID string) {
	if !s.isFlowEngineDriven(parentRunID) {
		return
	}
	if hubID := hubInlineNodeID(s.activeFlowNodesFor(parentRunID)); hubID != "" {
		s.setFlowStepStatus(ctx, parentRunID, hubID, StepStatusWaitingUserApr)
		return
	}
	// Fallback for flows without hub.inline (rag-harness audit): mark the
	// RUNNING node (e.g. audit) as WAITING_USER so F2/Thinking stop. The hub
	// path above is review-loop's "synthesis"; rag-harness's escalate was
	// otherwise a no-op and left audit RUNNING (run-125458).
	// CA-623: if some step is already WAITING (e.g. freeze stamped first), do
	// not stamp a second one (audit) — prevents freeze→audit confusion.
	if steps, err := s.workflowStore.LoadRunSteps(ctx, parentRunID); err == nil {
		for _, st := range steps {
			if st.Status == StepStatusWaitingUserApr {
				return
			}
		}
		for _, st := range steps {
			if st.Status == StepStatusRunning {
				s.setFlowStepStatus(ctx, parentRunID, st.ID, StepStatusWaitingUserApr)
				return
			}
		}
		// No RUNNING found (e.g. already DONE/FAILED) — still try audit if present.
		for _, st := range steps {
			if st.ID == "audit" {
				s.setFlowStepStatus(ctx, parentRunID, st.ID, StepStatusWaitingUserApr)
				return
			}
		}
	}
}

// settleFlowChildStepAwaitingUserLocked stamps the parent flow step for a
// labeled child to WAITING_USER_APPROVAL when that child surfaces a permission
// or question gate (BUG-288 #22). Caller must hold s.mu (emitLocked path).
// No-op for root turns, unlabeled children, or non-flow-engine parents.
func (s *InteractiveService) settleFlowChildStepAwaitingUserLocked(rs *interactiveRun) {
	if rs == nil || rs.parentRunID == "" || strings.TrimSpace(rs.label) == "" {
		return
	}
	parent := s.runs[rs.parentRunID]
	if parent == nil || !parent.flowEngineDriven {
		return
	}
	s.setFlowStepStatusLocked(context.Background(), rs.parentRunID, rs.label, StepStatusWaitingUserApr)
}

// setFlowStepPosture stamps nodeID's OWN actually-resolved provider/model
// (BUG-228 display follow-up) so the step-timeline UI shows what that node
// really ran on. Without this, every flow-engine step row stayed blank
// (flowStepRowsFromNodes never sets Provider/Model, unlike the classic
// planner's step seeding) and the desktop fell back to displaying the run's
// single baseline posture for every row — masking a node's own resolved
// model even when spawnChildRun/resolveFlowNodeModel gave it one different
// from the run's baseline. Best-effort, like setFlowStepStatus: an unknown
// node id no-ops, and any error is logged rather than propagated.
func (s *InteractiveService) setFlowStepPosture(ctx context.Context, parentRunID, nodeID, provider, model string) {
	if nodeID == "" {
		return
	}
	s.flowDiagLog(parentRunID, "step_posture_transition", "updating flow step posture",
		"node_id", nodeID,
		"provider", provider,
		"model", model,
	)
	if err := s.workflowStore.ApplyStepTransition(ctx, parentRunID, WorkflowStepTransition{
		StepID: nodeID,
		Patch:  WorkflowStepPatch{Provider: strptr(provider), Model: strptr(model)},
	}); err != nil {
		log.Printf("[flow-step] set node %q posture provider=%q model=%q on run %q failed: %v", nodeID, provider, model, parentRunID, err)
		s.flowDiagLog(parentRunID, "step_posture_transition_failed", "flow step posture update failed",
			"node_id", nodeID,
			"provider", provider,
			"model", model,
			"error", err.Error(),
		)
		return
	}
	// Task-239: posture-only line (empty Status) so provider/model survive restart.
	// setFlowStepPosture is only called outside s.mu (spawn/reinvoke paths).
	s.appendStepTransitionLog(parentRunID, nodeID, "", provider, model, false /* muHeld */)
}

// appendStepTransitionLog writes one transition/posture line when the store
// implements StepTransitionLogStore and the run is flow-engine-driven (Task-239).
// Errors are log-warn only — step writes must never break orchestration.
//
// muHeld must be true when the caller already holds s.mu (emitLocked settle path);
// false otherwise. Do NOT use TryLock to guess ownership — that races when
// another goroutine holds the mutex.
func (s *InteractiveService) appendStepTransitionLog(parentRunID, nodeID, status, provider, model string, muHeld bool) {
	if parentRunID == "" || nodeID == "" {
		return
	}
	var driven bool
	if muHeld {
		driven = s.flowEngineDrivenUnlocked(parentRunID)
	} else {
		driven = s.isFlowEngineDriven(parentRunID)
	}
	if !driven {
		return
	}
	tlog, ok := s.workflowStore.(StepTransitionLogStore)
	if !ok {
		return
	}
	line := stepTransitionLine{
		RunID:    parentRunID,
		NodeID:   nodeID,
		Status:   status,
		Provider: provider,
		Model:    model,
		TS:       time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := tlog.AppendStepTransition(context.Background(), parentRunID, line); err != nil {
		log.Printf("[flow-step] append transition log run=%q node=%q status=%q: %v", parentRunID, nodeID, status, err)
		// BUG-288 P2-03: Task-239's contract is "every transition is persisted;
		// restore replays the log". A log-warn-only failure silently breaks
		// that invariant. Surfacing this as a hard error from ApplyStepTransition
		// (rejecting/reverting the transition) would ripple through ~20+
		// call sites that treat setFlowStepStatus/setFlowStepPosture as
		// fire-and-forget — too large a blast radius for this fix. Instead,
		// durably record an explicit "degraded" marker on the run so
		// restart/replay logic can detect the step timeline is not fully
		// trustworthy and escalate, instead of silently treating in-memory
		// state as authoritative.
		s.markTransitionLogDegraded(parentRunID, nodeID, status, err, muHeld)
	}
}

// markTransitionLogDegraded durably records that a step-transition-log append
// failed for parentRunID (BUG-288 P2-03). muHeld mirrors appendStepTransitionLog's
// caller-lock convention: when true the caller already holds s.mu, so this
// only mutates the in-memory flag here and defers the durable persist to a
// short-lived goroutine (re-acquiring the lock itself) rather than doing I/O
// while a caller-owned lock is held.
func (s *InteractiveService) markTransitionLogDegraded(parentRunID, nodeID, status string, appendErr error, muHeld bool) {
	reason := fmt.Sprintf("append node=%q status=%q: %v", nodeID, status, appendErr)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if muHeld {
		if rs := s.runs[parentRunID]; rs != nil {
			rs.transitionLogDegraded = true
			rs.transitionLogDegradedAt = now
			rs.transitionLogDegradedReason = reason
		}
		go s.persistTransitionLogDegradedMarker(parentRunID)
		return
	}
	s.mu.Lock()
	rs := s.runs[parentRunID]
	if rs == nil {
		s.mu.Unlock()
		return
	}
	rs.transitionLogDegraded = true
	rs.transitionLogDegradedAt = now
	rs.transitionLogDegradedReason = reason
	snap := sessionStateOf(rs)
	s.mu.Unlock()
	if err := s.persistProviderSession(snap); err != nil {
		log.Printf("[flow-step] persist transition-log-degraded marker run=%q: %v", parentRunID, err)
	}
}

// persistTransitionLogDegradedMarker re-reads the current snapshot and
// persists it — used when markTransitionLogDegraded is invoked from a
// muHeld=true call site (the RAM mutation already happened synchronously
// under the caller's lock; only the durable write is deferred).
func (s *InteractiveService) persistTransitionLogDegradedMarker(parentRunID string) {
	s.mu.Lock()
	rs := s.runs[parentRunID]
	if rs == nil {
		s.mu.Unlock()
		return
	}
	snap := sessionStateOf(rs)
	s.mu.Unlock()
	if err := s.persistProviderSession(snap); err != nil {
		log.Printf("[flow-step] persist transition-log-degraded marker run=%q: %v", parentRunID, err)
	}
}

// markFlowRunComplete is the terminal transition when the flow's control tool
// reports "done". It is authoritative: running work is settled to DONE and
// unstarted work is settled to SKIPPED, while previous terminal failures stay
// visible. The bulk planner is gated off for flowEngineDriven runs, so this is
// the sole terminal settler.
func (s *InteractiveService) markFlowRunComplete(ctx context.Context, parentRunID string) {
	s.flowDiagLog(parentRunID, "flow_run_complete_begin", "marking flow run complete")
	if steps, err := s.workflowStore.LoadRunSteps(ctx, parentRunID); err == nil {
		for _, st := range steps {
			switch st.Status {
			case StepStatusDone, StepStatusFailed, StepStatusSkipped, StepStatusCanceled:
				continue // already terminal
			case StepStatusPending:
				s.setFlowStepStatus(ctx, parentRunID, st.ID, StepStatusSkipped)
			default:
				s.setFlowStepStatus(ctx, parentRunID, st.ID, StepStatusDone)
			}
		}
	} else {
		log.Printf("[flow-step] mark run %q done: LoadRunSteps failed: %v", parentRunID, err)
		s.flowDiagLog(parentRunID, "flow_run_complete_load_steps_failed", "failed to load run steps while completing flow",
			"error", err.Error(),
		)
		// Fall back to at least settling the inline hub node.
		if hubID := hubInlineNodeID(s.activeFlowNodesFor(parentRunID)); hubID != "" {
			s.setFlowStepStatus(ctx, parentRunID, hubID, StepStatusDone)
		}
	}
	if err := s.workflowStore.SetRunStatus(ctx, parentRunID, RunStatusEngineDone, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		log.Printf("[flow-step] mark run %q done failed: %v", parentRunID, err)
		s.flowDiagLog(parentRunID, "flow_run_complete_set_status_failed", "failed to set terminal flow run status",
			"error", err.Error(),
		)
	}
	// BUG-235: the step timeline above is authoritative for the flow's own nodes,
	// but a cohort member's underlying agent RUN can independently linger in a
	// non-terminal status (running/waiting_*) in the Agents panel and the main
	// chat's inline run card, even though the flow that spawned it is now done.
	// Settle any such orphaned child so "flow done" is a clean, fully-terminal
	// state on both the step timeline and the agent-run list.
	s.settleParentRunOnFlowDone(parentRunID)
	s.reconcileChildRunsOnFlowDone(parentRunID)
	s.flowDiagLog(parentRunID, "flow_run_complete_done", "flow run marked complete")
}

func (s *InteractiveService) settleParentRunOnFlowDone(parentRunID string) {
	var approvalSnapshot *ProviderApprovalState
	var questionSnapshot *ProviderQuestionState
	s.mu.Lock()
	if parent := s.runs[parentRunID]; parent != nil {
		parent.status = RunStatusCompleted
		parent.agentStatus = string(RunStatusCompleted)
		s.maybeEmitWorktreeMergeRequest(parent) // CP-71: flow-done merge card
		if id := parent.pendingApprovalID; id != "" {
			if rec := s.approvals[id]; rec != nil && rec.status == "pending" {
				rec.status = "expired"
				rec.revision++
				state := approvalStateFromRecord(parent, rec, "")
				approvalSnapshot = &state
			}
			parent.pendingApprovalID = ""
		}
		if id := parent.pendingQuestionID; id != "" {
			if rec := s.questions[id]; rec != nil && rec.status == "pending" {
				rec.status = "expired"
				rec.revision++
				state := questionStateFromRecord(rec, "", "")
				questionSnapshot = &state
			}
			parent.pendingQuestionID = ""
		}
	}
	s.mu.Unlock()
	if approvalSnapshot != nil {
		_ = s.persistApproval(*approvalSnapshot)
	}
	if questionSnapshot != nil {
		_ = s.persistQuestion(*questionSnapshot)
	}
}

// reconcileChildRunsOnFlowDone (BUG-235) force-settles any child of parentRunID
// still reporting running/waiting_approval/waiting_question to completed, once
// the flow itself has reached "done". A child with an actual turn in flight is
// left alone — its own completion event will settle it normally; this only
// catches a child whose completion should already have landed (the barrier
// that gates a flow's "done" requires every cohort member finished) but whose
// run-level status update did not land for some reason. Emits one fresh
// agent_graph_updated so the desktop's Agents panel and main-chat run card
// clear immediately instead of waiting for an event that will never come.
func (s *InteractiveService) reconcileChildRunsOnFlowDone(parentRunID string) {
	changed := false
	s.mu.Lock()
	for _, childID := range s.agentOrchestrator.listChildren(parentRunID) {
		child := s.runs[childID]
		if child == nil || child.turnInFlight {
			continue
		}
		switch child.status {
		case RunStatusRunning, RunStatusWaitingApproval, RunStatusWaitingQuestion:
		default:
			continue
		}
		child.status = RunStatusCompleted
		child.agentStatus = string(RunStatusCompleted)
		changed = true
	}
	s.mu.Unlock()
	if !changed {
		return
	}
	// BUG-289 M5/F-10: apply loop 1's status filter to loop 2 — never promote
	// a Failed child panel summary to Completed (inverse of BUG-257).
	for _, childID := range s.agentOrchestrator.listChildren(parentRunID) {
		summary, ok := s.agentOrchestrator.currentSummary(parentRunID, childID)
		if !ok || summary.Status == RunStatusCompleted {
			continue
		}
		if summary.Status == RunStatusFailed || summary.Status == RunStatusCancelled {
			continue
		}
		// Also skip when the live run is already terminal-failed.
		s.mu.Lock()
		child := s.runs[childID]
		failedLive := child != nil && (child.status == RunStatusFailed || child.status == RunStatusCancelled)
		s.mu.Unlock()
		if failedLive {
			continue
		}
		summary.Status = RunStatusCompleted
		summary.AgentStatus = string(RunStatusCompleted)
		s.agentOrchestrator.upsertSummary(parentRunID, summary)
	}
	s.emitAgentGraph(parentRunID, s.agentOrchestrator.graphSnapshot(parentRunID))
}

// hubInlineNodeID returns the id of the flow's inline hub node (behavior
// hub.inline) — review-loop's "synthesis" — or "" if the flow has none.
func hubInlineNodeID(nodes []agentpack.FlowNode) string {
	for _, n := range nodes {
		if canonical, ok := agentpack.NormalizeBehaviorID(n.Behavior); ok && canonical == "hub.inline" {
			return n.ID
		}
	}
	return ""
}

// countHubInlineNodes reports how many hub.inline nodes the flow declares.
// Every pre-CP-58 built-in flow has at most one, where the first-match
// hubInlineNodeID fallback is unambiguous; dual-hub harnesses
// (task-harness/cp-harness-smoke, CP-58 Task-304) have two and must drive the
// run's activeHubNodeID from cohort joins instead.
func countHubInlineNodes(nodes []agentpack.FlowNode) int {
	count := 0
	for _, n := range nodes {
		if canonical, ok := agentpack.NormalizeBehaviorID(n.Behavior); ok && canonical == "hub.inline" {
			count++
		}
	}
	return count
}

// hubNodeIDForCohortJoin returns the hub.inline node a just-joined delegate
// cohort feeds into, resolved from the joined members' own forward edges, and
// ok=true only for dual-hub flows (CP-58 Task-304). Single-hub flows return
// ok=false so every caller keeps Task-235's activeHubNodeID-then-first-match
// tracking byte-identical; a dual-hub flow whose joined members don't name a
// hub.inline successor (e.g. the code-loop anchor validate failed) also
// returns ok=false and keeps the previously tracked hub.
func hubNodeIDForCohortJoin(nodes []agentpack.FlowNode, edges []agentpack.FlowEdge, joinedNodeIDs []string) (string, bool) {
	if countHubInlineNodes(nodes) <= 1 {
		return "", false
	}
	for _, id := range joinedNodeIDs {
		if id == "" {
			continue
		}
		for _, e := range edges {
			if !strings.EqualFold(strings.TrimSpace(e.Kind), "forward") || strings.TrimSpace(e.From) != id {
				continue
			}
			if node, ok := findFlowNode(nodes, strings.TrimSpace(e.To)); ok {
				if canonical, ok2 := agentpack.NormalizeBehaviorID(node.Behavior); ok2 && canonical == "hub.inline" {
					return node.ID, true
				}
			}
		}
	}
	return "", false
}

// flowEntryNodeID returns the id of the flow's entry delegate node (review-
// loop's "coder"), the node a "continue" back-edge re-runs, or "" if none.
func flowEntryNodeID(nodes []agentpack.FlowNode) string {
	entries := entryDelegateNodes(agentpack.FlowDefinition{Nodes: nodes})
	if len(entries) > 0 {
		return entries[0].ID
	}
	return ""
}
