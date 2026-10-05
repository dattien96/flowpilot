package runner

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"flowpilot-runner/internal/workingmode"
)

const (
	vibeAdoptFlowID            = "vibe-adopt"
	vibeAdoptSprintFlowID      = "vibe-adopt-sprint"
	vibeAdoptSelectNodeID      = "adopt_select"
	vibeAdoptSelectBlockReason = "vibe_adopt_select"
	// vibeAdoptSelectQuestionKind marks the engine-emitted adopt-scope /
	// candidate cards so AnswerQuestion routes the resolved choice to
	// applyVibeAdoptSelectAnswer (same contract as usageBudgetQuestionKind).
	// Rehydrated records recover the kind from the persisted prompt prefix.
	vibeAdoptSelectQuestionKind = "vibe_adopt_select"
)

var vibeAdoptTaskIDRe = regexp.MustCompile(`(?i)Task-0*([0-9]+)`)
var vibeAdoptCPIDRe = regexp.MustCompile(`(?i)(CP-[0-9]+)`)

// vibeSprintIsAdopt reports whether this run's sprint chain mounts
// vibe-adopt-sprint graphs — set by the vibe-adopt wrapper (chatFlowRef is
// either the wrapper itself or a mounted adopt-sprint, since the sprint
// mount stamps chatFlowRef with the child ref).
func vibeSprintIsAdopt(rs *interactiveRun) bool {
	if rs == nil {
		return false
	}
	bare := workingmode.BareFlowID(rs.chatFlowRef)
	return bare == vibeAdoptFlowID || bare == vibeAdoptSprintFlowID
}

// vibeSprintFlowRefFor returns the flow ref mounted per plan slot:
// vibe-tasks mounts vibe-sprint; vibe-adopt mounts vibe-adopt-sprint
// (verify-first topology, Task-459).
func vibeSprintFlowRefFor(rs *interactiveRun) string {
	if vibeSprintIsAdopt(rs) {
		return workingmode.PackPrefix + vibeAdoptSprintFlowID
	}
	return workingmode.PackPrefix + vibeSprintFlowID
}

// vibeAdoptSelectStageFromPrompt recovers which select stage a persisted
// question belongs to. Prompts are prefixed `vibe_adopt_select:<stage>:` so
// the stage survives restart the same way the kind does.
func vibeAdoptSelectStageFromPrompt(prompt string) string {
	rest := strings.TrimPrefix(prompt, vibeAdoptSelectQuestionKind+":")
	if rest == prompt {
		return ""
	}
	if i := strings.Index(rest, ":"); i >= 0 {
		return strings.TrimSpace(rest[:i])
	}
	return ""
}

// collectVibeAdoptTaskDocs enumerates Task-*.md candidates for single-task
// adoption — todo/, inprogress/ and done/ are all valid adopt targets (the
// point of adopt is the code already exists regardless of doc lane).
func collectVibeAdoptTaskDocs(cwd string) []string {
	return globVibeDocRelPaths(cwd, "requirements", "08-Task", "Task-")
}

// collectVibeAdoptCPDocs enumerates CP-*.md candidates for whole-CP adoption.
func collectVibeAdoptCPDocs(cwd string) []string {
	return globVibeDocRelPaths(cwd, "requirements", "07-Coding-Plan", "CP-")
}

func globVibeDocRelPaths(cwd, dir1, dir2, prefix string) []string {
	if strings.TrimSpace(cwd) == "" {
		return nil
	}
	var matches []string
	for _, lane := range []string{"todo", "inprogress", "approved", "done"} {
		got, err := filepath.Glob(filepath.Join(cwd, dir1, dir2, lane, prefix+"*.md"))
		if err != nil {
			continue
		}
		matches = append(matches, got...)
	}
	if len(matches) == 0 {
		return nil
	}
	out := make([]string, 0, len(matches))
	for _, abs := range matches {
		if strings.HasPrefix(strings.ToUpper(filepath.Base(abs)), "FORMAT-") {
			continue
		}
		rel, err := filepath.Rel(cwd, abs)
		if err != nil {
			rel = abs
		}
		out = append(out, filepath.ToSlash(rel))
	}
	sortVibeDocPathsByNumericID(out)
	return out
}

// sortVibeDocPathsByNumericID orders task/cp doc paths by their embedded
// numeric id so a mixed todo+done plan reads Task-051, Task-052, … rather
// than grouping by directory.
func sortVibeDocPathsByNumericID(paths []string) {
	num := func(p string) int {
		if m := vibeAdoptTaskIDRe.FindStringSubmatch(filepath.Base(p)); len(m) == 2 {
			if n, err := strconv.Atoi(m[1]); err == nil {
				return n
			}
		}
		if m := vibeAdoptCPIDRe.FindStringSubmatch(filepath.Base(p)); len(m) == 2 {
			if n, err := strconv.Atoi(strings.TrimPrefix(strings.ToUpper(m[1]), "CP-")); err == nil {
				return n
			}
		}
		return 0
	}
	sort.SliceStable(paths, func(i, j int) bool {
		ni, nj := num(paths[i]), num(paths[j])
		if ni != nj {
			return ni < nj
		}
		return paths[i] < paths[j]
	})
}

// collectVibeAdoptPlanForCP is the adopt-chain plan: every Task parented to
// cpID under todo/ AND done/ (adopt verifies already-landed work — a task in
// done/ is the primary adopt target, not a reason to skip it).
func collectVibeAdoptPlanForCP(cwd, cpID string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range collectVibeTaskPlanForCP(cwd, cpID) {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, p := range collectVibeDoneTasksForCP(cwd, cpID) {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sortVibeDocPathsByNumericID(out)
	return out
}

// vibeAdoptSelectPendingLocked reports whether an adopt-select question is
// still awaiting an answer for this run. Caller holds s.mu.
func (s *InteractiveService) vibeAdoptSelectPendingLocked(runID string) bool {
	for _, rec := range s.questions {
		if rec == nil || rec.runID != runID {
			continue
		}
		if questionRecordKind(rec) == vibeAdoptSelectQuestionKind &&
			(rec.status == "pending" || rec.status == "resolving") {
			return true
		}
	}
	return false
}

// parkVibeAdoptSelect is the vibe-adopt wrapper's only node body: it parks
// adopt_select behind an engine question card. Stage "scope" offers
// [task, cp]; the resolved stage then re-emits the matching candidate list.
// When a select card is already pending (flow re-mount after rehydrate or a
// re-entry while the operator is mid-pick) the node re-parks behind the live
// card instead of asking twice.
func (s *InteractiveService) parkVibeAdoptSelect(parentRunID string) {
	s.mu.Lock()
	pending := s.vibeAdoptSelectPendingLocked(parentRunID)
	s.mu.Unlock()
	if pending {
		s.setFlowStepStatus(context.Background(), parentRunID, vibeAdoptSelectNodeID, StepStatusWaitingUserApr)
		snap := s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
			st.Status = "blocked"
			st.BlockReason = vibeAdoptSelectBlockReason
			st.ActiveNode = vibeAdoptSelectNodeID
			return st
		})
		s.parkFlowForAwaitingUser(parentRunID)
		s.emitAgentGraph(parentRunID, snap)
		return
	}
	s.emitVibeAdoptSelectQuestion(parentRunID, "scope", []QuestionOption{
		{Label: "task", Description: "adopt one task — verify-first sprint over a single Task doc", Value: "task"},
		{Label: "cp", Description: "adopt a whole CP — sprint chain over every Task parented to it", Value: "cp"},
	}, "adopt scope: adopt a single task or an entire CP?")
}

// emitVibeAdoptSelectQuestion creates+persists+emits one adopt-select card
// and parks the node. Mirrors the usage_budget emit shape: durable record
// first, then the card event; the node waits with the loop blocked so no
// other surface can drive the run while the operator is picking.
func (s *InteractiveService) emitVibeAdoptSelectQuestion(parentRunID, stage string, options []QuestionOption, human string) {
	if strings.TrimSpace(parentRunID) == "" {
		return
	}
	expiresAt := time.Now().UTC().Add(s.questionTTL).Format(time.RFC3339Nano)
	prompt := fmt.Sprintf("%s:%s: %s", vibeAdoptSelectQuestionKind, stage, human)
	s.mu.Lock()
	rs := s.runs[parentRunID]
	if rs == nil {
		s.mu.Unlock()
		return
	}
	rec := &questionRecord{
		id:        s.nextID("q"),
		runID:     parentRunID,
		prompt:    prompt,
		options:   options,
		status:    "pending",
		resolve:   make(chan questionResolveResult, 1),
		expiresAt: expiresAt,
		revision:  1,
		createdAt: time.Now().UTC().Format(time.RFC3339Nano),
		kind:      vibeAdoptSelectQuestionKind,
	}
	s.questions[rec.id] = rec
	rs.pendingQuestionID = rec.id
	questionID := rec.id
	snapshot := questionStateFromRecord(rec, "", expiresAt)
	s.mu.Unlock()

	// Persist pending BEFORE emit (BUG-288 P1-07 pattern) so restart never
	// resurrects a card that was never durably asked.
	if err := s.persistQuestion(snapshot); err != nil {
		s.mu.Lock()
		delete(s.questions, questionID)
		if rs := s.runs[parentRunID]; rs != nil && rs.pendingQuestionID == questionID {
			rs.pendingQuestionID = ""
		}
		s.mu.Unlock()
		s.parkVibeRequirementFrom(parentRunID, "vibe-adopt: failed to persist select card: "+err.Error(), vibeAdoptSelectNodeID)
		return
	}

	s.mu.Lock()
	if rs := s.runs[parentRunID]; rs != nil {
		s.emitLocked(rs, ProviderEvent{
			Type:       EventUserQuestionRequired,
			QuestionID: questionID,
			Prompt:     prompt,
			Options:    options,
		})
	}
	s.mu.Unlock()

	s.setFlowStepStatus(context.Background(), parentRunID, vibeAdoptSelectNodeID, StepStatusWaitingUserApr)
	snap := s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = vibeAdoptSelectBlockReason
		st.GateReason = human
		st.ActiveNode = vibeAdoptSelectNodeID
		return st
	})
	s.parkFlowForAwaitingUser(parentRunID)
	s.emitAgentGraph(parentRunID, snap)
	go s.persistParentSession(parentRunID)
}

// applyVibeAdoptSelectAnswer consumes a resolved adopt-select card. Called
// from AnswerQuestion's kind router after the durable resolved-commit. The
// stage comes from the persisted prompt prefix, so a rehydrated card drives
// the same transition.
func (s *InteractiveService) applyVibeAdoptSelectAnswer(rs *interactiveRun, rec *questionRecord) {
	if s == nil || rs == nil || rec == nil || len(rec.choice) == 0 {
		return
	}
	choice := strings.TrimSpace(rec.choice[0])
	stage := vibeAdoptSelectStageFromPrompt(rec.prompt)
	s.mu.Lock()
	cur := s.runs[rs.id]
	cwd := ""
	if cur != nil {
		cwd = cur.workspaceCwd
	}
	s.mu.Unlock()

	switch stage {
	case "scope":
		switch strings.ToLower(choice) {
		case "task":
			var options []QuestionOption
			for _, p := range collectVibeAdoptTaskDocs(cwd) {
				options = append(options, QuestionOption{
					Label: filepath.Base(p),
					Description: p,
					Value: p,
				})
			}
			if len(options) == 0 {
				s.parkVibeRequirementFrom(rs.id, "vibe-adopt: no Task-*.md under requirements/08-Task/{todo,inprogress,done}/ — nothing to adopt", vibeAdoptSelectNodeID)
				return
			}
			s.emitVibeAdoptSelectQuestion(rs.id, "task", options, "pick the task to adopt (one verify-first sprint)")
		case "cp":
			var options []QuestionOption
			for _, p := range collectVibeAdoptCPDocs(cwd) {
				id := p
				if m := vibeAdoptCPIDRe.FindStringSubmatch(filepath.Base(p)); len(m) == 2 {
					id = strings.ToUpper(m[1])
				}
				options = append(options, QuestionOption{
					Label: filepath.Base(p),
					Description: p,
					Value: id,
				})
			}
			if len(options) == 0 {
				s.parkVibeRequirementFrom(rs.id, "vibe-adopt: no CP-*.md under requirements/07-Coding-Plan/{todo,inprogress,approved,done}/ — nothing to adopt", vibeAdoptSelectNodeID)
				return
			}
			s.emitVibeAdoptSelectQuestion(rs.id, "cp", options, "pick the CP to adopt (chains one adopt sprint per task)")
		default:
			s.emitVibeAdoptSelectQuestion(rs.id, "scope", []QuestionOption{
				{Label: "task", Description: "adopt one task", Value: "task"},
				{Label: "cp", Description: "adopt a whole CP", Value: "cp"},
			}, "adopt scope: adopt a single task or an entire CP?")
		}
	case "task":
		// choice is the task doc rel path (option Value).
		if !strings.HasPrefix(strings.ToUpper(filepath.Base(choice)), "TASK-") {
			s.parkVibeRequirementFrom(rs.id, "vibe-adopt: select returned a non-Task target: "+choice, vibeAdoptSelectNodeID)
			return
		}
		s.mu.Lock()
		if cur := s.runs[rs.id]; cur != nil {
			cur.vibeTaskPlan = []string{filepath.ToSlash(choice)}
			cur.vibeSprintIndex = 0
			cur.vibeSprintBoundaryPending = false
			cur.vibeSprintBoundaryTask = ""
			if scaled := vibeSprintBudgetForPlan(1); scaled > cur.vibeSprintBudget {
				cur.vibeSprintBudget = scaled
			}
		}
		s.mu.Unlock()
		s.finishVibeAdoptSelect(rs.id)
	case "cp":
		cpID := strings.ToUpper(choice)
		plan := collectVibeAdoptPlanForCP(cwd, cpID)
		if len(plan) == 0 {
			// Re-emit the CP card rather than dead-ending: the picked CP owns
			// no tasks in either lane, so adoption is undefined for it.
			var options []QuestionOption
			for _, p := range collectVibeAdoptCPDocs(cwd) {
				id := p
				if m := vibeAdoptCPIDRe.FindStringSubmatch(filepath.Base(p)); len(m) == 2 {
					id = strings.ToUpper(m[1])
				}
				options = append(options, QuestionOption{
					Label: filepath.Base(p),
					Description: p,
					Value: id,
				})
			}
			s.emitVibeAdoptSelectQuestion(rs.id, "cp", options,
				fmt.Sprintf("%s has no Task-*.md parented to it (checked todo/ and done/) — pick another CP", cpID))
			return
		}
		s.mu.Lock()
		if cur := s.runs[rs.id]; cur != nil {
			cur.vibeCpDocID = cpID
			cur.vibeTaskPlan = plan
			cur.vibeSprintIndex = 0
			cur.vibeSprintBoundaryPending = false
			cur.vibeSprintBoundaryTask = ""
			if scaled := vibeSprintBudgetForPlan(len(plan)); scaled > cur.vibeSprintBudget {
				cur.vibeSprintBudget = scaled
			}
		}
		s.mu.Unlock()
		s.finishVibeAdoptSelect(rs.id)
	default:
		s.parkVibeRequirementFrom(rs.id, "vibe-adopt: select card carried an unknown stage — refusing to guess the plan", vibeAdoptSelectNodeID)
	}
}

// finishVibeAdoptSelect completes adopt_select after the plan is set:
// clear the block, stamp the node DONE, and hand the completion to the
// chain seam (onVibeCpNodeDone -> maybeStartNextVibeSprint), which mounts
// the first vibe-adopt-sprint.
func (s *InteractiveService) finishVibeAdoptSelect(parentRunID string) {
	s.setFlowStepStatus(context.Background(), parentRunID, vibeAdoptSelectNodeID, StepStatusDone)
	cleared := s.agentOrchestrator.mutateLoop(parentRunID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		st.BlockReason = ""
		st.GateReason = ""
		return st
	})
	s.emitAgentGraph(parentRunID, cleared)
	go s.persistParentSession(parentRunID)
	s.tryAdvanceFlowFromNode(parentRunID, vibeAdoptSelectNodeID, "adopt target selected")
}
