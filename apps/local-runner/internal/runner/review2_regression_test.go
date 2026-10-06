package runner

// Second-pass review of CA-1224..1232 — four findings verified as real
// defects inside/adjacent to the batch:
//
//  I-1  resumeVerdictDeficientMembers armed verdictRepromptInFlight in the
//      scan loop, then reinvokeMatchingFlowChild cleared it as a stale
//      marker before the turn ever ran — the BUG-559 draft shield was dead
//      code on this path (run-60899 wedge class). The marker now re-arms on
//      the driven leg after the helper returns.
//  I-2  the same scan armed reinvokeInFlight/verdictRepromptCount on the
//      FIRST live leg in spawn order while the drive scans NEWEST-first —
//      with duplicate live labels the flags landed on a stale sibling
//      (same first-vs-newest class as BUG-639). Flags now arm inside the
//      match predicate = the leg actually driven.
//  I-3  closed-leg ghosts (status=running + legState=closed, produced by
//      quota-veto respawn and claim reclaim) were counted by
//      openCohortMemberRuns / pick / deadDispatchNonTerminalChildExists —
//      the BUG-643 DOA arm made them stall-visible for the first time, so a
//      ghost could park member_stalled while its successor worked.
//  I-4  handleAmendFlow sprayed the path union into EVERY active contract
//      of the run (my BUG-637 enumeration widened the target set beyond
//      the mounted-graph writer nodes it replaced). Targets now scope to
//      gated/live steps with the full-set fallback that keeps BUG-637's
//      nested-debate contract reachable.
//  M-1  reinvokeExistingFlowChild pre-registered the cohort before knowing
//      a live leg existed — a no-match + failed spawn fallback left a
//      phantom open barrier (same wedge class as C-1 one layer up).
//      Registration moved inside the match predicate.

import (
	"testing"
	"time"

	"flowpilot-runner/internal/changecontract"
)

// --- I-1 + I-2 ---------------------------------------------------------------

// Two live same-label legs: the scan marks `seen` on the first in spawn
// order, but reinvokeMatchingFlowChild drives the newest. Flags must land
// on the driven leg, and verdictRepromptInFlight must survive the helper's
// stale-marker clear.
func TestReview2_DeficientVerdictFlagsLandOnDrivenLeg(t *testing.T) {
	svc, runID := newFlowTestRun(t)
	edges, nodes := ca1098VerdictTopology()
	setRun198699Topology(t, svc, runID, edges, nodes, "synthesis")
	staleID := bug565ReviewerChild(t, svc, runID, ProviderKeyCodex, RunStatusFailed)
	drivenID := runID + "-rev-live-2"
	svc.mu.Lock()
	svc.runs[drivenID] = &interactiveRun{
		id:                drivenID,
		parentRunID:       runID,
		label:             "reviewer",
		role:              "reviewer",
		agentName:         "reviewer",
		status:            RunStatusFailed,
		agentStatus:       string(RunStatusFailed),
		providerKey:       ProviderKeyCodex,
		providerAccountID: svc.runs[runID].providerAccountID,
		flowCohortId:      "flow-auto-validate-round-0",
		stepID:            "reviewer",
		subs:              map[int64]chan ProviderEvent{},
	}
	svc.mu.Unlock()
	svc.agentOrchestrator.registerChild(runID, drivenID)
	svc.agentOrchestrator.mutateLoop(runID, func(st AgentLoopState) AgentLoopState {
		st.Status = "running"
		return st
	})

	if !svc.resumeVerdictDeficientMembers(runID, "") {
		t.Fatal("deficient member with live legs must re-drive")
	}

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.runs[staleID].reinvokeInFlight || svc.runs[staleID].verdictRepromptInFlight {
		t.Fatal("flags armed on the stale spawn-order sibling — they must land on the leg actually driven")
	}
	driven := svc.runs[drivenID]
	if !driven.verdictRepromptInFlight {
		t.Fatal("verdictRepromptInFlight lost — helper's stale-marker clear killed the armed flag before the turn ran (I-1)")
	}
}

// --- I-3 ---------------------------------------------------------------------

// A closed-leg ghost (status=running, legState=closed, zero events, aged
// createdAt) must not DOA-stall the cohort while a live successor works —
// pre-BUG-643 these legs were invisible to the stall sweep; the DOA arm
// must not make them visible.
func TestReview2_ClosedLegGhostDoesNotStallCohort(t *testing.T) {
	svc, parentID := bug641ReinvokeFixture(t)
	bug641AddLeg(t, svc, parentID, "ghost", "reviewer", RunStatusRunning, true)
	bug641AddLeg(t, svc, parentID, "successor", "reviewer", RunStatusRunning, false)
	svc.mu.Lock()
	svc.runs["ghost"].flowCohortId = "flow-auto-review-round-0"
	svc.runs["ghost"].createdAt = time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339Nano)
	succ := svc.runs["successor"]
	succ.flowCohortId = "flow-auto-review-round-0"
	succ.lastProviderEventAt = time.Now().UTC()
	succ.turnInFlight = true
	svc.mu.Unlock()
	svc.agentOrchestrator.preRegisterCohort(parentID, "flow-auto-review-round-0", 1)

	if svc.checkAndBlockStalledMembers(parentID) {
		t.Fatal("closed-leg ghost parked the cohort as member_stalled — ghosts must not count as live members")
	}
}

// Same ghost must not suppress the dead-dispatch fresh spawn — it can never
// be re-driven (BUG-641), so counting it live wedges the seat.
func TestReview2_DeadDispatchIgnoresClosedGhost(t *testing.T) {
	svc, parentID := bug641ReinvokeFixture(t)
	bug641AddLeg(t, svc, parentID, "ghost", "coder", RunStatusRunning, true)

	if svc.deadDispatchNonTerminalChildExists(parentID, "coder") {
		t.Fatal("closed ghost counted as a live leg — suppresses the spawn that would actually fill the seat")
	}
}

// memberAction's pick must resolve to the live successor, not the newer
// closed ghost carrying the same label+cohort.
func TestReview2_MemberActionPrefersLiveOverClosedGhost(t *testing.T) {
	svc, parentID := bug641ReinvokeFixture(t)
	bug641AddLeg(t, svc, parentID, "live", "reviewer", RunStatusRunning, false)
	bug641AddLeg(t, svc, parentID, "ghost", "reviewer", RunStatusRunning, true)
	svc.mu.Lock()
	svc.runs["live"].flowCohortId = "flow-auto-review-round-0"
	ghost := svc.runs["ghost"]
	ghost.flowCohortId = "flow-auto-review-round-0"
	ghost.createdAt = time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339Nano)
	svc.mu.Unlock()
	svc.agentOrchestrator.preRegisterCohort(parentID, "flow-auto-review-round-0", 1)
	svc.agentOrchestrator.mutateLoop(parentID, func(st AgentLoopState) AgentLoopState {
		st.Status = "blocked"
		st.BlockReason = "member_stalled"
		return st
	})

	svc.handleMemberAction(parentID, MemberAction{Action: "retry", Node: "reviewer"})

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.runs["ghost"].status != RunStatusRunning || svc.runs["ghost"].legState != LegStateClosed {
		t.Fatalf("ghost leg mutated by member action: status=%s leg=%s", svc.runs["ghost"].status, svc.runs["ghost"].legState)
	}
	if svc.runs["live"].lastProviderEventAt.IsZero() {
		t.Fatal("retry never reached the live leg — the newer closed ghost shadowed it")
	}
}

// --- M-1 ---------------------------------------------------------------------

// reinvokeExistingFlowChild must not register the cohort when no live leg
// matches — the pre-registration deletes the drained tombstone, so a miss
// + failed spawn fallback leaves a phantom open barrier.
func TestReview2_ReinvokeExistingNoPhantomCohortOnMiss(t *testing.T) {
	svc, parentID := bug641ReinvokeFixture(t)
	bug641AddLeg(t, svc, parentID, "leg-coder", "coder", RunStatusCancelled, false)
	svc.agentOrchestrator.preRegisterCohort(parentID, "flow-auto-code-round-1", 1)
	svc.agentOrchestrator.appendCohortResult(parentID, "flow-auto-code-round-1",
		cohortEntry{Label: "coder", Status: "completed"})
	svc.agentOrchestrator.drainCohort(parentID, "flow-auto-code-round-1")
	if svc.agentOrchestrator.hasOpenCohort(parentID) {
		t.Fatal("fixture broken: drained cohort reads open")
	}

	if svc.reinvokeExistingFlowChild(parentID, "coder", "resume", "flow-auto-code-round-1", 1) {
		t.Fatal("cancelled-only label re-driven")
	}
	if svc.agentOrchestrator.hasOpenCohort(parentID) {
		t.Fatal("phantom open cohort: preRegister ran with no live leg to fill the re-opened barrier")
	}
}

// The positive arm: a live match still registers + re-tags — the reused leg
// must join THIS round's barrier, not the drained one.
func TestReview2_ReinvokeExistingRegistersOnLiveMatch(t *testing.T) {
	svc, parentID := bug641ReinvokeFixture(t)
	bug641AddLeg(t, svc, parentID, "leg-coder", "coder", RunStatusCompleted, false)
	svc.mu.Lock()
	svc.runs["leg-coder"].flowCohortId = "flow-auto-code-round-0"
	svc.mu.Unlock()

	if !svc.reinvokeExistingFlowChild(parentID, "coder", "round-2 fixes", "flow-auto-code-round-1", 1) {
		t.Fatal("live completed leg not re-driven")
	}
	svc.mu.Lock()
	got := svc.runs["leg-coder"].flowCohortId
	svc.mu.Unlock()
	if got != "flow-auto-code-round-1" {
		t.Fatalf("reused leg not re-tagged to this round's cohort: %q", got)
	}
	svc.agentOrchestrator.mu.Lock()
	exp := svc.agentOrchestrator.cohortExpected[cohortKey(parentID, "flow-auto-code-round-1")]
	svc.agentOrchestrator.mu.Unlock()
	if exp != 1 {
		t.Fatalf("cohort not pre-registered on match: expected=%d", exp)
	}
}

// --- I-4 ---------------------------------------------------------------------

func TestReview2_FilterAmendTargets(t *testing.T) {
	recs := []changecontract.FrozenContractRecord{
		{CoderStepID: "writer-a"},
		{CoderStepID: "writer-b"},
	}
	if got := filterAmendTargets(recs, map[string]bool{"writer-a": true}, map[string]bool{"writer-b": true}); len(got) != 1 || got[0].CoderStepID != "writer-a" {
		t.Fatalf("gated step must win: %+v", got)
	}
	if got := filterAmendTargets(recs, nil, map[string]bool{"writer-b": true}); len(got) != 1 || got[0].CoderStepID != "writer-b" {
		t.Fatalf("live step must scope: %+v", got)
	}
	if got := filterAmendTargets(recs, map[string]bool{"nope": true}, nil); len(got) != 2 {
		t.Fatalf("no identifiable step must fall back to the full set (BUG-637 reachability): %+v", got)
	}
}

// --- M-3 ---------------------------------------------------------------------

// Alias paths ("./x", "dir/../x") must hit the same snapshot key as the
// git-canonical form — a missed fingerprint lookup falls through to the
// git arm and false-positives as "changed".
func TestReview2_UnchangedSinceTurnStartCleansAlias(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "x.go", "package x\n")
	rs := &interactiveRun{
		turnStartWorktree: map[string]string{
			"x.go": worktreeFileFingerprint(dir, "x.go"),
		},
	}
	if !unchangedSinceTurnStart(dir, "./x.go", rs) {
		t.Fatal("'./x.go' missed the fingerprint arm — alias must canonicalize to the snapshot key")
	}
	if unchangedSinceTurnStart(dir, "", rs) {
		t.Fatal("empty path must fail closed, not exempt")
	}
}
