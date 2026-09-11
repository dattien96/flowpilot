package runner

import (
	"context"
	"log"
	"path/filepath"
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
	s.startResolvedFlow(context.Background(), parentRunID, ref, d.Task)
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
	s.mu.Unlock()

	if !vibeCPArtifactsPresent(cwd) || !vibeSSLockArtifactsPresent(cwd, rs) {
		return false
	}
	if len(collectVibeTaskPlan(cwd)) > 0 {
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
	cwd := rs.workspaceCwd
	rs.vibeAwaitingLock = false
	rs.chatFlowRef = workingmode.PackPrefix + vibeCpIngestFlowID
	s.mu.Unlock()
	prompt := collectLatestVibeCP(cwd)
	if prompt == "" {
		prompt = "requirements/07-Coding-Plan/todo/"
	}
	s.startResolvedFlowFromNode(context.Background(), parentRunID, workingmode.PackPrefix+vibeCpIngestFlowID, prompt, vibeTaskSlicerNodeID)
}

func (s *InteractiveService) maybeChainVibeSprint(parentRunID, completedNodeID string) {
	if completedNodeID != "audit" {
		return
	}
	s.mu.Lock()
	rs := s.runs[parentRunID]
	// The sprint-boundary Continue gate owns the next start once parked; a
	// stray chain (e.g. a replayed audit advance) must not double-start, and
	// neither a just-started sprint (start-in-flight) nor a declined run may
	// chain into a new one.
	ok := rs != nil && rs.workingMode == workingmode.Vibe && len(rs.vibeTaskPlan) > 0 &&
		!rs.vibeSprintBoundaryPending && !rs.vibeSprintStartInFlight && !rs.vibeSprintBoundaryDeclined
	s.mu.Unlock()
	if !ok {
		return
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
		var cwd string
		if rs := s.runs[parentRunID]; rs != nil {
			cwd = rs.workspaceCwd
		}
		s.mu.Unlock()
		if strings.TrimSpace(cwd) != "" && len(collectVibeTaskPlan(cwd)) == 0 {
			// BUG-364: stamp the completed node DONE before parking.
			// tryAdvanceFlowFromNode calls onVibeCpNodeDone BEFORE its
			// loop-liveness gate and DONE writes (flow_executor.go:1160 vs
			// :1166/:1203), so parking here without stamping strands the
			// step RUNNING forever with no card and no watchdog (live
			// run-640953). Mirrors the cohort self-settle and the :1203
			// terminal write. Unlocked variant: s.mu is not held here.
			s.setFlowStepStatus(context.Background(), parentRunID, completedNodeID, StepStatusDone)
			s.parkVibeRequirement(parentRunID, "task_slicer produced no Task files under requirements/08-Task/todo/; refusing to sprint from fallback")
			return
		}
		s.mu.Lock()
		var plan []string
		if rs := s.runs[parentRunID]; rs != nil {
			if len(rs.vibeTaskPlan) == 0 {
				if collected := collectVibeSprintPlan(rs.workspaceCwd); len(collected) > 0 {
					rs.vibeTaskPlan = collected
				} else {
					rs.vibeTaskPlan = []string{"sprint-0"}
				}
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
