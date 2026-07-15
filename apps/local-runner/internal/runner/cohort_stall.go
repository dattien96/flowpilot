package runner

import (
	"context"
	"fmt"
	"log"
	"strings"
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

// maybeScheduleStallCheck arms a delayed stall sweep so silent cohort stalls
// surface without waiting for the user to press Continue (Codex review
// Important #2). Safe to call often; checkAndBlockStalledMembers is idempotent
// when the loop is already blocked/done.
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
	// +1s buffer past the policy window so age >= timeout when the timer fires.
	time.AfterFunc(timeout+time.Second, func() {
		s.checkAndBlockStalledMembers(parentRunID)
	})
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
		hasGate := child.pendingApprovalID != "" || child.pendingQuestionID != ""
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

	switch act {
	case "skip":
		// Mark FAILED, append cohort entry, join if complete (I-11).
		if s.isFlowEngineDriven(parentRunID) {
			s.setFlowStepStatus(context.Background(), parentRunID, nodeID, StepStatusFailed)
		}
		if cohortID != "" {
			provider := ""
			if child != nil {
				provider = string(child.providerKey)
			}
			s.agentOrchestrator.appendCohortResult(parentRunID, cohortID, cohortEntry{
				Label: nodeID, Provider: provider, Status: "failed", Err: "skipped by user (stalled)",
			})
			if child != nil {
				s.mu.Lock()
				child.status = RunStatusFailed
				child.agentStatus = string(RunStatusFailed)
				child.turnInFlight = false
				s.mu.Unlock()
			}
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
		// Clear stall and re-arm the member if we can reinvoke.
		s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
			st.Status = "running"
			st.BlockReason = ""
			st.GateReason = ""
			return st
		})
		if child != nil {
			s.mu.Lock()
			child.lastProviderEventAt = time.Now().UTC()
			s.mu.Unlock()
			// Best-effort reinvoke: start a new turn on the same child run.
			prompt := fmt.Sprintf("[flow-engine] Retry: member %q was stalled; continue your work.", nodeID)
			go func(runID, prompt string) {
				if _, err := s.startTurn(runID, TurnInput{StepID: "chat-" + runID, Prompt: prompt}, "", ""); err != nil {
					log.Printf("[stall] retry startTurn child=%q: %v", runID, err)
				}
			}(child.id, prompt)
		}
	}
	snap := s.agentGraphSnapshot(parentRunID)
	s.emitAgentGraph(parentRunID, snap)
	go s.persistParentSession(parentRunID)
	return snap, true, nil
}
