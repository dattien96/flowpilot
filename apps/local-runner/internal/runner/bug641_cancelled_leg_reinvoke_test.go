package runner

import (
	"testing"

	"flowpilot-runner/internal/agentpack"
)

// BUG-641 ghost-resurrection arm (live run-306526): legs terminalized via
// agent-loop/stop flipped back to `running` in memory minutes later —
// run-348382 answered `stopped` then re-appeared running ~4 min on, same for
// run-309960/run-402250 — and the undead rows kept hub reinvokes deferring
// on "children running or waiting" forever. Root cause:
// reinvokeMatchingFlowChild scans children newest-first and unconditionally
// re-stamps `status=running` on the first label/agent match — callers match
// by label only, so a review-loop continue back-edge resurrected the
// operator-stopped coder leg (and stamped its summary running). Explicit
// terminal (Cancelled) or closed legs (member-skip, reclaim) must never be
// re-driven in place — the designed re-drive is a FRESH leg via spawn;
// Completed/Failed legs stay matchable (BUG-Rnd2 re-entry / retry-in-place).

func bug641ReinvokeFixture(t *testing.T) (*InteractiveService, string) {
	t.Helper()
	svc := bug289Service(t)
	svc.dispatchStore = NewMemoryDispatchStore()
	parent, err := svc.createRun(StartRunInput{ProjectID: "proj", ChatMode: "normal_chat", ProviderKey: ProviderKeyCodex})
	if err != nil {
		t.Fatalf("createRun: %v", err)
	}
	svc.mu.Lock()
	prs := svc.runs[parent.RunID]
	prs.status = RunStatusRunning
	prs.flowEngineDriven = true
	prs.activeFlowNodes = []agentpack.FlowNode{
		{ID: "coder", Behavior: "agent.code", Agent: "agents/coder.md"},
	}
	svc.mu.Unlock()
	return svc, parent.RunID
}

func bug641AddLeg(t *testing.T, svc *InteractiveService, parentID, id, label string, status RunStatus, legClosed bool) {
	t.Helper()
	svc.mu.Lock()
	rs := &interactiveRun{
		id:          id,
		parentRunID: parentID,
		label:       label,
		stepID:      label,
		agentName:   label,
		status:      status,
		agentStatus: string(status),
		subs:        map[int64]chan ProviderEvent{},
	}
	rs.legState = LegStateActive
	if legClosed {
		rs.legState = LegStateClosed
	}
	svc.runs[id] = rs
	svc.agentOrchestrator.registerChild(parentID, id)
	svc.mu.Unlock()
}

func TestBug641_ReinvokeNeverResurrectsCancelledLeg(t *testing.T) {
	svc, parentID := bug641ReinvokeFixture(t)
	bug641AddLeg(t, svc, parentID, "leg-coder", "coder", RunStatusCancelled, false)

	matched := svc.reinvokeMatchingFlowChild(parentID, "resume coder", func(child *interactiveRun) bool {
		return child.label == "coder"
	})

	if matched {
		svc.mu.Lock()
		st := svc.runs["leg-coder"].status
		svc.mu.Unlock()
		t.Fatalf("cancelled leg resurrected by reinvoke: matched=%v status=%s", matched, st)
	}
	svc.mu.Lock()
	st := svc.runs["leg-coder"].status
	svc.mu.Unlock()
	if st != RunStatusCancelled {
		t.Fatalf("cancelled leg mutated: %s", st)
	}
}

func TestBug641_ReinvokeNeverResurrectsClosedLeg(t *testing.T) {
	svc, parentID := bug641ReinvokeFixture(t)
	// Member-skip stamps Failed + LegStateClosed (handleMemberAction skip) —
	// a closed leg is an explicit terminal even though the status is the
	// retryable-looking Failed.
	bug641AddLeg(t, svc, parentID, "leg-coder", "coder", RunStatusFailed, true)

	if svc.reinvokeMatchingFlowChild(parentID, "resume coder", func(child *interactiveRun) bool {
		return child.label == "coder"
	}) {
		t.Fatal("closed leg (member-skip) matched by reinvoke")
	}
	svc.mu.Lock()
	st := svc.runs["leg-coder"].status
	svc.mu.Unlock()
	if st != RunStatusFailed {
		t.Fatalf("closed leg mutated: %s", st)
	}
}

func TestBug641_ReinvokeStillMatchesCompletedLeg(t *testing.T) {
	svc, parentID := bug641ReinvokeFixture(t)
	// BUG-Rnd2 contract: a review-loop round-2+ continue re-drives the SAME
	// completed leg in place — that is the designed completed→running
	// transition, not resurrection.
	bug641AddLeg(t, svc, parentID, "leg-coder", "coder", RunStatusCompleted, false)

	if !svc.reinvokeMatchingFlowChild(parentID, "round-2 fixes", func(child *interactiveRun) bool {
		return child.label == "coder"
	}) {
		t.Fatal("completed leg no longer re-drivable — BUG-Rnd2 re-entry regressed")
	}
	svc.mu.Lock()
	st := svc.runs["leg-coder"].status
	svc.mu.Unlock()
	if st != RunStatusRunning {
		t.Fatalf("completed leg not re-driven to running: %s", st)
	}
}

func TestBug641_ReinvokePicksNewerLiveLegOverCancelledSibling(t *testing.T) {
	svc, parentID := bug641ReinvokeFixture(t)
	// Newest-first scan: an operator-stopped NEWER leg must not shadow the
	// older still-live leg of the same label — skip past it.
	bug641AddLeg(t, svc, parentID, "leg-coder-old", "coder", RunStatusCompleted, false)
	bug641AddLeg(t, svc, parentID, "leg-coder-new", "coder", RunStatusCancelled, false)

	if !svc.reinvokeMatchingFlowChild(parentID, "resume coder", func(child *interactiveRun) bool {
		return child.label == "coder"
	}) {
		t.Fatal("no live leg matched — cancelled sibling shadowed it")
	}
	svc.mu.Lock()
	oldSt := svc.runs["leg-coder-old"].status
	newSt := svc.runs["leg-coder-new"].status
	svc.mu.Unlock()
	if newSt != RunStatusCancelled {
		t.Fatalf("cancelled newer leg mutated: %s", newSt)
	}
	if oldSt != RunStatusRunning {
		t.Fatalf("older live leg not re-driven: %s", oldSt)
	}
}

// Reviewer C-1 on the BUG-641 fix: several callers' match predicates have
// side effects — rearmCohortIfDrainedLocked re-opens a drained cohort
// (preRegisterCohort deletes the cohortDrained tombstone) and another caller
// pre-registers + re-tags flowCohortId. If the predicate fires on a leg the
// terminal skip then refuses to drive, the cohort re-opens with nobody to
// fill it — a phantom open cohort: continue/done reject
// `rejected_cohort_incomplete` forever and dead-dispatch/audit redrives stay
// suppressed (exactly the live run-306526 zombie-barrier class). The
// terminal check must run BEFORE match() so skipped legs see no predicate.
func TestBug641_ReinvokeSkipsTerminalBeforeMatchNoPhantomCohort(t *testing.T) {
	svc, parentID := bug641ReinvokeFixture(t)
	// Member leg skipped via member_action: Failed + LegStateClosed, cohort
	// already drained and tombstoned.
	bug641AddLeg(t, svc, parentID, "leg-coder", "coder", RunStatusFailed, true)
	svc.mu.Lock()
	svc.runs["leg-coder"].flowCohortId = "flow-auto-review-round-0"
	svc.mu.Unlock()
	svc.agentOrchestrator.preRegisterCohort(parentID, "flow-auto-review-round-0", 1)
	svc.agentOrchestrator.appendCohortResult(parentID, "flow-auto-review-round-0",
		cohortEntry{Label: "coder", Status: "failed", Err: "skipped by user (stalled)"})
	svc.agentOrchestrator.drainCohort(parentID, "flow-auto-review-round-0")
	if svc.agentOrchestrator.hasOpenCohort(parentID) {
		t.Fatal("fixture broken: drained cohort reads open")
	}

	predicateRan := false
	// Replicate the failedDelegate retry caller's predicate shape
	// (interactive_service.go): label match + rearmCohortIfDrainedLocked.
	matched := svc.reinvokeMatchingFlowChild(parentID, "retry coder", func(child *interactiveRun) bool {
		if child.label != "coder" {
			return false
		}
		predicateRan = true
		svc.rearmCohortIfDrainedLocked(child)
		return true
	})

	if matched {
		t.Fatal("closed leg matched — the terminal skip must run before match()")
	}
	if predicateRan {
		t.Fatal("match predicate ran on a terminal leg — side-effecting callers re-arm drained cohorts")
	}
	if svc.agentOrchestrator.hasOpenCohort(parentID) {
		t.Fatal("phantom open cohort: drained cohort re-armed by a predicate on a leg nobody drives — rejected_cohort_incomplete wedge")
	}
}

// Same ordering requirement for the Cancelled arm — an operator-stopped leg
// must not run caller predicates either.
func TestBug641_ReinvokeSkipsCancelledBeforeMatchNoPhantomCohort(t *testing.T) {
	svc, parentID := bug641ReinvokeFixture(t)
	bug641AddLeg(t, svc, parentID, "leg-coder", "coder", RunStatusCancelled, false)
	svc.mu.Lock()
	svc.runs["leg-coder"].flowCohortId = "flow-auto-review-round-0"
	svc.mu.Unlock()
	svc.agentOrchestrator.preRegisterCohort(parentID, "flow-auto-review-round-0", 1)
	svc.agentOrchestrator.appendCohortResult(parentID, "flow-auto-review-round-0",
		cohortEntry{Label: "coder", Status: "cancelled"})
	svc.agentOrchestrator.drainCohort(parentID, "flow-auto-review-round-0")

	matched := svc.reinvokeMatchingFlowChild(parentID, "retry coder", func(child *interactiveRun) bool {
		if child.label != "coder" {
			return false
		}
		svc.rearmCohortIfDrainedLocked(child)
		return true
	})

	if matched || svc.agentOrchestrator.hasOpenCohort(parentID) {
		t.Fatalf("cancelled leg ran predicate: matched=%v openCohort=%v", matched,
			svc.agentOrchestrator.hasOpenCohort(parentID))
	}
}
