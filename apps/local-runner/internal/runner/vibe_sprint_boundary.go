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
	cwd := rs.workspaceCwd
	planPeek := append([]string(nil), rs.vibeTaskPlan...)
	started := rs.vibeSprintIndex
	if rs.vibeSprintIndex >= budget || rs.vibeSprintIndex >= len(rs.vibeTaskPlan) {
		s.mu.Unlock()
		// Last sprint (or budget): tick that Task's DoD, never status=done.
		stampCompletedVibeTask(cwd, planPeek, started)
		return false
	}
	if rs.vibeSprintStartInFlight {
		// A Continue already consumed the gate and is starting that sprint;
		// a repark landing in this window must not park a second one.
		s.mu.Unlock()
		return false
	}
	plan := append([]string(nil), rs.vibeTaskPlan...)
	index := rs.vibeSprintIndex
	rs.vibeSprintBoundaryPending = true
	rs.vibeSprintBoundaryTask = plan[index]
	s.mu.Unlock()
	stampCompletedVibeTask(cwd, plan, index)

	if s.isFlowEngineDriven(parentRunID) {
		s.setFlowStepStatus(ctx, parentRunID, auditNodeID, StepStatusDone)
	}
	reason := vibeSprintBoundaryReasonText(plan, index)
	// Ownership + conditional mutate share one critical section with the run
	// lock (Stop takes the same lock; s.mu -> o.mu is the established order):
	// a Continue that consumed this flag while the park was in store I/O has
	// already advanced index / cleared pending, so this park aborts instead of
	// re-blocking the freshly started sprint behind a cardless gate.
	s.mu.Lock()
	r := s.runs[parentRunID]
	if r == nil || !r.vibeSprintBoundaryPending || r.vibeSprintBoundaryTask != plan[index] || r.vibeSprintIndex != index {
		// A pure index move (concurrent chain take) with our own flag still
		// set would leave a stale Continue card over a running sprint: drop
		// only our flag (task still ours, index already advanced).
		if r != nil && r.vibeSprintBoundaryPending && r.vibeSprintBoundaryTask == plan[index] && r.vibeSprintIndex != index {
			r.vibeSprintBoundaryPending = false
			r.vibeSprintBoundaryTask = ""
		}
		s.mu.Unlock()
		return false
	}
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
		// Sealed (or paused / foreign card) won the race: drop our own flag,
		// leave settle semantics alone.
		if r := s.runs[parentRunID]; r != nil {
			if r.vibeSprintBoundaryTask == plan[index] && r.vibeSprintIndex == index {
				r.vibeSprintBoundaryPending = false
				r.vibeSprintBoundaryTask = ""
			}
		}
		s.mu.Unlock()
		return false
	}
	if r := s.runs[parentRunID]; r != nil {
		// Boundary wins over any stale resume-confirm underneath: one gate
		// on screen, one router (matches runSnapshot/SubmitGateDecision).
		r.vibeResumeConfirm = false
		r.vibeResumeFromNode = ""
	}
	s.mu.Unlock()
	// Re-check sealed after the win: Stop may have landed between mutate
	// and the post-mutate side effects. Never emit a stale blocked graph
	// over a stopped loop.
	if s.loopSealedForReinvoke(parentRunID) {
		return true
	}
	// Final ownership check before side effects: if a Continue consumed the
	// gate after the mutate's critical section, the card is already gone and
	// parking the flow here would cancel the sprint it just started.
	s.mu.Lock()
	stillOwned := false
	if r := s.runs[parentRunID]; r != nil {
		stillOwned = r.vibeSprintBoundaryPending && r.vibeSprintBoundaryTask == plan[index] && r.vibeSprintIndex == index
	}
	s.mu.Unlock()
	if !stillOwned {
		return true
	}
	// Task-351 (CP-62 P-6): the sprint that just reached the boundary IS
	// finished — write its handoff here, on the STANDARD park path, pinned to
	// the just-finished index (a concurrent Continue advancing the cursor
	// must not attribute sprint N's data to handoff N+1). Best-effort: a
	// write failure only degrades the next sprint's context.
	if r := s.runs[parentRunID]; r != nil {
		s.emitSprintHandoffAt(r, index)
	}
	s.parkFlowForAwaitingUser(parentRunID)
	s.emitAgentGraph(parentRunID, snap)
	// Persist synchronously: an async goroutine here could outlive a later
	// decline/Stop write and append a stale snapshot after it.
	s.persistParentSession(parentRunID)
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
		rs.vibeAwaitingLock || len(rs.vibeTaskPlan) == 0 || rs.vibeSprintBoundaryPending ||
		rs.vibeSprintStartInFlight {
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
		// too — a later reopen after unpause retries the offer (repark is
		// only invoked on open/reopen, never on unpause itself).
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
		// Task-351 (CP-62 P-6): the boundary-continue start is the STANDARD
		// next-sprint entry — it must carry the previous sprint's handoff
		// exactly like maybeStartNextVibeSprint does (d.Sprint is already
		// N+1 after takeNext; the handoff read is sprint N = index-1).
		// Missing file → empty (graceful fallback), never blocks the start.
		if handoff := previousSprintHandoffContext(rs.workspaceCwd, d.Sprint); handoff != "" {
			prompt = prompt + "\n\n" + handoff
		}
		takenTask := d.Task
		prevDeclined := rs.vibeSprintBoundaryDeclined
		rs.vibeSprintBoundaryPending = false
		rs.vibeSprintBoundaryTask = ""
		rs.vibeSprintBoundaryDeclined = false
		rs.vibeSprintStartGen++
		startGen := rs.vibeSprintStartGen
		rs.vibeSprintStartInFlight = true
		// The operator is driving the flow again: a stale resume-confirm
		// must not pop a second card over the sprint we are starting.
		rs.vibeResumeConfirm = false
		rs.vibeResumeFromNode = ""
		if len(rs.activeFlowNodes) > 0 {
			rs.autoOrchestrate = true
		}
		s.mu.Unlock()
		s.releaseHubStopFenceForFollowUp(context.Background(), parentRunID)
		// Conditional mutate: a concurrent Stop/done between unlock and
		// here must win — never flip sealed back to running. The index is
		// rolled back, the durable decline marker restored, and the gate
		// restored only while the loop is still resumable (no ghost card on
		// a stopped/done run).
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
			s.rollbackTakenVibeSprintLocked(parentRunID, takenTask, prevDeclined)
			s.mu.Unlock()
			return true
		}
		if s.loopSealedForReinvoke(parentRunID) {
			// Sealed between mutate and emit: do not emit a stale running
			// graph over Stop/done, and give the consumed task back.
			s.mu.Lock()
			if r := s.runs[parentRunID]; r != nil {
				r.vibeSprintIndex--
				r.vibeSprintStartInFlight = false
			}
			s.mu.Unlock()
			return true
		}
		s.emitAgentGraph(parentRunID, snap)
		s.persistParentSession(parentRunID)
		s.startTakenVibeSprint(parentRunID, prompt, takenTask, prevDeclined, startGen)
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
	prevDeclined := rs.vibeSprintBoundaryDeclined
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
		// stranding blocked-with-no-card, and restore whatever decline
		// marker existed before this call (never invent a decline).
		s.mu.Lock()
		if r := s.runs[parentRunID]; r != nil {
			r.vibeSprintBoundaryPending = true
			r.vibeSprintBoundaryTask = task
			r.vibeSprintBoundaryDeclined = prevDeclined
		}
		s.mu.Unlock()
		return false
	}
	return true
}

// rollbackTakenVibeSprintLocked undoes a consumed boundary take that could not
// start (Stop/done won the race). The task goes back so it is never skipped,
// the previous decline marker is restored, and the gate is re-shown only while
// the loop is still resumable — a sealed (stopped/done) run must not get a
// ghost actionable card back. Caller must hold s.mu (s.mu -> o.mu order).
func (s *InteractiveService) rollbackTakenVibeSprintLocked(parentRunID, takenTask string, prevDeclined bool) {
	r := s.runs[parentRunID]
	if r == nil {
		return
	}
	r.vibeSprintIndex--
	r.vibeSprintBoundaryDeclined = prevDeclined
	r.vibeSprintStartInFlight = false
	if s.loopSealedForReinvoke(parentRunID) {
		return
	}
	r.vibeSprintBoundaryPending = true
	r.vibeSprintBoundaryTask = takenTask
	// Re-park the loop so the restored gate really owns the next decision:
	// a resolve-failed start would otherwise leave it running with a pending
	// card the engine could advance past.
	s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
		if st.Status == "stopped" || st.Status == "done" || st.Status == "paused" {
			return st
		}
		st.Status = "blocked"
		st.BlockReason = vibeSprintBoundaryReason
		st.GateReason = "Sprint did not start. Continue to retry the next sprint?"
		return st
	})
}

// startTakenVibeSprint starts a vibe-sprint flow for an already-taken prompt
// (the index was consumed by takeNextVibeSprintLocked under s.mu). The
// start-in-flight flag (gen-stamped) is held for the whole spawn so
// repark/chain cannot double-start, and a Stop that won after the continue
// mutate aborts the spawn instead of orphaning a just-started child. If the
// spawn produced nothing live (sealed abort, resolve failure, refused entry,
// aborted child turn) the take is rolled back so a later offer cannot
// silently skip the task.
func (s *InteractiveService) startTakenVibeSprint(parentRunID, prompt, takenTask string, prevDeclined bool, startGen int64) {
	defer func() {
		s.mu.Lock()
		if r := s.runs[parentRunID]; r != nil && r.vibeSprintStartGen == startGen {
			r.vibeSprintStartInFlight = false
		}
		s.mu.Unlock()
	}()
	s.mu.Lock()
	rs := s.runs[parentRunID]
	if rs == nil || !rs.vibeSprintStartInFlight || rs.vibeSprintStartGen != startGen {
		s.mu.Unlock()
		return
	}
	if s.loopSealedForReinvoke(parentRunID) {
		// Stop/done landed between the continue mutate and here: give the
		// task back (no gate on a sealed loop) and spawn nothing.
		s.rollbackTakenVibeSprintLocked(parentRunID, takenTask, prevDeclined)
		s.mu.Unlock()
		return
	}
	cwd := rs.workspaceCwd
	s.mu.Unlock()
	abandonActiveFrozenContractsForRun(cwd, parentRunID, "vibe-sprint next task")
	stampVibeTaskInProgress(cwd, takenTask)
	before := make(map[string]struct{})
	for _, cid := range s.agentOrchestrator.listChildren(parentRunID) {
		before[cid] = struct{}{}
	}
	ref := workingmode.PackPrefix + vibeSprintFlowID
	s.startResolvedFlow(context.Background(), parentRunID, ref, prompt)
	// Only a new, non-cancelled child proves the sprint actually started: a
	// spawn aborted by Stop registers its child and then cancels it, so a
	// plain child count would mistake that for a start.
	spawned := false
	for _, cid := range s.agentOrchestrator.listChildren(parentRunID) {
		if _, seen := before[cid]; seen {
			continue
		}
		s.mu.Lock()
		child := s.runs[cid]
		cancelled := child == nil || child.status == RunStatusCancelled
		s.mu.Unlock()
		if !cancelled {
			spawned = true
			break
		}
	}
	if !spawned {
		s.mu.Lock()
		s.rollbackTakenVibeSprintLocked(parentRunID, takenTask, prevDeclined)
		s.mu.Unlock()
	}
}
