package runner

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"flowpilot-runner/internal/agentpack"
	"flowpilot-runner/internal/workingmode"
)

const (
	defaultVibeSprintBudget   = 8
	vibeIngestFlowID          = "vibe-ingest"
	vibeCpIngestFlowID        = "vibe-cp-ingest"
	vibeSprintFlowID          = "vibe-sprint"
	vibeOwnerDebateFlowID     = "vibe-owner-debate"
	vibeCpLockNodeID          = "cp_lock"
	vibeSSLockNodeID          = "ss_lock"
	vibeSSValidatorNodeID     = "ss_validator"
	vibeCPValidatorNodeID     = "cp_validator"
	vibeTaskSlicerNodeID      = "task_slicer"
	vibeSprintSlicerNodeID    = "sprint_slicer"
	vibeCpWriterNodeID        = "cp_writer"
	vibeDebateSynthesisNodeID = "debate_synthesis"
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
	case ids["owner_1"] && ids["owner_2"]:
		return pick(vibeOwnerDebateFlowID)
	case ids["tdd"] && ids["coder"]:
		return pick(vibeSprintFlowID)
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
	case vibeCpWriterNodeID, vibeTaskSlicerNodeID:
		return true
	default:
		return false
	}
}

// vibeLinearWriterNode is a spawnable vibe node whose --done--> done is a
// linear chain, not a review-cohort feeding the previous hub.inline.
func vibeLinearWriterNode(nodeID string) bool {
	switch strings.TrimSpace(nodeID) {
	case vibeCpWriterNodeID, vibeTaskSlicerNodeID:
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

func decideNextVibeSprint(awaitingLock bool, tasks []string, index, budget int) vibeSprintDecision {
	if awaitingLock {
		return vibeSprintDecision{Locked: true}
	}
	if budget <= 0 {
		budget = defaultVibeSprintBudget
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
	return rs != nil && rs.vibeAwaitingLock
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
	}
	return d
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
	if rs == nil || rs.vibeSprintStartInFlight || rs.vibeSprintBoundaryPending {
		s.mu.Unlock()
		return
	}
	d := s.takeNextVibeSprintLocked(rs)
	cwd := ""
	if rs != nil {
		cwd = rs.workspaceCwd
	}
	if d.Start {
		// A new sprint start supersedes any earlier boundary decline: the
		// run is active again and must stay offerable at its next boundary.
		rs.vibeSprintBoundaryDeclined = false
	}
	s.mu.Unlock()
	if d.Start {
		abandonActiveFrozenContractsForRun(cwd, parentRunID, "vibe-sprint next task")
		stampVibeTaskInProgress(cwd, d.Task)
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
		return
	}
	ref := workingmode.PackPrefix + vibeSprintFlowID
	// CP-62 P-6 (Task-342): carry the previous sprint's verified handoff into
	// the entry prompt — decisions/findings survive across sprints. A missing
	// file degrades to the bare task reference (graceful fallback, T-4).
	prompt := d.Task
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

// validateVibeCpIngestSource enforces BUG-399 (live run-3439): a turn that
// launches vibe-cp-ingest must name a CP-shaped source before cp_reader can
// draft anything — requirements/07-Coding-Plan/**/CP-*.md whose file exists
// and carries `Document ID: CP-*`. The TUI `/flow` picker runs DetectVibeEntry
// but API clients pin flowRef directly, so the deterministic check lives at
// turn admission. Source resolution order: explicit SourceDocID (the launch
// arm's `@path`, stripped), then the first CP-shaped token in the prompt.
// Fail-closed: no source, unreadable file, or missing Document ID all reject
// with 422 — nothing is drafted. Called with s.mu held.
func (s *InteractiveService) validateVibeCpIngestSource(rs *interactiveRun, in TurnInput) *apiErr {
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
			"vibe-cp-ingest requires a requirements/07-Coding-Plan/**/CP-*.md source document")
	}
	abs := src
	if !filepath.IsAbs(abs) && strings.TrimSpace(rs.workspaceCwd) != "" {
		abs = filepath.Join(rs.workspaceCwd, filepath.FromSlash(src))
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return newAPIErr(http.StatusUnprocessableEntity, "invalid_cp_source",
			fmt.Sprintf("vibe-cp-ingest source %q is not readable: %v", src, err))
	}
	if !workingmode.HasCPDocumentID(string(b)) {
		return newAPIErr(http.StatusUnprocessableEntity, "invalid_cp_source",
			fmt.Sprintf("vibe-cp-ingest source %q is not a CP document (missing `Document ID: CP-*`)", src))
	}
	// BUG-468: pin the ingested CP so the sprint plan and every "this run's
	// tasks" presence check scope to tasks parented to this document —
	// foreign/stale Task files on a shared bed must never join the plan.
	rs.vibeCpDocID = workingmode.CPDocumentID(string(b))
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
	if len(collectVibeTaskPlanForCP(cwd, cpID)) > 0 {
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

func (s *InteractiveService) stashVibeFlowForDebate(parentRunID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rs := s.runs[parentRunID]
	if rs == nil || len(rs.vibeParkedNodes) > 0 {
		return
	}
	rs.vibeParkedNodes = append([]agentpack.FlowNode(nil), rs.activeFlowNodes...)
	rs.vibeParkedEdges = append([]agentpack.FlowEdge(nil), rs.activeFlowEdges...)
	rs.vibeParkedAcceptance = append([]string(nil), rs.activeFlowAcceptanceNodes...)
	rs.vibeParkedFlowRef = rs.chatFlowRef
	// CP-62 P-1 T-3 (Task-337): the debate turn must assemble with the full
	// violation context — drop any pending drift-ladder context reduction
	// (note/narrow) before the debate prompt is packed.
	if st := driftStateFor(s, parentRunID); st != nil {
		st.mu.Lock()
		st.pendingNote = ""
		st.pendingNarrow = false
		st.mu.Unlock()
	}
}

func (s *InteractiveService) restoreVibeFlowAfterDebate(parentRunID string) bool {
	s.mu.Lock()
	rs := s.runs[parentRunID]
	if rs == nil || len(rs.vibeParkedNodes) == 0 {
		s.mu.Unlock()
		return false
	}
	rs.activeFlowNodes = rs.vibeParkedNodes
	rs.activeFlowEdges = rs.vibeParkedEdges
	rs.activeFlowAcceptanceNodes = rs.vibeParkedAcceptance
	if rs.vibeParkedFlowRef != "" {
		rs.chatFlowRef = rs.vibeParkedFlowRef
	}
	nodes := append([]agentpack.FlowNode(nil), rs.activeFlowNodes...)
	rs.vibeParkedNodes = nil
	rs.vibeParkedEdges = nil
	rs.vibeParkedAcceptance = nil
	rs.vibeParkedFlowRef = ""
	s.mu.Unlock()
	if s.isFlowEngineDriven(parentRunID) {
		s.reseedFlowStepRuntime(parentRunID, nodes)
	}
	return true
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
	case vibeTaskSlicerNodeID, vibeSprintSlicerNodeID:
		// BUG-363: fail closed when the slicer wrote nothing. With a visible
		// workspace and zero Task files, starting a sprint from the SS-glob
		// fallback silently runs the wrong plan (live run-635006: no CP, no
		// Tasks, 1 sprint from SS, flow done looking healthy). Park for the
		// operator instead. The gate runs BEFORE any plan mutation so a park
		// leaves no stale SS-fallback in rs.vibeTaskPlan for a later retry.
		// Empty-cwd shapes keep legacy behavior (CA-791, CA-783, Task-321/326
		// unit shapes); the SS fallback in collectVibeSprintPlan itself is
		// unchanged (pinned by CA-783).
		s.mu.Lock()
		var cwd, cpID string
		if rs := s.runs[parentRunID]; rs != nil {
			cwd = rs.workspaceCwd
			cpID = rs.vibeCpDocID
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
			reason := "task_slicer produced no Task files under requirements/08-Task/todo/; refusing to sprint from fallback"
			if strings.TrimSpace(cpID) != "" {
				reason = fmt.Sprintf("task_slicer produced no Task files parented to %s under requirements/08-Task/todo/; refusing to sprint foreign/stale tasks", cpID)
			}
			s.parkVibeRequirement(parentRunID, reason)
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
