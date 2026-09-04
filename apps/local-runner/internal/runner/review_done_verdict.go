package runner

import (
	"fmt"
	"strings"

	"flowpilot-runner/internal/agentpack"
)

const synthesisAcceptanceNodeID = "synthesis"

// flowRequiresSynthesisMachineVerdict reports whether this run's active flow
// declares synthesis as an acceptance node (CP-53 P-2 / Task-274). Only those
// flows enforce reviewer machine verdicts before hub approved→done.
func flowRequiresSynthesisMachineVerdict(rs *interactiveRun) bool {
	if rs == nil {
		return false
	}
	for _, id := range rs.activeFlowAcceptanceNodes {
		if strings.TrimSpace(id) == synthesisAcceptanceNodeID {
			return true
		}
	}
	return false
}

// reviewCohortNodeLabels returns flow node ids for cohort=review members.
func reviewCohortNodeLabels(nodes []agentpack.FlowNode) []string {
	if len(nodes) == 0 {
		return nil
	}
	out := make([]string, 0, 2)
	for _, n := range nodes {
		if strings.TrimSpace(n.Cohort) != "review" {
			continue
		}
		if id := strings.TrimSpace(n.ID); id != "" {
			out = append(out, id)
		}
	}
	return out
}

func (s *InteractiveService) recordReviewCohortMemberVerdict(parentRunID, label, domainStatus string) {
	label = strings.TrimSpace(label)
	domainStatus = strings.TrimSpace(domainStatus)
	if parentRunID == "" || label == "" || domainStatus == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	parent := s.runs[parentRunID]
	if parent == nil {
		return
	}
	if parent.pendingReviewVerdictByLabel == nil {
		parent.pendingReviewVerdictByLabel = make(map[string]string)
	}
	parent.pendingReviewVerdictByLabel[label] = domainStatus
}

func (s *InteractiveService) snapshotReviewCohortVerdicts(parentRunID string, entries []cohortEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.snapshotReviewCohortVerdictsLocked(parentRunID, entries)
}

// snapshotReviewCohortVerdictsLocked writes lastReviewCohortVerdicts.
// Caller holds s.mu (settleFlowChildTurnCompletedLocked already holds it).
func (s *InteractiveService) snapshotReviewCohortVerdictsLocked(parentRunID string, entries []cohortEntry) {
	parent := s.runs[parentRunID]
	if parent == nil {
		return
	}
	verdicts := make(map[string]string, len(entries))
	for _, e := range entries {
		label := strings.TrimSpace(e.Label)
		if label == "" {
			continue
		}
		if v := strings.TrimSpace(e.MachineVerdict); v != "" {
			verdicts[label] = v
		}
	}
	parent.lastReviewCohortVerdicts = verdicts
}

// hubProseVerdictDerivesFlowStatus reports the flow transition a prose-only hub
// synthesis turn (no submit_review_outcome call) should drive, derived from the
// machine verdicts the just-joined cohort recorded (run-200816):
//
//   - "done"     — every verdict is approved        → advance (plan approve → freeze)
//   - "continue" — any verdict is changes_requested → re-enter the writer loop
//   - ""         — verdicts empty / blocked / mixed → caller keeps BUG-226 escalate
func (s *InteractiveService) hubProseVerdictDerivesFlowStatus(parentRunID string) string {
	s.mu.Lock()
	parent := s.runs[parentRunID]
	var verdicts map[string]string
	if parent != nil {
		verdicts = parent.lastReviewCohortVerdicts
	}
	s.mu.Unlock()
	if len(verdicts) == 0 {
		return ""
	}
	anyChanges := false
	for _, v := range verdicts {
		switch v {
		case "approved":
		case "changes_requested":
			anyChanges = true
		default:
			return "" // blocked / unknown → keep escalate
		}
	}
	if anyChanges {
		return "continue"
	}
	return "done"
}

// advanceHubFromCohortMachineVerdicts drives the flow transition derived from
// the last joined cohort's machine verdicts when the hub finished in prose.
// Returns true when a transition was applied (done/continue) so the caller
// skips BUG-226's escalate — mirrors what the hub's own submit_review_outcome
// call would have done.
func (s *InteractiveService) advanceHubFromCohortMachineVerdicts(parentRunID string) bool {
	status := s.hubProseVerdictDerivesFlowStatus(parentRunID)
	if status != "done" && status != "continue" {
		return false
	}
	_, err := s.applyFlowControl(parentRunID, FlowControlInput{Status: status})
	return err == nil
}

// synthesisDoneVerdictError describes why hub approved→done was rejected.
func (s *InteractiveService) synthesisDoneVerdictError(parentRunID string) error {
	s.mu.Lock()
	parent := s.runs[parentRunID]
	var expected []string
	var verdicts map[string]string
	if parent != nil {
		expected = reviewCohortNodeLabels(parent.activeFlowNodes)
		verdicts = parent.lastReviewCohortVerdicts
	}
	s.mu.Unlock()

	if len(expected) == 0 {
		return fmt.Errorf("applyFlowControl: synthesis done requires reviewer machine verdicts but no review cohort nodes are configured")
	}
	var missing, notApproved []string
	for _, label := range expected {
		v, ok := verdicts[label]
		if !ok || v == "" {
			missing = append(missing, label)
			continue
		}
		if v != "approved" {
			notApproved = append(notApproved, label+"="+v)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("applyFlowControl: synthesis done blocked — missing machine verdict from reviewer(s): %s", strings.Join(missing, ", "))
	}
	if len(notApproved) > 0 {
		return fmt.Errorf("applyFlowControl: synthesis done blocked — reviewer verdict not approved: %s", strings.Join(notApproved, ", "))
	}
	return nil
}
