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
// (CA-818: a park must never strand its own step RUNNING). allowSealed is
// only true for the reopen offer below (a vetted done-loop re-offer); live
// audit paths pass false so a concurrent Stop/done can never be flipped
// back to blocked. Returns true when parked; false leaves all existing
// settle behavior untouched (last sprint, budget, non-vibe, lock waiting,
// open cohort, sealed loop, already parked).
func (s *InteractiveService) maybeParkVibeSprintBoundary(ctx context.Context, parentRunID, auditNodeID string, allowSealed bool) bool {
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

	if s.isFlowEngineDriven(parentRunID) {
		s.setFlowStepStatus(ctx, parentRunID, auditNodeID, StepStatusDone)
	}
	reason := vibeSprintBoundaryReasonText(plan, index)
	// Conditional mutate under the orchestrator mutex (same mutex stop()
	// takes): a concurrent Stop/done wins the race instead of being
	// overwritten — check-and-write is atomic, no TOCTOU. Live audits also
	// refuse paused loops and foreign blocked cards (mirror the repark
	// guard): only stopped/done (sealed), paused, or another gate's card
	// can refuse the park.
	parked := false
	snap := s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
		if st.Status == "stopped" || (!allowSealed && st.Status == "done") {
			return st
		}
		if st.Status == "paused" {
			return st
		}
		if st.Status == "blocked" && st.BlockReason != "" && st.BlockReason != vibeSprintBoundaryReason {
			return st
		}
		st.Status = "blocked"
		st.BlockReason = vibeSprintBoundaryReason
		st.GateReason = reason
		st.ActiveNode = auditNodeID
		parked = true
		return st
	})
	if !parked {
		// Sealed (or paused / foreign card) won the race: drop the flag,
		// leave settle semantics alone.
		s.mu.Lock()
		if r := s.runs[parentRunID]; r != nil {
			// Only clear our own flag to avoid clobbering a newer park.
			if r.vibeSprintBoundaryTask == plan[index] {
				r.vibeSprintBoundaryPending = false
				r.vibeSprintBoundaryTask = ""
			}
		}
		s.mu.Unlock()
		return false
	}
	// Re-check sealed after the win: Stop may have landed between mutate
	// and the post-mutate side effects. Never emit a stale blocked graph
	// over a stopped loop.
	if s.loopSealedForReinvoke(parentRunID) {
		return true
	}
	s.mu.Lock()
	if r := s.runs[parentRunID]; r != nil {
		// Boundary wins over any stale resume-confirm underneath: one gate
		// on screen, one router (matches runSnapshot/SubmitGateDecision).
		r.vibeResumeConfirm = false
		r.vibeResumeFromNode = ""
	}
	s.mu.Unlock()
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
// (steps in the workflow store, plan/index in the session). A loop sealed by
// Stop stays sealed; a loop sealed by silent-done (audit done, tasks left,
// never declined — e.g. runs settled before this gate existed) gets one
// Continue offer. Starting a sprint reseeds its steps to PENDING, so a
// running next sprint (audit no longer DONE) never re-parks.
func (s *InteractiveService) maybeReparkVibeSprintBoundary(parentRunID string) {
	if strings.TrimSpace(parentRunID) == "" {
		return
	}
	if s.lookupFlowStepStatus(parentRunID, "audit") != StepStatusDone {
		return
	}
	s.mu.Lock()
	rs := s.runs[parentRunID]
	if rs == nil || rs.parentRunID != "" || !inVibeSprintTopology(rs) ||
		rs.vibeAwaitingLock || len(rs.vibeTaskPlan) == 0 || rs.vibeSprintBoundaryPending {
		s.mu.Unlock()
		return
	}
	if rs.vibeSprintBoundaryDeclined {
		// An explicit decline suppresses every reopen offer, regardless of
		// loop state — the marker exists exactly for this.
		s.mu.Unlock()
		return
	}
	plan := append([]string(nil), rs.vibeTaskPlan...)
	index := rs.vibeSprintIndex
	budget := rs.vibeSprintBudget
	if budget <= 0 {
		budget = defaultVibeSprintBudget
	}
	awaitingLock := rs.vibeAwaitingLock
	s.mu.Unlock()
	loop := s.agentOrchestrator.loopStateFor(parentRunID)
	switch loop.Status {
	case "stopped", "paused":
		// Stop wins, even with tasks left. An explicit user pause is kept
		// too — the next reopen after unpause retries the offer.
		return
	case "done":
		// Offer only to silently-settled runs: tasks remain AND the operator
		// never declined. Finished plans stay done.
		if !decideNextVibeSprint(awaitingLock, plan, index, budget).Start {
			return
		}
		s.maybeParkVibeSprintBoundary(context.Background(), parentRunID, "audit", true)
	default:
		// Never steal a card owned by another gate: only re-derive onto a
		// clean loop or our own boundary reason.
		if loop.Status == "blocked" && loop.BlockReason != vibeSprintBoundaryReason {
			return
		}
		s.maybeParkVibeSprintBoundary(context.Background(), parentRunID, "audit", false)
	}
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
		takenTask := d.Task
		rs.vibeSprintBoundaryPending = false
		rs.vibeSprintBoundaryTask = ""
		rs.vibeSprintBoundaryDeclined = false
		if len(rs.activeFlowNodes) > 0 {
			rs.autoOrchestrate = true
		}
		s.mu.Unlock()
		s.releaseHubStopFenceForFollowUp(context.Background(), parentRunID)
		// Conditional mutate: a concurrent Stop/done between unlock and
		// here must win — never flip sealed back to running, and roll the
		// consumed index back so no task is skipped and the gate survives.
		started := false
		snap := s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
			if st.Status == "stopped" || st.Status == "done" {
				return st
			}
			st.Status = "running"
			st.BlockReason = ""
			st.GateReason = ""
			st.ActiveNode = ""
			started = true
			return st
		})
		if !started {
			s.mu.Lock()
			if r := s.runs[parentRunID]; r != nil {
				r.vibeSprintIndex--
				r.vibeSprintBoundaryPending = true
				r.vibeSprintBoundaryTask = takenTask
			}
			s.mu.Unlock()
			return true
		}
		if s.loopSealedForReinvoke(parentRunID) {
			// Sealed between mutate and emit: do not emit a stale running
			// graph over Stop/done.
			return true
		}
		s.emitAgentGraph(parentRunID, snap)
		go s.persistParentSession(parentRunID)
		s.startTakenVibeSprint(parentRunID, prompt)
		return true
	case d.Budget:
		rs.vibeSprintBoundaryPending = false
		rs.vibeSprintBoundaryTask = ""
		s.mu.Unlock()
		if s.loopSealedForReinvoke(parentRunID) {
			return true
		}
		s.parkVibeSprintBudget(parentRunID)
		return true
	case d.Locked:
		// A lock re-armed under the park: stay parked, operator can Stop.
		s.mu.Unlock()
		return true
	default:
		// Plan emptied under the park: settle done instead of stranding the
		// loop running with nothing to start — without recording a decline
		// (the operator said go; a restored plan must stay offerable).
		s.mu.Unlock()
		return s.declineVibeSprintBoundaryWithSummary(parentRunID, "Sprint plan emptied under the boundary gate; finished sprints stand.", false)
	}
}

// declineVibeSprintBoundary consumes the boundary park and settles the run
// done: finished sprints stand, remaining plan tasks stay untouched under
// requirements/08-Task/todo/ for a later run. Returns false when no park is
// pending or the settle is refused (gate stays parked).
func (s *InteractiveService) declineVibeSprintBoundary(parentRunID string) bool {
	return s.declineVibeSprintBoundaryWithSummary(parentRunID, "", true)
}

func (s *InteractiveService) declineVibeSprintBoundaryWithSummary(parentRunID, summary string, markDeclined bool) bool {
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
	// Durable decline (operator cancel only — the emptied-plan auto-settle
	// passes markDeclined=false): reopening must not re-offer Continue for
	// a declined run, only for silently-settled ones.
	rs.vibeSprintBoundaryDeclined = markDeclined
	if strings.TrimSpace(summary) == "" {
		summary = fmt.Sprintf("Operator declined further sprints after sprint %d/%d; remaining plan tasks stay in requirements/08-Task/todo/.", doneNum, total)
	}
	s.mu.Unlock()
	res, err := s.applyFlowControl(parentRunID, FlowControlInput{Status: "done", Summary: summary})
	if err != nil || (res.NextAction == "rejected_cohort_incomplete") {
		// Settle refused (duplicate decision / terminal race) or soft-
		// deferred on an open cohort: put the gate back instead of
		// stranding blocked-with-no-card (and never record a decline).
		s.mu.Lock()
		if r := s.runs[parentRunID]; r != nil {
			r.vibeSprintBoundaryPending = true
			r.vibeSprintBoundaryTask = task
			r.vibeSprintBoundaryDeclined = false
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
