package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Task-342 (CP-62 P-6): sprint-handoff artifact. The vibe-sprint audit hub
// writes one handoff per sprint from VERIFIED run state only (completed step
// nodes + the flow-control verdict rows the reviewers submitted) — the CP-49
// hard-ceiling rule applied to runtime: what/why recorded, never invented.
// The next sprint's entry prompt carries the previous handoff verbatim so
// decisions survive across sprints, restarts, and machines (CP-62 D-6).
//
// Storage note: the file is JSON (a valid YAML 1.2 subset) so no new module
// dependency is introduced; the schema field names match CP-62 P-6 exactly.

type SprintDecision struct {
	What         string   `json:"what"`
	Why          string   `json:"why,omitempty"`
	Alternatives []string `json:"alternatives,omitempty"`
}

type WeakenedTest struct {
	Path          string `json:"path"`
	Line          int    `json:"line,omitempty"`
	Justification string `json:"justification,omitempty"`
}

type SprintHandoffV1 struct {
	Sprint        int              `json:"sprint"`
	Task          string           `json:"task,omitempty"`
	Done          []string         `json:"done,omitempty"`
	Decisions     []SprintDecision `json:"decisions,omitempty"`
	Open          []string         `json:"open,omitempty"`
	Risks         []string         `json:"risks,omitempty"`
	WeakenedTests []WeakenedTest   `json:"weakened_tests,omitempty"`
}

func sprintHandoffPath(workspaceCwd string, sprint int) string {
	return filepath.Join(workspaceCwd, "requirements", ".flowpilot", "vibe", "handoffs",
		fmt.Sprintf("handoff-sprint-%d.yaml", sprint))
}

// completedSprintNodeIDs lists the flow step node ids that reached DONE for
// the run (best-effort: a workflow store miss yields just the audit node —
// the handoff must never fail the sprint chain over observability).
func (s *InteractiveService) completedSprintNodeIDs(parentRunID string) []string {
	done := []string{"audit"}
	if s == nil || s.workflowStore == nil {
		return done
	}
	steps, err := s.workflowStore.LoadRunSteps(context.Background(), parentRunID)
	if err != nil {
		return done
	}
	done = done[:0]
	for _, step := range steps {
		if step.Status != StepStatusDone {
			continue
		}
		id := strings.TrimSpace(step.NodeID)
		if id == "" {
			id = strings.TrimSpace(step.ID)
		}
		if id != "" {
			done = append(done, id)
		}
	}
	return done
}

// emitSprintHandoff writes the handoff for the sprint that just finished
// (audit completed). Verified inputs only: completed step nodes + the run's
// captured flow-control verdict rows. Best-effort: an I/O failure is logged
// and never blocks the sprint chain (the next sprint falls back gracefully).
func (s *InteractiveService) emitSprintHandoff(rs *interactiveRun) string {
	if s == nil || rs == nil || strings.TrimSpace(rs.workspaceCwd) == "" {
		return ""
	}
	s.mu.Lock()
	sprint, task := rs.vibeSprintIndex, rs.vibeTaskName
	verdicts := append([]VerdictRow(nil), rs.lastFlowVerdicts...)
	var card *UserDecisionCard
	if rs.decisionCard != nil {
		cardCopy := *rs.decisionCard
		card = &cardCopy
	}
	chosen := strings.TrimSpace(rs.decisionCardChosen)
	tampered := append([]string(nil), rs.lastTamperedTestPaths...)
	cwd := rs.workspaceCwd
	s.mu.Unlock()

	handoff := SprintHandoffV1{Sprint: sprint, Task: task, Done: s.completedSprintNodeIDs(rs.id)}
	for _, row := range verdicts {
		what := strings.TrimSpace(row.ACID) + ": " + row.Verdict
		dec := SprintDecision{What: what, Why: strings.TrimSpace(row.Note)}
		if row.Verdict == "fail" || row.Verdict == "blocked" {
			dec.What = "open finding " + what
			handoff.Risks = append(handoff.Risks, dec.What)
		}
		handoff.Decisions = append(handoff.Decisions, dec)
	}
	// Task-346: the parked decision card (Task-339/345) is a verified decision
	// source — the question plus the human's choice, or the runner's
	// recommendation when the answer was prose. Remaining options become the
	// alternatives. CP-49 hard ceiling: only what actually happened.
	if card != nil && len(card.Options) > 0 {
		why := strings.TrimSpace(card.Detail)
		var alternatives []string
		for _, opt := range card.Options {
			if chosen != "" && strings.EqualFold(strings.TrimSpace(opt.ID), chosen) {
				why = opt.Label + " — " + opt.Consequence
				continue
			}
			alternatives = append(alternatives, opt.Label)
		}
		if why == "" {
			why = "recommended: " + strings.TrimSpace(card.Recommended)
		}
		what := card.Question
		if chosen != "" {
			what = "user chose " + chosen + " — " + card.Question
		}
		handoff.Decisions = append(handoff.Decisions, SprintDecision{What: what, Why: why, Alternatives: alternatives})
	}
	// Task-346: oracle-guard tampered test files record as weakened_tests so
	// the next sprint knows which pre-existing tests were touched and why.
	for _, p := range tampered {
		handoff.WeakenedTests = append(handoff.WeakenedTests, WeakenedTest{
			Path:          p,
			Justification: "oracle guard flagged this pre-existing test file as changed during the sprint",
		})
	}
	path := sprintHandoffPath(cwd, sprint)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return ""
	}
	data, err := json.MarshalIndent(handoff, "", "  ")
	if err != nil {
		return ""
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return ""
	}
	return path
}

// previousSprintHandoffContext reads the handoff of the sprint that just
// ended (index-1 after the cursor increment) for the next sprint's entry
// prompt. Missing/unreadable file → empty string (graceful fallback, T-4).
func previousSprintHandoffContext(workspaceCwd string, currentSprintIndex int) string {
	if strings.TrimSpace(workspaceCwd) == "" || currentSprintIndex <= 1 {
		return ""
	}
	data, err := os.ReadFile(sprintHandoffPath(workspaceCwd, currentSprintIndex-1))
	if err != nil {
		return ""
	}
	content := strings.TrimSpace(string(data))
	if content == "" {
		return ""
	}
	return "[Sprint Handoff — verified context from the previous sprint (sprint " +
		fmt.Sprint(currentSprintIndex-1) + ")]\n" + content
}

// setTamperedTestPaths records the oracle guard's tampered pre-existing test
// files on run state (Task-346): the sprint handoff's weakened_tests source.
func (s *InteractiveService) setTamperedTestPaths(rs *interactiveRun, paths []string) {
	if s == nil || rs == nil || len(paths) == 0 {
		return
	}
	s.mu.Lock()
	rs.lastTamperedTestPaths = append([]string(nil), paths...)
	s.mu.Unlock()
}

// captureDecisionChoice records the human's choice on a parked decision card
// (Task-346): continue feedback matching an option id or label
// (case-insensitive) stamps the choice for the sprint handoff; prose answers
// leave the choice empty — the fallback never guesses.
func (s *InteractiveService) captureDecisionChoice(runID, feedback string) {
	feedback = strings.TrimSpace(feedback)
	if feedback == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rs := s.runs[runID]
	if rs == nil || rs.decisionCard == nil {
		return
	}
	for _, opt := range rs.decisionCard.Options {
		if strings.EqualFold(feedback, strings.TrimSpace(opt.ID)) || strings.EqualFold(feedback, strings.TrimSpace(opt.Label)) {
			rs.decisionCardChosen = strings.TrimSpace(opt.ID)
			return
		}
	}
}
