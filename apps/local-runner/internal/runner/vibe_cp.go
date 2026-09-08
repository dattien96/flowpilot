package runner

import (
	"context"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"flowpilot-runner/internal/workingmode"
)

const (
	defaultVibeSprintBudget = 8
	vibeIngestFlowID        = "vibe-ingest"
	vibeCpIngestFlowID      = "vibe-cp-ingest"
	vibeSprintFlowID        = "vibe-sprint"
	vibeCpLockNodeID        = "cp_lock"
	vibeSSLockNodeID        = "ss_lock"
	vibeSSValidatorNodeID   = "ss_validator"
	vibeCPValidatorNodeID   = "cp_validator"
	vibeTaskSlicerNodeID    = "task_slicer"
	vibeSprintSlicerNodeID  = "sprint_slicer"
	vibeCpWriterNodeID      = "cp_writer"
)

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
	ok := rs != nil && rs.workingMode == workingmode.Vibe && len(rs.vibeTaskPlan) > 0
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
	case vibeTaskSlicerNodeID, vibeSprintSlicerNodeID:
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
}
