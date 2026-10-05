package runner

import (
	"context"
	"fmt"
	"strings"
	"time"

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

// snapshotReviewCohortVerdictsLocked folds a just-joined cohort's verdicts
// into lastReviewCohortVerdicts. Caller holds s.mu
// (settleFlowChildTurnCompletedLocked already holds it).
//
// BUG-580 (live run-100368): this used to REPLACE the map on every join, so a
// later non-review cohort (owner_debate) erased the reviewer's recorded
// verdict and the synthesis gate read missing forever. Per-label merge: a
// member's fresh verdict overwrites only its own label; a member re-joining
// verdict-less clears its own stale entry (a superseded approval must not
// satisfy the gate for a round that produced nothing); labels absent from
// this join are left untouched.
func (s *InteractiveService) snapshotReviewCohortVerdictsLocked(parentRunID string, entries []cohortEntry) {
	parent := s.runs[parentRunID]
	if parent == nil {
		return
	}
	if parent.lastReviewCohortVerdicts == nil {
		parent.lastReviewCohortVerdicts = make(map[string]string, len(entries))
	}
	for _, e := range entries {
		label := strings.TrimSpace(e.Label)
		if label == "" {
			continue
		}
		if v := strings.TrimSpace(e.MachineVerdict); v != "" {
			parent.lastReviewCohortVerdicts[label] = v
			// BUG-1184: a landing verdict is progress — reset the auto-redrive
			// budget so the NEXT missing-verdict episode gets a fresh bound.
			parent.verdictAutoRedrives = 0
		} else {
			delete(parent.lastReviewCohortVerdicts, label)
		}
	}
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
		// BUG-597 follow-up: the merged map may now also carry verdicts from
		// cohorts whose hub is not machine-verdict gated (owner_debate
		// members record even when requireVerdict is false). Every gate
		// reader filters to the active hub's expected labels — the prose
		// derive must do the same or a stray owner verdict could drive a
		// sprint continue/done. Unmapped hubs and graphs without cohort
		// nodes keep the legacy whole-map behavior.
		hubID := strings.TrimSpace(parent.activeHubNodeID)
		if hubID == "" {
			hubID = hubInlineNodeID(parent.activeFlowNodes)
		}
		if cohort := hubInboundCohortName(hubID); cohort != "" {
			if expected := cohortNodeLabels(parent.activeFlowNodes, cohort); len(expected) > 0 {
				filtered := make(map[string]string, len(expected))
				for _, label := range expected {
					if v, ok := verdicts[label]; ok {
						filtered[label] = v
					}
				}
				verdicts = filtered
			}
		}
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
	// Engine-derived transition from machine verdicts — not an operator
	// decision; the unvisited-spine guard applies (run-2062497).
	_, err := s.applyFlowControl(parentRunID, FlowControlInput{Status: status, viaEngineEdge: true})
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

// missingVerdictLabelsWithLiveMember returns the expected inbound-cohort
// labels for hubID whose machine verdict is still missing AND whose member
// leg is still live — a non-terminal child run with the label, or the flow
// step row still RUNNING (hub ad-hoc spawns carry no flow_cohort_id, so the
// cohort barrier alone misses them — BUG-561's same class on the audit side).
// BUG-565 (live run-69320): escalating on a missing verdict while its member
// is still in flight parks the flow and cancels the member mid-turn, so the
// verdict can never arrive. Callers must DEFER, not escalate.
//
// BUG-1176 (live run-183756): "live" must mean a path still exists that can
// yield the verdict — an in-flight turn, a queued turn prompt, a live user/
// post-turn gate, an armed reprompt/resume intent, or a Starting run —
// mirroring hasActiveFlowChild's activity set. A bare non-terminal record
// with none of those is a zombie: its turn ended without a durable
// transition, no settle or user action will ever fire, and deferring on it
// hung the flow at the sprint boundary until cancellation (the
// no-auto-next-task regression — the boundary auto-advance never ran).
// The step-RUNNING channel is kept only while no non-terminal child record
// carries the label AND the stamp is inside the spawn grace — a stale
// RUNNING step is a dead dispatch, not a live member.
func (s *InteractiveService) missingVerdictLabelsWithLiveMember(parentRunID, hubID string) []string {
	cohort := hubInboundCohortName(hubID)
	if cohort == "" {
		return nil
	}
	s.mu.Lock()
	parent := s.runs[parentRunID]
	var nodes []agentpack.FlowNode
	var verdicts map[string]string
	if parent != nil {
		nodes = parent.activeFlowNodes
		verdicts = mergePendingReviewVerdictsLocked(parent, parent.lastReviewCohortVerdicts)
	}
	expected := cohortNodeLabels(nodes, cohort)
	live := map[string]bool{}
	childSeen := map[string]bool{}
	for _, childID := range s.agentOrchestrator.listChildren(parentRunID) {
		child := s.runs[childID]
		if child == nil {
			continue
		}
		switch child.status {
		case RunStatusCompleted, RunStatusFailed, RunStatusCancelled:
			// A terminal child cannot yield a new verdict, and it must not
			// suppress the step channel — a re-dispatch stamped RUNNING may
			// not have inserted its child row yet.
			continue
		}
		childSeen[child.label] = true
		if s.memberVerdictStillLive(child) {
			live[child.label] = true
		}
	}
	s.mu.Unlock()
	var out []string
	for _, label := range expected {
		if strings.TrimSpace(verdicts[label]) != "" {
			continue // verdict already recorded — not provisional
		}
		if live[label] {
			out = append(out, label)
			continue
		}
		if childSeen[label] {
			// A non-terminal child record exists — its own liveness above is
			// authoritative; a stale RUNNING step stamp must not resurrect it.
			continue
		}
		if s.flowStepRunningWithinSpawnGrace(parentRunID, label) {
			out = append(out, label)
		}
	}
	return out
}

// memberVerdictStillLive reports whether a non-terminal member child still
// has a path that can yield its machine verdict (BUG-1176): a live provider
// turn, a queued turn prompt, a user-visible approval/question gate, an
// armed gate-reprompt or resume intent, a live post-turn gate, or the
// Starting spawn window — the same activity set hasActiveFlowChild and the
// cohort stall sweep already treat as real work. Advisory cards
// (context-pressure / usage-budget) are excluded: they gate nothing and can
// never produce the verdict — live run-204891 wedged the done-edge defer on
// exactly that card (BUG-1186). Called with s.mu held.
func (s *InteractiveService) memberVerdictStillLive(child *interactiveRun) bool {
	if child == nil {
		return false
	}
	turnBusy := child.turnInFlight && child.postTurnGateCancel == nil
	repromptArmed := strings.TrimSpace(child.pendingGateRepromptPrompt) != "" ||
		strings.TrimSpace(child.pendingGateRepromptStepID) != ""
	return turnBusy ||
		strings.TrimSpace(child.pendingTurnPrompt) != "" ||
		child.pendingApprovalID != "" ||
		s.pendingQuestionGatesWorkLocked(child) ||
		repromptArmed ||
		strings.TrimSpace(child.pendingResumePrompt) != "" ||
		gateCancelLive(child.postTurnGateStartedAt, child.postTurnGateCancel) ||
		child.status == RunStatusStarting
}

// pendingQuestionGatesWorkLocked reports whether a run is held by a REAL
// user gate — a pending question whose answer changes the member's work
// (approval cards, quota routing). Advisory cards — context_pressure_90 and
// usage_budget_exceeded — surface a decision but neither block the member's
// turn path nor yield a machine verdict, so they must not count toward
// liveness, stall-suppression or quiet checks (BUG-1186). A dangling
// pendingQuestionID (record not resolvable) is unverifiable durable state:
// fail closed and count it live (BUG-565 posture). Caller holds s.mu.
func (s *InteractiveService) pendingQuestionGatesWorkLocked(rs *interactiveRun) bool {
	if rs == nil {
		return false
	}
	if id := rs.pendingQuestionID; id != "" {
		rec := s.questions[id]
		if rec == nil {
			return true
		}
		if rec.kind != contextPressureQuestionKind && rec.kind != usageBudgetQuestionKind {
			return true
		}
	}
	// pendingQuestionID is single-valued — an advisory card may have
	// overwritten it while an earlier gating question is still open; scan for
	// any surviving gate.
	for _, rec := range s.questions {
		if rec != nil && rec.runID == rs.id &&
			(rec.status == "pending" || rec.status == "resolving") &&
			rec.kind != contextPressureQuestionKind && rec.kind != usageBudgetQuestionKind {
			return true
		}
	}
	return false
}

// flowStepRunningWithinSpawnGrace reports whether the flow step for nodeID
// is RUNNING and still inside the spawn grace — the only shape where the
// missing-verdict defer may count a member live with NO child run record
// (a dispatch in flight before the child row lands). A RUNNING step older
// than defaultStallTimeout with no child is a dead dispatch: escalate and
// let Continue redrive the member. An unparseable/zero StartedAt counts as
// live — BUG-565's safe default on unverifiable rows.
func (s *InteractiveService) flowStepRunningWithinSpawnGrace(parentRunID, nodeID string) bool {
	if s == nil || s.workflowStore == nil {
		return false
	}
	steps, err := s.workflowStore.LoadRunSteps(context.Background(), parentRunID)
	if err != nil {
		return false
	}
	for _, st := range steps {
		if st.ID != nodeID && st.NodeID != nodeID {
			continue
		}
		if st.Status != StepStatusRunning {
			return false
		}
		started, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(st.StartedAt))
		if err != nil {
			return true
		}
		return time.Since(started) < defaultStallTimeout
	}
	return false
}

// maxVerdictAutoRedrives bounds the done-edge auto-redrive of verdict-
// deficient members (BUG-1184): a terminal-failed member leg gets one
// automatic resurrection before the gate escalates to a human park — the
// counter resets whenever a fresh member verdict lands, so the bound applies
// per missing-verdict episode rather than per run lifetime.
const maxVerdictAutoRedrives = 2

// autoRedriveVerdictDeficientMembers re-drives verdict-deficient cohort
// members whose leg is no longer live — a terminal member (failed/cancelled/
// completed verdict-less) cannot produce the missing verdict and previously
// escalated straight to a human park even though resumeVerdictDeficientMembers
// (CA-1098) already resurrects dead legs or spawns replacements (BUG-1184).
// Scoped: the trigger requires a MISSING verdict backed by a terminal member
// record — never-spawned members and bare-zombie children keep the pinned
// escalate contract (CP-61 / BUG-1176), and a recorded-but-not-approved
// verdict stays human-adjudicated. Bounded: verdictAutoRedrives caps attempts
// per episode and resets when a fresh member verdict lands
// (snapshotReviewCohortVerdictsLocked). Returns true when a member was
// re-driven or spawned — the caller must then DEFER the hub done decision
// exactly like the member-in-flight branch.
func (s *InteractiveService) autoRedriveVerdictDeficientMembers(parentRunID string) bool {
	s.mu.Lock()
	rs := s.runs[parentRunID]
	if rs == nil || rs.verdictAutoRedrives >= maxVerdictAutoRedrives {
		s.mu.Unlock()
		return false
	}
	hubID := strings.TrimSpace(rs.activeHubNodeID)
	if hubID == "" {
		hubID = hubInlineNodeID(rs.activeFlowNodes)
	}
	expected := cohortNodeLabels(rs.activeFlowNodes, hubInboundCohortName(hubID))
	verdicts := mergePendingReviewVerdictsLocked(rs, rs.lastReviewCohortVerdicts)
	missing := map[string]bool{}
	for _, label := range expected {
		if strings.TrimSpace(verdicts[label]) == "" {
			missing[label] = true
		}
	}
	terminalMember := false
	for _, childID := range s.agentOrchestrator.listChildren(parentRunID) {
		child := s.runs[childID]
		if child == nil || !missing[child.label] {
			continue
		}
		switch child.status {
		case RunStatusCompleted, RunStatusFailed, RunStatusCancelled:
			terminalMember = true
		}
		if terminalMember {
			break
		}
	}
	if !terminalMember {
		s.mu.Unlock()
		return false
	}
	rs.verdictAutoRedrives++
	attempt := rs.verdictAutoRedrives
	s.mu.Unlock()
	if !s.resumeVerdictDeficientMembers(parentRunID, "") {
		return false
	}
	s.flowDiagLog(parentRunID, "verdict_deficient_member_auto_redrive",
		"done-edge verdict gate: auto re-driving terminal deficient member(s)",
		"attempt", fmt.Sprintf("%d/%d", attempt, maxVerdictAutoRedrives))
	return true
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

// gateReasonNamesDeficientMemberVerdict reports whether an escalate gate
// reason adjudicates a deficient inbound-cohort member — it names the
// member's label in verdict notation ("SPEC_ALIGN: blocked", "reviewer =
// changes_requested"). run-2062497 D6: the synthesis hub authored the
// cohort-split park in free text ("Cohort split needs human adjudication …
// SPEC_ALIGN: blocked"), which never matched isReviewVerdictGateReason — so
// Continue ran the generic hub reinvoke and re-parked on the identical stale
// verdict. Only verdict-producing inbound cohorts (plan/review) count, and
// the label must be followed by a verdict token — narrating a member in
// passing ("the joined reviewer verdict describes…") does not trigger.
func (s *InteractiveService) gateReasonNamesDeficientMemberVerdict(parentRunID, reason string) bool {
	reason = strings.ToLower(strings.TrimSpace(reason))
	if reason == "" {
		return false
	}
	s.mu.Lock()
	parent := s.runs[parentRunID]
	var expected []string
	var verdicts map[string]string
	if parent != nil {
		hubID := strings.TrimSpace(parent.activeHubNodeID)
		if hubID == "" {
			hubID = hubInlineNodeID(parent.activeFlowNodes)
		}
		expected = cohortNodeLabels(parent.activeFlowNodes, hubInboundCohortName(hubID))
		verdicts = mergePendingReviewVerdictsLocked(parent, parent.lastReviewCohortVerdicts)
	}
	s.mu.Unlock()
	for _, label := range expected {
		v := strings.TrimSpace(verdicts[label])
		if v == "approved" {
			continue
		}
		if reasonNamesVerdict(reason, label) {
			return true
		}
	}
	return false
}

// reasonNamesVerdict reports whether the reason mentions label followed by a
// verdict value in notation position ("<label>: blocked", "<label> =
// changes_requested"). A bare label mention — or one separated from the
// verdict by other words — does not count.
func reasonNamesVerdict(reason, label string) bool {
	l := strings.ToLower(strings.TrimSpace(label))
	if l == "" {
		return false
	}
	for idx := 0; idx <= len(reason); {
		i := strings.Index(reason[idx:], l)
		if i < 0 {
			return false
		}
		pos := idx + i
		rest := strings.TrimLeft(reason[pos+len(l):], " :=_(")
		for _, vv := range []string{"approved", "blocked", "changes_requested", "changes requested", "missing", "not approved"} {
			if strings.HasPrefix(rest, vv) {
				return true
			}
		}
		idx = pos + len(l)
	}
	return false
}

// resumeVerdictDeficientMembers re-drives every review-cohort member child
// whose machine verdict is missing or not approved for the run's active hub
// (CA-1098). Each matched member's settle-time reprompt budget is reset — a
// user-driven Continue is a fresh attempt, not a continuation of the
// exhausted budget. Returns false when no deficient member maps to a live
// child, letting the caller fall through to the generic resume. The human's
// continue feedback is prepended to the re-drive prompt (run-2062497 D6: a
// cohort-split adjudication must REACH the deficient member so it
// re-evaluates with the ruling in context, not re-park on the stale verdict).
func (s *InteractiveService) resumeVerdictDeficientMembers(parentRunID, feedback string) bool {
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
	// BUG-565 (live run-69320): a deficient label may have NO child at all —
	// the member was never dispatched (e.g. a stale-hub-done consumed the
	// dispatch outcome before the reviewer leg spawned). Re-drive only covers
	// existing children; collect the never-spawned labels so the caller can
	// spawn a fresh member leg instead of looping the hub re-prompt.
	// parent was captured above — reuse its node list for the spawn lookup.
	nodes := parent.activeFlowNodes
	s.mu.Unlock()

	prompt := "[flow-engine] The synthesis gate rejected the hub's done: your machine verdict " +
		"was missing or not approved. Re-evaluate and call submit_review_outcome as the FIRST " +
		"action of this turn with status=approved|changes_requested|blocked and a verdicts " +
		"array containing one row per acceptance criterion. Wait for the tool result before " +
		"writing any summary text."
	if strings.TrimSpace(feedback) != "" {
		prompt = "Human adjudication on the decision card: " + strings.TrimSpace(feedback) + "\n\n---\n\n" + prompt
	}
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
	var spawned []string
	for label := range deficient {
		if seen[label] {
			continue
		}
		node, ok := findFlowNode(nodes, label)
		if !ok {
			continue
		}
		if s.spawnFlowDelegateLeg(context.Background(), parentRunID, node, prompt) {
			spawned = append(spawned, label)
			redriven = true
		}
	}
	if redriven {
		s.flowDiagLog(parentRunID, "verdict_deficient_member_redrive",
			"missing/deficient verdict park: re-driving cohort members",
			"labels", strings.Join(labels, ","), "spawned", strings.Join(spawned, ","))
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
