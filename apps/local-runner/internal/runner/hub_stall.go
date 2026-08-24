package runner

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// defaultHubStallTimeout is the hub/root I-16 extension (BUG-289 F-0).
// Shorter than member stall default so silent hub hangs surface before
// cohort members would; operators can still Stop/Retry via the card.
const defaultHubStallTimeout = 2 * time.Minute

// hubStallTimer tracks one resettable hub watchdog per root run.
var (
	hubStallTimerMu sync.Mutex
	hubStallTimers  = map[string]*time.Timer{}
)

func hubStallTimerKey(s *InteractiveService, runID string) string {
	return fmt.Sprintf("%p:hub:%s", s, runID)
}

// touchHubProgressLocked stamps last activity under s.mu (caller holds lock).
func touchHubProgressLocked(rs *interactiveRun) {
	if rs == nil {
		return
	}
	rs.hubLastProgressAt = time.Now().UTC()
}

// touchParentHubProgressFromChildLocked stamps the flow hub's last progress when
// a child terminal event is accepted. Caller holds s.mu.
//
// run-43831 residual (after CA-361): hubLastProgressAt is only stamped on hub-side
// work. While a child runs longer than defaultHubStallTimeout, hasActiveFlowChild
// correctly re-arms F-0. The moment the child becomes terminal DONE there is a gap
// before the next spawn is active; age is already >2m so checkAndBlockStalledHub
// falsely parks the flow mid auto-advance. Refreshing on accepted child terminal
// progress closes that gap without treating pendingHubReinvoke or stale settle as busy.
func (s *InteractiveService) touchParentHubProgressFromChildLocked(child *interactiveRun) {
	if s == nil || child == nil || child.parentRunID == "" {
		return
	}
	parent := s.runs[child.parentRunID]
	if parent == nil || parent.parentRunID != "" || !parent.flowEngineDriven {
		return
	}
	touchHubProgressLocked(parent)
}

// hasActiveFlowChild reports whether a child/sub-agent is still doing or
// awaiting work for this parent. The hub watchdog must not convert that state
// into hub_stalled; child/member stall handling owns those cases.
//
// CA-616: a terminal child (Failed/Completed/Cancelled) is NOT active even if
// turnInFlight still lingers for the few ms between EventTurnFailed and its
// clearing — otherwise a planner that just 400'd keeps the hub hub_parked
// forever and F-0 never surfaces (run-135037).
func (s *InteractiveService) hasActiveFlowChild(parentRunID string) bool {
	for _, childID := range s.agentOrchestrator.listChildren(parentRunID) {
		s.mu.Lock()
		child := s.runs[childID]
		if child == nil {
			s.mu.Unlock()
			continue
		}
		if child.status == RunStatusFailed || child.status == RunStatusCompleted || child.status == RunStatusCancelled {
			s.mu.Unlock()
			continue
		}
		active := child.turnInFlight ||
			child.pendingTurnPrompt != "" ||
			child.pendingApprovalID != "" ||
			child.pendingQuestionID != "" ||
			child.postTurnGateCancel != nil ||
			child.pendingFlowGateSettle ||
			child.status == RunStatusStarting ||
			child.status == RunStatusRunning ||
			child.status == RunStatusWaitingApproval ||
			child.status == RunStatusWaitingQuestion
		s.mu.Unlock()
		if active {
			return true
		}
	}
	return false
}

// shouldParkHubWriteTurn is CP-51 A1 residual (run-9437 class): while any flow
// child is still RUNNING / waiting_*, the hub must not start a concurrent
// write/tool/gate-reprompt turn. This parks the hub session only — it does not
// serialize graph delegate fan-out or change inline flow_control waits.
func (s *InteractiveService) shouldParkHubWriteTurn(parentRunID string) bool {
	if parentRunID == "" || s == nil {
		return false
	}
	s.mu.Lock()
	rs := s.runs[parentRunID]
	driven := rs != nil && rs.parentRunID == "" && rs.flowEngineDriven
	s.mu.Unlock()
	if !driven {
		return false
	}
	return s.hasActiveFlowChild(parentRunID)
}

// clearHubGateRepromptFieldsLocked zeroes durable hub gate-reprompt intent
// fields in RAM. Caller must persist sessionStateOf(rs) for durability.
// Caller holds s.mu.
func clearHubGateRepromptFieldsLocked(rs *interactiveRun) {
	if rs == nil {
		return
	}
	rs.pendingGateRepromptPrompt = ""
	rs.pendingGateRepromptStepID = ""
	rs.pendingGateRepromptGen = 0
	rs.pendingGateRepromptDeliveredGen = 0
	rs.pendingGateRepromptAcceptedTurn = ""
	rs.pendingGateRepromptFailCount = 0
	rs.pendingGateRepromptFailGen = 0
	rs.pendingGateRepromptProvenanceRunID = ""
}

// clearHubGateRepromptIfContinueDelegatedLocked drops a hub gate-reprompt that
// would re-enter the hub for missing-doc remediation after flow_control continue
// already advanced a delegate writer on the SAME provider turn. Suppression is
// strictly turn-scoped: turnID must equal hubContinueDelegatedTurnID. On match,
// reprompt fields are cleared and the durable marker is consumed so a later
// hub gate on turn N+1 is not suppressed. Caller holds s.mu and must persist.
func clearHubGateRepromptIfContinueDelegatedLocked(rs *interactiveRun, turnID string) bool {
	if rs == nil || rs.parentRunID != "" {
		return false
	}
	if rs.hubContinueDelegatedTurnID == "" || turnID == "" {
		return false
	}
	if rs.hubContinueDelegatedTurnID != turnID {
		return false
	}
	// Same-transfer only: clear any queued reprompt and consume the marker so
	// a later hub gate on turn N+1 is not suppressed.
	if strings.TrimSpace(rs.pendingGateRepromptPrompt) != "" {
		clearHubGateRepromptFieldsLocked(rs)
	}
	rs.hubContinueDelegatedTurnID = ""
	return true
}

// persistContinueDelegateSuppressionLocked snapshots hub continue-delegate
// state (marker + cleared reprompt) for durable session write. Caller holds s.mu.
func (s *InteractiveService) persistContinueDelegateSuppressionLocked(rs *interactiveRun) ProviderSessionState {
	snap := sessionStateOf(rs)
	if rs != nil && rs.parentRunID == "" {
		snap.LoopState = s.agentOrchestrator.loopStateFor(rs.id)
	}
	return snap
}

// durableSuppressHubGateRepromptAfterContinue clears same-turn hub gate
// reprompt intent and persists atomically, consuming the continue-delegate
// marker so only that transfer is suppressed.
func (s *InteractiveService) durableSuppressHubGateRepromptAfterContinue(runID, turnID string) bool {
	s.mu.Lock()
	rs := s.runs[runID]
	if rs == nil {
		s.mu.Unlock()
		return false
	}
	if !clearHubGateRepromptIfContinueDelegatedLocked(rs, turnID) {
		s.mu.Unlock()
		return false
	}
	// clearHubGateRepromptIfContinueDelegatedLocked always returns true when
	// turn matches (and consumes the marker). Only treat as "suppressed a
	// reprompt" when we actually had work — callers use the bool to skip
	// startTurn; marker-only consume still must not start a reprompt that was
	// already empty.
	snap := s.persistContinueDelegateSuppressionLocked(rs)
	// Re-check: if we only consumed a marker with no reprompt, still "handled"
	// for the schedule path (nothing to start).
	s.mu.Unlock()
	if err := s.persistProviderSession(snap); err != nil {
		log.Printf("[hub-park] persist continue-delegate suppress failed run=%s: %v", runID, err)
	}
	s.flowDiagLog(runID, "hub_gate_reprompt_suppressed_after_continue",
		"gate reprompt dropped durably: flow_control continue already delegated a writer",
		"turn_id", turnID,
	)
	return true
}

// stampHubContinueDelegatedDurable marks the current hub turn as
// continue-delegated, clears any pending hub gate reprompt for this turn, and
// persists atomically so restart cannot revive a forbidden root reprompt.
func (s *InteractiveService) stampHubContinueDelegatedDurable(parentRunID string) {
	s.mu.Lock()
	rs := s.runs[parentRunID]
	if rs == nil || rs.currentTurnID == "" {
		s.mu.Unlock()
		return
	}
	rs.hubContinueDelegatedTurnID = rs.currentTurnID
	clearHubGateRepromptFieldsLocked(rs)
	snap := s.persistContinueDelegateSuppressionLocked(rs)
	s.mu.Unlock()
	if err := s.persistProviderSession(snap); err != nil {
		log.Printf("[hub-park] persist continue-delegate stamp failed run=%s: %v", parentRunID, err)
	}
}

// clearHubContinueDelegatedMarkerLocked drops a stale continue-delegate marker
// when a new hub turn begins (turn N+1). Caller holds s.mu; persist with the
// turn start session snapshot.
func clearHubContinueDelegatedMarkerLocked(rs *interactiveRun, newTurnID string) {
	if rs == nil || rs.hubContinueDelegatedTurnID == "" {
		return
	}
	if newTurnID != "" && rs.hubContinueDelegatedTurnID == newTurnID {
		// Same turn (re-entry) keeps the marker until same-turn suppress consumes it.
		return
	}
	rs.hubContinueDelegatedTurnID = ""
}

// scheduleRootGateRepromptOrPark starts a hub gate reprompt only when the hub is
// not parked for active flow children and continue did not already delegate on
// this turn. When parked, the durable intent stays for a later idle flush.
// Continue-suppress clears PendingGateReprompt* on disk so restart cannot revive it.
func (s *InteractiveService) scheduleRootGateRepromptOrPark(runID, stepID, prompt, turnID string, gen int64) {
	if runID == "" || stepID == "" || prompt == "" {
		return
	}
	if s.durableSuppressHubGateRepromptAfterContinue(runID, turnID) {
		return
	}
	if s.shouldParkHubWriteTurn(runID) {
		s.flowDiagLog(runID, "hub_parked_gate_reprompt",
			"gate reprompt deferred: hub parked while flow children are active",
			"turn_id", turnID,
		)
		return
	}
	go s.startTurnClearingIntent(runID, stepID, prompt, "reprompt", gen)
}

// maybeScheduleHubStallCheck arms a delayed hub watchdog (BUG-289 F-0 / I-16 hub).
// Asserts: loop running/WAITING ⇒ turn in flight OR gate/approval surfaced OR
// reinvoke pending OR actionable park. On violation for T, block with hub_stalled.
func (s *InteractiveService) maybeScheduleHubStallCheck(runID string) {
	if runID == "" || s == nil {
		return
	}
	s.mu.Lock()
	rs := s.runs[runID]
	if rs == nil || rs.parentRunID != "" || !rs.flowEngineDriven {
		s.mu.Unlock()
		return
	}
	timeout := rs.stallTimeout
	if timeout <= 0 {
		timeout = defaultHubStallTimeout
	}
	// Prefer a shorter hub window when parent uses the long member default.
	if timeout > defaultHubStallTimeout {
		timeout = defaultHubStallTimeout
	}
	last := rs.hubLastProgressAt
	s.mu.Unlock()

	loop := s.agentOrchestrator.loopStateFor(runID)
	switch loop.Status {
	case "blocked", "done", "stopped", "paused":
		return
	}

	now := time.Now().UTC()
	age := time.Duration(0)
	if !last.IsZero() {
		age = now.Sub(last)
	}
	remaining := timeout - age
	if remaining < 0 {
		remaining = 0
	}
	d := remaining + time.Second
	key := hubStallTimerKey(s, runID)
	hubStallTimerMu.Lock()
	if t, ok := hubStallTimers[key]; ok {
		t.Stop()
	}
	hubStallTimers[key] = time.AfterFunc(d, func() {
		hubStallTimerMu.Lock()
		delete(hubStallTimers, key)
		hubStallTimerMu.Unlock()
		s.checkAndBlockStalledHub(runID)
	})
	hubStallTimerMu.Unlock()
}

// checkAndBlockStalledHub implements BUG-289 F-0: hub/root escape hatch when
// the loop is non-terminal but nothing is progressing or actionable.
func (s *InteractiveService) checkAndBlockStalledHub(runID string) bool {
	s.mu.Lock()
	rs := s.runs[runID]
	if rs == nil || rs.parentRunID != "" {
		s.mu.Unlock()
		return false
	}
	timeout := rs.stallTimeout
	if timeout <= 0 || timeout > defaultHubStallTimeout {
		timeout = defaultHubStallTimeout
	}
	// Live progress only. pendingHubReinvoke alone is NOT busy (H1 review):
	// an undrained pending slot after startTurn reject is exactly the hang
	// F-0 must detect — treating it as busy re-armed the watchdog forever
	// while nothing drained or escalated.
	//
	// H-C / run-1618: pendingFlowGateSettle WITHOUT a live postTurnGateCancel
	// is also NOT busy. flowStartOnly and other stale stamps left settle=true
	// with no gate running; counting settle alone as busy made F-0 re-arm
	// forever and never surface hub_stalled. A real post-turn gate always
	// arms postTurnGateCancel for the evaluate window.
	busy := rs.turnInFlight || rs.reinvokeInFlight ||
		rs.pendingApprovalID != "" || rs.pendingQuestionID != "" ||
		rs.postTurnGateCancel != nil ||
		strings.TrimSpace(rs.pendingGateRepromptPrompt) != "" ||
		strings.TrimSpace(rs.pendingResumePrompt) != ""
	last := rs.hubLastProgressAt
	status := rs.status
	s.mu.Unlock()

	loop := s.agentOrchestrator.loopStateFor(runID)
	switch loop.Status {
	case "blocked", "done", "stopped", "paused",
		// run-2047: rejected/approved are keyword-era terminal signals — never
		// overwrite with hub_stalled (and blocked must not be clobbered by child
		// cancel → rejected; see agent_orchestrator.transition).
		"rejected", "approved":
		return false
	}

	// WaitingApproval/Question with a live card is actionable.
	if status == RunStatusWaitingApproval || status == RunStatusWaitingQuestion {
		s.mu.Lock()
		hasCard := false
		if r := s.runs[runID]; r != nil {
			hasCard = r.pendingApprovalID != "" || r.pendingQuestionID != ""
		}
		s.mu.Unlock()
		if hasCard {
			return false
		}
		// Status says waiting but no card — stalled (H4 class).
	} else if busy {
		// Still progressing — re-arm for later.
		s.maybeScheduleHubStallCheck(runID)
		return false
	}
	if s.hasActiveFlowChild(runID) {
		s.maybeScheduleHubStallCheck(runID)
		return false
	}

	now := time.Now().UTC()
	age := time.Duration(0)
	if !last.IsZero() {
		age = now.Sub(last)
	} else {
		// Never stamped — treat as stalled only after timeout from now re-arm.
		s.mu.Lock()
		if r := s.runs[runID]; r != nil && r.hubLastProgressAt.IsZero() {
			r.hubLastProgressAt = now
		}
		s.mu.Unlock()
		s.maybeScheduleHubStallCheck(runID)
		return false
	}
	if age < timeout {
		s.maybeScheduleHubStallCheck(runID)
		return false
	}

	reason := fmt.Sprintf("hub has made no progress for %s (no turn, gate, or reinvoke in flight)", timeout)
	s.setFlowStepAwaitingUser(context.Background(), runID)
	snap := s.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = "hub_stalled"
		st.GateReason = reason
		return st
	})
	// Same freeze as escalate: no auto-reprompt/reinvoke behind the stall card.
	s.parkFlowForAwaitingUser(runID)
	s.emitAgentGraph(runID, snap)
	go s.persistParentSession(runID)
	s.flowDiagLog(runID, "hub_stalled", "hub/root stalled; blocked with actionable card",
		"age", age.String(), "timeout", timeout.String(),
	)
	return true
}
