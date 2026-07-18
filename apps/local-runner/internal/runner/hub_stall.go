package runner

import (
	"context"
	"fmt"
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

// hasActiveFlowChild reports whether a child/sub-agent is still doing or
// awaiting work for this parent. The hub watchdog must not convert that state
// into hub_stalled; child/member stall handling owns those cases.
func (s *InteractiveService) hasActiveFlowChild(parentRunID string) bool {
	for _, childID := range s.agentOrchestrator.listChildren(parentRunID) {
		s.mu.Lock()
		child := s.runs[childID]
		active := child != nil && (child.turnInFlight ||
			child.pendingTurnPrompt != "" ||
			child.pendingApprovalID != "" ||
			child.pendingQuestionID != "" ||
			child.postTurnGateCancel != nil ||
			child.pendingFlowGateSettle ||
			child.status == RunStatusStarting ||
			child.status == RunStatusRunning ||
			child.status == RunStatusWaitingApproval ||
			child.status == RunStatusWaitingQuestion)
		s.mu.Unlock()
		if active {
			return true
		}
	}
	return false
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
