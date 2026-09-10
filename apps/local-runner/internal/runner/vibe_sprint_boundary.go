package runner

import (
	"context"
	"fmt"
	"strings"

	"flowpilot-runner/internal/workingmode"
)

// vibeSprintBoundaryReason is the loop BlockReason for the sprint-boundary
// Continue gate: a vibe-sprint audit completed and plan tasks remain. The
// operator confirms [ok] (or empty Continue) to start the next sprint in the
// same session, or [cancel]/Stop to settle the run done with the finished
// sprints standing. Without this gate the flow settles done after the first
// sprint and the remaining plan tasks never run (live run-223416 / CA-817
// run-635006: 1 sprint then done).
const vibeSprintBoundaryReason = "vibe_sprint_boundary"

// shortTaskName returns the file name of a workspace-relative task path for
// gate copy (full paths would overflow the blocked card).
func shortTaskName(p string) string {
	p = strings.TrimSpace(p)
	if i := strings.LastIndex(p, "/"); i >= 0 {
		return p[i+1:]
	}
	return p
}

// normalizeBoundaryNote maps the blocked-chip Retry payload ("continue") to
// empty so it never leaks into the next sprint prompt as an operator note.
func normalizeBoundaryNote(feedback string) string {
	if strings.EqualFold(strings.TrimSpace(feedback), "continue") {
		return ""
	}
	return strings.TrimSpace(feedback)
}

// vibeSprintPromptWithNote appends a short operator note to a sprint task
// prompt (boundary Continue with feedback). Pure: unit-tested directly.
func vibeSprintPromptWithNote(task, note string) string {
	task = strings.TrimSpace(task)
	if strings.TrimSpace(note) == "" {
		return task
	}
	return task + "\n\nOperator note:\n" + strings.TrimSpace(note)
}

// vibeSprintBoundaryResumeLabel is the PendingGate ResumeFrom text: the TUI
// renders "[GATE] Resume from <label>? Continue? [ok] [cancel]". The parked
// task is preferred over plan[index] so the label always matches the copy
// shown when the gate parked, even if Task files changed under the park.
func vibeSprintBoundaryResumeLabel(plan []string, index int, parkedTask string) string {
	name := strings.TrimSpace(parkedTask)
	total := len(plan)
	if name == "" && index >= 0 && index < total {
		name = plan[index]
	}
	if total <= 0 {
		return fmt.Sprintf("sprint (%s)", shortTaskName(name))
	}
	next := index + 1
	if next > total {
		next = total
	}
	if next < 1 {
		next = 1
	}
	return fmt.Sprintf("sprint %d/%d (%s)", next, total, shortTaskName(name))
}

// vibeSprintBoundaryReasonText is the blocked-card reason: which sprint just
// finished and which one Continue starts.
func vibeSprintBoundaryReasonText(plan []string, index int) string {
	total := len(plan)
	next := ""
	if index >= 0 && index < total {
		next = shortTaskName(plan[index])
	}
	prev := ""
	if index-1 >= 0 && index-1 < total {
		prev = shortTaskName(plan[index-1])
	}
	return fmt.Sprintf("Sprint %d/%d done (%s). Continue to sprint %d/%d (%s)?", index, total, prev, index+1, total, next)
}

// inVibeSprintTopology reports the inclusive vibe predicate shared with the
// audit settle call sites (isVibeWorkingMode): strict working mode, or a
// tdd+coder sprint topology. Call with s.mu held.
func inVibeSprintTopology(rs *interactiveRun) bool {
	if rs == nil {
		return false
	}
	if rs.workingMode == workingmode.Vibe {
		return true
	}
	return runHasFlowNode(rs, "tdd") && runHasFlowNode(rs, "coder")
}

// maybeParkVibeSprintBoundary parks the sprint-boundary Continue gate when a
// vibe-sprint audit just completed and plan tasks remain. It peeks the next
// sprint WITHOUT consuming it (takeNext increments on start) so ok/Continue
// and cancel see the same decision. The audit step is stamped DONE first
// (CA-818: a park must never strand its own step RUNNING). Returns true when
// parked; false leaves all existing settle behavior untouched (last sprint,
// budget, non-vibe, lock waiting, open cohort, sealed loop, already parked).
func (s *InteractiveService) maybeParkVibeSprintBoundary(ctx context.Context, parentRunID, auditNodeID string) bool {
	s.mu.Lock()
	rs := s.runs[parentRunID]
	if rs == nil || rs.parentRunID != "" || !inVibeSprintTopology(rs) ||
		rs.vibeAwaitingLock || len(rs.vibeTaskPlan) == 0 || rs.vibeSprintBoundaryPending {
		s.mu.Unlock()
		return false
	}
	if s.agentOrchestrator.hasOpenCohort(parentRunID) {
		// Barrier still open: let the join complete first (mirrors the
		// applyFlowControl soft-defer); the audit re-drives after.
		s.mu.Unlock()
		return false
	}
	budget := rs.vibeSprintBudget
	if budget <= 0 {
		budget = defaultVibeSprintBudget
	}
	if rs.vibeSprintIndex >= budget || rs.vibeSprintIndex >= len(rs.vibeTaskPlan) {
		s.mu.Unlock()
		return false
	}
	plan := append([]string(nil), rs.vibeTaskPlan...)
	index := rs.vibeSprintIndex
	rs.vibeSprintBoundaryPending = true
	rs.vibeSprintBoundaryTask = plan[index]
	s.mu.Unlock()

	if s.loopSealedForReinvoke(parentRunID) {
		// Stop/done won the race after the flag set (Stop clears it; this is
		// defense-in-depth for a concurrent seal): clear and do NOT flip a
		// sealed loop back to blocked.
		s.mu.Lock()
		if r := s.runs[parentRunID]; r != nil {
			r.vibeSprintBoundaryPending = false
			r.vibeSprintBoundaryTask = ""
		}
		s.mu.Unlock()
		return false
	}
	if s.isFlowEngineDriven(parentRunID) {
		s.setFlowStepStatus(ctx, parentRunID, auditNodeID, StepStatusDone)
	}
	reason := vibeSprintBoundaryReasonText(plan, index)
	snap := s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = vibeSprintBoundaryReason
		st.GateReason = reason
		st.ActiveNode = auditNodeID
		return st
	})
	s.parkFlowForAwaitingUser(parentRunID)
	s.emitAgentGraph(parentRunID, snap)
	go s.persistParentSession(parentRunID)
	s.flowDiagLog(parentRunID, "vibe_sprint_boundary_parked", "sprint done with plan tasks left; parked Continue gate",
		"done_sprints", index,
		"total_sprints", len(plan),
		"next_task", plan[index],
	)
	return true
}

// maybeReparkVibeSprintBoundary re-derives the boundary park on open/reopen:
// the pending flag is memory-only, but audit DONE + remaining plan is durable
// (steps in the workflow store, plan/index in the session). Sealed loops
// (Stop / already done) are respected — no resurrection. Starting a sprint
// reseeds its steps to PENDING, so a running next sprint (audit no longer
// DONE) never re-parks.
func (s *InteractiveService) maybeReparkVibeSprintBoundary(parentRunID string) {
	if strings.TrimSpace(parentRunID) == "" {
		return
	}
	if s.loopSealedForReinvoke(parentRunID) {
		return
	}
	if s.lookupFlowStepStatus(parentRunID, "audit") != StepStatusDone {
		return
	}
	s.maybeParkVibeSprintBoundary(context.Background(), parentRunID, "audit")
}

// continueVibeSprintBoundary consumes the boundary park and starts the next
// sprint (shared by gate ok and empty Continue). The take happens inside the
// same critical section as the consume, so a concurrent chain can neither
// double-start nor skip: only d.Start flips the loop to running. A plan that
// changed under the park is handled, never stranded: budget re-parks budget,
// a re-armed lock stays parked, an emptied plan settles done. Returns false
// when no park is pending.
func (s *InteractiveService) continueVibeSprintBoundary(parentRunID, note string) bool {
	s.mu.Lock()
	rs := s.runs[parentRunID]
	if rs == nil || !rs.vibeSprintBoundaryPending {
		s.mu.Unlock()
		return false
	}
	if s.loopSealedForReinvoke(parentRunID) {
		// Stop won the race after the park (Stop clears the flag; this is
		// defense-in-depth): swallow, never resurrect a sealed loop.
		rs.vibeSprintBoundaryPending = false
		rs.vibeSprintBoundaryTask = ""
		s.mu.Unlock()
		return true
	}
	// takeNext increments the index ONLY on Start; every other decision leaves
	// it untouched, so peeking here is side-effect free unless we start.
	d := s.takeNextVibeSprintLocked(rs)
	switch {
	case d.Start:
		prompt := vibeSprintPromptWithNote(d.Task, note)
		rs.vibeSprintBoundaryPending = false
		rs.vibeSprintBoundaryTask = ""
		if len(rs.activeFlowNodes) > 0 {
			rs.autoOrchestrate = true
		}
		s.mu.Unlock()
		s.releaseHubStopFenceForFollowUp(context.Background(), parentRunID)
		snap := s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
			st.Status = "running"
			st.BlockReason = ""
			st.GateReason = ""
			st.ActiveNode = ""
			return st
		})
		s.emitAgentGraph(parentRunID, snap)
		go s.persistParentSession(parentRunID)
		s.startTakenVibeSprint(parentRunID, prompt)
		return true
	case d.Budget:
		rs.vibeSprintBoundaryPending = false
		rs.vibeSprintBoundaryTask = ""
		s.mu.Unlock()
		s.parkVibeSprintBudget(parentRunID)
		return true
	case d.Locked:
		// A lock re-armed under the park: stay parked, operator can Stop.
		s.mu.Unlock()
		return true
	default:
		// Plan emptied under the park: settle done instead of stranding the
		// loop running with nothing to start.
		s.mu.Unlock()
		return s.declineVibeSprintBoundaryWithSummary(parentRunID, "Sprint plan emptied under the boundary gate; finished sprints stand.")
	}
}

// declineVibeSprintBoundary consumes the boundary park and settles the run
// done: finished sprints stand, remaining plan tasks stay untouched under
// requirements/08-Task/todo/ for a later run. Returns false when no park is
// pending or the settle is refused (gate stays parked).
func (s *InteractiveService) declineVibeSprintBoundary(parentRunID string) bool {
	return s.declineVibeSprintBoundaryWithSummary(parentRunID, "")
}

func (s *InteractiveService) declineVibeSprintBoundaryWithSummary(parentRunID, summary string) bool {
	s.mu.Lock()
	rs := s.runs[parentRunID]
	if rs == nil || !rs.vibeSprintBoundaryPending {
		s.mu.Unlock()
		return false
	}
	if s.agentOrchestrator.hasOpenCohort(parentRunID) {
		// Barrier still open: keep the gate parked rather than wedging
		// half-settled (mirrors the applyFlowControl soft-defer).
		s.mu.Unlock()
		return false
	}
	doneNum := rs.vibeSprintIndex
	total := len(rs.vibeTaskPlan)
	if doneNum > total {
		doneNum = total
	}
	task := rs.vibeSprintBoundaryTask
	rs.vibeSprintBoundaryPending = false
	rs.vibeSprintBoundaryTask = ""
	if strings.TrimSpace(summary) == "" {
		summary = fmt.Sprintf("Operator declined further sprints after sprint %d/%d; remaining plan tasks stay in requirements/08-Task/todo/.", doneNum, total)
	}
	s.mu.Unlock()
	if _, err := s.applyFlowControl(parentRunID, FlowControlInput{Status: "done", Summary: summary}); err != nil {
		// Settle refused (duplicate decision / terminal race): put the gate
		// back instead of stranding blocked-with-no-card.
		s.mu.Lock()
		if r := s.runs[parentRunID]; r != nil {
			r.vibeSprintBoundaryPending = true
			r.vibeSprintBoundaryTask = task
		}
		s.mu.Unlock()
		return false
	}
	return true
}

// startTakenVibeSprint starts a vibe-sprint flow for an already-taken prompt
// (the index was consumed by takeNextVibeSprintLocked under s.mu).
func (s *InteractiveService) startTakenVibeSprint(parentRunID, prompt string) {
	ref := workingmode.PackPrefix + vibeSprintFlowID
	s.startResolvedFlow(context.Background(), parentRunID, ref, prompt)
}
