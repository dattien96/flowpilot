package runner

import (
	"context"
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

// cohortNodeLabels returns flow node ids for members of the given cohort.
func cohortNodeLabels(nodes []agentpack.FlowNode, cohort string) []string {
	if len(nodes) == 0 {
		return nil
	}
	cohort = strings.TrimSpace(cohort)
	if cohort == "" {
		return nil
	}
	out := make([]string, 0, 2)
	for _, n := range nodes {
		if strings.TrimSpace(n.Cohort) != cohort {
			continue
		}
		if id := strings.TrimSpace(n.ID); id != "" {
			out = append(out, id)
		}
	}
	return out
}

// reviewCohortNodeLabels returns flow node ids for cohort=review members.
func reviewCohortNodeLabels(nodes []agentpack.FlowNode) []string {
	return cohortNodeLabels(nodes, "review")
}

// hubInboundCohortName maps a harness hub to the cohort whose machine verdicts
// must PASS before its done-successor may dispatch (CP-61 P-1). Unknown hubs
// return "" and stay ungated, same as today.
func hubInboundCohortName(hubID string) string {
	switch strings.TrimSpace(hubID) {
	case "plan_synthesis", "cp_synthesis":
		return "plan"
	case "synthesis":
		return "review"
	default:
		return ""
	}
}

// flowRequiresHubMachineVerdict reports whether the given hub's done edge is
// gated on inbound-cohort machine verdicts: the hub is a known harness hub
// and the flow declares inbound cohort nodes for it.
func flowRequiresHubMachineVerdict(rs *interactiveRun, hubID string) bool {
	if rs == nil {
		return false
	}
	cohort := hubInboundCohortName(hubID)
	if cohort == "" {
		return false
	}
	return len(cohortNodeLabels(rs.activeFlowNodes, cohort)) > 0
}

func (s *InteractiveService) recordReviewCohortMemberVerdict(parentRunID, label, domainStatus, detail string) {
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
	if detail = strings.TrimSpace(detail); detail != "" {
		if parent.pendingReviewVerdictDetailByLabel == nil {
			parent.pendingReviewVerdictDetailByLabel = make(map[string]string)
		}
		parent.pendingReviewVerdictDetailByLabel[label] = detail
	}
}

// reviewVerdictDetailForCohort renders the verdict CONTENT buffered beside
// the status (CA-1095): feedback summary + per-AC verdict rows + issues, so
// the joined cohort note carries the reviewer's actual decision — a
// verdict_only owner child's thin final message is no longer the only text
// the synthesis hub sees.
func reviewVerdictDetailForCohort(in FlowControlInput) string {
	var b strings.Builder
	if s := strings.TrimSpace(in.Summary); s != "" {
		b.WriteString(s)
	}
	if rows, ok := in.Payload["verdicts"].([]VerdictRow); ok {
		for _, r := range rows {
			line := strings.TrimSpace(r.ACID) + ": " + strings.TrimSpace(r.Verdict)
			if n := strings.TrimSpace(r.Note); n != "" {
				line += " — " + n
			}
			if strings.TrimSpace(line) != ":" {
				fmt.Fprintf(&b, "\n  - %s", line)
			}
		}
	}
	if issues, ok := in.Payload["issues"].([]ReviewIssue); ok {
		for _, is := range issues {
			line := strings.TrimSpace(is.Title)
			if f := strings.TrimSpace(is.File); f != "" {
				line += " (" + f + ")"
			}
			if line != "" {
				fmt.Fprintf(&b, "\n  - issue: %s", line)
			}
		}
	}
	return b.String()
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
		verdicts = mergePendingReviewVerdictsLocked(parent, parent.lastReviewCohortVerdicts)
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
	if status == "done" {
		// run-200816/run-201295: a prose-approved plan must advance through the
		// hub's own done-successor edge (plan_synthesis -> preflight_contract_freeze)
		// exactly like the hub's submit_review_outcome call would — NOT settle the
		// whole flow via applyFlowControl("done"), which skips the freeze node and
		// left the next step PENDING with no successor running.
		if _, handled := s.advanceHubDoneThroughEdge(parentRunID, FlowControlInput{Status: "done", Summary: "Approved by cohort machine verdicts"}); handled {
			return true
		}
	}
	_, err := s.applyFlowControl(parentRunID, FlowControlInput{Status: status})
	return err == nil
}

// mergePendingReviewVerdictsLocked folds reviewer verdicts that landed after
// their member's turn settle into the cohort verdict view. Deferred tool
// transports (e.g. grok's search_tool/use_tool hop) can deliver the
// submit_review_outcome MCP call AFTER the ACP turn-end already consumed
// pendingReviewVerdictByLabel into the cohort entry — the record then sits in
// the pending map forever while the hub reports a missing verdict. Credited
// entries are consumed so they cannot leak into a later cohort round.
// Caller holds s.mu.
func mergePendingReviewVerdictsLocked(parent *interactiveRun, verdicts map[string]string) map[string]string {
	if parent == nil || len(parent.pendingReviewVerdictByLabel) == 0 {
		return verdicts
	}
	merged := make(map[string]string, len(verdicts)+len(parent.pendingReviewVerdictByLabel))
	for k, v := range verdicts {
		merged[k] = v
	}
	for k, v := range parent.pendingReviewVerdictByLabel {
		if _, ok := merged[k]; !ok && strings.TrimSpace(v) != "" {
			merged[k] = v
			delete(parent.pendingReviewVerdictByLabel, k)
		}
	}
	return merged
}

// synthesisDoneVerdictError describes why hub approved→done was rejected.
func (s *InteractiveService) synthesisDoneVerdictError(parentRunID string) error {
	s.mu.Lock()
	parent := s.runs[parentRunID]
	var expected []string
	var verdicts map[string]string
	hasOwnerDebate := false
	if parent != nil {
		expected = reviewCohortNodeLabels(parent.activeFlowNodes)
		verdicts = mergePendingReviewVerdictsLocked(parent, parent.lastReviewCohortVerdicts)
		hasOwnerDebate = len(cohortNodeLabels(parent.activeFlowNodes, "owner_debate")) > 0
	}
	s.mu.Unlock()

	if len(expected) == 0 {
		if hasOwnerDebate {
			return nil
		}
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

// hubDoneVerdictError describes why a harness hub's approved→done was
// rejected (CP-61 P-1). Expected labels come from the hub's inbound cohort
// (plan_synthesis/cp_synthesis → plan, synthesis → review), not from every
// cohort:review node on the graph.
func (s *InteractiveService) hubDoneVerdictError(parentRunID, hubID string) error {
	hubID = strings.TrimSpace(hubID)
	cohort := hubInboundCohortName(hubID)
	if cohort == "" {
		return fmt.Errorf("advanceHubDoneThroughEdge: %s done blocked — unknown hub %q", hubID, hubID)
	}
	s.mu.Lock()
	parent := s.runs[parentRunID]
	var expected []string
	var verdicts map[string]string
	if parent != nil {
		expected = cohortNodeLabels(parent.activeFlowNodes, cohort)
		verdicts = mergePendingReviewVerdictsLocked(parent, parent.lastReviewCohortVerdicts)
	}
	s.mu.Unlock()

	if len(expected) == 0 {
		return fmt.Errorf("advanceHubDoneThroughEdge: %s done requires %s reviewer machine verdicts but no %s cohort nodes are configured", hubID, cohort, cohort)
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
		return fmt.Errorf("advanceHubDoneThroughEdge: %s done blocked — missing machine verdict from %s reviewer(s): %s", hubID, cohort, strings.Join(missing, ", "))
	}
	if len(notApproved) > 0 {
		return fmt.Errorf("advanceHubDoneThroughEdge: %s done blocked — reviewer verdict not approved: %s", hubID, strings.Join(notApproved, ", "))
	}
	return nil
}

// isReviewVerdictGateReason reports whether an escalate GateReason belongs to
// the hub-done verdict-gate family (hubDoneVerdictError /
// synthesisDoneVerdictError): a cohort member produced no machine verdict, or
// a recorded verdict that is not approved. Continue on those parks must
// re-drive the deficient member — a hub re-prompt can never produce the
// missing verdict (CA-1098, live run-31884).
func isReviewVerdictGateReason(reason string) bool {
	r := strings.ToLower(strings.TrimSpace(reason))
	return strings.Contains(r, "done blocked") &&
		(strings.Contains(r, "machine verdict") || strings.Contains(r, "verdict not approved"))
}

// resumeVerdictDeficientMembers re-drives every review-cohort member child
// whose machine verdict is missing or not approved for the run's active hub
// (CA-1098). Each matched member's settle-time reprompt budget is reset — a
// user-driven Continue is a fresh attempt, not a continuation of the
// exhausted budget. Returns false when no deficient member maps to a live
// child, letting the caller fall through to the generic resume.
func (s *InteractiveService) resumeVerdictDeficientMembers(parentRunID string) bool {
	s.mu.Lock()
	parent := s.runs[parentRunID]
	if parent == nil {
		s.mu.Unlock()
		return false
	}
	hubID := strings.TrimSpace(parent.activeHubNodeID)
	if hubID == "" {
		hubID = hubInlineNodeID(parent.activeFlowNodes)
	}
	expected := cohortNodeLabels(parent.activeFlowNodes, hubInboundCohortName(hubID))
	if len(expected) == 0 {
		s.mu.Unlock()
		return false
	}
	verdicts := mergePendingReviewVerdictsLocked(parent, parent.lastReviewCohortVerdicts)
	deficient := map[string]bool{}
	for _, label := range expected {
		if v := strings.TrimSpace(verdicts[label]); v != "approved" {
			deficient[label] = true
		}
	}
	if len(deficient) == 0 {
		s.mu.Unlock()
		return false
	}
	var labels []string
	seen := map[string]bool{}
	for _, childID := range s.agentOrchestrator.listChildren(parentRunID) {
		child := s.runs[childID]
		if child == nil || !deficient[child.label] || seen[child.label] {
			continue
		}
		seen[child.label] = true
		child.verdictRepromptCount = 0
		// BUG-403: arm the flag so a failed scheduled turn retries/drains
		// instead of silently dropping the verdict retry.
		child.reinvokeInFlight = true
		// BUG-559: same marker as the settle-path reprompt — this re-drive is
		// a verdict turn, not a draft attempt; its completion must not clear
		// a stashed preflight draft.
		child.verdictRepromptInFlight = true
		labels = append(labels, child.label)
	}
	s.mu.Unlock()
	if len(labels) == 0 {
		return false
	}
	prompt := "[flow-engine] The synthesis gate rejected the hub's done: your machine verdict " +
		"was missing or not approved. Re-evaluate and call submit_review_outcome as the FIRST " +
		"action of this turn with status=approved|changes_requested|blocked and a verdicts " +
		"array containing one row per acceptance criterion. Wait for the tool result before " +
		"writing any summary text."
	redriven := false
	for _, label := range labels {
		want := label
		if s.reinvokeMatchingFlowChild(parentRunID, prompt, func(child *interactiveRun) bool {
			return child.label == want
		}) {
			s.setFlowStepStatus(context.Background(), parentRunID, label, StepStatusRunning)
			redriven = true
		}
	}
	if redriven {
		s.flowDiagLog(parentRunID, "verdict_deficient_member_redrive",
			"missing/deficient verdict park: re-driving cohort members",
			"labels", strings.Join(labels, ","))
	}
	return redriven
}

// hubDoneCohortHasChangesRequested reports whether any expected inbound-cohort
// verdict for the hub is changes_requested (CP-61 P-1 continue-vs-escalate split).
func (s *InteractiveService) hubDoneCohortHasChangesRequested(parentRunID, hubID string) bool {
	cohort := hubInboundCohortName(hubID)
	if cohort == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	parent := s.runs[parentRunID]
	if parent == nil {
		return false
	}
	verdicts := mergePendingReviewVerdictsLocked(parent, parent.lastReviewCohortVerdicts)
	for _, label := range cohortNodeLabels(parent.activeFlowNodes, cohort) {
		if verdicts[label] == "changes_requested" {
			return true
		}
	}
	return false
}
