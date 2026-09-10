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
	defaultVibeSprintBudget     = 8
	vibeIngestFlowID            = "vibe-ingest"
	vibeCpIngestFlowID          = "vibe-cp-ingest"
	vibeSprintFlowID            = "vibe-sprint"
	vibeOwnerDebateFlowID       = "vibe-owner-debate"
	vibeCpLockNodeID            = "cp_lock"
	vibeSSLockNodeID            = "ss_lock"
	vibeSSValidatorNodeID       = "ss_validator"
	vibeCPValidatorNodeID       = "cp_validator"
	vibeTaskSlicerNodeID        = "task_slicer"
	vibeSprintSlicerNodeID      = "sprint_slicer"
	vibeCpWriterNodeID          = "cp_writer"
	vibeDebateSynthesisNodeID   = "debate_synthesis"
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
	s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = "budget"
		st.GateReason = "vibe total-sprint budget exceeded"
		return st
	})
	s.mu.Lock()
	if rs := s.runs[parentRunID]; rs != nil {
		rs.status = RunStatusWaitingUserApr
		rs.agentStatus = string(RunStatusWaitingUserApr)
	}
	s.mu.Unlock()
}

func (s *InteractiveService) maybeStartNextVibeSprint(parentRunID string) {
	s.mu.Lock()
	rs := s.runs[parentRunID]
	if rs == nil {
		s.mu.Unlock()
		return
	}
	d := s.takeNextVibeSprintLocked(rs)
	s.mu.Unlock()
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
	// The sprint-boundary Continue gate owns the next start once parked;
	// a stray chain (e.g. a replayed audit advance) must not double-start.
	ok := rs != nil && rs.workingMode == workingmode.Vibe && len(rs.vibeTaskPlan) > 0 && !rs.vibeSprintBoundaryPending
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
