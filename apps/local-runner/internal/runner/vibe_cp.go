package runner

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/changecontract"
	"flowpilot-runner/internal/workingmode"
)

const (
	defaultVibeSprintBudget = 8
	// defaultVibeTaskRoundCap is the default max round cap per Task (vibe
	// user contract): a CP's total cap is len(taskPlan) × 20 — CP-03 with 10
	// tasks has a 200-round total. It mirrors the vibe-sprint pack policy.cap
	// and also scales the sprint-count budget so every detected task fires.
	defaultVibeTaskRoundCap   = 20
	vibeIngestFlowID          = "vibe-ingest"
	vibeCpIngestFlowID        = "vibe-cp-ingest"
	vibeTasksFlowID           = "vibe-tasks"
	vibeSprintFlowID          = "vibe-sprint"
	vibeOwnerDebateFlowID     = "vibe-owner-debate"
	vibeCpLockNodeID          = "cp_lock"
	vibeSSLockNodeID          = "ss_lock"
	vibeSSValidatorNodeID     = "ss_validator"
	vibeCPValidatorNodeID     = "cp_validator"
	vibeTaskSlicerNodeID      = "task_slicer"
	vibeTaskPlanReaderNodeID  = "task_plan_reader"
	vibeSprintSlicerNodeID    = "sprint_slicer"
	vibeCpWriterNodeID        = "cp_writer"
	vibeDebateSynthesisNodeID = "debate_synthesis"
	// vibeCompletionPlanComplete is the AgentLoopState.CompletionKind stamped
	// when a vibe run settles with its sprint plan fully consumed — the
	// typed "plan_complete" terminal that distinguishes "all planned tasks
	// delivered" from a wedged slicer park (R.2-2).
	vibeCompletionPlanComplete = "plan_complete"
)

// inferPackFlowRefFromNodes corrects a stale ChatFlowRef after overlay
// (live run-220036: nodes are vibe-sprint, persisted ref stayed vibe-cp-ingest).
// Unknown topologies keep fallback.
func inferPackFlowRefFromNodes(nodes []agentpack.FlowNode, fallback string) string {
	ids := make(map[string]bool, len(nodes))
	for _, n := range nodes {
		if id := strings.TrimSpace(n.ID); id != "" {
			ids[id] = true
		}
	}
	pick := func(id string) string {
		return workingmode.PackPrefix + id
	}
	switch {
	case ids[vibeAdoptSelectNodeID]:
		return pick(vibeAdoptFlowID)
	case ids["owner_1"] && ids["owner_2"]:
		return pick(vibeOwnerDebateFlowID)
	case ids["tdd"] && ids["coder"]:
		// Task-459: adopt-sprint mounts share the vibe-sprint node set — the
		// topology alone cannot tell them apart, so an explicit adopt ref is
		// authoritative (a stale non-adopt ref still corrects to vibe-sprint).
		if workingmode.BareFlowID(fallback) == vibeAdoptSprintFlowID {
			return fallback
		}
		return pick(vibeSprintFlowID)
	// CP-90: vibe-tasks shares cp_reader/cp_lock with vibe-cp-ingest — the
	// reader node is the only discriminator, so it must win first.
	case ids[vibeTaskPlanReaderNodeID]:
		return pick(vibeTasksFlowID)
	case ids[vibeTaskSlicerNodeID] || ids[vibeCpLockNodeID] || ids["cp_reader"]:
		return pick(vibeCpIngestFlowID)
	case ids[vibeSSLockNodeID] || ids["ingest_reader"]:
		return pick(vibeIngestFlowID)
	default:
		return fallback
	}
}

// vibeInheritsSessionModel is true when the node must not pick the generic
// Flow: Doc Writer role model (gpt-5.4). Empty → spawn inherits the run's
// session model. Per-node Settings (flow-scoped step row) still win.
func vibeInheritsSessionModel(nodeID string) bool {
	switch strings.TrimSpace(nodeID) {
	case vibeCpWriterNodeID, vibeTaskSlicerNodeID, vibeTaskPlanReaderNodeID:
		return true
	default:
		return false
	}
}

// vibeChainHandoffNode reports whether onVibeCpNodeDone consumes the node's
// completion into a chain dispatch that can swap activeFlowNodes under it —
// the sprint chain (slicer/reader → vibe-sprint), the cp-ingest chain
// (cp_writer → vibe-cp-ingest), or the post-debate topology restore
// (debate_synthesis). When the swap already landed, the completed node is
// legitimately absent from the active topology and tryAdvanceFlowFromNode
// must claim it rather than reinvoke the hub (CA-1087, live run-2302).
func vibeChainHandoffNode(nodeID string) bool {
	switch strings.TrimSpace(nodeID) {
	case vibeCpWriterNodeID, vibeDebateSynthesisNodeID,
		vibeTaskSlicerNodeID, vibeSprintSlicerNodeID, vibeTaskPlanReaderNodeID,
		vibeAdoptSelectNodeID:
		return true
	default:
		return false
	}
}

// vibeLinearWriterNode is a spawnable vibe node whose --done--> done is a
// linear chain, not a review-cohort feeding the previous hub.inline.
func vibeLinearWriterNode(nodeID string) bool {
	switch strings.TrimSpace(nodeID) {
	case vibeCpWriterNodeID, vibeTaskSlicerNodeID, vibeTaskPlanReaderNodeID:
		return true
	default:
		return false
	}
}

func vibeLinearWriterCohort(entries []cohortEntry) bool {
	saw := false
	for _, e := range entries {
		if e.Status != "completed" {
			continue
		}
		if !vibeLinearWriterNode(e.Label) {
			return false
		}
		saw = true
	}
	return saw
}

type vibeSprintDecision struct {
	Start  bool
	Budget bool
	Done   bool
	Locked bool
	Task   string
	// Sprint is the 1-based sprint number of the sprint being started (CP-62
	// P-6, Task-342): the previous handoff file is sprint-1.
	Sprint int
}

// vibeSprintBudgetForPlan scales the sprint-count budget to the detected task
// plan: len(plan) × defaultVibeTaskRoundCap (20 rounds per Task), so a 10-task
// CP gets a 200 total cap and every detected task triggers. The fixed default
// remains the floor for empty/pre-plan states.
func vibeSprintBudgetForPlan(taskCount int) int {
	if taskCount <= 0 {
		return defaultVibeSprintBudget
	}
	if budget := taskCount * defaultVibeTaskRoundCap; budget > defaultVibeSprintBudget {
		return budget
	}
	return defaultVibeSprintBudget
}

func decideNextVibeSprint(awaitingLock bool, tasks []string, index, budget int) vibeSprintDecision {
	if awaitingLock {
		return vibeSprintDecision{Locked: true}
	}
	if budget <= 0 {
		budget = vibeSprintBudgetForPlan(len(tasks))
	}
	if index >= budget {
		return vibeSprintDecision{Budget: true}
	}
	if index >= len(tasks) {
		return vibeSprintDecision{Done: true}
	}
	return vibeSprintDecision{Start: true, Task: tasks[index]}
}

func (s *InteractiveService) vibeSprintStartBlocked(parentRunID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	rs := s.runs[parentRunID]
	// BUG-594 (live run-139670): the debate claim also blocks a sprint
	// mount — a sprint start under it clobbered the mounted debate graph.
	// Refusing here (not deferring) is correct: every sprint-start caller
	// owns a rollback/re-fire path — takeNext's index is returned, the
	// boundary take rolls back, the chain re-fires post-restore.
	return rs != nil && (rs.vibeAwaitingLock || len(rs.vibeParkedNodes) > 0)
}

func (s *InteractiveService) takeNextVibeSprintLocked(rs *interactiveRun) vibeSprintDecision {
	d := decideNextVibeSprint(rs.vibeAwaitingLock, rs.vibeTaskPlan, rs.vibeSprintIndex, rs.vibeSprintBudget)
	if d.Start {
		rs.vibeSprintIndex++
		d.Sprint = rs.vibeSprintIndex
		// Task-350: cross-sprint verified-state reset. The handoff/coverage
		// sources belong to the sprint that just ended — carrying verdict
		// rows, tampered-test paths, or a stale decision card (and its
		// choice) into sprint N+1 misattributes them (CP-49 hard ceiling).
		// The AC cache is keyed to the governing task doc, which advances
		// with the sprint, so it must be re-resolved too.
		rs.lastFlowVerdicts = nil
		rs.lastTamperedTestPaths = nil
		rs.decisionCard = nil
		rs.decisionCardChosen = ""
		rs.expectedACsCache = nil
		rs.expectedACsResolved = false
		// BUG-595: a new sprint takes a fresh owner-debate remediation
		// budget — the mount counter must not carry over (live run-100368
		// looped seven debates on one child).
		rs.vibeDebateMounts = 0
		rs.vibeDebateMountsByEntity = nil
		// Every Task owns its own defaultVibeTaskRoundCap budget (vibe
		// contract): the shared loop counter must reset with the take or
		// sprint N's consumed rounds starve sprint N+1 (live run-262417
		// opened Task-042 at 7/20 after Task-041 burned 7). ExtendCount
		// resets with it so the mount seed re-applies the policy cap instead
		// of carrying the prior task's extension; NegotiationRound is the
		// same phase-scoped budget class. Locked/Budget/Done takes return
		// before here and never touch the in-flight sprint's budget.
		s.agentOrchestrator.mutateLoop(rs.id, func(st AgentLoopState) AgentLoopState {
			st.Round = 0
			st.ExtendCount = 0
			st.NegotiationRound = 0
			return st
		})
	}
	return d
}

// vibePlanDrainedLocked reports whether every task in the run's sprint plan
// has been started and finished — the cursor advanced past the plan. Callers
// must hold s.mu. An empty plan is NOT drained: a run that never sliced has
// nothing delivered (that is the slicer-failure shape, not plan_complete).
func vibePlanDrainedLocked(rs *interactiveRun) bool {
	return rs != nil && len(rs.vibeTaskPlan) > 0 && rs.vibeSprintIndex >= len(rs.vibeTaskPlan)
}

// settleVibePlanComplete lands the run on the typed terminal state
// (R.2-2): loop done + CompletionKind "plan_complete", run status completed.
// It exists so "todo/ drained, every sprint delivered" is distinguishable
// from a wedged slicer park — a plan-exhausted flow used to either silent-
// return (maybeStartNextVibeSprint swallowed d.Done) or park
// flow_parked_awaiting_user looking identical to a failure, with Continue
// swallowed. Conditional like every other settle seam: a stopped/paused loop
// keeps its operator verdict — the loop mutate runs BEFORE the run-status
// flip so a Stop landing in the window is never overwritten to completed.
func (s *InteractiveService) settleVibePlanComplete(parentRunID, summary string) {
	s.mu.Lock()
	rs := s.runs[parentRunID]
	if rs == nil || !vibePlanDrainedLocked(rs) {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	// Operator verdict wins: the loop settle is conditional BEFORE any
	// run-status flip — markFlowRunComplete only runs once the loop actually
	// sealed, so a Stop landing in the window is never overwritten to
	// completed (same contract as parkVibeSprintBudget's conditional mutate).
	settled := false
	snap := s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
		if st.Status == "stopped" || st.Status == "paused" {
			return st
		}
		if st.Status == "done" && st.CompletionKind == vibeCompletionPlanComplete {
			return st // already stamped — re-settle is a no-op
		}
		st.Status = "done"
		st.BlockReason = ""
		st.GateReason = ""
		st.OpenIssues = 0
		st.CompletionKind = vibeCompletionPlanComplete
		settled = true
		return st
	})
	if !settled {
		return
	}
	if s.isFlowEngineDriven(parentRunID) {
		s.markFlowRunComplete(context.Background(), parentRunID)
	} else {
		s.settleParentRunOnFlowDone(parentRunID)
	}
	s.appendPendingAgentContext(parentRunID, strings.TrimSpace("Vibe sprint plan complete — all planned tasks delivered. "+summary))
	s.emitAgentGraph(parentRunID, snap)
	s.persistParentSession(parentRunID)
	s.flowDiagLog(parentRunID, "vibe_plan_complete", "vibe sprint plan drained; run settled plan_complete")
}

func (s *InteractiveService) parkVibeSprintBudget(parentRunID string) {
	// Conditional mutate: a Stop/done landing between the caller's sealed
	// check and here must win instead of being overwritten to blocked/budget.
	parked := false
	s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
		if st.Status == "stopped" || st.Status == "done" {
			return st
		}
		st.Status = "blocked"
		st.BlockReason = "budget"
		st.GateReason = "vibe total-sprint budget exceeded"
		parked = true
		return st
	})
	if !parked {
		return
	}
	s.mu.Lock()
	// Re-check sealed before flipping run status: a Stop landing between the
	// mutate and here must not be relabeled WaitingUserApr.
	if rs := s.runs[parentRunID]; rs != nil && !s.loopSealedForReinvoke(parentRunID) {
		rs.status = RunStatusWaitingUserApr
		rs.agentStatus = string(RunStatusWaitingUserApr)
	}
	s.mu.Unlock()
}

func (s *InteractiveService) maybeStartNextVibeSprint(parentRunID string) {
	s.mu.Lock()
	rs := s.runs[parentRunID]
	// A boundary Continue that is mid-start already owns the next sprint, and
	// a pending boundary gate owns the next decision — a stray slicer/chain
	// completion in either window must not take a second index.
	// BUG-594: the debate claim also fences the take — the index must NOT be
	// consumed under it (the take rolls forward with no mount possible and
	// the re-fire post-restore takes a SECOND index, skipping a task).
	if rs == nil || rs.vibeSprintStartInFlight || rs.vibeSprintBoundaryPending ||
		len(rs.vibeParkedNodes) > 0 {
		s.mu.Unlock()
		return
	}
	d := s.takeNextVibeSprintLocked(rs)
	cwd := ""
	adopt := false
	if rs != nil {
		cwd = rs.workspaceCwd
		adopt = vibeSprintIsAdopt(rs)
	}
	if d.Start {
		// A new sprint start supersedes any earlier boundary decline: the
		// run is active again and must stay offerable at its next boundary.
		rs.vibeSprintBoundaryDeclined = false
	}
	s.mu.Unlock()
	if d.Start {
		abandonActiveFrozenContractsForRun(cwd, parentRunID, "vibe-sprint next task")
		// Task-459: an adopt sprint verifies pre-existing work — it does not
		// author, so the doc's status must not flip back to in_progress.
		if !adopt {
			stampVibeTaskInProgress(cwd, d.Task)
		}
	}
	if d.Locked {
		log.Printf("[vibe-cp] skip vibe-sprint; cp_lock still waiting run=%s", parentRunID)
		return
	}
	if d.Budget {
		s.parkVibeSprintBudget(parentRunID)
		return
	}
	if !d.Start {
		if d.Done {
			// R.2-2: the chain decision's typed terminal was previously
			// swallowed here — the loop stayed "running" forever after the
			// last sprint delivered. Land it as plan_complete so "nothing
			// left to sprint" is distinguishable from a wedged slicer.
			s.settleVibePlanComplete(parentRunID, "sprint chain exhausted")
		}
		return
	}
	ref := vibeSprintFlowRefFor(rs)
	// CP-62 P-6 (Task-342): carry the previous sprint's verified handoff into
	// the entry prompt — decisions/findings survive across sprints. A missing
	// file degrades to the bare task reference (graceful fallback, T-4).
	prompt := d.Task
	if adopt {
		prompt = prompt + "\n\n[vibe-adopt] The implementation for this task already exists (written outside FlowPilot). You are verifying alignment against the requirement chain, not authoring it."
	}
	if handoff := previousSprintHandoffContext(cwd, d.Sprint); handoff != "" {
		prompt = prompt + "\n\n" + handoff
	}
	s.startResolvedFlow(context.Background(), parentRunID, ref, prompt)
}

func collectLatestVibeCP(cwd string) string {
	if strings.TrimSpace(cwd) == "" {
		return ""
	}
	var matches []string
	for _, pat := range []string{
		filepath.Join(cwd, "requirements", "07-Coding-Plan", "todo", "CP-*.md"),
		filepath.Join(cwd, "requirements", "07-Coding-Plan", "inprogress", "CP-*.md"),
	} {
		got, err := filepath.Glob(pat)
		if err != nil {
			continue
		}
		matches = append(matches, got...)
	}
	if len(matches) == 0 {
		return ""
	}
	sort.Strings(matches)
	rel, err := filepath.Rel(cwd, matches[len(matches)-1])
	if err != nil {
		return filepath.ToSlash(matches[len(matches)-1])
	}
	return filepath.ToSlash(rel)
}

// vibeCPIDForPath reads the CP document at rel (cwd-relative or absolute) and
// returns its Document ID value (e.g. "CP-02"). Best-effort — empty on any
// failure, matching collectLatestVibeCP's tolerate-missing contract.
func vibeCPIDForPath(cwd, rel string) string {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return ""
	}
	abs := rel
	if !filepath.IsAbs(abs) && strings.TrimSpace(cwd) != "" {
		abs = filepath.Join(cwd, filepath.FromSlash(rel))
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return ""
	}
	return workingmode.CPDocumentID(string(b))
}

// vibeCPArtifactsPresent reports whether a non-empty CP-*.md exists under the
// coding-plan tree. Empty cwd = unknown/present (Task-328 / Task-327 T-1).
func vibeCPArtifactsPresent(cwd string) bool {
	if strings.TrimSpace(cwd) == "" {
		return true
	}
	p := collectLatestVibeCP(cwd)
	return p != "" && vibeWorkspaceFileExists(cwd, p)
}

// restartVibeCpWriterForMissingCP implements R-CP-D1 (Task-328): SS remains,
// CP deleted → restart vibe-ingest at cp_writer only (no ss_lock card).
//
// Live run-225468: after cp_writer, chatFlowRef may already be vibe-cp-ingest
// (CA-791 join) while vibe_locked_ss was never persisted — still rewrite CP.
func (s *InteractiveService) restartVibeCpWriterForMissingCP(parentRunID string) bool {
	s.mu.Lock()
	rs := s.runs[parentRunID]
	if rs == nil || rs.workingMode != workingmode.Vibe {
		s.mu.Unlock()
		return false
	}
	// Still parked on SS Preview — never steal the lock card to rewrite CP
	// (Task-327 R-SS-K / TestTask327_ReconstructWithSSKeepsLockPark).
	if rs.vibeAwaitingLock && (rs.vibeLockNodeID == "" || rs.vibeLockNodeID == vibeSSLockNodeID) {
		s.mu.Unlock()
		return false
	}
	bare := workingmode.BareFlowID(rs.chatFlowRef)
	okFlow := bare == vibeIngestFlowID || bare == vibeCpIngestFlowID ||
		runHasFlowNode(rs, vibeCpWriterNodeID) || runHasFlowNode(rs, "ingest_reader") ||
		rs.vibeCheckpointNode == vibeCpWriterNodeID || rs.vibeCheckpointNode == vibeSSLockNodeID
	if !okFlow {
		s.mu.Unlock()
		return false
	}
	cwd := rs.workspaceCwd
	s.mu.Unlock()
	if !vibeSSLockArtifactsPresent(cwd, rs) {
		return false
	}
	if vibeCPArtifactsPresent(cwd) {
		return false
	}

	s.mu.Lock()
	rs = s.runs[parentRunID]
	if rs == nil {
		s.mu.Unlock()
		return false
	}
	prompt := strings.TrimSpace(rs.lastPrompt)
	if prompt == "" {
		prompt = strings.TrimSpace(rs.lastFullPrompt)
	}
	if prompt == "" {
		prompt = "[vibe] CP artifact missing; rewriting CP from locked SS (Task-328)."
	}
	rs.vibeAwaitingLock = false
	rs.vibeLockNodeID = ""
	rs.vibeLockPath = ""
	rs.vibeResumeConfirm = false
	rs.vibeResumeFromNode = ""
	rs.chatFlowRef = workingmode.PackPrefix + vibeIngestFlowID
	// R-TK-D2 live: stale vibeSprintIndex survived CP rewrite → chip task 2/3
	// with Task-904 still draft. Clear sprint cursor with the old plan.
	clearVibeSprintCursor(rs)
	s.mu.Unlock()

	// Stopped/done loops must be unsealed so cp_writer can spawn again.
	s.releaseHubStopFenceForFollowUp(context.Background(), parentRunID)
	s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		st.BlockReason = ""
		st.GateReason = ""
		st.ActiveNode = ""
		return st
	})
	s.mu.Lock()
	if r := s.runs[parentRunID]; r != nil {
		if r.status == RunStatusCancelled || r.status == RunStatusFailed || r.status == RunStatusCompleted {
			r.status = RunStatusRunning
			r.agentStatus = string(RunStatusRunning)
		}
		r.autoOrchestrate = true
		r.flowEngineDriven = true
	}
	s.mu.Unlock()
	s.setFlowStepStatus(context.Background(), parentRunID, vibeCpWriterNodeID, StepStatusPending)
	s.startResolvedFlowFromNode(context.Background(), parentRunID, workingmode.PackPrefix+vibeIngestFlowID, prompt, vibeCpWriterNodeID)
	go s.persistParentSession(parentRunID)
	return true
}

// maybeParkVibeCpJoinResume implements R-CP-K (Task-328): CP on disk, no Tasks
// yet, vibe-ingest finished cp_writer → park Resume so OK joins task_slicer
// (CA-791). pendingVibeResumeFromNode cannot see this — edge is cp_writer→done.
func (s *InteractiveService) maybeParkVibeCpJoinResume(parentRunID string) bool {
	if s == nil || strings.TrimSpace(parentRunID) == "" {
		return false
	}
	s.mu.Lock()
	rs := s.runs[parentRunID]
	if rs == nil || rs.parentRunID != "" || rs.workingMode != workingmode.Vibe {
		s.mu.Unlock()
		return false
	}
	if rs.vibeResumeConfirm || rs.vibeAwaitingLock || rs.vibeSprintBoundaryPending || rs.vibeSprintBoundaryDeclined {
		s.mu.Unlock()
		return false
	}
	if vibePlanDrainedLocked(rs) {
		// R.2-2: a fully-delivered plan with an empty todo/ is the drained
		// terminal state, not "CP written but never sliced". Offering the
		// cp_writer → task_slicer resume here would re-park the finished run
		// behind a zero-task failure card on every reopen.
		s.mu.Unlock()
		return false
	}
	// Already joined cp-ingest with Tasks → nothing to park here.
	cwd := rs.workspaceCwd
	bare := workingmode.BareFlowID(rs.chatFlowRef)
	cpID := rs.vibeCpDocID
	s.mu.Unlock()

	if !vibeCPArtifactsPresent(cwd) || !vibeSSLockArtifactsPresent(cwd, rs) {
		return false
	}
	if len(collectVibeTaskPlanForCP(cwd, cpID)) > 0 {
		return false
	}
	// vibe-cp-ingest without Tasks yet: still need Resume → task_slicer.
	if bare != vibeIngestFlowID && bare != vibeCpIngestFlowID && bare != "" {
		return false
	}

	s.mu.Lock()
	r := s.runs[parentRunID]
	if r == nil || r.vibeResumeConfirm || r.vibeAwaitingLock || r.vibeSprintBoundaryPending {
		s.mu.Unlock()
		return false
	}
	r.vibeResumeConfirm = true
	r.vibeResumeFromNode = vibeCpWriterNodeID
	s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = vibeResumePausedReason
		st.GateReason = "Resume confirmation\nConfirm before continuing this vibe flow (cp_writer → task_slicer).\n"
		return st
	})
	s.mu.Unlock()
	s.parkFlowForAwaitingUser(parentRunID)
	return true
}

// isVibeCpSourcedFlowID reports whether a bare flow id takes a CP-*.md
// source at admission (CP-90: vibe-tasks shares the CP-source contract with
// vibe-cp-ingest — the difference starts after cp_lock).
func isVibeCpSourcedFlowID(bareID string) bool {
	return bareID == vibeCpIngestFlowID || bareID == vibeTasksFlowID
}

// vibeFenceFlowID resolves which vibe entry the admission check guards:
// the turn's own FlowRef wins, else the run's pinned ref (forward turns).
func vibeFenceFlowID(rs *interactiveRun, in TurnInput) string {
	if id := workingmode.BareFlowID(strings.TrimSpace(in.FlowRef)); id != "" {
		return id
	}
	if rs != nil {
		return workingmode.BareFlowID(rs.chatFlowRef)
	}
	return ""
}

// validateVibeCpIngestSource enforces BUG-399 (live run-3439): a turn that
// launches a CP-sourced vibe flow must name a CP-shaped source before
// cp_reader can draft anything — requirements/07-Coding-Plan/**/CP-*.md
// whose file exists and carries `Document ID: CP-*`. The TUI `/flow` picker
// runs DetectVibeEntry but API clients pin flowRef directly, so the
// deterministic check lives at turn admission. Source resolution order:
// explicit SourceDocID (the launch arm's `@path`, stripped), then the first
// CP-shaped token in the prompt. Fail-closed: no source, unreadable file,
// or missing Document ID all reject with 422 — nothing is drafted.
// CP-90: vibe-tasks additionally requires the CP to already own ≥1 Task
// file under requirements/08-Task/todo/ parented to it — that entry sprints
// a pre-broken plan; a task-less CP belongs to vibe-cp-ingest (the slicer).
// Empty workspaceCwd defers that check to the reader node's fail-closed
// park. Called with s.mu held.
func (s *InteractiveService) validateVibeCpIngestSource(rs *interactiveRun, in TurnInput) *apiErr {
	flowID := vibeFenceFlowID(rs, in)
	src := strings.TrimPrefix(strings.TrimSpace(in.SourceDocID), "@")
	if !workingmode.IsCodingPlanCPPath(src) {
		src = ""
		for _, tok := range tokenizePromptTokens(in.Prompt) {
			cand := strings.TrimPrefix(strings.TrimSpace(tok), "@")
			if workingmode.IsCodingPlanCPPath(cand) {
				src = cand
				break
			}
		}
	}
	if src == "" {
		return newAPIErr(http.StatusUnprocessableEntity, "invalid_cp_source",
			fmt.Sprintf("%s requires a requirements/07-Coding-Plan/**/CP-*.md source document", flowID))
	}
	abs := src
	if !filepath.IsAbs(abs) && strings.TrimSpace(rs.workspaceCwd) != "" {
		abs = filepath.Join(rs.workspaceCwd, filepath.FromSlash(src))
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return newAPIErr(http.StatusUnprocessableEntity, "invalid_cp_source",
			fmt.Sprintf("%s source %q is not readable: %v", flowID, src, err))
	}
	if !workingmode.HasCPDocumentID(string(b)) {
		return newAPIErr(http.StatusUnprocessableEntity, "invalid_cp_source",
			fmt.Sprintf("%s source %q is not a CP document (missing `Document ID: CP-*`)", flowID, src))
	}
	// BUG-468: pin the ingested CP so the sprint plan and every "this run's
	// tasks" presence check scope to tasks parented to this document —
	// foreign/stale Task files on a shared bed must never join the plan.
	rs.vibeCpDocID = workingmode.CPDocumentID(string(b))
	if flowID == vibeTasksFlowID && strings.TrimSpace(rs.workspaceCwd) != "" &&
		len(collectVibeTaskPlanForCP(rs.workspaceCwd, rs.vibeCpDocID)) == 0 {
		return newAPIErr(http.StatusUnprocessableEntity, "no_cp_tasks",
			fmt.Sprintf("vibe-tasks requires existing Task-*.md files parented to %s under requirements/08-Task/todo/ (use vibe-cp-ingest to slice tasks first)", rs.vibeCpDocID))
	}
	return nil
}

// maybeStartVibeCpIngest overlays vibe-cp-ingest after ingest wrote a CP.
// Ingest already locked SS — skip cp_reader/cp_lock and spawn task_slicer.
// User-start vibe-cp-ingest is a no-op here so cp_lock still runs.
func (s *InteractiveService) maybeStartVibeCpIngest(parentRunID string) {
	s.mu.Lock()
	rs := s.runs[parentRunID]
	if rs == nil {
		s.mu.Unlock()
		return
	}
	if workingmode.BareFlowID(rs.chatFlowRef) == vibeCpIngestFlowID {
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	s.forceStartVibeTaskSlicer(parentRunID)
}

// vibeTaskPlanExpectsFiles reports plan entries that name Task artifacts
// (not placeholders like "sprint-0" from older fixtures / CA-770).
func vibeTaskPlanExpectsFiles(plan []string) bool {
	for _, p := range plan {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if strings.Contains(p, "Task-") || strings.Contains(filepath.ToSlash(p), "08-Task/") {
			return true
		}
	}
	return false
}

// restartVibeTaskSlicerForMissingTasks implements R-TK-D1 (Task-329): Task
// files deleted but CP remains → re-run task_slicer. Must run BEFORE
// maybeParkVibeResumeConfirm; otherwise stale vibeTaskPlan + vibe-sprint
// parks "Resume from tdd?" (live run-225468 after deleting Task-904/905/906).
func (s *InteractiveService) restartVibeTaskSlicerForMissingTasks(parentRunID string) bool {
	s.mu.Lock()
	rs := s.runs[parentRunID]
	if rs == nil || rs.workingMode != workingmode.Vibe {
		s.mu.Unlock()
		return false
	}
	cwd := rs.workspaceCwd
	bare := workingmode.BareFlowID(rs.chatFlowRef)
	plan := append([]string(nil), rs.vibeTaskPlan...)
	cpNode := rs.vibeCheckpointNode
	cpID := rs.vibeCpDocID
	s.mu.Unlock()

	// Empty cwd = unknown (Task-327 T-1 / CA-770 reconstruct fixtures).
	if strings.TrimSpace(cwd) == "" {
		return false
	}
	if !vibeCPArtifactsPresent(cwd) {
		return false
	}
	// BUG-591 (live run-139670): "missing" means absent from BOTH todo/ and
	// done/ — a finished CP's tasks move to done/, leaving the scoped todo/
	// glob empty; the bare check then read a completed sprint as a deletion
	// and force-started the slicer, which rebound the run to a foreign CP.
	if len(collectVibeTaskPlanForCP(cwd, cpID)) > 0 || len(collectVibeDoneTasksForCP(cwd, cpID)) > 0 {
		return false
	}
	// Require evidence we already passed slicer / entered sprint — not mere
	// vibe-cp-ingest without Tasks (that is R-CP-K Resume join).
	expected := vibeTaskPlanExpectsFiles(plan) || bare == vibeSprintFlowID ||
		cpNode == "tdd" || cpNode == vibeTaskSlicerNodeID
	if !expected {
		return false
	}

	s.mu.Lock()
	if r := s.runs[parentRunID]; r != nil {
		clearVibeSprintCursor(r)
		r.vibeResumeConfirm = false
		r.vibeResumeFromNode = ""
	}
	s.mu.Unlock()
	s.forceStartVibeTaskSlicer(parentRunID)
	return true
}

// clearVibeSprintCursor drops plan + index + boundary so a later slicer /
// first sprint starts at task 1/N (stale-index fix after R-TK-D2).
func clearVibeSprintCursor(rs *interactiveRun) {
	if rs == nil {
		return
	}
	rs.vibeTaskPlan = nil
	rs.vibeSprintIndex = 0
	rs.vibeSprintBoundaryPending = false
	rs.vibeSprintBoundaryTask = ""
	rs.vibeSprintBoundaryDeclined = false
	rs.vibeSprintStartInFlight = false
}

// vibeSprintCurrentPlanIndex maps vibeSprintIndex (started-count / chip N in
// task N/M) to the 0-based plan slot for the active sprint.
func vibeSprintCurrentPlanIndex(index, planLen int) int {
	if planLen <= 0 {
		return 0
	}
	cur := index - 1
	if cur < 0 {
		cur = 0
	}
	if cur >= planLen {
		cur = planLen - 1
	}
	return cur
}

// reconcileVibeSprintCursor clamps vibeSprintIndex when an earlier plan slot
// is still `draft` (never started). Live: index=2 while Task-904 draft → chip
// task 2/3. Returns true when the cursor was moved.
func reconcileVibeSprintCursor(rs *interactiveRun) bool {
	if rs == nil || strings.TrimSpace(rs.workspaceCwd) == "" || len(rs.vibeTaskPlan) == 0 {
		return false
	}
	if rs.vibeSprintIndex <= 0 {
		return false
	}
	for i := 0; i < len(rs.vibeTaskPlan) && i < rs.vibeSprintIndex; i++ {
		if readVibeDocStatus(rs.workspaceCwd, rs.vibeTaskPlan[i]) != "draft" {
			continue
		}
		// Chip / started-count for this unstarted task should be i+1.
		want := i + 1
		if rs.vibeSprintIndex > want {
			rs.vibeSprintIndex = want
			return true
		}
		return false
	}
	return false
}

// forceStartVibeTaskSlicer joins/restarts task_slicer from the latest CP.
// Unlike maybeStartVibeCpIngest, this still runs when chatFlowRef is already
// vibe-cp-ingest (live hang: Resume Continue after stop cancelled slicer —
// maybeStartVibeCpIngest no-op'd and the UI sat idle).
func (s *InteractiveService) forceStartVibeTaskSlicer(parentRunID string) {
	s.mu.Lock()
	rs := s.runs[parentRunID]
	if rs == nil || rs.workingMode != workingmode.Vibe {
		s.mu.Unlock()
		return
	}
	cwd := rs.workspaceCwd
	prompt := collectLatestVibeCP(cwd)
	if prompt == "" {
		prompt = "requirements/07-Coding-Plan/todo/"
	}
	// BUG-468: the slicer consumes this CP — pin its Document ID so the sprint
	// plan scopes to tasks parented to it (live run-91517/91606 sprinted a
	// stale calc task under a snake CP on a shared bed).
	if id := vibeCPIDForPath(cwd, prompt); id != "" {
		rs.vibeCpDocID = id
	}
	// BUG-469: same pin for the slicer's bound cp_md input — the resolved
	// path overrides the generic "newest matching" input mention so a
	// multi-CP bed can't slice a different CP than this run's source.
	if strings.HasSuffix(prompt, ".md") {
		rs.vibeLockedCP = prompt
	}
	rs.vibeAwaitingLock = false
	rs.vibeResumeConfirm = false
	rs.vibeResumeFromNode = ""
	rs.chatFlowRef = workingmode.PackPrefix + vibeCpIngestFlowID
	rs.autoOrchestrate = true
	rs.flowEngineDriven = true
	clearVibeSprintCursor(rs)
	if rs.status == RunStatusCancelled || rs.status == RunStatusFailed || rs.status == RunStatusCompleted {
		rs.status = RunStatusRunning
		rs.agentStatus = string(RunStatusRunning)
	}
	s.mu.Unlock()

	s.releaseHubStopFenceForFollowUp(context.Background(), parentRunID)
	s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		st.BlockReason = ""
		st.GateReason = ""
		st.ActiveNode = ""
		return st
	})
	s.setFlowStepStatus(context.Background(), parentRunID, vibeTaskSlicerNodeID, StepStatusPending)
	s.startResolvedFlowFromNode(context.Background(), parentRunID, workingmode.PackPrefix+vibeCpIngestFlowID, prompt, vibeTaskSlicerNodeID)
	go s.persistParentSession(parentRunID)
}

// vibeResolvedSlicerSource (BUG-469) returns the run's pinned source
// document for templated file_artifact INPUT bindings — non-empty only for
// Vibe runs that pinned a CP (cp-ingest admission + cp_lock approval stamp
// rs.vibeLockedCP; forceStartVibeTaskSlicer pins the CP it resolved). The
// task_slicer delegate prompt uses it to override the generic "find the
// newest matching" input mention, which on a multi-CP bed resolved to a
// different CP than the one the run ingested (live run-96970).
func (s *InteractiveService) vibeResolvedSlicerSource(parentRunID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	rs := s.runs[parentRunID]
	if rs == nil || rs.workingMode != workingmode.Vibe {
		return ""
	}
	return strings.TrimSpace(rs.vibeLockedCP)
}

func (s *InteractiveService) maybeChainVibeSprint(parentRunID, completedNodeID string) {
	if completedNodeID != "audit" {
		return
	}
	s.mu.Lock()
	rs := s.runs[parentRunID]
	ok := rs != nil && rs.workingMode == workingmode.Vibe && len(rs.vibeTaskPlan) > 0 &&
		!rs.vibeSprintBoundaryPending && !rs.vibeSprintStartInFlight && !rs.vibeSprintBoundaryDeclined
	sprintRan := rs != nil && rs.vibeSprintIndex > 0
	s.mu.Unlock()
	if !ok {
		return
	}
	// CP-62 P-6 (Task-342): the sprint that just finished writes its handoff
	// from verified run state before the next one starts. Best-effort — an
	// I/O failure never blocks the chain (next sprint falls back).
	if sprintRan {
		s.emitSprintHandoff(rs)
	}
	go s.maybeStartNextVibeSprint(parentRunID)
}

func collectVibeTaskPlan(cwd string) []string {
	if strings.TrimSpace(cwd) == "" {
		return nil
	}
	matches, err := filepath.Glob(filepath.Join(cwd, "requirements", "08-Task", "todo", "Task-*.md"))
	if err != nil || len(matches) == 0 {
		return nil
	}
	sort.Strings(matches)
	out := make([]string, 0, len(matches))
	for _, abs := range matches {
		rel, err := filepath.Rel(cwd, abs)
		if err != nil {
			rel = abs
		}
		out = append(out, filepath.ToSlash(rel))
	}
	return out
}

var vibeTaskParentDocsLine = regexp.MustCompile(`(?im)^\s*-?\s*Parent Documents\s*:(.*)$`)
var vibeTaskCPRef = regexp.MustCompile(`CP-[0-9]+`)

// collectVibeTaskPlanForCP is the BUG-468 scoped variant: only tasks whose
// `Parent Documents` metadata line names cpID belong to this run's plan.
// The Parent-Documents line alone is parsed — a Related-Documents mention of
// another CP (Task-12 names CP-01 as a prior plan) must not pull the task in.
// Empty cpID returns the legacy unscoped glob so pre-fix runs and fixtures
// keep identical behavior.
func collectVibeTaskPlanForCP(cwd, cpID string) []string {
	cpID = strings.ToUpper(strings.TrimSpace(cpID))
	if cpID == "" {
		return collectVibeTaskPlan(cwd)
	}
	all := collectVibeTaskPlan(cwd)
	out := make([]string, 0, len(all))
	for _, rel := range all {
		b, err := os.ReadFile(filepath.Join(cwd, filepath.FromSlash(rel)))
		if err != nil {
			continue
		}
		m := vibeTaskParentDocsLine.FindSubmatch(b)
		if len(m) < 2 {
			continue
		}
		for _, ref := range vibeTaskCPRef.FindAll(m[1], -1) {
			if strings.ToUpper(string(ref)) == cpID {
				out = append(out, rel)
				break
			}
		}
	}
	return out
}

// collectVibeDoneTasksForCP is the done/ counterpart of
// collectVibeTaskPlanForCP: same Parent-Documents scoping over
// requirements/08-Task/done/. Presence in done/ proves the CP's tasks exist —
// only a file absent from both directories counts as deleted (BUG-591).
func collectVibeDoneTasksForCP(cwd, cpID string) []string {
	if strings.TrimSpace(cwd) == "" {
		return nil
	}
	matches, err := filepath.Glob(filepath.Join(cwd, "requirements", "08-Task", "done", "Task-*.md"))
	if err != nil || len(matches) == 0 {
		return nil
	}
	cpID = strings.ToUpper(strings.TrimSpace(cpID))
	if cpID == "" {
		out := make([]string, 0, len(matches))
		for _, abs := range matches {
			if rel, err := filepath.Rel(cwd, abs); err == nil {
				out = append(out, filepath.ToSlash(rel))
			}
		}
		sort.Strings(out)
		return out
	}
	out := make([]string, 0, len(matches))
	for _, abs := range matches {
		b, err := os.ReadFile(abs)
		if err != nil {
			continue
		}
		m := vibeTaskParentDocsLine.FindSubmatch(b)
		if len(m) < 2 {
			continue
		}
		for _, ref := range vibeTaskCPRef.FindAll(m[1], -1) {
			if strings.ToUpper(string(ref)) == cpID {
				if rel, err := filepath.Rel(cwd, abs); err == nil {
					out = append(out, filepath.ToSlash(rel))
				}
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// collectVibeSprintPlanForCP scopes the sprint plan the same way; unlike
// collectVibeSprintPlan there is no SS fallback — a scoped run whose slicer
// produced no matching tasks must hit the fail-closed park, never sprint a
// fallback plan.
func collectVibeSprintPlanForCP(cwd, cpID string) []string {
	if strings.TrimSpace(cpID) == "" {
		return collectVibeSprintPlan(cwd)
	}
	return collectVibeTaskPlanForCP(cwd, cpID)
}

func collectVibeSprintPlan(cwd string) []string {
	if tasks := collectVibeTaskPlan(cwd); len(tasks) > 0 {
		return tasks
	}
	if strings.TrimSpace(cwd) == "" {
		return nil
	}
	matches, err := filepath.Glob(filepath.Join(cwd, "requirements", "05-System-Specs", "SS-*.md"))
	if err != nil || len(matches) == 0 {
		return nil
	}
	sort.Strings(matches)
	out := make([]string, 0, len(matches))
	for _, abs := range matches {
		base := filepath.Base(abs)
		if strings.HasPrefix(strings.ToUpper(base), "FORMAT-") {
			continue
		}
		rel, err := filepath.Rel(cwd, abs)
		if err != nil {
			rel = abs
		}
		out = append(out, filepath.ToSlash(rel))
	}
	return out
}

// stashVibeFlowForDebate parks the sprint topology for the debate and
// reports whether THIS call performed the park — the single atomic mount
// decision (CA-1095). A caller that gets false must NOT launch another
// debate flow: one is already mounted, and this call still recorded the
// gated child for its post-debate reprompt.
func (s *InteractiveService) stashVibeFlowForDebate(parentRunID, gatedRunID string) (parkedNow bool) {
	s.mu.Lock()
	// The stale-drop diag does file I/O — registered before the unlock
	// defer so it runs after s.mu is released (defers run LIFO).
	staleDropped := false
	defer func() {
		if staleDropped {
			s.flowDiagLog(parentRunID, "vibe_debate_claim_stale_dropped",
				"stale parked claim dropped before re-park",
				"gated_run_id", gatedRunID)
		}
	}()
	defer s.mu.Unlock()
	rs := s.runs[parentRunID]
	if rs == nil {
		return false
	}
	// BUG-594 (live run-139670): a parked snapshot recording a DIFFERENT
	// flow than the live graph is a detached stale claim — the debate
	// overlay was clobbered and can never come back. Suppressing on it
	// strands every gated outcome forever (the durable record persisted
	// exactly this shape). Drop it; the divert then re-parks the LIVE
	// topology and mounts a real debate.
	if vibeDebateClaimForeignLocked(rs) {
		dropVibeDebateStaleClaimLocked(rs)
		staleDropped = true
	}
	// Record the interrupted child even when the topology is already parked
	// (a second gate fire mid-debate): every diverted node completion owes a
	// resume reprompt once the debate resolves.
	if gatedRunID != "" && gatedRunID != parentRunID && !slices.Contains(rs.vibeParkedGatedRunIDs, gatedRunID) {
		rs.vibeParkedGatedRunIDs = append(rs.vibeParkedGatedRunIDs, gatedRunID)
	}
	if len(rs.vibeParkedNodes) > 0 {
		return false
	}
	// CA-1095: a debate flow already mounted as the chat flow while the
	// parked topology was somehow cleared still owns the run — never
	// remount onto it.
	if workingmode.BareFlowID(rs.chatFlowRef) == vibeOwnerDebateFlowID {
		return false
	}
	rs.vibeParkedNodes = append([]agentpack.FlowNode(nil), rs.activeFlowNodes...)
	rs.vibeParkedEdges = append([]agentpack.FlowEdge(nil), rs.activeFlowEdges...)
	rs.vibeParkedAcceptance = append([]string(nil), rs.activeFlowAcceptanceNodes...)
	rs.vibeParkedFlowRef = rs.chatFlowRef
	// CA-1088: the parked flow's hub pointer is meaningless inside the debate
	// topology — leaving it resolves post-mount reinvokes onto the stashed
	// hub (live run-3362: sprint `synthesis` reinvoke churn feeding the
	// escalate loop). Empty re-resolves via hubInlineNodeID on the mounted
	// debate graph; the sprint hub id needs no restore — the post-restore
	// fallback resolves it from the same nodes.
	rs.activeHubNodeID = ""
	// CP-62 P-1 T-3 (Task-337): the debate turn must assemble with the full
	// violation context — drop any pending drift-ladder context reduction
	// (note/narrow) before the debate prompt is packed.
	if st := driftStateFor(s, parentRunID); st != nil {
		st.mu.Lock()
		st.pendingNote = ""
		st.pendingNarrow = false
		st.mu.Unlock()
	}
	// BUG-595: every fresh park counts toward the sprint mount cap.
	// BUG-1182: also ledger it per gated entity — the cap is per entity,
	// with the total still bounded by the sprint ceiling.
	rs.vibeDebateMounts++
	if rs.vibeDebateMountsByEntity == nil {
		rs.vibeDebateMountsByEntity = map[string]int{}
	}
	rs.vibeDebateMountsByEntity[s.vibeDebateEntityKeyLocked(gatedRunID)]++
	return true
}

func (s *InteractiveService) restoreVibeFlowAfterDebate(parentRunID string) bool {
	s.mu.Lock()
	rs := s.runs[parentRunID]
	if rs == nil || len(rs.vibeParkedNodes) == 0 {
		s.mu.Unlock()
		return false
	}
	// live-039 (run-183756): capture the overlay's node ids before the swap —
	// its rows persist through the sprint reseed, and any still non-terminal
	// (a WAITING debate_trigger park is the live case) must be finalized or
	// the durable step ledger reports a gate that is still parked forever.
	var overlayNodeIDs []string
	for _, n := range rs.activeFlowNodes {
		overlayNodeIDs = append(overlayNodeIDs, strings.TrimSpace(n.ID))
	}
	rs.activeFlowNodes = rs.vibeParkedNodes
	rs.activeFlowEdges = rs.vibeParkedEdges
	rs.activeFlowAcceptanceNodes = rs.vibeParkedAcceptance
	if rs.vibeParkedFlowRef != "" {
		rs.chatFlowRef = rs.vibeParkedFlowRef
	}
	nodes := append([]agentpack.FlowNode(nil), rs.activeFlowNodes...)
	parentCwd := strings.TrimSpace(rs.workspaceCwd)
	gatedIDs := append([]string(nil), rs.vibeParkedGatedRunIDs...)
	// BUG-594: drain flow starts deferred while the claim held. Capture
	// before clearing so a restart between restore and dispatch still has
	// the queue durably (persisted with parentSnap below).
	deferred := append([]VibeDeferredFlowStart(nil), rs.vibeDeferredFlowStarts...)
	rs.vibeParkedNodes = nil
	rs.vibeParkedEdges = nil
	rs.vibeParkedAcceptance = nil
	rs.vibeParkedFlowRef = ""
	rs.vibeParkedGatedRunIDs = nil
	rs.vibeDeferredFlowStarts = nil
	verdicts := append([]VerdictRow(nil), rs.lastFlowVerdicts...)
	// The gate diverted each gated child's node completion into the debate
	// before tryAdvanceFlowFromNode could fire its done-edge — the debate then
	// ran on the parent hub, so the child's own session never saw the verdict.
	// Arm the standard gate-reprompt intent on each gated child with the
	// verdict carried inline: its re-completion re-runs the post-turn gate and
	// advances the restored sprint chain through the normal completion path.
	type gatedReprompt struct {
		runID  string
		stepID string
		prompt string
		gen    int64
	}
	var reprompts []gatedReprompt
	var childSnaps []ProviderSessionState
	for _, gid := range gatedIDs {
		if gid == "" || gid == parentRunID {
			continue
		}
		ch := s.runs[gid]
		if ch == nil || ch.legState == LegStateClosed || ch.status == RunStatusCancelled {
			continue
		}
		stepID := strings.TrimSpace(ch.lastTurnStepID)
		if stepID == "" {
			stepID = strings.TrimSpace(ch.label)
		}
		prompt := s.vibeDebateResumeRepromptPrompt(parentRunID, stepID, ch.workspaceCwd, verdicts)
		ch.pendingGateRepromptPrompt = prompt
		ch.pendingGateRepromptStepID = stepID
		ch.pendingGateRepromptGen++
		reprompts = append(reprompts, gatedReprompt{gid, stepID, prompt, ch.pendingGateRepromptGen})
		childSnaps = append(childSnaps, sessionStateOf(ch))
	}
	parentSnap := sessionStateOf(rs)
	if rs.parentRunID == "" {
		parentSnap.LoopState = s.agentOrchestrator.loopStateFor(rs.id)
	}
	s.mu.Unlock()
	if s.isFlowEngineDriven(parentRunID) {
		s.reseedFlowStepRuntime(parentRunID, nodes)
		s.finalizeConcludedOverlaySteps(parentRunID, overlayNodeIDs, nodes)
	}
	// Durable-first: the cleared parked topology and the armed reprompts must
	// both survive a restart landing between restore and dispatch.
	_ = s.persistProviderSession(parentSnap)
	for _, snap := range childSnaps {
		_ = s.persistProviderSession(snap)
	}
	if len(reprompts) == 0 {
		// No gated child recorded (drift-only debate on the hub, or the child
		// row is gone) — fall back to the hub continuation so the restored
		// loop still has work in flight instead of stalling (run-136/9597).
		s.maybeAutoReinvokeHubWithPrompt(parentRunID, s.vibeDebateResumePrompt(parentRunID, parentCwd))
	} else {
		for _, rp := range reprompts {
			go s.startTurnClearingIntent(rp.runID, rp.stepID, rp.prompt, "reprompt", rp.gen)
		}
	}
	// BUG-594: drain flow starts deferred while the debate claim held — the
	// parked topology is restored, so non-debate mounts are legal again. A
	// vibe-sprint entry re-derives through maybeStartNextVibeSprint so the
	// index/task/handoff are taken fresh under the restored state (and its
	// producers' refusals never queue, so this is belt-and-braces).
	// Non-sprint drains run in one goroutine: request order is preserved
	// (last-writer-wins, matching undeferred mounts), each mount re-checks
	// the claim fence, and entries can never race one another's swap.
	if len(deferred) > 0 {
		go func(starts []VibeDeferredFlowStart) {
			for _, d := range starts {
				if workingmode.BareFlowID(d.FlowRef) == vibeSprintFlowID {
					s.maybeStartNextVibeSprint(parentRunID)
					continue
				}
				s.flowDiagLog(parentRunID, "vibe_flow_start_deferred_drained",
					"draining deferred flow start after debate restore",
					"flow_ref", d.FlowRef, "start_node_id", d.StartNodeID)
				s.startResolvedFlowFromNode(context.Background(), parentRunID, d.FlowRef, d.UserPrompt, d.StartNodeID)
			}
		}(deferred)
	}
	return true
}

// finalizeConcludedOverlaySteps stamps the unmounted overlay's step rows
// SKIPPED when they are still non-terminal at restore time — the overlay
// concluded, so a WAITING/PENDING row would otherwise report a park that can
// never resolve (live-039: debate_trigger stayed WAITING_USER_APPROVAL in
// the durable ledger after the debate concluded and the sprint resumed).
// Ids that collide with the restored topology and RUNNING rows with a live
// leg are left alone — live work owns its own settle.
func (s *InteractiveService) finalizeConcludedOverlaySteps(parentRunID string, overlayNodeIDs []string, restoredNodes []agentpack.FlowNode) {
	if len(overlayNodeIDs) == 0 {
		return
	}
	restored := make(map[string]bool, len(restoredNodes))
	for _, n := range restoredNodes {
		restored[strings.TrimSpace(n.ID)] = true
	}
	for _, nodeID := range overlayNodeIDs {
		if nodeID == "" || restored[nodeID] {
			continue
		}
		switch s.lookupFlowStepStatus(parentRunID, nodeID) {
		case StepStatusDone, StepStatusSkipped, StepStatusFailed, StepStatusCanceled, "":
			continue
		}
		if s.vibeNodeHasLiveWork(parentRunID, nodeID) {
			continue
		}
		s.flowDiagLog(parentRunID, "overlay_step_finalized_on_restore",
			"concluded overlay's non-terminal step finalized",
			"node_id", nodeID)
		s.setFlowStepStatus(context.Background(), parentRunID, nodeID, StepStatusSkipped)
	}
}

// vibeDebateResumeRepromptPrompt builds the gated child's remediation
// reprompt. The owner debate ran on the parent hub, so the verdict rows are
// carried inline; the child's re-completion then fires the interrupted
// node's done-edge through the normal completion path.
//
// BUG-626 (live run-174243): the verdict arrived verbatim — including a
// demand to edit a path the child's own contract had frozen read-only — so
// the reprompt ordered something the contract forbids. When the gated step
// has a governing contract with a read-only surface, the reprompt names it
// and forbids the edit: the demand routes back as "belongs to the owning
// step's leg" instead of becoming an attempted write that the planner-purity
// fingerprint then flags.
func (s *InteractiveService) vibeDebateResumeRepromptPrompt(parentRunID, stepID, workspace string, verdicts []VerdictRow) string {
	const base = "[flow-engine] The post-turn gate on your last turn was routed to the owner-debate remediation flow, which has now resolved. Re-examine your output for this node against the remediation verdict — apply the decided rework, or confirm the output already satisfies the node's contract — then complete normally so the sprint chain advances."
	var b strings.Builder
	b.WriteString(base)
	if len(verdicts) > 0 {
		b.WriteString(" Debate verdict:")
		for _, v := range verdicts {
			b.WriteString(" [")
			b.WriteString(strings.TrimSpace(v.ACID))
			b.WriteString("] ")
			b.WriteString(strings.TrimSpace(v.Verdict))
			if note := strings.TrimSpace(v.Note); note != "" {
				b.WriteString(" — ")
				b.WriteString(note)
			}
			b.WriteString(";")
		}
	}
	if ro := vibeFrozenReadOnlyPathsForStep(workspace, parentRunID, stepID); len(ro) > 0 {
		b.WriteString(" Contract scope: your frozen contract marks these paths read-only — ")
		b.WriteString(strings.Join(ro, ", "))
		b.WriteString(". A verdict cannot order you to edit them: if a demanded change requires a read-only path, do NOT edit it yourself — that work belongs to the owning step's leg. State the routing in your completion and confirm.")
	}
	return b.String()
}

// vibeFrozenReadOnlyPathsForStep returns the ReadOnlyPaths of the step's
// governing frozen contract (sorted, normalized). Empty when no contract
// governs the step or the store is unreadable — the reprompt clause is
// contract-derived, never boilerplate.
func vibeFrozenReadOnlyPathsForStep(workspace, parentRunID, stepID string) []string {
	cwd := strings.TrimSpace(workspace)
	if cwd == "" || strings.TrimSpace(parentRunID) == "" || strings.TrimSpace(stepID) == "" {
		return nil
	}
	store, err := changecontract.NewFrozenStore(cwd)
	if err != nil {
		return nil
	}
	rec, ok, err := store.GetFrozenForStep(parentRunID, stepID)
	if err != nil || !ok {
		return nil
	}
	return changecontract.ReadOnlyLockedPaths(rec)
}

// vibeDebateResumePrompt builds the hub prompt used when the parked sprint
// flow is restored after an owner-debate completes but no gated child was
// recorded — the re-invoked hub re-evaluates the interrupted node with the
// remediation verdict and drives the restored chain via flow_control.
//
// BUG-626/BUG-627 (live run-174243): the bare prompt let the hub cast itself
// as the "materializing node" and edit contract-frozen files. The prompt now
// pins the orchestrator role and, when contracts exist, enumerates the
// union of contract-owned paths the hub must never write — rework on them
// routes to the owning step's leg (the bridge denies the write regardless;
// this keeps the hub from trying).
func (s *InteractiveService) vibeDebateResumePrompt(parentRunID, workspace string) string {
	const base = "[flow-engine] The owner-debate remediation resolved and the parked sprint flow is restored. Re-evaluate the interrupted node's outcome with the debate verdict applied, then call submit_review_outcome / flow_control to advance the sprint chain — re-run the gated work only if the verdict requires rework."
	var b strings.Builder
	b.WriteString(base)
	b.WriteString(" You are the orchestrator — never write or edit project artifact files yourself; rework routes to the owning step's leg.")
	if owned := vibeHubContractOwnedPaths(workspace, parentRunID); len(owned) > 0 {
		b.WriteString(" Contract-owned paths under active frozen contracts (do not touch): ")
		b.WriteString(strings.Join(owned, ", "))
		b.WriteString(".")
	}
	return b.String()
}

// vibeHubContractOwnedPaths returns the sorted union of every path owned by
// an active frozen contract for the run — ReadOnlyPaths ∪ DeclaredPaths ∪
// AllowedExtraPaths across steps. Empty when no contract is active.
func vibeHubContractOwnedPaths(workspace, parentRunID string) []string {
	cwd := strings.TrimSpace(workspace)
	if cwd == "" || strings.TrimSpace(parentRunID) == "" {
		return nil
	}
	store, err := changecontract.NewFrozenStore(cwd)
	if err != nil {
		return nil
	}
	recs, err := store.ListActiveForRun(parentRunID)
	if err != nil || len(recs) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var out []string
	for _, rec := range recs {
		for _, set := range [][]string{rec.ReadOnlyPaths, rec.DeclaredPaths, rec.AllowedExtraPaths} {
			for _, p := range set {
				p = strings.TrimSpace(filepath.ToSlash(p))
				if p == "" || seen[p] {
					continue
				}
				seen[p] = true
				out = append(out, p)
			}
		}
	}
	sort.Strings(out)
	return out
}

func (s *InteractiveService) onVibeCpNodeDone(parentRunID, completedNodeID string) {
	switch completedNodeID {
	case vibeCpLockNodeID, vibeSSLockNodeID:
		s.mu.Lock()
		if rs := s.runs[parentRunID]; rs != nil {
			rs.vibeAwaitingLock = false
		}
		s.mu.Unlock()
	case vibeCpWriterNodeID:
		s.maybeStartVibeCpIngest(parentRunID)
	case vibeDebateSynthesisNodeID:
		s.restoreVibeFlowAfterDebate(parentRunID)
	case vibeAdoptSelectNodeID:
		// Task-459: adopt_select's apply already wrote vibeTaskPlan +
		// vibeCpDocID + index — the completion only needs the chain seam to
		// mount the first vibe-adopt-sprint.
		s.maybeStartNextVibeSprint(parentRunID)
	case vibeTaskSlicerNodeID, vibeSprintSlicerNodeID, vibeTaskPlanReaderNodeID:
		// BUG-363: fail closed when the slicer wrote nothing. With a visible
		// workspace and zero Task files, starting a sprint from the SS-glob
		// fallback silently runs the wrong plan (live run-635006: no CP, no
		// Tasks, 1 sprint from SS, flow done looking healthy). Park for the
		// operator instead. The gate runs BEFORE any plan mutation so a park
		// leaves no stale SS-fallback in rs.vibeTaskPlan for a later retry.
		// Empty-cwd shapes keep legacy behavior (CA-791, CA-783, Task-321/326
		// unit shapes); the SS fallback in collectVibeSprintPlan itself is
		// unchanged (pinned by CA-783).
		// CP-90: task_plan_reader (vibe-tasks) shares this arm — it produces
		// no tasks, it DISCOVERS the ones already parented to the locked CP;
		// an empty scope means the entry was pointed at the wrong CP.
		s.mu.Lock()
		var cwd, cpID string
		drained := false
		if rs := s.runs[parentRunID]; rs != nil {
			cwd = rs.workspaceCwd
			cpID = rs.vibeCpDocID
			drained = vibePlanDrainedLocked(rs)
		}
		s.mu.Unlock()
		if strings.TrimSpace(cwd) != "" && len(collectVibeTaskPlanForCP(cwd, cpID)) == 0 {
			// BUG-364: stamp the completed node DONE before parking.
			// tryAdvanceFlowFromNode calls onVibeCpNodeDone BEFORE its
			// loop-liveness gate and DONE writes (flow_executor.go:1160 vs
			// :1166/:1203), so parking here without stamping strands the
			// step RUNNING forever with no card and no watchdog (live
			// run-640953). Mirrors the cohort self-settle and the :1203
			// terminal write. Unlocked variant: s.mu is not held here.
			s.setFlowStepStatus(context.Background(), parentRunID, completedNodeID, StepStatusDone)
			verb := "produced"
			if completedNodeID == vibeTaskPlanReaderNodeID {
				verb = "found"
			}
			if drained {
				// R.2-2: a late/stale slicer completion on a fully-delivered
				// plan is NOT a slicer failure — the empty todo/ is the
				// drained state. Land the typed terminal instead of parking
				// the finished run behind a "produced no Task files" card.
				s.settleVibePlanComplete(parentRunID, fmt.Sprintf("%s %s no new Task files; sprint plan already delivered", completedNodeID, verb))
				return
			}
			reason := fmt.Sprintf("%s %s no Task files under requirements/08-Task/todo/; refusing to sprint from fallback", completedNodeID, verb)
			if strings.TrimSpace(cpID) != "" {
				reason = fmt.Sprintf("%s %s no Task files parented to %s under requirements/08-Task/todo/; refusing to sprint foreign/stale tasks", completedNodeID, verb, cpID)
			}
			s.parkVibeRequirementFrom(parentRunID, reason, completedNodeID)
			return
		}
		s.mu.Lock()
		var plan []string
		if rs := s.runs[parentRunID]; rs != nil {
			// Disk Tasks after slicer are authoritative. Always reload and
			// reset cursor — keeping a non-empty stale plan skipped the
			// reload and left vibeSprintIndex>0 (chip task 2/3, 904 draft).
			// BUG-468: scope to this run's CP so foreign/stale Task files
			// never enter the sprint plan (live run-91517/91606).
			if collected := collectVibeSprintPlanForCP(rs.workspaceCwd, rs.vibeCpDocID); len(collected) > 0 {
				rs.vibeTaskPlan = collected
				// Cap contract: default max is 20 rounds per Task — total CP
				// cap = len(plan) × 20 (CP-03's 10 tasks → 200). The budget
				// must scale with the detected plan or a fixed 8 would stop
				// a 10-task CP after sprint 8. Only ever raised here — a
				// reload must not shrink an operator-extended budget.
				if scaled := vibeSprintBudgetForPlan(len(collected)); scaled > rs.vibeSprintBudget {
					rs.vibeSprintBudget = scaled
				}
				rs.vibeSprintIndex = 0
				rs.vibeSprintBoundaryPending = false
				rs.vibeSprintBoundaryTask = ""
				rs.vibeSprintStartInFlight = false
			} else if len(rs.vibeTaskPlan) == 0 {
				rs.vibeTaskPlan = []string{"sprint-0"}
			}
			plan = append([]string(nil), rs.vibeTaskPlan...)
		}
		s.mu.Unlock()
		if s.runArtifacts != nil && len(plan) > 0 {
			s.runArtifacts.record(parentRunID, buildFlowChildArtifactRecords(parentRunID, completedNodeID, plan, time.Now()))
		}
		s.maybeStartNextVibeSprint(parentRunID)
	}
	s.tryCommitVibeCheckpoint(parentRunID, completedNodeID)
}
