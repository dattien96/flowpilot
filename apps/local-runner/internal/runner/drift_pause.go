package runner

import (
	"fmt"
	"log"
	"strings"

	"flowpilot-runner/internal/driftdetect"
	"flowpilot-runner/internal/workingmode"
)

// Task-348 (CP-23 ladder completion / CP-62 P-1 companion): the drift ladder's
// 80+ pause leg wired for real. In dev mode a drift ≥80 parks the run with a
// "drift" block reason and emits drift_pause_required — the human is actually
// asked, and the resume goes through the ordinary continue/feedback channel
// (the same machinery gates and decision cards use), so no client can be
// stranded on an unanswerable modal (the Task-335 deferral concern). Vibe mode
// never pauses on drift: P-1's owner-debate routing owns it (SS-18 AC-7).

// EventDriftPauseRequired is emitted once per drift pause. Clients that do
// not know the type ignore it; the parked run's blocked card + gate reason
// carry the same information additively.
const EventDriftPauseRequired ProviderEventType = "drift_pause_required"

// DriftPauseBlockReason is the AgentLoopState.BlockReason stamped on a
// drift-parked run.
const DriftPauseBlockReason = "drift"

// armDriftPause parks rs's governing run for a drift pause. idempotent: an
// already drift-parked run is not re-parked or re-emitted. Returns true when
// the pause landed. Lock order respected: takes s.mu only (never st.mu).
func (s *InteractiveService) armDriftPause(rs *interactiveRun, event driftdetect.DriftEvent) bool {
	if s == nil || rs == nil {
		return false
	}
	// Resolve the governing run: flow children park their parent (the hub
	// drives the loop); plain chat runs park themselves.
	targetID := rs.id
	s.mu.Lock()
	if strings.TrimSpace(rs.parentRunID) != "" {
		targetID = strings.TrimSpace(rs.parentRunID)
	}
	target := s.runs[targetID]
	mode := rs.workingMode
	s.mu.Unlock()
	if target == nil {
		return false
	}
	// Vibe mode: drift ≥80 routes into the owner-debate flow (P-1
	// applyVibeDriftOnlyResolver) — the user is never asked for drift.
	if mode == workingmode.Vibe {
		return false
	}
	if s.agentOrchestrator == nil {
		return false
	}

	gateReason := fmt.Sprintf("drift score %d (signals: %s) at step %q — confirm to continue",
		event.DriftScore, strings.Join(event.TriggeredSignals, ", "), event.StepID)

	// Idempotence: a run already parked on drift is left alone (no duplicate
	// events, no reason clobbering). Block reasons live in the loop state.
	if st := s.agentOrchestrator.loopStateFor(targetID); st.BlockReason == DriftPauseBlockReason {
		return false
	}

	snap := s.agentOrchestrator.mutateLoop(targetID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = DriftPauseBlockReason
		st.GateReason = gateReason
		return st
	})
	s.parkFlowForAwaitingUser(targetID)
	s.emitAgentGraph(targetID, snap)
	go s.persistParentSession(targetID)

	s.mu.Lock()
	s.emitLocked(target, ProviderEvent{
		Type:           EventDriftPauseRequired,
		ProviderTurnID: target.currentTurnID,
		Input: map[string]any{
			"runId":   targetID,
			"score":   event.DriftScore,
			"signals": event.TriggeredSignals,
			"stepId":  event.StepID,
			"reason":  gateReason,
		},
	})
	s.mu.Unlock()
	log.Printf("[drift-pause] run=%s parked waiting for human confirm (score=%d)", targetID, event.DriftScore)
	return true
}
