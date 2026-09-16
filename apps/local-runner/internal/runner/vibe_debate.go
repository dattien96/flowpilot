package runner

import (
	"context"
	"strings"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

const maxVibeOwnerFailRetries = 2

func vibeOwnerDebateGraph(nodes []agentpack.FlowNode) bool {
	var o1, o2, syn bool
	for _, n := range nodes {
		switch strings.TrimSpace(n.ID) {
		case "owner_1":
			o1 = true
		case "owner_2":
			o2 = true
		case vibeDebateSynthesisNodeID:
			syn = true
		}
	}
	return o1 && o2 && syn
}

func vibeOwnerStepFailed(st RuntimeWorkflowStepStatus) bool {
	return st == StepStatusFailed || st == StepStatusCanceled
}

// maybeSettleVibeOwnerDebate retries or parks when both owners failed and
// debate_synthesis has not started. Prevents owner-fail → 2m hub_stalled.
// Returns true when it retried the debate or parked cap.
func (s *InteractiveService) maybeSettleVibeOwnerDebate(parentRunID string) bool {
	if s == nil || strings.TrimSpace(parentRunID) == "" {
		return false
	}
	s.mu.Lock()
	rs := s.runs[parentRunID]
	if rs == nil || rs.parentRunID != "" || rs.workingMode != workingmode.Vibe {
		s.mu.Unlock()
		return false
	}
	if !vibeOwnerDebateGraph(rs.activeFlowNodes) {
		s.mu.Unlock()
		return false
	}
	if rs.vibeOwnerSettleInFlight {
		s.mu.Unlock()
		return false
	}
	rs.vibeOwnerSettleInFlight = true
	retries := rs.vibeOwnerFailRetries
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if r := s.runs[parentRunID]; r != nil {
			r.vibeOwnerSettleInFlight = false
		}
		s.mu.Unlock()
	}()

	loop := s.agentOrchestrator.loopStateFor(parentRunID)
	if loop.Status == "blocked" && loop.BlockReason != "hub_stalled" && loop.BlockReason != "" {
		return false
	}

	for _, childID := range s.agentOrchestrator.listChildren(parentRunID) {
		s.mu.Lock()
		ch := s.runs[childID]
		s.mu.Unlock()
		if ch == nil {
			continue
		}
		if ch.label != "owner_1" && ch.label != "owner_2" {
			continue
		}
		if ch.status != RunStatusFailed && ch.status != RunStatusCompleted && ch.status != RunStatusCancelled {
			return false
		}
	}

	st1, st2, stSyn := s.vibeOwnerDebateStepStatuses(parentRunID)
	if !vibeOwnerStepFailed(st1) || !vibeOwnerStepFailed(st2) {
		return false
	}
	if stSyn == StepStatusRunning || stSyn == StepStatusDone || stSyn == StepStatusWaitingUserApr {
		return false
	}

	if retries >= maxVibeOwnerFailRetries {
		s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
			st.Status = "blocked"
			st.BlockReason = "cap"
			st.GateReason = "vibe owner debate members failed"
			st.ActiveNode = vibeDebateSynthesisNodeID
			return st
		})
		// CP-65 P-4 (Task-371): flag-gated rescue replaces the owner-fail park.
		// On escalation the legacy park below is skipped (the parent waits on
		// the tournament child, not on a human form); flag off keeps it.
		if s.maybeEscalateCapToTournament(parentRunID, "vibe owner debate stalled (owner-fail cap)") {
			return true
		}
		s.parkFlowForAwaitingUser(parentRunID)
		s.flowDiagLog(parentRunID, "vibe_owner_fail_cap",
			"owner debate members failed; parked cap not hub_stalled",
		)
		return true
	}

	if loop.Status == "blocked" && loop.BlockReason == "hub_stalled" {
		s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
			st.Status = "running"
			st.BlockReason = ""
			st.GateReason = ""
			return st
		})
	}

	s.mu.Lock()
	if r := s.runs[parentRunID]; r != nil {
		r.vibeOwnerFailRetries = retries + 1
		touchHubProgressLocked(r)
	}
	s.mu.Unlock()
	s.flowDiagLog(parentRunID, "vibe_owner_fail_retry",
		"both owners failed; retrying vibe-owner-debate",
		"retry", retries+1,
	)
	s.startResolvedFlow(context.Background(), parentRunID, workingmode.PackPrefix+vibeOwnerDebateFlowID, "owner members failed; retry debate")
	return true
}

func (s *InteractiveService) vibeOwnerDebateStepStatuses(parentRunID string) (st1, st2, stSyn RuntimeWorkflowStepStatus) {
	if s.workflowStore == nil {
		return "", "", ""
	}
	steps, err := s.workflowStore.LoadRunSteps(context.Background(), parentRunID)
	if err != nil {
		return "", "", ""
	}
	for _, step := range steps {
		switch strings.TrimSpace(step.NodeID) {
		case "owner_1":
			st1 = step.Status
		case "owner_2":
			st2 = step.Status
		case vibeDebateSynthesisNodeID:
			stSyn = step.Status
		}
	}
	return st1, st2, stSyn
}
