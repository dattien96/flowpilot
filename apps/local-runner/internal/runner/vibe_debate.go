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

	ownerChildren := 0
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
		ownerChildren++
		if ch.status != RunStatusFailed && ch.status != RunStatusCompleted && ch.status != RunStatusCancelled {
			return false
		}
	}

	stTrig, st1, st2, stSyn := s.vibeOwnerDebateStepStatuses(parentRunID)
	ownersFailed := vibeOwnerStepFailed(st1) && vibeOwnerStepFailed(st2)
	// CA-1088 (live run-3362): a mount starved by child_spawn_refused_blocked_
	// loop leaves both owner steps PENDING with zero owner children — the same
	// wedge class as both-failed, so it retries through the same ladder.
	// debate_trigger RUNNING means the mount is still dispatching; a parallel
	// restart would double-mount the debate.
	ownersStarved := ownerChildren == 0 &&
		stTrig != "" && stTrig != StepStatusRunning &&
		st1 != StepStatusRunning && st1 != StepStatusDone &&
		st2 != StepStatusRunning && st2 != StepStatusDone
	if !ownersFailed && !ownersStarved {
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

// vibeDebateSynthesisResumePrompt re-drives the debate hub's synthesis turn
// after a restart orphaned it: the owners settled but the cohort join that
// re-invokes the hub lived in RAM and died with the process (BUG-567).
const vibeDebateSynthesisResumePrompt = "[flow-engine] The owner-debate flow was interrupted by a restart: both owner legs already settled, but the debate_synthesis evaluation turn was lost with the process. Synthesize the owners' verdicts from the joined results and call flow_control with status=done (or submit_review_outcome) so the parked sprint flow restores and the chain resumes."

// maybeResumeVibeDebateSynthesis unsticks an owner-debate overlay that a
// restart left mounted past its conclusion (BUG-567, live run-100368):
// without this the overlay stays mounted forever — the review cohort is
// invisible to flowRequiresHubMachineVerdict and submit_review_outcome is
// never offered to review legs.
//
//   - debate_synthesis DONE but the restore died with the process → replay
//     restoreVibeFlowAfterDebate directly (idempotent).
//   - both owners DONE but synthesis never ran → re-invoke the debate hub so
//     it can emit done and unmount the overlay.
//
// Other shapes are owned elsewhere: both-failed / starved owners retry through
// maybeSettleVibeOwnerDebate, a live owner still owes its own completion, and
// a lopsided terminal pair must NOT respawn the debate (CA-796).
func (s *InteractiveService) maybeResumeVibeDebateSynthesis(parentRunID string) {
	if s == nil || strings.TrimSpace(parentRunID) == "" {
		return
	}
	s.mu.Lock()
	rs := s.runs[parentRunID]
	ok := rs != nil && rs.parentRunID == "" && rs.workingMode == workingmode.Vibe &&
		len(rs.vibeParkedNodes) > 0 && vibeOwnerDebateGraph(rs.activeFlowNodes)
	s.mu.Unlock()
	if !ok {
		return
	}
	_, st1, st2, stSyn := s.vibeOwnerDebateStepStatuses(parentRunID)
	if stSyn == StepStatusRunning || stSyn == StepStatusWaitingUserApr {
		return // a synthesis evaluation is already owed or in flight
	}
	if stSyn != StepStatusDone && (st1 != StepStatusDone || st2 != StepStatusDone) {
		return // fail / starved / lopsided owners: the retry ladder owns it
	}
	// Revive the interrupted run the same way the vibe resume helpers do —
	// a reconstructed run normalized to cancelled, so the reinvoke guard
	// (autoOrchestrate) and spawn paths would refuse the redrive otherwise.
	s.mu.Lock()
	if r := s.runs[parentRunID]; r != nil {
		if r.status == RunStatusCancelled || r.status == RunStatusFailed {
			r.status = RunStatusRunning
			r.agentStatus = string(RunStatusRunning)
		}
		r.autoOrchestrate = true
	}
	s.mu.Unlock()
	s.releaseHubStopFenceForFollowUp(context.Background(), parentRunID)
	s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		st.BlockReason = ""
		st.GateReason = ""
		return st
	})
	switch stSyn {
	case StepStatusDone:
		// The debate resolved pre-restart but the restore transition died
		// with it — replay the missed handoff (idempotent restore).
		s.flowDiagLog(parentRunID, "vibe_debate_synthesis_replayed",
			"debate_synthesis DONE before restart; replaying parked-flow restore")
		s.restoreVibeFlowAfterDebate(parentRunID)
	default:
		s.flowDiagLog(parentRunID, "vibe_debate_synthesis_redrive",
			"owners settled but debate_synthesis never ran; re-driving synthesis hub")
		s.maybeAutoReinvokeHubWithPrompt(parentRunID, vibeDebateSynthesisResumePrompt)
	}
}

// maybeResolveZombieVibeDebate heals a debate overlay that is claimed but
// dead: vibeParkedNodes still owns the real flow and every subsequent gate
// divert is swallowed by "debate already active" while nothing can ever emit
// the done that unmounts it (live run-139670: the owner cohort join
// mis-dispatched into the sprint's `synthesis` step, the debate_synthesis hub
// reinvoke failed, and the coder's turn_completed was orphaned twice).
//
// Healing only fires on unambiguous terminal shapes — the same shapes the
// restart path already resolves (BUG-567): synthesis DONE but the restore
// missed (replay it — idempotent), or both owners DONE with synthesis never
// started (re-drive the synthesis hub). A debate with any live member
// (running owner/synthesis, or a pending debate decision) is left alone —
// suppression stays suppression.
func (s *InteractiveService) maybeResolveZombieVibeDebate(parentRunID string) {
	if s == nil || strings.TrimSpace(parentRunID) == "" {
		return
	}
	s.mu.Lock()
	rs := s.runs[parentRunID]
	ok := rs != nil && rs.parentRunID == "" && rs.workingMode == workingmode.Vibe &&
		len(rs.vibeParkedNodes) > 0 &&
		(vibeOwnerDebateGraph(rs.activeFlowNodes) ||
			workingmode.BareFlowID(rs.chatFlowRef) == vibeOwnerDebateFlowID)
	s.mu.Unlock()
	if !ok {
		return
	}
	_, st1, st2, stSyn := s.vibeOwnerDebateStepStatuses(parentRunID)
	if stSyn == StepStatusRunning || stSyn == StepStatusWaitingUserApr ||
		st1 == StepStatusRunning || st2 == StepStatusRunning {
		return // the debate is still live — let it conclude itself
	}
	if stSyn == StepStatusDone {
		s.flowDiagLog(parentRunID, "vibe_debate_zombie_restored",
			"debate claim outlived a resolved synthesis; replaying parked-flow restore")
		s.restoreVibeFlowAfterDebate(parentRunID)
		return
	}
	if st1 == StepStatusDone && st2 == StepStatusDone {
		s.flowDiagLog(parentRunID, "vibe_debate_zombie_redrive",
			"owners settled but debate_synthesis never ran on the live path; re-driving synthesis hub")
		s.maybeResumeVibeDebateSynthesis(parentRunID)
	}
	// owner-fail / starved / lopsided shapes stay with maybeSettleVibeOwnerDebate.
}

func (s *InteractiveService) vibeOwnerDebateStepStatuses(parentRunID string) (stTrig, st1, st2, stSyn RuntimeWorkflowStepStatus) {
	if s.workflowStore == nil {
		return "", "", "", ""
	}
	steps, err := s.workflowStore.LoadRunSteps(context.Background(), parentRunID)
	if err != nil {
		return "", "", "", ""
	}
	for _, step := range steps {
		switch strings.TrimSpace(step.NodeID) {
		case "debate_trigger":
			stTrig = step.Status
		case "owner_1":
			st1 = step.Status
		case "owner_2":
			st2 = step.Status
		case vibeDebateSynthesisNodeID:
			stSyn = step.Status
		}
	}
	return stTrig, st1, st2, stSyn
}
