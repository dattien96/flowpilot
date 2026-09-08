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
	vibeTaskSlicerNodeID    = "task_slicer"
	vibeSprintSlicerNodeID  = "sprint_slicer"
)

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

func (s *InteractiveService) onVibeCpNodeDone(parentRunID, completedNodeID string) {
	switch completedNodeID {
	case vibeCpLockNodeID, vibeSSLockNodeID:
		s.mu.Lock()
		if rs := s.runs[parentRunID]; rs != nil {
			rs.vibeAwaitingLock = false
		}
		s.mu.Unlock()
	case vibeTaskSlicerNodeID, vibeSprintSlicerNodeID:
		s.mu.Lock()
		var plan []string
		if rs := s.runs[parentRunID]; rs != nil {
			if len(rs.vibeTaskPlan) == 0 {
				if collected := collectVibeTaskPlan(rs.workspaceCwd); len(collected) > 0 {
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
