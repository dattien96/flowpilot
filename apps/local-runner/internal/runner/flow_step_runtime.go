package runner

import (
	"context"
	"log"
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

// isFlowEngineDriven reports whether runID's step timeline is owned by the flow
// executor rather than the legacy bulk planner.
func (s *InteractiveService) isFlowEngineDriven(runID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	seeder.seed(parentRunID, flowStepRowsFromNodes(nodes, StepStatusPending, ""))
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
	seeder, ok := s.workflowStore.(workflowRunSeeder)
	if !ok || len(nodes) == 0 {
		return
	}
	status := StepStatusPending
	ts := ""
	if completed {
		status = StepStatusDone
		ts = time.Now().UTC().Format(time.RFC3339Nano)
	}
	seeder.seed(runID, flowStepRowsFromNodes(nodes, status, ts))
}

// flowStepRowsFromNodes builds one step-runtime row per flow node (ID == NodeID
// so a transition can address it directly by node id). When status is DONE, ts
// stamps started/finished so the row reads as a settled step.
func flowStepRowsFromNodes(nodes []agentpack.FlowNode, status RuntimeWorkflowStepStatus, ts string) []RuntimeWorkflowStep {
	steps := make([]RuntimeWorkflowStep, 0, len(nodes))
	for _, node := range nodes {
		behaviorID := ""
		if canonical, ok := agentpack.NormalizeBehaviorID(node.Behavior); ok {
			behaviorID = canonical
		}
		row := RuntimeWorkflowStep{
			ID:         node.ID,
			StepType:   node.ID,
			NodeID:     node.ID,
			BehaviorID: behaviorID,
			AgentRef:   flowNodeAgentName(node),
			Status:     status,
		}
		if status == StepStatusDone {
			row.StartedAt = ts
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
func (s *InteractiveService) setFlowStepStatus(ctx context.Context, parentRunID, nodeID string, status RuntimeWorkflowStepStatus) {
	if nodeID == "" {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	patch := WorkflowStepPatch{Status: status}
	switch status {
	case StepStatusRunning:
		patch.StartedAt = strptr(now)
		patch.FinishedAt = strptr("")
	case StepStatusDone, StepStatusFailed:
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
	}
}

// markFlowRunComplete is the terminal transition when the flow's control tool
// reports "done": mark the inline hub node (review-loop's "synthesis") DONE and
// the run DONE, so the timeline settles honestly instead of relying on the
// bulk planner (which is gated off for flowEngineDriven runs).
func (s *InteractiveService) markFlowRunComplete(ctx context.Context, parentRunID string) {
	if hubID := hubInlineNodeID(s.activeFlowNodesFor(parentRunID)); hubID != "" {
		s.setFlowStepStatus(ctx, parentRunID, hubID, StepStatusDone)
	}
	if err := s.workflowStore.SetRunStatus(ctx, parentRunID, RunStatusEngineDone, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		log.Printf("[flow-step] mark run %q done failed: %v", parentRunID, err)
	}
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

// flowEntryNodeID returns the id of the flow's entry delegate node (review-
// loop's "coder"), the node a "continue" back-edge re-runs, or "" if none.
func flowEntryNodeID(nodes []agentpack.FlowNode) string {
	entries := entryDelegateNodes(agentpack.FlowDefinition{Nodes: nodes})
	if len(entries) > 0 {
		return entries[0].ID
	}
	return ""
}
