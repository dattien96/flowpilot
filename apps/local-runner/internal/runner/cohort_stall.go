package runner

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// defaultStallTimeout is Task-241 T-11 when flow policy leaves StallTimeoutSec=0.
const defaultStallTimeout = 10 * time.Minute

// MemberAction is the Task-241 card action for a stalled cohort member.
type MemberAction struct {
	Action string `json:"action"` // "retry" | "skip"
	Node   string `json:"node,omitempty"`
}

// openCohortMemberRuns returns child run IDs that still belong to an incomplete
// cohort of parentRunID (Task-241 stall sweep).
func (s *InteractiveService) openCohortMemberRuns(parentRunID string) []*interactiveRun {
	if !s.agentOrchestrator.hasOpenCohort(parentRunID) {
		return nil
	}
	var out []*interactiveRun
	for _, childID := range s.agentOrchestrator.listChildren(parentRunID) {
		s.mu.Lock()
		child := s.runs[childID]
		s.mu.Unlock()
		if child == nil || child.flowCohortId == "" {
			continue
		}
		// Still expected in an open cohort if not yet in the buffer.
		if s.agentOrchestrator.memberAlreadyBuffered(parentRunID, child.flowCohortId, child.label) {
			continue
		}
		out = append(out, child)
	}
	return out
}

// memberAlreadyBuffered reports whether label already has a cohort entry.
func (o *AgentOrchestrator) memberAlreadyBuffered(parentRunID, cohortID, label string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	k := cohortKey(parentRunID, cohortID)
	label = strings.TrimSpace(label)
	for _, e := range o.cohort[k] {
		if strings.TrimSpace(e.Label) == label {
			return true
		}
	}
	return false
}

// stallTimer tracks one resettable stall sweep per parent (BUG-288 #19).
// V9-23: key includes service pointer so multi-instance tests cannot collide.
var (
	stallTimerMu sync.Mutex
	stallTimers  = map[string]*time.Timer{}
)

func stallTimerKey(s *InteractiveService, parentRunID string) string {
	return fmt.Sprintf("%p:%s", s, parentRunID)
}

// maybeScheduleStallCheck arms a delayed stall sweep so silent cohort stalls
// surface without waiting for the user to press Continue (Codex review
// Important #2). One timer per parent (BUG-288 #19), but the deadline is the
// earliest member stall time — not a full reset on every noisy sibling event
// (V9-05). A streaming reviewer A must not hide a silent reviewer B forever.
func (s *InteractiveService) maybeScheduleStallCheck(parentRunID string) {
	if parentRunID == "" || s == nil {
		return
	}
	s.mu.Lock()
	parent := s.runs[parentRunID]
	if parent == nil || !parent.flowEngineDriven {
		s.mu.Unlock()
		return
	}
	timeout := parent.stallTimeout
	if timeout <= 0 {
		timeout = defaultStallTimeout
	}
	s.mu.Unlock()
	if !s.agentOrchestrator.hasOpenCohort(parentRunID) {
		return
	}
	// Compute remaining time until the quietest open member would stall.
	now := time.Now().UTC()
	var soonest time.Duration
	found := false
	for _, child := range s.openCohortMemberRuns(parentRunID) {
		s.mu.Lock()
		hasGate := child.pendingApprovalID != "" || child.pendingQuestionID != ""
		last := child.lastProviderEventAt
		inFlight := child.turnInFlight
		status := child.status
		created := child.createdAt
		s.mu.Unlock()
		if hasGate {
			continue
		}
		if status == RunStatusCompleted || status == RunStatusFailed || status == RunStatusCancelled {
			continue
		}
		var age time.Duration
		if !last.IsZero() {
			age = now.Sub(last)
		} else if inFlight {
			if t, err := time.Parse(time.RFC3339Nano, created); err == nil {
				age = now.Sub(t)
			} else if t, err := time.Parse(time.RFC3339, created); err == nil {
				age = now.Sub(t)
			} else {
				continue
			}
		} else {
			continue
		}
		remaining := timeout - age
		if remaining < 0 {
			remaining = 0
		}
		if !found || remaining < soonest {
			soonest = remaining
			found = true
		}
	}
	if !found {
		return
	}
	// +1s buffer so age >= timeout when the timer fires.
	d := soonest + time.Second
	key := stallTimerKey(s, parentRunID)
	stallTimerMu.Lock()
	if t, ok := stallTimers[key]; ok {
		// Always recompute absolute earliest deadline (do not extend noisy members).
		t.Stop()
	}
	stallTimers[key] = time.AfterFunc(d, func() {
		stallTimerMu.Lock()
		delete(stallTimers, key)
		stallTimerMu.Unlock()
		s.checkAndBlockStalledMembers(parentRunID)
	})
	stallTimerMu.Unlock()
}

// checkAndBlockStalledMembers implements Task-241 T-11(b) / I-16: if a cohort
// member has produced no provider events for stallTimeout AND is not holding a
// user-visible gate, park the hub as blocked with BlockReason=member_stalled.
//
// Ordering (Task-240 I-1/I-2): setFlowStepAwaitingUser → mutateLoop → emit → persist.
func (s *InteractiveService) checkAndBlockStalledMembers(parentRunID string) bool {
	s.mu.Lock()
	parent := s.runs[parentRunID]
	if parent == nil {
		s.mu.Unlock()
		return false
	}
	timeout := parent.stallTimeout
	if timeout <= 0 {
		timeout = defaultStallTimeout
	}
	// Only act while the loop is still advancing.
	s.mu.Unlock()
	loop := s.agentOrchestrator.loopStateFor(parentRunID)
	if loop.Status == "blocked" || loop.Status == "done" || loop.Status == "stopped" || loop.Status == "paused" {
		return false
	}
	if !s.agentOrchestrator.hasOpenCohort(parentRunID) {
		return false
	}
	now := time.Now().UTC()
	for _, child := range s.openCohortMemberRuns(parentRunID) {
		s.mu.Lock()
		// Gate visible → wait forever (T-11(a)); do not stall.
		// BUG-288 R13-06: post-turn gate in progress is also a "gate" — not stalled.
		hasGate := child.pendingApprovalID != "" || child.pendingQuestionID != "" ||
			child.postTurnGateCancel != nil || child.pendingFlowGateSettle
		last := child.lastProviderEventAt
		label := child.label
		status := child.status
		inFlight := child.turnInFlight
		s.mu.Unlock()
		if hasGate {
			continue
		}
		// Terminal members should already be buffered; skip.
		if status == RunStatusCompleted || status == RunStatusFailed || status == RunStatusCancelled {
			continue
		}
		// No event yet: use run creation as baseline via zero last → stall only
		// if still in flight past timeout from... we need a start time. Use
		// lastProviderEventAt; if zero, treat as stalled only when inFlight and
		// we cannot know age — use updatedAt parse best-effort.
		age := time.Duration(0)
		if !last.IsZero() {
			age = now.Sub(last)
		} else if inFlight {
			// No events at all while in flight — use a conservative stall if
			// the child has been around longer than timeout via createdAt.
			s.mu.Lock()
			created := child.createdAt
			s.mu.Unlock()
			if t, err := time.Parse(time.RFC3339Nano, created); err == nil {
				age = now.Sub(t)
			} else if t, err := time.Parse(time.RFC3339, created); err == nil {
				age = now.Sub(t)
			}
		} else {
			continue
		}
		if age < timeout {
			continue
		}
		// Stall this member.
		reason := fmt.Sprintf("member %q has produced no events for %s", label, timeout)
		s.setFlowStepAwaitingUser(context.Background(), parentRunID)
		snap := s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
			st.Status = "blocked"
			st.BlockReason = "member_stalled"
			st.GateReason = reason
			st.ActiveNode = label
			return st
		})
		s.emitAgentGraph(parentRunID, snap)
		go s.persistParentSession(parentRunID)
		s.flowDiagLog(parentRunID, "member_stalled", "cohort member stalled; hub blocked",
			"member", label, "age", age.String(), "timeout", timeout.String(),
		)
		return true
	}
	return false
}

// handleMemberAction applies Retry/Skip for a member_stalled block (Task-241 T-4).
// Returns true when the action was handled (caller should not also resume as continue).
func (s *InteractiveService) handleMemberAction(parentRunID string, action MemberAction) (AgentGraphSnapshot, bool, error) {
	act := strings.ToLower(strings.TrimSpace(action.Action))
	if act != "retry" && act != "skip" {
		return AgentGraphSnapshot{}, false, nil
	}
	loop := s.agentOrchestrator.loopStateFor(parentRunID)
	if loop.Status != "blocked" || loop.BlockReason != "member_stalled" {
		return AgentGraphSnapshot{}, false, fmt.Errorf("member_action requires blockReason=member_stalled")
	}
	nodeID := strings.TrimSpace(action.Node)
	if nodeID == "" {
		nodeID = strings.TrimSpace(loop.ActiveNode)
	}
	if nodeID == "" {
		return AgentGraphSnapshot{}, false, fmt.Errorf("member_action requires node")
	}
	// V9-07: reject empty/wrong node that is not the stalled ActiveNode — do not
	// clear the block without targeting a real member.
	if active := strings.TrimSpace(loop.ActiveNode); active != "" && nodeID != active {
		return AgentGraphSnapshot{}, false, fmt.Errorf("member_action node %q is not the stalled active node %q", nodeID, active)
	}

	// Find the child run for this node label.
	var child *interactiveRun
	var cohortID string
	for _, childID := range s.agentOrchestrator.listChildren(parentRunID) {
		s.mu.Lock()
		c := s.runs[childID]
		s.mu.Unlock()
		if c != nil && c.label == nodeID {
			child = c
			cohortID = c.flowCohortId
			break
		}
	}
	if child == nil {
		return AgentGraphSnapshot{}, false, fmt.Errorf("member_action: no child run for node %q", nodeID)
	}

	switch act {
	case "skip":
		// Mark FAILED, cancel live turn, append cohort entry, join if complete (I-11 / BUG-288 #6).
		if s.isFlowEngineDriven(parentRunID) {
			s.setFlowStepStatus(context.Background(), parentRunID, nodeID, StepStatusFailed)
		}
		var childSnap ProviderSessionState
		var haveChildSnap bool
		if child != nil {
			s.mu.Lock()
			cancel := child.turnCancel
			// BUG-288 R13-06: gate may still be running (turnCancel=nil, pending settle).
			// Invalidate the gate epoch and clear settle so a late gate-pass cannot
			// overwrite Failed with Completed.
			child.gateEpoch++
			if child.postTurnGateCancel != nil {
				gCancel := child.postTurnGateCancel
				child.postTurnGateCancel = nil
				child.pendingFlowGateSettle = false
				child.pendingFlowGateFinalMsg = ""
				child.pendingFlowGateOccurredAt = ""
				child.pendingFlowGateTurnID = ""
				child.pendingGateChangedFiles = nil
				s.mu.Unlock()
				gCancel()
				s.mu.Lock()
			} else if child.pendingFlowGateSettle {
				child.pendingFlowGateSettle = false
				child.pendingFlowGateFinalMsg = ""
				child.pendingFlowGateOccurredAt = ""
				child.pendingFlowGateTurnID = ""
				child.pendingGateChangedFiles = nil
			}
			child.turnCancel = nil
			child.turnInFlight = false
			child.status = RunStatusFailed
			child.agentStatus = string(RunStatusFailed)
			// V9-25: mark synthetic skip so late provider TurnFailed cannot re-append.
			child.cohortSkipConsumed = true
			// BUG-288 P1-14/P1-19 + R14-01: only set stalledSkipCause when a
			// turn-ctx cancel will reach finishTurn. Gate-in-flight skip
			// (cancel==nil) already stamps Failed directly — setting the flag
			// unconditionally leaked into a later Stop and misclassified it as
			// "skipped by user (stalled)".
			if cancel != nil {
				child.stalledSkipCause = true
			}
			// BUG-288 R13-08: durable child FAILED snapshot (not only parent).
			childSnap = sessionStateOf(child)
			haveChildSnap = true
			s.mu.Unlock()
			if cancel != nil {
				cancel()
			}
			if haveChildSnap {
				if err := s.persistProviderSession(childSnap); err != nil {
					log.Printf("[stall] persist skip child=%q: %v", child.id, err)
				}
			}
		}
		if cohortID != "" {
			provider := ""
			if child != nil {
				provider = string(child.providerKey)
			}
			s.agentOrchestrator.appendCohortResult(parentRunID, cohortID, cohortEntry{
				Label: nodeID, Provider: provider, Status: "failed", Err: "skipped by user (stalled)",
			})
			if s.agentOrchestrator.cohortComplete(parentRunID, cohortID) {
				entries := s.agentOrchestrator.drainCohort(parentRunID, cohortID)
				round := s.agentOrchestrator.loopStateFor(parentRunID).Round
				note := buildCohortNote(parentRunID, cohortID, entries, round)
				s.appendPendingAgentContext(parentRunID, note)
				s.mu.Lock()
				if parent := s.runs[parentRunID]; parent != nil {
					parent.lastCohortNote = note
				}
				s.mu.Unlock()
				// Clear stall block then reinvoke hub.
				s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
					st.Status = "running"
					st.BlockReason = ""
					st.GateReason = ""
					return st
				})
				if hubID := hubInlineNodeID(s.activeFlowNodesFor(parentRunID)); hubID != "" {
					s.setFlowStepStatus(context.Background(), parentRunID, hubID, StepStatusRunning)
				}
				go s.maybeAutoReinvokeHubWithNote(parentRunID, note)
			} else {
				// Still waiting other members — clear stall, stay running.
				s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
					st.Status = "running"
					st.BlockReason = ""
					st.GateReason = ""
					return st
				})
			}
		}
	case "retry":
		// Clear stall and re-arm the member (BUG-288 #5): cancel in-flight turn first
		// so startTurn is not rejected with turn_in_progress.
		s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
			st.Status = "running"
			st.BlockReason = ""
			st.GateReason = ""
			return st
		})
		if child != nil {
			prompt := fmt.Sprintf("[flow-engine] Retry: member %q was stalled; continue your work.", nodeID)
			s.mu.Lock()
			child.lastProviderEventAt = time.Now().UTC()
			runID := child.id
			stepID := child.stepID
			if stepID == "" {
				stepID = "chat-" + runID
			}
			inFlight := child.turnInFlight
			cancel := child.turnCancel
			gateBusy := child.postTurnGateCancel != nil || child.pendingFlowGateSettle
			// BUG-288 R13-07: post-turn gate window (inFlight + cancel==nil) must
			// park restart intent instead of force-clearing inFlight and startTurn
			// which fails with gate_in_progress and loses the retry.
			if (inFlight && cancel != nil) || gateBusy {
				var parentSnap ProviderSessionState
				var havePersist bool
				if parent := s.runs[parentRunID]; parent != nil {
					parent.pendingRestartRunID = runID
					parent.pendingRestartPrompt = prompt
					// BUG-288 R15-P0: generation for durable claim + idempotency.
					if parent.pendingRestartGen <= 0 {
						parent.pendingRestartGen = 1
					} else {
						parent.pendingRestartGen++
					}
					parentSnap = sessionStateOf(parent)
					havePersist = true
				}
				if cancel != nil {
					child.stalledRetryCause = true
					child.turnCancel = nil
				}
				// When only gate is busy, keep pendingFlowGateSettle; finishTurn
				// tail will consume pendingRestart after gate settles / cancels.
				s.mu.Unlock()
				if havePersist {
					if err := s.persistProviderSession(parentSnap); err != nil {
						log.Printf("[stall] persist restart intent parent=%q child=%q: %v (retry kept in-memory only; may be lost on crash)", parentRunID, runID, err)
					}
				}
				if cancel != nil {
					cancel()
				}
				break
			}
			child.turnInFlight = false
			s.mu.Unlock()
			go func(runID, stepID, prompt string) {
				if _, err := s.startTurn(runID, TurnInput{StepID: stepID, Prompt: prompt}, "", ""); err != nil {
					log.Printf("[stall] retry startTurn child=%q: %v", runID, err)
				}
			}(runID, stepID, prompt)
		}
	}
	snap := s.agentGraphSnapshot(parentRunID)
	s.emitAgentGraph(parentRunID, snap)
	go s.persistParentSession(parentRunID)
	// BUG-288 R13-09: one-shot stall timer is cleared on fire; re-arm so siblings
	// that were already silent still get member_stalled after skip/retry of another.
	s.maybeScheduleStallCheck(parentRunID)
	return snap, true, nil
}
